# Guided hints (idea for a later version)

Status: idea
Source: user feedback, 2026-09-26

## What

An opt-in mode (e.g. `--guide` or a config flag) where a failed check comes with more help on what to do next, not only what went wrong. Examples:
- wrong fail reason → how to make the test reach its assertion in this language (stub signature, import);
- several new tests → which one to keep for now and how to park the others (skip/comment);
- simpler change possible → the concrete next-smaller transformation to try;
- refactor now → a first concrete refactoring move for the named problem.

## Open

- Where the extra text comes from: static per check + language, or generated (the judge only scores options today).
