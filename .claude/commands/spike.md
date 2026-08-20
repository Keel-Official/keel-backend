---
description: Run a time-boxed spike, write the finding, do not write production code
argument-hint: [the question to answer]
allowed-tools: Read, Write, Grep, Glob, Bash(go run:*), WebFetch
---

Spike to answer: $ARGUMENTS

This is a spike, not an implementation. The rules:

1. The goal is to answer ONE factual question, not to build a feature.
2. Spike code goes in `scripts/spike/` and is disposable. Do not touch
   `internal/`.
3. When the spike is done, write the finding into `docs/decisions/` as a short
   note: the question, what was tried, what came out, what was decided.
4. If the answer is "cannot be done" or "the data is not there", that is a valid
   and valuable result. Say it plainly.

Do not move on to implementation without Al agreeing first.
