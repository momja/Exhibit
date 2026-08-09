/**
 * Guardrails — fast heuristic prefilter.
 *
 * Regexes are cheap and instant, so they gate whether the (slow) guardrail LLM
 * is consulted. A hit does not itself decide the verdict unless
 * `prefilter.blockDirect` is enabled — by default it only routes the prompt to
 * the guardrail LLM, which has the final say.
 */
import type { GuardrailConfig } from "./config.ts";

export interface PrefilterHit {
  /** "injection" for instruction-override patterns, otherwise the topicPatterns key. */
  category: string;
  /** The regex source that matched. */
  pattern: string;
}

const regexCache = new Map<string, RegExp | undefined>();

function compile(pattern: string): RegExp | undefined {
  let cached = regexCache.get(pattern);
  if (cached !== undefined || regexCache.has(pattern)) return cached;
  try {
    cached = new RegExp(pattern, "i");
  } catch {
    cached = undefined; // malformed user regex — skip it
  }
  regexCache.set(pattern, cached);
  return cached;
}

export function prefilterPrompt(
  text: string,
  cfg: GuardrailConfig,
): PrefilterHit | undefined {
  if (!cfg.prefilter.enabled) return undefined;

  for (const pattern of cfg.prefilter.injectionPatterns) {
    const re = compile(pattern);
    if (re && re.test(text)) return { category: "injection", pattern };
  }

  for (const [category, patterns] of Object.entries(cfg.prefilter.topicPatterns)) {
    for (const pattern of patterns) {
      const re = compile(pattern);
      if (re && re.test(text)) return { category, pattern };
    }
  }

  return undefined;
}
