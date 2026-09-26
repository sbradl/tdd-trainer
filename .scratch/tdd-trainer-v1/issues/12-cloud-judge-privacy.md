# Cloud judge and privacy

Type: grilling
Status: resolved
Blocked by: 11
Map: ../map.md

## Question

How is the optional cloud judge configured (provider, API key location, which gates may fall back), and what does the learner consent to about code leaving the machine? Is fallback on by default once a key is present, or per-gate opt-in?

See [Cloud judge options](11-cloud-judge-options.md): OpenAI-compatible logprob backends keep the option scoring and confidence; Anthropic only gives a structured-output verdict without confidence; Gemini deferred (free tier trains on inputs).

## Out of scope

User decision (2026-09-26): no cloud judge in v1. The judge is local only; `uncertain` stays silent. Research in [Cloud judge options](11-cloud-judge-options.md) remains as input for a later effort.
