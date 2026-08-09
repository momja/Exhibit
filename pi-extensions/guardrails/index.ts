/**
 * Guardrails — user prompt guardrail for pi.
 *
 * A supplemental (small) LLM acts as gatekeeper around user prompts before
 * they reach the main coding agent. It blocks or warns on:
 *   - prompt-injection / instruction-override ("ignore previous instructions")
 *   - prompts steering the conversation off-topic
 *   - disallowed / explicit topics (configurable policy)
 *
 * Fast path: a regex prefilter decides whether the guardrail LLM needs to be
 * consulted at all ("heuristic" check mode, the default), so ordinary prompts
 * add ~zero latency.
 *
 * Configuration: ~/.pi/agent/guardrails.json (global) and
 * <project>/.pi/guardrails.json (project, only when trusted). See
 * guardrails.example.json and README.md.
 *
 * Commands:
 *   /guardrails              — show status + configuration
 *   /guardrails test <text>  — run the guardrail on <text> without sending it
 *   /guardrails toggle       — enable/disable
 *   /guardrails reload       — reload config from disk
 */
import type { ExtensionAPI, ExtensionContext } from "@earendil-works/pi-coding-agent";
import type { Model } from "@earendil-works/pi-ai";
import { Box, Text } from "@earendil-works/pi-tui";
import {
  DEFAULT_CONFIG,
  loadConfig,
  type GuardrailConfig,
} from "./config.ts";
import { prefilterPrompt, type PrefilterHit } from "./prefilter.ts";
import {
  buildContextText,
  callGuardrail,
  resolveGuardrailModel,
  type GuardrailVerdict,
} from "./guardrail.ts";

const AUDIT_TYPE = "guardrails";

/** Debug trace, enabled with PI_GUARDRAILS_DEBUG=1. */
function debug(...args: unknown[]) {
  if (process.env.PI_GUARDRAILS_DEBUG === "1" || process.env.PI_GUARDRAILS_DEBUG === "true") {
    console.log("[guardrails]", ...args);
  }
}

interface AuditData {
  ts: number;
  verdict: string;
  severity: string;
  category: string | null;
  reason: string;
  prompt: string;
  source: string;
  model: string;
  hit?: PrefilterHit;
}

const activeCfg: { config: GuardrailConfig; sources: string[] } = {
  config: structuredClone(DEFAULT_CONFIG),
  sources: [],
};

function notify(ctx: ExtensionContext, message: string, kind: "info" | "warning" | "error") {
  if (ctx.hasUI) ctx.ui.notify(message, kind);
}

function statusLine(cfg: GuardrailConfig, model: Model | undefined): string {
  if (!cfg.enabled || cfg.mode === "off" || cfg.check === "off") return "guardrails: off";
  const modelPart = model ? `${model.provider}/${model.id}` : "no model";
  return `guardrails: ${cfg.mode} · ${cfg.check} · ${modelPart}`;
}

async function audit(pi: ExtensionAPI, data: AuditData): Promise<void> {
  try {
    await pi.appendEntry(AUDIT_TYPE, data);
  } catch (error) {
    debug("appendEntry failed:", (error as Error).message);
  }
}

debug("extension loaded");

export default function (pi: ExtensionAPI) {
  let guardrailModel: Model | undefined;
  let modelResolvedAt = 0;
  const lastVerdicts: Array<{ ts: number; verdict: string; reason: string }> = [];

  const rememberVerdict = (verdict: GuardrailVerdict) => {
    lastVerdicts.unshift({ ts: Date.now(), verdict: verdict.verdict, reason: verdict.reason });
    if (lastVerdicts.length > 10) lastVerdicts.pop();
  };

  const refreshModel = async (ctx: ExtensionContext) => {
    const cfg = activeCfg.config;
    if (!cfg.enabled || cfg.mode === "off" || cfg.check === "off") {
      guardrailModel = undefined;
      return;
    }
    if (guardrailModel && Date.now() - modelResolvedAt < 60_000) return;
    guardrailModel = resolveGuardrailModel(ctx, cfg);
    modelResolvedAt = Date.now();
    if (ctx.hasUI) {
      ctx.ui.setStatus("guardrails", statusLine(cfg, guardrailModel));
      if (!guardrailModel) {
        ctx.ui.setStatus(
          "guardrails",
          "guardrails: on · NO MODEL — set guardrails.model or configure a provider",
        );
      }
    }
  };

  // ---- lifecycle ---------------------------------------------------------

  pi.on("session_start", async (_event, ctx) => {
    const loaded = await loadConfig(ctx.cwd, ctx.isProjectTrusted());
    activeCfg.config = loaded.config;
    activeCfg.sources = loaded.sources;
    guardrailModel = undefined;
    modelResolvedAt = 0;
    await refreshModel(ctx);
  });

  pi.on("model_select", async (_event, ctx) => {
    guardrailModel = undefined;
    modelResolvedAt = 0;
    await refreshModel(ctx);
  });

  // ---- the gatekeeper ------------------------------------------------------

  pi.on("input", async (event, ctx) => {
    debug("input event:", JSON.stringify({ text: event.text, source: event.source }));
    const cfg = activeCfg.config;
    if (!cfg.enabled || cfg.mode === "off" || cfg.check === "off") {
      return { action: "continue" };
    }
    if (event.source === "extension" && cfg.skipExtensionSources) {
      return { action: "continue" };
    }
    if (event.source === "rpc" && cfg.skipRpcSources) {
      return { action: "continue" };
    }
    const text = event.text?.trim() ?? "";
    if (!text) return { action: "continue" };
    if (cfg.skipSlashCommands && text.startsWith("/")) return { action: "continue" };

    // Fast prefilter — most prompts pass through with zero latency.
    const hit = cfg.check === "always" ? undefined : prefilterPrompt(text, cfg);
    debug("prefilter hit:", hit);

    // Prefilter-only mode: hit blocks immediately, no LLM involved.
    if (hit && cfg.prefilter.blockDirect) {
      const reason = `Blocked by guardrail filter (${hit.category}).`;
      await audit(pi, {
        ts: Date.now(),
        verdict: "block",
        severity: "high",
        category: hit.category,
        reason,
        prompt: text,
        source: "prefilter",
        model: guardrailModel ? `${guardrailModel.provider}/${guardrailModel.id}` : "none",
        hit,
      });
      notify(ctx, reason, "error");
      return { action: "handled" };
    }

    // Heuristic mode with no hit → skip the LLM entirely.
    if (cfg.check === "heuristic" && !hit) return { action: "continue" };

    // ---- guardrail LLM ------------------------------------------------
    await refreshModel(ctx);
    if (!guardrailModel) {
      if (hit) {
        notify(
          ctx,
          "Guardrail: prompt matches a filter but no guardrail model is configured — sending to agent.",
          "warning",
        );
      }
      return { action: "continue" };
    }

    const contextText = buildContextText(ctx, cfg);
    let verdict: GuardrailVerdict | null = null;
    try {
      debug("calling guardrail model:", guardrailModel.provider, guardrailModel.id);
      verdict = await callGuardrail(ctx, cfg, guardrailModel, text, contextText);
      debug("verdict:", verdict);
    } catch (e) {
      debug("guardrail call error:", (e as Error).message);
      verdict = null;
    }

    if (!verdict) {
      // Guardrail LLM unreachable or confused.
      if (cfg.failClosed) {
        const reason = "Guardrail unreachable and failClosed is enabled — prompt blocked.";
        await audit(pi, {
          ts: Date.now(),
          verdict: "block",
          severity: "high",
          category: "policy",
          reason,
          prompt: text,
          source: "fail-closed",
          model: `${guardrailModel.provider}/${guardrailModel.id}`,
        });
        notify(ctx, reason, "error");
        return { action: "handled" };
      }
      notify(
        ctx,
        "Guardrail LLM could not be reached — prompt passed through (fail-open).",
        "warning",
      );
      return { action: "continue" };
    }

    rememberVerdict(verdict);
    const modelLabel = `${guardrailModel.provider}/${guardrailModel.id}`;

    // Off-topic prompts get their own policy switch (the LLM classifies these
    // as low-severity, harmless "allow" by default).
    if (verdict.category === "offtopic") {
      if (cfg.policy.offTopicHandling === "block") {
        verdict = { ...verdict, verdict: "block", severity: "medium" };
      } else if (cfg.policy.offTopicHandling === "warn") {
        verdict = { ...verdict, verdict: "warn", severity: "low" };
      }
    }

    const shouldLog =
      cfg.logAll || verdict.verdict !== "allow" || verdict.severity === "high";

    if (verdict.verdict === "block") {
      const reason = verdict.reason || "Blocked by guardrails.";
      const label = `Guardrail blocked your prompt (${verdict.category ?? "policy"}): ${reason}`;
      if (shouldLog) {
        await audit(pi, {
          ts: Date.now(),
          verdict: "block",
          severity: verdict.severity,
          category: verdict.category,
          reason,
          prompt: text,
          source: "llm",
          model: modelLabel,
        });
      }
      if (cfg.mode === "block") {
        notify(ctx, label, "error");
        return { action: "handled" };
      }
      // mode "warn": downgrade block → warn
      notify(ctx, label, "warning");
      return { action: "continue" };
    }

    if (verdict.verdict === "warn") {
      const reason = verdict.reason || "Prompt flagged by guardrails.";
      if (shouldLog) {
        await audit(pi, {
          ts: Date.now(),
          verdict: "warn",
          severity: verdict.severity,
          category: verdict.category,
          reason,
          prompt: text,
          source: "llm",
          model: modelLabel,
        });
      }
      notify(ctx, `Guardrail (${verdict.category ?? "warning"}): ${reason}`, "warning");
      return { action: "continue" };
    }

    // allow
    if (cfg.logAll) {
      await audit(pi, {
        ts: Date.now(),
        verdict: "allow",
        severity: verdict.severity,
        category: verdict.category,
        reason: verdict.reason,
        prompt: text,
        source: "llm",
        model: modelLabel,
      });
    }
    return { action: "continue" };
  });

  // ---- commands ------------------------------------------------------------

  pi.registerCommand("guardrails", {
    description:
      "Show guardrail status and config. Subcommands: test <text>, toggle, reload",
    handler: async (args, ctx) => {
      const [sub, ...rest] = (args ?? "").trim().split(/\s+/);
      const cfg = activeCfg.config;

      if (sub === "toggle") {
        cfg.enabled = !cfg.enabled;
        notify(ctx, `Guardrails ${cfg.enabled ? "enabled" : "disabled"}.`, "info");
        await refreshModel(ctx);
        return;
      }

      if (sub === "reload") {
        const loaded = await loadConfig(ctx.cwd, ctx.isProjectTrusted());
        activeCfg.config = loaded.config;
        activeCfg.sources = loaded.sources;
        guardrailModel = undefined;
        modelResolvedAt = 0;
        await refreshModel(ctx);
        notify(ctx, "Guardrail config reloaded.", "info");
        return;
      }

      if (sub === "test") {
        const probe = rest.join(" ").trim();
        if (!probe) {
          notify(ctx, "Usage: /guardrails test <prompt text>", "warning");
          return;
        }
        if (!cfg.enabled || cfg.mode === "off" || cfg.check === "off") {
          notify(ctx, "Guardrails are disabled.", "warning");
          return;
        }
        await refreshModel(ctx);
        if (!guardrailModel) {
          notify(ctx, "No guardrail model available — configure guardrails.model.", "warning");
          return;
        }
        notify(ctx, `Running guardrail on: ${probe.slice(0, 80)}…`, "info");
        const contextText = buildContextText(ctx, cfg);
        try {
          const verdict = await callGuardrail(ctx, cfg, guardrailModel, probe, contextText);
          if (!verdict) {
            notify(ctx, "Guardrail LLM returned no verdict (fail-open in practice).", "warning");
            return;
          }
          rememberVerdict(verdict);
          notify(
            ctx,
            `Verdict: ${verdict.verdict} (${verdict.severity}${verdict.category ? `, ${verdict.category}` : ""}) — ${verdict.reason || "no reason given"}`,
            verdict.verdict === "block" ? "error" : verdict.verdict === "warn" ? "warning" : "info",
          );
        } catch (error) {
          notify(ctx, `Guardrail call failed: ${(error as Error).message}`, "error");
        }
        return;
      }

      // status
      const lines = [
        `Guardrails: ${cfg.enabled ? "enabled" : "DISABLED"}`,
        `Mode:     ${cfg.mode}${cfg.mode === "warn" ? " (blocks downgraded to warnings)" : ""}`,
        `Check:    ${cfg.check}${cfg.check === "heuristic" ? " (prefilter → LLM on match)" : ""}`,
        `Model:    ${guardrailModel ? `${guardrailModel.provider}/${guardrailModel.id}` : "none (auto-pick failed)"}`,
        `Prefilter:${cfg.prefilter.enabled ? "on" : "off"}${cfg.prefilter.blockDirect ? " (blockDirect)" : ""}`,
        `Fail-open:${cfg.failClosed ? " false (block on failure)" : " true (allow on failure)"}`,
        `Sources:  ${activeCfg.sources.length ? activeCfg.sources.join(", ") : "(defaults only)"}`,
      ];
      if (lastVerdicts.length > 0) {
        lines.push("");
        lines.push("Recent verdicts:");
        for (const v of lastVerdicts.slice(0, 5)) {
          lines.push(`  ${new Date(v.ts).toLocaleTimeString()} ${v.verdict.padEnd(5)} ${v.reason}`);
        }
      }
      notify(ctx, lines.join("\n"), "info");
    },
  });

  // ---- audit rendering -------------------------------------------------------

  pi.registerEntryRenderer(AUDIT_TYPE, (entry, { expanded }, theme) => {
    const data = entry.data as AuditData;
    const box = new Box(1, 1, (text) => theme.bg("toolBoxBg", text));
    const color =
      data.verdict === "block" ? "error" : data.verdict === "warn" ? "warning" : "success";
    box.addChild(
      new Text(
        `${theme.fg(color, theme.bold(`Guardrail · ${data.verdict}`))} ${theme.fg("dim", `[${data.category ?? "n/a"}]`)}`,
      ),
    );
    box.addChild(new Text(theme.fg("muted", data.reason)));
    if (expanded) {
      box.addChild(new Text(theme.fg("dim", `prompt: ${data.prompt}`)));
      box.addChild(new Text(theme.fg("dim", `model: ${data.model} · source: ${data.source} · ${new Date(data.ts).toLocaleString()}`)));
    }
    return box;
  });
}
