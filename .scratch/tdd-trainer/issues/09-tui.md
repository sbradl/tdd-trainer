# TUI: Quiet coach + Dashboard

Status: done
Blocked by: 08
Spec: ../spec.md (§8)

## What

Quiet coach default view, Dashboard on Tab, hotkeys r/g/f/b with toast, report-now key, q. Use the prototype on branch prototype/verdict-ux as the visual reference (views C and B).

## Done when

A live kata session shows steps and verdicts as specified; works in Windows Terminal and common Linux terminals.

## Notes from implementation

- Bubble Tea v1 + lipgloss (`internal/tui`); `internal/app` orchestrates watcher/runner/snapshots/steps/coach behind a mutex and sends everything to a sink (TUI, or plain lines when stdout is not a terminal).
- Keys: Tab view, r/g/f phase (r = writing a test → Red in progress, g = making it pass, f = refactoring), b baseline reset (reruns tests at once), w report now, q/Ctrl-C quit.
- Controller calls from keys run as commands: calling them inside Update deadlocked (the app sends back into the program).
- Verified on Linux in a real pty rendered with pyte (status line, Red-in-progress card, cycle collapse, missed-refactor card, Dashboard gate table with uncertain greyed, override toast, clean quit). **Windows Terminal not verified.**
- Note: Bubble Tea queries the terminal background colour at package init; terminals that never answer OSC 11 delay startup by up to 5 s.
