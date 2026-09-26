# Cloud judge options

Type: research
Status: resolved
Blocked by: none
Map: ../map.md

## Question

Which cloud LLM APIs can serve as the optional fallback judge when the local judge is 'uncertain' (see [Judge eval prototype](07-judge-eval.md))? For each (Anthropic, OpenAI, Google Gemini, others worth considering): can it score declared options via logprobs, or only via generated structured output? Latency and cost per gate for a typical step (~350–2000 tokens). Data retention and training-use terms for API traffic (for a privacy decision). Minimal Go client options.

## Answer

Findings: [cloud-judge-options.md](../research/cloud-judge-options.md) (sources checked 2026-09-26).

Gist: OpenAI (reasoning effort `none`) and OpenAI-compatible open-weight hosts (Together, self-hosted llama-server/vLLM) return `top_logprobs` (max 20), so the cloud judge can use the same option-letter readout as the local judge. Anthropic has no logprobs and rejects prefill on 4.6+, so it can only give structured output (enum) without a confidence. Gemini documents logprobs, but they have been switched off per model several times, and its free tier trains on inputs. Cost is cents per session. No paid API trains on API data by default.

| Provider / model | Option scoring | Per gate (650–2,300 in) | Retention / training |
|---|---|---|---|
| OpenAI `gpt-6-luna` ($0.10/$0.50) | logprobs, effort `none` | $0.00007–0.00023 | abuse logs ≤30 d; no training |
| Together `Qwen/Qwen3.5-9B` ($0.17/$0.25) | logprobs (+prompt logprobs via echo) | $0.00011–0.00039 | stores prompts by default unless ZDR; training opt-in |
| Anthropic `claude-haiku-4-5` ($1/$5), `claude-sonnet-5` ($2/$10) | structured-output enum only | $0.00075–0.0024 / $0.0019–0.0062 | deleted ≤30 d (flagged ≤2 y); no training |
| Gemini `gemini-3.5-flash-lite` ($0.30/$2.50) | enum; logprobs documented but unreliable | $0.00025–0.00074 | paid: short abuse logs, no training; **free: trains + human review** |
| Groq | enum only (logprobs → 400) | — | — |

Latency: no provider publishes figures. The expectation is sub-second to a few seconds with no reasoning, which is below the local ~7 s. Measure it with `--regress`.
Go: official SDKs exist (`anthropic-sdk-go` v1.75.0, `openai-go/v3` v3.66.0, `google.golang.org/genai` v1.71.0). A hand-rolled `net/http` client for the OpenAI-compatible shape covers OpenAI, Together and self-hosted with one code path.

**Recommendation:** use the same judge interface for cloud and local, returning an option distribution. Backend 1 is OpenAI-compatible logprobs (hand-rolled HTTP): gpt-6-luna, Qwen3.5-9B@Together, or self-hosted. Backend 2 is Anthropic structured output with an explicit `uncertain` option (Sonnet 5 at low effort; Haiku 4.5 may retire from 2026-10-15). Defer Gemini. Pick the default model by running the 22 uncertain eval fixtures through the candidates. Cloud is off by default, and enabling it names the provider and its retention policy.
