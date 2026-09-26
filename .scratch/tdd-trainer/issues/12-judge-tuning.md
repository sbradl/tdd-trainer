# Tune weak gates

Status: done
Blocked by: 07
Spec: ../spec.md (§7)

## What

Improve multi, structural, tpp (variable, list, {}→Nil for new files, pattern-clause selection), one-behaviour 'no', refactor-effect 'worsens', cheating. Change questions/options only against `tddt judge --regress`; add fixtures for real misses.

## Done when

Confident+correct improves over the baseline with 0 confident-wrong and no regressed fixture.

## Notes from implementation

Original 88 fixtures, reuse mode: GPU 69 → 81, CPU 69 → 82; 0 confident-wrong, no regressed fixture on either backend. With 7 new fixtures: 86/95 on both.

- tpp: sharper option texts (nil = new code returning nothing, constant incl. new code returning literals, variable incl. concatenation/interpolation, selection incl. new guarded/pattern clause, recursion on the tail, list "[x] instead of x") + "look at added vs removed lines": 13 → 19/19.
- red-check: names stub exceptions (NotImplementedError, null reference) as wrong reason. New real fixtures `red-check.no.cs-notimpl` (was **confidently wrong**: yes p=0.83) and `red-check.no.ex-raise`.
- one-behaviour: "yes only if all assertions check one outcome of one call or scenario": 3 → 6/6.
- structural: lists pipeline rewrites as structural and changed literal/default/condition/boundary as behaviour: 5 → 6/6.
- review-pragmatic: "hard-coded values" narrowed to environment-specific values (hosts, URLs, paths, credentials, limits). New fixture `review-pragmatic.no.go-fake-it` from a live session (was **confidently wrong**: yes p=0.92). Fake-it fixtures for all lenses added; ddd/philosophy stay uncertain on it (silent live), smells uncertain on CPU.
- Not improved: multi (tried 5 variants, all worse), refactor-effect worsens, review-ddd.no.error-message, review-philosophy.yes, step-size.constant.go-add-marker.
- Prompt-parity tests now cover only untuned gates (tuned ones differ from the Python reference by design).
- `internal/judge/lab_test.go`: env-gated lab for comparing variants.
