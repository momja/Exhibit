/**
 * Guardrails — configuration.
 *
 * Config is loaded from (later sources win):
 *   1. Built-in defaults
 *   2. $PI_GUARDRAILS_CONFIG file
 *   3. ~/.pi/agent/guardrails.json      (global, all projects)
 *   4. <cwd>/.pi/guardrails.json        (project-local, only when the project is trusted)
 *   5. Environment overrides (PI_GUARDRAILS_ENABLED / _MODEL / _MODE / _CHECK)
 */
import { readFile } from "node:fs/promises";
import { homedir } from "node:os";
import { join } from "node:path";
import { CONFIG_DIR_NAME } from "@earendil-works/pi-coding-agent";

export type GuardMode = "block" | "warn" | "off";
export type CheckMode = "always" | "heuristic" | "off";
export type VerdictKind = "allow" | "warn" | "block";
export type Severity = "low" | "medium" | "high";
export type OffTopicHandling = "allow" | "warn" | "block";

export interface GuardrailConfig {
  /** Master switch. */
  enabled: boolean;
  /**
   * Guardrail model as "provider/model-id" (e.g. "openai/gpt-4.1-mini").
   * Empty string selects automatically: preferred cheap models, then the
   * cheapest available non-reasoning model in the registry.
   */
  model: string;
  /**
   * How to treat a guardrail "block" verdict:
   *  - "block": reject the prompt (agent never sees it)
   *  - "warn":  allow it but show a warning
   *  - "off":   guardrail disabled entirely
   */
  mode: GuardMode;
  /**
   * When to consult the guardrail LLM:
   *  - "always":    every prompt (highest latency)
   *  - "heuristic": only when a fast prefilter flags the prompt (default)
   *  - "off":       never call the LLM (prefilter-only, if blockDirect is set)
   */
  check: CheckMode;
  /** Skip prompts injected by extensions (e.g. the agent's own follow-ups). */
  skipExtensionSources: boolean;
  /** Skip prompts arriving over RPC. */
  skipRpcSources: boolean;
  /** Skip inputs starting with "/" (slash commands, skills, templates). */
  skipSlashCommands: boolean;
  /** Truncate the prompt before sending it to the guardrail LLM. */
  maxPromptChars: number;
  /** Timeout for a single guardrail LLM call (ms). */
  timeoutMs: number;
  /**
   * If true, an unreachable/failing guardrail LLM blocks the prompt instead of
   * failing open. Use with care — a broken config can lock you out of asking
   * questions. Default false (fail open).
   */
  failClosed: boolean;
  /** Audit every verdict via appendEntry, not just non-"allow" ones. */
  logAll: boolean;
  context: {
    /** Recent user/assistant messages included as context (per role). */
    recentMessages: number;
    /** Total character budget for the conversation context. */
    maxChars: number;
  };
  prefilter: {
    /** Enable the fast heuristic prefilter. */
    enabled: boolean;
    /**
     * If true, a prefilter hit blocks immediately without consulting the LLM
     * (fast, but dumb — no nuance). Default false: prefilter only routes the
     * prompt to the guardrail LLM for a final verdict.
     */
    blockDirect: boolean;
    /** Regex patterns signalling prompt-injection / instruction-override. */
    injectionPatterns: string[];
    /** Regex patterns per category (e.g. "explicit") signalling disallowed topics. */
    topicPatterns: Record<string, string[]>;
  };
  policy: {
    /**
     * How to treat prompts the guardrail classifies as off-topic:
     *  - "warn":  allow but show a warning (default)
     *  - "block": reject like other blocks (subject to `mode` downgrade)
     *  - "allow": pass silently
     * Note: off-topic classification only happens when the guardrail LLM is
     * consulted, i.e. `check: "always"` or a prefilter hit.
     */
    offTopicHandling: OffTopicHandling;
    /**
     * Topics that are always acceptable in this project. Listed here so the
     * guardrail does not flag them even if they touch a sensitive area.
     */
    allowedTopics: string[];
    /** Topics that are never acceptable. */
    disallowedTopics: string[];
    /** Extra natural-language policy instructions for the guardrail LLM. */
    extraInstructions: string;
  };
}

export const DEFAULT_CONFIG: GuardrailConfig = {
  enabled: true,
  model: "",
  mode: "block",
  check: "heuristic",
  skipExtensionSources: true,
  skipRpcSources: false,
  skipSlashCommands: true,
  maxPromptChars: 4000,
  timeoutMs: 20000,
  failClosed: false,
  logAll: false,
  context: { recentMessages: 8, maxChars: 6000 },
  prefilter: {
    enabled: true,
    blockDirect: false,
    injectionPatterns: [
      "ignore\\s+(all\\s+|any\\s+|the\\s+|your\\s+)?(previous|prior|earlier)?\\s*(instructions|prompts|messages|commands|rules)",
      "disregard\\s+(all\\s+|any\\s+|the\\s+)?(previous|prior|earlier)?\\s*(instructions|prompts|messages|commands|rules)",
      "forget\\s+(everything|all\\s+(previous|prior|earlier)|your\\s+instructions)",
      "reveal\\s+(your|the)\\s+(system\\s+)?(prompt|instructions)",
      "show\\s+(me\\s+)?(your|the)\\s+(system\\s+)?(prompt|instructions)",
      "you\\s+are\\s+now\\s+(an?\\s+)?(unfiltered|uncensored|unaligned|free|jailbroken|evil)",
      "act\\s+as\\s+(an?\\s+)?(unfiltered|uncensored|unaligned|free|jailbroken)\\s+(assistant|model|ai)",
      "no\\s+(rules|restrictions|constraints|limits|boundaries)",
      "override\\s+(your|the|all)\\s+(instructions|rules|prompt|guidelines|settings)",
      "new\\s+system\\s+prompt",
      "developer\\s+message\\s*:",
      "\\[sysmsg\\]",
      "ignore\\s+all\\s+prior\\s+context",
    ],
    topicPatterns: {
      explicit: [
        "\\bnsfw\\b",
        "\\bporn(ography)?\\b",
        "\\bexplicit\\s+(sexual|sex|content)\\b",
        "\\bnude(s|ity|ness)?\\b",
        "\\bsex(ual|y|ualize)?\\b",
        "\\bgore\\b",
      ],
      /**
       * High-precision topic families that almost never appear in a coding
       * conversation. A hit only routes the prompt to the guardrail LLM — it
       * does not block. The LLM may still allow it (harmless request), in
       * which case `policy.offTopicHandling` decides whether to flag it.
       * Add project-relevant topics to `policy.allowedTopics` to silence them.
       */
      offtopic: [
        "\\b(recipe|recipes|cooking|cookbook|baking|lasagna|pizza|dinner|breakfast|lunch)\\b",
        "\\b(weather|forecast|temperature|rain|snow) (today|tomorrow|outside|this week)\\b",
        "\\bhoroscope|astrology\\b",
        "\\blottery (numbers|results|winning)\\b",
        "\\bcelebrity (gossip|news|drama)\\b",
      ],
    },
  },
  policy: {
    offTopicHandling: "warn",
    allowedTopics: [],
    disallowedTopics: [
      "explicit sexual content or pornography",
      "non-consensual sexual content",
      "content that sexualizes minors",
      "detailed instructions for illegal activity",
      "harassment, stalking, or doxxing of individuals",
    ],
    extraInstructions:
      "This assistant is a coding/software engineering agent. A prompt is off-topic only if it clearly abandons the user's project or programming work for something entirely unrelated; a general question asked in service of the work is fine.",
  },
};

export interface LoadedConfig {
  config: GuardrailConfig;
  /** Highest-priority config file that was found, if any. */
  path: string | undefined;
  /** All config files that contributed, lowest priority first. */
  sources: string[];
}

/** Merge `part` over `base`. Objects merge recursively; arrays and scalars replace. */
export function mergeConfig(
  base: GuardrailConfig,
  part: Partial<GuardrailConfig>,
): GuardrailConfig {
  const out = { ...base } as GuardrailConfig;
  for (const key of Object.keys(part) as Array<keyof GuardrailConfig>) {
    const value = (part as Record<string, unknown>)[key];
    if (value === undefined) continue;
    const current = (out as Record<string, unknown>)[key];
    if (
      current &&
      typeof current === "object" &&
      !Array.isArray(current) &&
      value &&
      typeof value === "object" &&
      !Array.isArray(value)
    ) {
      (out as Record<string, unknown>)[key] = { ...(current as object), ...(value as object) };
    } else {
      (out as Record<string, unknown>)[key] = value;
    }
  }
  return validateConfig(out);
}

function validateConfig(cfg: GuardrailConfig): GuardrailConfig {
  const modes: GuardMode[] = ["block", "warn", "off"];
  const checks: CheckMode[] = ["always", "heuristic", "off"];
  const offTopic: OffTopicHandling[] = ["allow", "warn", "block"];
  if (!modes.includes(cfg.mode)) cfg.mode = "block";
  if (!checks.includes(cfg.check)) cfg.check = "heuristic";
  if (!offTopic.includes(cfg.policy.offTopicHandling)) cfg.policy.offTopicHandling = "warn";
  cfg.context.recentMessages = Math.max(0, Math.min(50, cfg.context.recentMessages));
  cfg.context.maxChars = Math.max(500, cfg.context.maxChars);
  cfg.maxPromptChars = Math.max(200, cfg.maxPromptChars);
  cfg.timeoutMs = Math.max(1000, cfg.timeoutMs);
  return cfg;
}

async function readConfigFile(path: string): Promise<Partial<GuardrailConfig> | undefined> {
  try {
    const raw = await readFile(path, "utf8");
    const parsed = JSON.parse(raw) as Partial<GuardrailConfig>;
    return parsed && typeof parsed === "object" ? parsed : undefined;
  } catch {
    return undefined; // missing or unparseable — ignore
  }
}

/** Apply PI_GUARDRAILS_* environment overrides on top of a merged config. */
export function applyEnvOverrides(cfg: GuardrailConfig): void {
  const env = process.env;
  if (env.PI_GUARDRAILS_ENABLED !== undefined) {
    cfg.enabled = env.PI_GUARDRAILS_ENABLED !== "false";
  }
  if (env.PI_GUARDRAILS_MODEL !== undefined && env.PI_GUARDRAILS_MODEL !== "") {
    cfg.model = env.PI_GUARDRAILS_MODEL;
  }
  if (env.PI_GUARDRAILS_MODE !== undefined) {
    const m = env.PI_GUARDRAILS_MODE as GuardMode;
    if (["block", "warn", "off"].includes(m)) cfg.mode = m;
  }
  if (env.PI_GUARDRAILS_CHECK !== undefined) {
    const c = env.PI_GUARDRAILS_CHECK as CheckMode;
    if (["always", "heuristic", "off"].includes(c)) cfg.check = c;
  }
}

export async function loadConfig(
  cwd: string,
  projectTrusted: boolean,
): Promise<LoadedConfig> {
  const sources: string[] = [];
  let merged: GuardrailConfig = structuredClone(DEFAULT_CONFIG);

  const apply = (part: Partial<GuardrailConfig> | undefined, path: string) => {
    if (!part) return;
    merged = mergeConfig(merged, part);
    sources.push(path);
  };

  const envFile = process.env.PI_GUARDRAILS_CONFIG;
  if (envFile) apply(await readConfigFile(envFile), envFile);

  const globalPath = join(homedir(), ".pi", "agent", "guardrails.json");
  apply(await readConfigFile(globalPath), globalPath);

  if (projectTrusted) {
    const projectPath = join(cwd, CONFIG_DIR_NAME, "guardrails.json");
    apply(await readConfigFile(projectPath), projectPath);
  }

  applyEnvOverrides(merged);
  return { config: merged, path: sources[sources.length - 1], sources };
}
