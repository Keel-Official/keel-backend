---
description: Review code Al wrote himself, without rewriting it
argument-hint: [file path]
allowed-tools: Read, Grep, Glob, Bash(go vet:*), Bash(go test:*)
---

Review the code Al wrote in: $ARGUMENTS

DO NOT rewrite the code. DO NOT paste a corrected version. Report findings as a
list, each one in this format:

- [BLOCKER / SERIOUS / MINOR] line ~N: what the problem is, why it matters, and a
  question that leads Al to the fix himself.

Order of inspection:

1. Financial correctness. Any float64? Any unintended rounding? Any division that
   can be by zero?
2. Domain correctness. Are SDEX and AMM depth combined at a shared marginal price
   limit rather than summed separately?
3. Reproducibility. Are map iterations sorted? Are LedgerSeq and
   MethodologyVersion carried through?
4. Error handling. Is any error swallowed silently?
5. Idiomatic Go. Last, and mark it MINOR.

If there is no BLOCKER, say so. Do not invent problems to look thorough.
