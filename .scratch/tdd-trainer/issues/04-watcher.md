# Watcher, debounce and cancellation

Status: done
Blocked by: 01
Spec: ../spec.md (§4)

## What

fsnotify watcher honouring tests/sources/ignore globs; debounced runs; cancel in-flight run on new save (process tree kill on Linux and Windows); slow-run warning.

## Done when

Rapid saves trigger one run; a save during a run cancels it; ignored files don't trigger; works on Windows (manual check).

## Notes from implementation

- `internal/watch` (fsnotify, recursive, new folders picked up, hidden + ignored folders skipped, 400 ms debounce) and `internal/session.Loop` (cancel in-flight run on a new batch, merged changes, no overlapping runs, slow-run event).
- Process tree kill: Linux process group + SIGKILL (mutation-tested); Windows `taskkill /T /F` — **not verified on Windows** (manual check still open).
- `tddt` now watches and prints plain lines until the TUI slice; `tddt --once` keeps the one-shot behaviour.
