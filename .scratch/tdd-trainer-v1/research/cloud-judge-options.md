# Cloud judge options

Research for [issue 11](../issues/11-cloud-judge-options.md). All sources checked **2026-09-26**. Prices are USD per 1M tokens (MTok), standard tier, no batch.

Context: the local Judge scores declared options SemIf-style: one forward pass, softmax over the option-letter logits, zero generated tokens ([semif-feasibility.md](semif-feasibility.md)). In the [judge eval](../issues/07-judge-eval.md), 22 of 88 gates came back `uncertain`. The cloud judge is an optional fallback for exactly those gates.

## TL;DR

- **Option scoring via logprobs is possible on OpenAI and on OpenAI-compatible open-weight hosts (Together AI, or any self-hosted llama-server/vLLM).** Force a one-token answer, ask for `top_logprobs` (max 20), and softmax over the option letters. This is the same readout as the local judge, but on the first *generated* token instead of the raw prefill logits.
- **Anthropic has no logprobs at all.** Its Messages API has no `logprobs`, `top_logprobs` or `logit_bias` parameter, and prefill returns a 400 error on Claude 4.6 and later. The only route is generated structured output (a JSON-schema `enum`). That gives a verdict but no confidence.
- **Gemini documents `responseLogprobs`/`logprobs` (0–20), but model support is patchy.** Forum reports say the feature was disabled without notice on gemini-2.5-flash (Oct 2025), on 3.x Pro previews (Mar 2026) and on 2.5 (Apr 2026). Do not rely on it; treat Gemini as structured output only unless a probe shows otherwise.
- **Cost is negligible.** A gate of 350–2,000 input tokens costs about $0.00007–0.00023 on gpt-6-luna and about $0.00075–0.0024 on claude-haiku-4-5 (with ~300 tokens of gate prompt). A session with 75 fallback calls costs cents at most.
- **Privacy:** none of the paid APIs train on API traffic by default. They differ on retention: OpenAI keeps abuse logs up to 30 days; Anthropic deletes within 30 days; Gemini paid keeps logs "for a limited period"; Together **stores prompts by default** unless ZDR is switched on. **The Gemini free tier trains on your data and uses human reviewers**, so the free tier must not be supported.
- **Go:** official SDKs exist for all three: `anthropic-sdk-go` v1.75.0, `openai-go/v3` v3.66.0 and `google.golang.org/genai` v1.71.0. For a single binary, though, a hand-written `net/http` client for the OpenAI-compatible chat-completions shape covers OpenAI, Together and self-hosted servers with one code path, the same one the local llama-server judge already uses.

## 1. Scoring method per provider

| Provider | Logprobs? | How to score declared options | Source |
|---|---|---|---|
| **OpenAI** (Chat Completions / Responses) | Yes: `logprobs: true`, `top_logprobs` 0–20; `logit_bias` −100…100 per token ID | `max_completion_tokens: 1` (or an enum schema), `reasoning_effort: "none"`, read `top_logprobs` of the first token, softmax over the letters. `logit_bias` can pin the output to the letter tokens. **Only with reasoning effort `none`.** "When reasoning effort is not `none`, remove `temperature`, `top_p`, and `top_logprobs`. For Chat Completions, also remove `logprobs`." gpt-6-sol and gpt-6-luna support `none`; gpt-6-astra does not. | [chat create ref](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create), [responses create ref](https://developers.openai.com/api/reference/resources/responses/methods/create), [latest-model guide](https://developers.openai.com/api/docs/guides/latest-model), [reasoning guide](https://developers.openai.com/api/docs/guides/reasoning) |
| **Anthropic** (Messages) | **No.** Request params are `max_tokens, messages, model, cache_control, container, diagnostics, inference_geo, metadata, output_config, service_tier, stop_sequences, stream, system, thinking, tool_choice, tools, temperature, top_k`. No logprobs or logit bias. | Structured outputs: `output_config.format` with a JSON schema whose `enum` is the option set. GA, no beta header, supported on all current models incl. haiku-4-5, sonnet-5, opus-5-5. Prefill is rejected (400) on 4.6+. No confidence signal; options: add an `uncertain` option to the enum, or sample N times (N× cost). | [messages create ref](https://platform.claude.com/docs/en/api/messages/create), [structured outputs](https://platform.claude.com/docs/en/build-with-claude/structured-outputs), [prefill](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/prefill-claudes-response) |
| **Google Gemini** (generateContent) | Documented: `responseLogprobs` bool, `logprobs` int "in the range of [0, 20]", response `logprobsResult.topCandidates[]`. **Not supported** in the OpenAI-compat layer or in the newer Interactions API. Per-model enablement is undocumented and has been switched off repeatedly (forum reports, no Google staff reply). | Structured outputs (enum) as the reliable path; logprobs only after a live probe per model. Gemini 3: `thinking_level: minimal` "does not guarantee that thinking is off"; Google "strongly recommend[s] keeping the temperature parameter at its default value of 1.0". | [generate-content ref](https://ai.google.dev/api/generate-content), [Gemini 3 guide](https://ai.google.dev/gemini-api/docs/gemini-3), forum: [2.5-flash disabled](https://discuss.ai.google.dev/t/logprobs-is-not-enabled-for-gemini-models/107989), [3/3.1 disabled](https://discuss.ai.google.dev/t/were-logprobs-disabled-for-gemini-3-3-1-in-vertex-api/132426), [Interactions API](https://discuss.ai.google.dev/t/missing-logprobs-support-in-next-gen-interactions-api-generationconfig-2/144837) |
| **Together AI** (open weights, OpenAI-compatible) | Yes: `logprobs` 0–20 on chat completions; `echo` + `logprobs` on completions also returns **prompt** token logprobs. Note: "logprobs returns Together's own shape, which is richer than OpenAI's." | Same letter-logprob readout. Serves `Qwen/Qwen3.5-9B`, the bigger sibling of the local Qwen3.5-4B, so the local prompts and option layout should transfer almost unchanged. | [logprobs](https://docs.together.ai/docs/logprobs), [serverless models](https://docs.together.ai/docs/serverless-models) |
| **Groq** | **No.** `logprobs`, `logit_bias`, `top_logprobs` → 400. | Structured output only. Not worth adding. | [OpenAI compat](https://console.groq.com/docs/openai) |
| **Self-hosted** (llama-server / vLLM on a stronger box) | Yes (same API the local judge uses) | Identical code path to the local judge; "cloud" = a URL. | [semif-feasibility.md](semif-feasibility.md) |

Mapping to the local judge's `uncertain` rule: with logprobs, the cloud verdict gets the same softmax and threshold treatment as a local one. With structured output only (Anthropic, Gemini), the cloud answer carries no probability, so the product must either trust it outright or offer an explicit `uncertain` option in the enum.

## 2. Candidate models and prices

| Model ID | In / Out per MTok | Notes | Source |
|---|---|---|---|
| `gpt-6-luna` | $0.10 / $0.50 (cached $0.01) | "Most efficient model for focused, high-volume tasks"; effort `none`…`max`, default `medium` | [pricing](https://developers.openai.com/api/docs/pricing), [model page](https://developers.openai.com/api/docs/models/gpt-6-luna) |
| `gpt-6-sol` | $2.00 / $10.00 | supports effort `none` | [pricing](https://developers.openai.com/api/docs/pricing) |
| `gpt-4.1-nano` / `gpt-4.1-mini` | $0.10 / $0.40; $0.40 / $1.60 | older non-reasoning models, classic logprobs | [pricing](https://developers.openai.com/api/docs/pricing) |
| `claude-haiku-4-5` (`-20251001`) | $1 / $5 | "Fastest"; **retirement not sooner than 2026-10-15**, so a successor is imminent | [models overview](https://platform.claude.com/docs/en/about-claude/models/overview), [pricing](https://platform.claude.com/docs/en/about-claude/pricing) |
| `claude-sonnet-5` | $2 / $10 | "Fast"; default effort `high` (set lower for a judge); new tokenizer, ~30% more tokens for the same text | same |
| `claude-opus-5-5` | $4 / $20 | "Moderate" latency; overkill for a gate | same |
| `gemini-3.5-flash-lite` | $0.30 / $2.50 | Stable, July 2026; "low-latency, cost-effective" | [pricing](https://ai.google.dev/gemini-api/docs/pricing), [model page](https://ai.google.dev/gemini-api/docs/models/gemini-3.5-flash-lite) |
| `gemini-3.1-flash-lite` | $0.25 / $1.50 | | [pricing](https://ai.google.dev/gemini-api/docs/pricing) |
| `gemini-3.8-flash` | $0.75 / $3.75 (through 2026-12-31) | | same |
| Together `Qwen/Qwen3.5-9B` | $0.17 / $0.25 | same family as the local judge | [Together pricing](https://www.together.ai/pricing), [serverless models](https://docs.together.ai/docs/serverless-models) |
| Together `Qwen/Qwen3.7-Plus` | $0.32 / $1.28 | bigger Qwen, if 9B is too weak | same |

## 3. Cost per gate

Assumptions: the step is 350–2,000 tokens, plus about 300 tokens of gate prompt, so **650–2,300 input tokens**. Output is 1 token in logprob mode, or about 20 tokens for a JSON enum verdict. Any thinking tokens are billed as output, so set effort to `none`/minimal.

| Model | Per gate (small → large step) | Session: 100 steps × 3 gates × 25% uncertain = 75 calls |
|---|---|---|
| gpt-6-luna (logprob, 1 token) | $0.00007 → $0.00023 | ≈ $0.01 |
| Qwen3.5-9B @ Together (logprob) | $0.00011 → $0.00039 | ≈ $0.02 |
| gemini-3.5-flash-lite (enum, 20 tokens) | $0.00025 → $0.00074 | ≈ $0.04 |
| claude-haiku-4-5 (enum, 20 tokens) | $0.00075 → $0.0024 | ≈ $0.12 |
| claude-sonnet-5 (enum, 20 tokens, ×1.3 tokenizer) | $0.0019 → $0.0062 | ≈ $0.30 |
| gpt-6-sol (logprob) | $0.0013 → $0.0046 | ≈ $0.25 |

The 25% uncertain rate is the judge eval's 22/88. Even on Sonnet, cost does not decide anything; model quality and privacy do.

## 4. Latency

No provider publishes absolute latency numbers. Anthropic ranks its models only relatively (Haiku "Fastest", Sonnet "Fast", Opus "Moderate") ([models overview](https://platform.claude.com/docs/en/about-claude/models/overview)). OpenAI recommends effort `none` for "latency-critical tasks that do not benefit from any reasoning" ([reasoning guide](https://developers.openai.com/api/docs/guides/reasoning)). Google calls 3.5 Flash-Lite "low-latency".

Expectation (not measured): with no reasoning and 1–20 output tokens, a gate is dominated by network round trip plus prefill of about 2k tokens, so roughly sub-second to a few seconds. That is well below the local ~7 s per gate. Gates already run asynchronously, so latency is not a blocker. **Measure it** by running `--regress` against the chosen cloud backend.

## 5. Data retention and training (API traffic)

| Provider | Trains on API data? | Retention | Opt-outs | Source |
|---|---|---|---|---|
| **OpenAI** | No, "unless you explicitly opt in" | Abuse-monitoring logs "retained for up to 30 days"; `/v1/chat/completions` and `/v1/responses` keep no application state by default (`store` false) | ZDR / Modified Abuse Monitoring (by approval); data residency in 12 regions incl. Europe | [your data](https://developers.openai.com/api/docs/guides/your-data) |
| **Anthropic** | No: "Retained data is never used for model training without your express permission" | "automatically delete inputs and outputs on our backend within 30 days"; if flagged by trust and safety, up to 2 years (scores 7 years). **Covered Models (Mythos-class, incl. Fable 5/5.1) are always kept 30 days.** | ZDR by agreement; `inference_geo: "us"` (1.1× price) | [API and data retention](https://platform.claude.com/docs/en/manage-claude/api-and-data-retention), [org data retention](https://privacy.claude.com/en/articles/7996866-how-long-do-you-store-my-organization-s-data), [covered models](https://support.claude.com/en/articles/15425996-data-retention-practices-for-covered-models) |
| **Gemini — paid** | No: "doesn't use your prompts … or responses to improve our products" | Logs kept "for a limited period of time, solely for detecting and preventing violations"; may be cached "in any country" where Google operates | — | [Gemini API terms](https://ai.google.dev/gemini-api/terms) (last modified 2026-03-23) |
| **Gemini — free** | **Yes**, and human reviewers may read inputs and outputs. "Do not submit sensitive, confidential, or personal information." | — | Apps offered to users in the EEA, Switzerland or the UK **must use paid services only** | same; [pricing](https://ai.google.dev/gemini-api/docs/pricing) marks the free tier "Content used to improve our products" |
| **Together AI** | Training share is opt-in, off by default, but "by default, Together stores the prompts you send and the responses … and may use them for product improvements" | Default period not stated | ZDR toggle in org privacy settings; serverless runs in North America | [privacy and security](https://docs.together.ai/docs/privacy-and-security) |

Note: the Learner's own API key means the Learner's own account terms apply. The coach should state which provider receives the code and link its policy.

## 6. Go client options

| Option | Module (latest on proxy.golang.org, 2026-09-26) | Fit |
|---|---|---|
| Anthropic official | `github.com/anthropics/anthropic-sdk-go` v1.75.0 (2026-09-22) | structured outputs; large generated SDK |
| OpenAI official | `github.com/openai/openai-go/v3` v3.66.0 (2026-09-23) | logprobs; `option.WithBaseURL` can target Together/llama-server, but Together's logprobs shape differs from OpenAI's |
| Google official | `google.golang.org/genai` v1.71.0 (2026-08-31) | Gemini API + Vertex |
| **Hand-rolled `net/http` + `encoding/json`** | stdlib | One request/response struct for the OpenAI-compatible chat-completions shape (≈100–200 lines), plus a small Anthropic `/v1/messages` adapter. No dependency weight, and it shares the code with the local llama-server judge. |

## Recommendation

1. **Make the cloud judge the same interface as the local one**, returning an option distribution. Build the **OpenAI-compatible logprob backend first** (hand-rolled `net/http`). It covers OpenAI `gpt-6-luna`/`gpt-6-sol` with `reasoning_effort: "none"`, Together `Qwen/Qwen3.5-9B`, and any self-hosted llama-server/vLLM. The `uncertain` threshold logic carries over unchanged.
2. **Add an Anthropic structured-output backend second** (enum + explicit `uncertain` option, no probabilities). Many Learners already have a Claude key, and quality is likely highest. Prefer `claude-sonnet-5` at low effort. Haiku 4.5 is cheaper but can retire from 2026-10-15.
3. **Defer Gemini.** Its logprobs are unreliable, it needs a paid tier (the free tier trains on code and is barred for EEA/UK/CH apps), and it adds nothing over the two backends above.
4. **Before choosing a default model**, run the 22 uncertain fixtures from the judge eval through gpt-6-luna, Qwen3.5-9B@Together and claude-sonnet-5, then compare accuracy and latency. Cost is irrelevant at cents per session.
5. **Privacy UX:** cloud is off by default. Enabling it names the provider and links its retention policy. Warn that Together stores prompts unless ZDR is on.
