# Watcher, debounce and cancellation

Status: ready-for-agent
Blocked by: 01
Spec: ../spec.md (§4)

## What

fsnotify watcher honouring tests/sources/ignore globs; debounced runs; cancel in-flight run on new save (process tree kill on Linux and Windows); slow-run warning.

## Done when

Rapid saves trigger one run; a save during a run cancels it; ignored files don't trigger; works on Windows (manual check).
