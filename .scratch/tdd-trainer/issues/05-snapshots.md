# Shadow snapshot store

Status: done
Blocked by: 04
Spec: ../spec.md (§5)

## What

go-git shadow repo in .tddtrainer/ (own git dir, project as work tree), self-ignoring .gitignore, one snapshot per test run, diff between snapshots split into test vs source files.

## Done when

Snapshots never alter the learner's .git; diffs between any two runs are retrievable; works without system git.

## Notes from implementation

- Deviation from spec §5 wording: no go-git worktree/index with the project as work tree. The store walks the project itself (hidden + ignored folders skipped) and writes blobs/trees/commits directly into a bare repo `.tddtrainer/snapshots.git` (ref `refs/heads/snapshots`). Only files matching `tests`/`sources` are snapshotted. Reason: go-git status over the whole tree (node_modules etc.) is slow; same guarantees (learner's .git untouched, no system git).
- Unchanged files are not re-read (mtime + size cache).
- Shadow repo passes `git fsck --strict` (tested when git is available).
- A snapshot is taken at the start of every run; `session.RunDone` carries its ID.
- Binary is now 18 MB (go-git); strip with `-ldflags "-s -w"` in slice 11.
