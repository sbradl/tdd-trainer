# Presets, init and auto-detection

Status: ready-for-agent
Blocked by: 02
Spec: ../spec.md (§3)

## What

Built-in presets for Go, Python, TypeScript (jest|vitest), .NET (xUnit|NUnit|MSTest), Lua (busted), Elixir; marker-file detection; `tddt init` with confirm/pick/blank flows; auto-init when config is missing; Elixir junit_formatter instructions; per-OS command overrides; tpp_order per preset.

## Done when

In sample projects of each of the six languages, `tddt init` proposes the right preset and the written config produces a correct Test state.
