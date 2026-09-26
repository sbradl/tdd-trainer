# Tune weak gates

Status: ready-for-agent
Blocked by: 07
Spec: ../spec.md (§7)

## What

Improve multi, structural, tpp (variable, list, {}→Nil for new files, pattern-clause selection), one-behaviour 'no', refactor-effect 'worsens', cheating. Change questions/options only against `tddt judge --regress`; add fixtures for real misses.

## Done when

Confident+correct improves over the baseline with 0 confident-wrong and no regressed fixture.
