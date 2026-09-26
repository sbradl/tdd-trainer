# Spike: Laya as a faster judge

Status: idea
Source: user suggestion, 2026-09-26
Link: https://github.com/NandhaKishorM/laya

## What

Laya (Apache-2.0) scores typed choices in one forward pass with an encoder (ModernBERT-large for English, mmBERT-base multilingual), without generating text: the same idea as our judge, reportedly about 33 ms per question on a T4. Our judge takes about 0.4 s per 128-token block on the Radeon 860M, about 4 s for the lens review after each Green.

Find out whether it can answer our gates well enough to replace or support Qwen3.5-4B.

## How

- Python script outside the product: run every regress fixture (`internal/judge/fixtures`, currently 149) through Laya with the same question and options.
- Compare with `tddt judge --regress`: confident and correct, confident and wrong (must stay 0), uncertain, per gate.
- Measure latency on this machine (Ryzen AI 7 350, CPU only: PyTorch doesn't support the 860M).

## Known obstacles

- **Runtime:** Python + PyTorch; tddt is one Go binary without cgo. Options if it wins: ONNX export plus a Go runtime (the common ONNX bindings use cgo), or a sidecar process.
- **Context:** 512 tokens (English) / 1,024 (multilingual, up to 8,192 claimed). Our evidence is often 2,000–4,000 tokens; parts are clipped at 6,000 characters each.
- **Code judgement:** published results are on general text decisions; TPP transformations and code smells are untested.
- **Calibration:** our 0.8 floor and the two-probe rule assume the judge's probabilities; Laya's calibration (trained against proper scoring rules) needs its own floor.

## Done when

A table of per-gate accuracy and latency, Laya vs. the current judge, and a go/no-go.
