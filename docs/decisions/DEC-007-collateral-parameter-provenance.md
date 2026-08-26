# DEC-007: `h` and `m` in C_max have no provenance, and the on-chain values do not fit them

**Status:** OPEN. Drafted by Claude 26 August 2026 as a question, not a decision. No
option below is chosen and none should be read as recommended by anything other than
its own argument. Al rules.
**Found:** 26 August 2026, while checking an outbound email that intended to ask Blend
for "the real protocol values" of these two parameters.
**Evidence:** `docs/evidences/2026-08-25-oracle-and-pool-config.md` section 4, read
from pool `CCCCIQSDILITHMM7PBSLVDT5MISSY7R26MNZXCX4H7J5JQ5FPIYOGYFS` at ledger
64119285.
**Touches:** `docs/methodology/08-collateral.md` section 1, `Params` in
`internal/domain/types.go`, `maxSafeCollateral*` in `docs/api/keel-openapi.yaml`.

---

## 1. The finding

`08-collateral.md` section 1 defines the only number in the methodology that tells a
lender what to do:

```
C_max = min( D_sell(δ_liquidation) × h , MC_orderbookOnly(δ_critical) × m )
```

Four parameters carry defaults. Two of them have a written rationale and two do not.

| Symbol | Meaning | Default | Rationale on record |
|---|---|---|---|
| `δ_liquidation` | liquidation discount | 0.10 | none |
| `h` | liquidation haircut | 0.5 | none |
| `δ_critical` | critical manipulation delta | 0.5 | yes, `08-collateral.md` section 1, monotonicity argument |
| `m` | manipulation safety margin | 0.25 | none |

The document states "Default values are **chosen, not calibrated**" and requires that
sentence to appear on the dashboard and in API responses, so the absence of provenance
is disclosed rather than hidden. That is the honest half. The other half is that
`PRD` section 12 question Q3 commits to replacing them: "The Blend risk parameters
actually in force in February 2026, for use as the C_max defaults", scheduled Week 1.
Q3 is still open, and this record is what stands between it and being answered wrongly.

## 2. Why Q3 cannot be answered as written

Q3 assumes the protocol has parameters that correspond to `h` and `m`. It does not.

The pool's reserve configuration was read on-chain and holds `c_factor` and
`l_factor` per asset, plus utilisation and interest-curve fields:

| Asset | `c_factor` | `l_factor` |
|---|---|---|
| USTRY, index 5 | 0, collateral disabled, post-incident remediation | 0.90 |
| USDC, index 1 | 0.95 | 0.95 |
| XLM, index 0 | 0.75 | 0.75 |

Neither field is what `h` or `m` multiplies.

- `c_factor` scales the **value of collateral a borrower has posted**, inside the
  protocol's own health calculation. It is a protocol solvency parameter.
- `h` scales **`D_sell(δ_liquidation)`, the market's sell-side depth at a 10 percent
  price drop**. It is a haircut on how much of that depth Keel is willing to believe
  is available to a liquidator.
- `m` scales **`MC_orderbookOnly(δ_critical)`, a quantity Keel itself measures**. No
  protocol publishes a margin on someone else's manipulation-cost metric, because no
  protocol computes that metric.

The magnitudes make the mismatch concrete rather than definitional. Every real
`c_factor` in that pool sits between 0.75 and 0.95. `h` is 0.5, below all of them. If
`h` were an attempt to model `c_factor`, it has been wrong by a wide margin since it
was written; read as Keel's own conservatism it is simply conservative. The second
reading is the one the methodology text supports.

So the answer to "where are the real values published" is that for `c_factor` and
`l_factor` they are not published at all, they are read from the pool contract, and
that reading is already done and recorded. For `h` and `m` there are no real values
anywhere, because they are not protocol quantities.

## 3. What is genuinely still unknown, and is not this question

One number blocks section 4 of the evidence file: `c_factor` for USTRY at ledger
61340408. Today it reads 0, which is post-incident remediation, and it must have been
above 0 in February because the attacker borrowed against USTRY as collateral. The
evidence file establishes that it is not recoverable from the incident transaction's
`resultMetaXdr`, because reserve configuration is read-only during a borrow and does
not enter the write meta. The authoritative source is the remediation `set_reserve`
transaction that followed 22 February 2026, whose `LEDGER_ENTRY_STATE` carries the
pre-change value.

That is an evidence-collection item, already tracked in section 6 of the evidence
file, and it is Al's. It is recorded here only so that this record is not mistaken for
that one. **Resolving it does not resolve this record**, and that is the trap worth
naming: an incident-time `c_factor` of, say, 0.96 is a real and interesting number
that still says nothing about what `h` should be, because they multiply different
quantities. Getting Q3's data and believing Q3 is therefore answered is the specific
error this record exists to prevent.

## 4. The options

**A. Keep `h` and `m` as declared choices, and say so in one more place.**
Change nothing numerically. Add to `08-collateral.md` section 1 a sentence stating
that `h` and `m` are Keel's own conservatism parameters, that no protocol publishes an
equivalent, and that `c_factor`/`l_factor` are not their analogues. Close Q3 as
malformed and replace it with the section 3 item.
*For:* it is already true, it is already disclosed, and it costs one paragraph.
*Against:* `C_max` keeps two constants that a reviewer or funder can ask about and
receive only "chosen" as an answer. For the one output that tells a lender what to do,
that may not survive review.

**B. Re-derive `C_max` so the protocol values enter where they actually belong.**
`c_factor` and `l_factor` do have a correct home: a borrower's position is unhealthy
when `collateral × c_factor < debt / l_factor`, so the protocol's own liquidation
trigger is expressible from values now read on-chain. `C_max` could bound the
collateral at which a manipulated price first makes that inequality bite, replacing
`h` with protocol parameters and leaving only `m` chosen.
*For:* it removes one unsourced constant and grounds the result in values that are
read rather than picked. It also makes `C_max` pool-specific, which is closer to what
a lender actually faces.
*Against:* it changes the shape of `C_max`, needs `LTV` or its equivalent as an input,
and reaches the contract, the mocks and the frontend. `08-collateral.md` already
defers a different re-derivation to 1.1 for exactly this reason, so this is a 1.1
candidate at the earliest, not a 1.0.x fix.

**C. Calibrate `h` and `m` against the incident.**
Pick the pair that would have flagged the USTRY position as unsafe before 22 February
while not flagging USDC or XLM in the same pool.
*For:* it replaces "chosen" with "calibrated against the one case we have".
*Against:* one incident is one data point, and the fixture's own manipulation term is
near zero, so almost any `m` above zero satisfies it. It would convert an honest
"chosen" into a "calibrated" that is not meaningfully stronger, which is worse than
leaving it, and `docs/methodology/11-limitations.md` would then have to explain why
calibrated does not mean validated.

**D. Do nothing and leave Q3 open.**
*For:* zero risk to a deliverable that is mid-sprint.
*Against:* the email that prompted this record was about to be sent. The next person
to read Q3, including a future Claude, will read it the same way and ask the same
malformed question of an outside party.

## 5. What this record does not decide

It does not decide the numbers. `h` and `m` are inputs to the paid deliverable and
`docs/methodology/` is RED, so any change to section 1 of `08-collateral.md` is Al's
to write. Options B and C would also change `Params` and, through
`maxSafeCollateral*`, the API contract, which brings DEC-003's freeze conditions into
scope. Neither is a comment-level edit.

## 6. Open items

- [ ] Al rules between A, B, C and D.
- [ ] If A: one paragraph in `08-collateral.md` section 1, and Q3 in the PRD is closed
      as malformed and replaced by the `c_factor` item.
- [ ] If B: this record is superseded by a 1.1 methodology record, not amended.
- [ ] Independent of the ruling: USTRY `c_factor` at ledger 61340408, tracked in
      `docs/evidences/2026-08-25-oracle-and-pool-config.md` section 6.
