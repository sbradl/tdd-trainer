# Shadow snapshot store

Status: ready-for-agent
Blocked by: 04
Spec: ../spec.md (§5)

## What

go-git shadow repo in .tddtrainer/ (own git dir, project as work tree), self-ignoring .gitignore, one snapshot per test run, diff between snapshots split into test vs source files.

## Done when

Snapshots never alter the learner's .git; diffs between any two runs are retrievable; works without system git.
