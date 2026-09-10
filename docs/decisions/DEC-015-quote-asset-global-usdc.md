# DEC-015: The quote asset is global and it is USDC

**Status:** Accepted
**Date:** 2026-09-05
**Kind:** Methodology. A definition changed, so this is a minor bump and not a patch:
the absolute thresholds acquire a unit they did not have, and the band rule acquires a
scope it did not have. No formula changed and no threshold VALUE changed.
**Drafted by:** Claude
**Decided by:** Al, 5 September 2026, recorded in `docs/methodology/02-pair-selection.md`
section 1, which is the source. That file is marked DECIDED at 1.1.0-draft.
**Zone:** `docs/decisions/` (YELLOW). Claude drafts and amends a record here and must
not create or reverse a decision. The decision in section 1 is Al's and is his to alter;
everything else in this record is drafting.
**Methodology version:** `1.0.8-draft` to `1.1.0-draft` for the whole document set, under
the one-version rule of DEC-014 section 1.
**Settles:** open question Q7, and PRD open question Q6 with it.

---

## 1. The decision

**The quote asset is global and it is USDC**, issuer
`GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN`. Every published depth,
manipulation cost, `maxSafeCollateral` and threshold comparison is denominated in that
asset. It is not chosen per asset.

It follows that the two absolute thresholds in `09-flags-and-bands.md` section 6 are
USDC figures: `ManipulationCheapAbsolute` is 10,000 USDC and `ThinDepth5PctAbsolute` is
50,000 USDC.

**The full text of the decision, its rationale and its consequence live in
`02-pair-selection.md` section 1.** This record does not restate them. `docs/methodology/`
is RED and it is the paid deliverable; a second home for a definition drifts, and this
repository has been bitten by that specifically. What this record carries is what the
decision did to everything outside that file.

## 2. The condition it meets, and why the version moves

`09-flags-and-bands.md` section 6 carried a subsection headed "An unresolved limitation
of units". Its last sentence read: *"This is open question Q7 and must be settled before
version 1.1."*

So the methodology set named its own condition for a 1.1 and then held at 1.0.x for eight
patch versions. **The condition is now met.** That is the whole reason this is 1.1.0 and
not 1.0.9: a document that says it may not reach 1.1 until X, and then reaches 1.1, has
to be able to point at X.

The subsection is replaced rather than annotated. It is not a decision being reversed, so
the append-only rule for decision records does not reach it; it is a worksheet paragraph
whose subject was decided, and leaving "unresolved" standing next to the resolution is the
two-homes defect this repository keeps paying for. The version-history row in that file
records that the paragraph was replaced and what it said.

## 3. What moved, under DEC-014's one-version rule

DEC-014 section 1 fixed one methodology version for the whole document set. Q7 was
resolved in one file, so the whole set moves.

| Moved | To | Content change? |
| --- | --- | --- |
| `02-pair-selection.md` | 1.1.0-draft | **Yes.** Al's, all seven decisions recorded |
| `09-flags-and-bands.md` | 1.1.0-draft | **Yes.** Section 6, per section 2 above |
| The other ten methodology files | 1.1.0-draft | No. Header and one history row |
| `docs/methodology/README.md` | 1.1.0-draft in force | Section 4 gains the row; the section 1 status of `02` moves off "worksheet" |

**Two files out of twelve changed content, and the row each of the other ten gained says
so in as many words.** DEC-014 established the shape of that row and the reason for it: a
reader who finds a file at 1.1.0-draft must be able to tell from the file itself whether
anything in it moved.

## 4. What did NOT move, and it is the part of this record worth reading

`internal/domain.MethodologyVersion` stays at `1.0.8-draft`. The contract's
`methodologyVersion` examples stay at `1.0.8-draft` with it, because DEC-014 section 5
established that they follow the code constant rather than the documents.

**This is a deliberate, dated divergence between the documents and the code, and it is
the first one since DEC-014 unified them.** The reasoning:

The constant is not a label on the documents. It is stamped on every stored row and on
every response, and NFR-9 promises that re-running at the same `ledgerSeq` with the same
`methodologyVersion` produces identical numbers. That promise is about numbers.

`02-pair-selection.md` section 2 defines a rule that changes numbers: the band is the
highest tier triggered on any evaluated pair, not on the primary pair alone. **Nothing
implements it yet**, and nothing may implement it yet, because `compute.go` is governed by
DEC-008's ordering rule and the expected values do not exist in the golden fixture.

So if the constant moved today, output computed on the primary pair alone would be stamped
`1.1.0-draft`, and output computed under section 2 would later be stamped `1.1.0-draft`
too. Two different computations under one version string is precisely what NFR-9 forbids,
and it is worse than the split DEC-014 closed, because a version split between documents
is visible to a reader while a version collision inside stored rows is not.

The alternative reading is available and is not absurd: nothing computed today changes,
because every existing figure in this repository is already USDC-denominated, so
`1.1.0-draft` would be an honest label for today's output and the implementation of
section 2 would simply bump again. **It rests on the implementation remembering to bump**,
and the rule as written in `types.go` only requires a bump when a definition or threshold
changes. Under that rule the implementation of an already-published definition does not
have to bump, which is the trap.

**Handed to Al.** The honest options are: move the constant now and write the required
second bump into the ordering rule so it cannot be forgotten, or hold the constant until
section 2 is implemented and move it then. Claude may report the divergence and may not
choose. `docs/methodology/README.md` states it in its header rather than leaving it to be
discovered, and no check in `scripts/audit-verification.sh` currently compares the two, so
the header is the only thing standing between this and a silent split.

## 5. The dependent amendments

`02-pair-selection.md` section 2 carries a table headed "Changes this decision requires".
Its state:

| File | Change | State |
| --- | --- | --- |
| `internal/domain/types.go` | `PrimaryQuote`, `PairsEvaluated []PairSummary`, `BandDrivenBy`, `XlmUsdcRate` on `AssetRisk`; warning `SECONDARY_PAIR_WORSE` | **Done.** Shapes only, see section 6 |
| `docs/api/keel-openapi.yaml` | the same four fields, mocks regenerated | **Done**, contract 1.5.0, see section 7 |
| `internal/store` | none needed; the `assets` unique constraint already covers `(code, issuer, quote_code, quote_issuer)` | **Confirmed, nothing done.** See section 6 |
| `09-flags-and-bands.md` section 6 | replace the unresolved-units paragraph | **Done**, see section 2 |
| PRD section 11 | mark Q6 and Q7 answered, pointing at `02` | **REFUSED, and it is Al's.** See section 8 |
| PRD FR-11 | note the candidate-set bound | **REFUSED, and it is Al's.** See section 8 |

## 6. The type changes are shapes, not computation, and that is the ordering rule

`internal/domain/types.go` is YELLOW and the four fields are declared there. **None of
them is populated**, in `compute.go` or anywhere else.

That is not an unfinished job. `compute.go` went YELLOW under DEC-008 and what replaced
the lock is an ordering rule: a function may only be written after its expected values
exist in the golden fixture. A `bandDrivenBy` needs a band computed on an XLM pair, and an
`xlmUsdcRate` needs a rate; both are numbers, and numbers that test the implementation may
not be produced by the thing they test. Writing the population now and the fixture
afterwards is exactly the inversion the ordering rule exists to prevent.

`internal/store` persists named columns rather than a blob, so four unpersisted fields on
`AssetRisk` now exist. **This is a known gap and it has a precedent that is deliberate**:
`Supporting.GenuineVolumeInWindow` has no column either, `metrics.go` opens with the
reason, and `TestGenuineVolumeInWindowIsNotPersisted` asserts the gap rather than leaving
it to be discovered. These four should be covered the same way when they are populated,
and the columns land in the same commit as the computation, not before it. A column now
would be, in `0001_core.sql`'s own words, "clutter that reads like a promise".

**The three sentences the YELLOW zone asks for, on the `types.go` change.** The four
fields are declared as shapes in the same pass that publishes the contract, so a consumer
and the store schema can both be designed against one agreed shape before any number
exists to put in it. `PairSummary` deliberately carries only quote, band and confidence
and not depth or cost, because those are denominated in their own pair's quote asset and
two units in one response invite the arithmetic section 1 of the methodology exists to
forbid. `SECONDARY_PAIR_WORSE` is added as a named code rather than as prose even though
`Warnings` is `[]string` and every other member of it is a sentence, because a consumer
has to branch on this one: the band and the depth beside it then describe different
markets.

**The alternative rejected:** making `Warnings` a typed enum so the new code is not a bare
string among sentences. It is the better shape and it was rejected as out of scope. It
would migrate five existing prose warnings into codes, change the `warnings` array in the
contract from free text to an enum, and break the one thing the contract says about that
field, which is that its contents are shown to the user. That is a contract change with
its own argument, and this decision does not license it.

## 7. The contract moved to 1.5.0 under DEC-003, and the freeze was checked first

DEC-003 governs when the contract stops moving. **It is not frozen.** Three of its four
freeze conditions in section 7 are met and the fourth needs the frontend builder. Checked
before touching the file, and again after:

| Freeze condition | Before | After |
| --- | --- | --- |
| 1. golden fixture filled in by hand | MET | MET, untouched |
| 2. no `TODO-FIXTURE` or `reachable: null` left | MET | MET, no example changed |
| 3. `spreadPct` scale agreed as percent | MET | MET, untouched |
| 4. section 6 questions answered by the frontend builder | OPEN | OPEN, unchanged |

**No freeze condition is broken by this change, and none is closed by it.** Condition 4
was open before and is open after; this pass neither depends on it nor advances it. The
contract has moved twice already while condition 4 was open, at 1.3.0 and 1.4.0, so this
is the established route rather than a new one.

Additive and minor: four optional fields and one new schema, `PairSummary`. Nothing was
renamed, no enum gained a member, no example changed, and `docs/api/mocks/` is
byte-identical after regeneration, which `make api-mocks-check` confirms.

**The four fields are OPTIONAL rather than required, and that is the judgement call.**
DEC-003 section 8.1 made three additive fields required, and the difference is that the
server already produced those three. Nothing produces these four. A required field the
server does not send is a contract that lies, and DEC-014 section 5 records what that has
already cost here: a `/methodology` example advertised a version the server did not
return, and the generated mock served it to the frontend.

**No example carries the new fields**, for the same reason and for one more.
`primaryQuote` could be filled honestly, since USDC is a constant of the decision rather
than a computed number, and it is still left out: a mock that shows a field no response
carries is the same failure from the other direction. `pairsEvaluated` and `xlmUsdcRate`
cannot be filled at all without inventing a band and a rate, and DEC-003 section 4 already
refused that reasoning once, in its own words: deriving the numbers in the contract "would
be handing over the answers to the worksheet".

**THE FRONTEND BUILDER HAS TO BE TOLD, and this is a new item on that message.** DEC-003
section 9.2 established that the `manipulationCost` rename travels in the same message as
the section 6 questions. This joins it. The thing to say is not "four fields were added";
it is that **an asset's band may now be set by a pair other than the one whose depth is
displayed beside it**, so a row showing a band and a depth as one reading is wrong in that
case. `bandDrivenBy` and the `SECONDARY_PAIR_WORSE` warning are how a display detects it.
That is the same class of damage as the `cost`/`reachable` pairing in DEC-003 section 1:
nothing fails, the number is still rendered, and only its meaning is wrong.

**The three sentences the YELLOW zone asks for, on the contract change.** The fields are
published now rather than with the computation so the frontend builder can answer the
section 6 questions and this change in one message instead of two, and condition 4 is the
last thing holding the freeze. They are optional and absent from every example so that the
contract describes what the server sends today and the mocks stay true, which is the one
property that makes generated mocks worth having. `PairSummary` is a named schema rather
than an inline object because `pairsEvaluated` will be read by the list page and the
detail page alike, and an inline object is a second home waiting to happen.

**The alternative rejected:** holding the whole contract change until the computation
lands, so the fields and their values arrive together. It is the tidier sequence and it
was rejected because it puts a schema change on the critical path behind a fixture that
Al has to compute by hand, and it splits the frontend message in two. The cost of the
route taken is that the contract declares four fields nothing sends, which is why all four
say so in their own descriptions rather than in a changelog nobody reads.

## 8. What Claude refused, and it is two of the six amendments

The PRD is in `docs/context/`, which is RED and enforced in the deny list and in the hook.
The two amendments the methodology's own table asks for there, marking Q6 and Q7 answered
in section 11 and noting the candidate-set bound on FR-11, **were not made and were not
attempted.**

This is not the hook being awkward. The PRD is the file that says what FINISHED means,
section 9 holds the acceptance criteria the work is scored against, and Claude is one of
the two things it measures. A file that measures Claude and that Claude may edit is not a
measurement.

**The drafted text is handed to Al in section 9** so that the refusal costs him the paste
and not the writing. The methodology table's own row stays unticked until he applies it,
and `02-pair-selection.md` section 2 will still be naming a change that has not happened
until then. That visible gap is the correct state and should not be tidied away.

## 9. Text drafted for Al to apply to the PRD

Nothing below is applied. Both are Al's.

**Section 11, the open questions table.** Q6 and Q7 are answered. Suggested replacement
rows:

```
| Q6 | The selection criteria for the 50 asset demonstration set | Week 2 | **ANSWERED 5 Sep 2026**, `docs/methodology/02-pair-selection.md` section 5. Criteria C1 to C7, committed before the selection run, not justified backwards from the 64 assets stored on 26 August |
| Q7 | Are the absolute flag thresholds expressed in XLM or in USDC? | Week 2 | **ANSWERED 5 Sep 2026**, `docs/methodology/02-pair-selection.md` section 1. USDC, globally. The consequence this row demanded is stated there: Keel now assumes the USDC peg holds, and XLM/USDC being a monitored pair is what makes that assumption visible in the output rather than hidden. DEC-015 |
```

The paragraph under that table, beginning *"Q7 matters more than it looks"*, is worth
keeping as written. It states the consequence the answer had to carry, and the answer
carries it. A sentence could be appended: *"Answered in USDC on 5 September 2026; the
consequence is recorded in `docs/methodology/02-pair-selection.md` section 1."*

**FR-11.** Suggested replacement row:

```
| FR-11 | Compute depth for every quote pair that has any liquidity, and designate a primary pair | S | **BOUNDED** in v1 to a declared candidate quote set of two, USDC and native XLM, per `docs/methodology/02-pair-selection.md` section 2. Every member with any liquidity is computed and the primary pair is always USDC. The bound is a stated limitation of the version and not an unstated shortfall; the reason is the NFR-6 request ceiling, worked in that section |
```

Whether FR-11's Prio column or the requirement text is the right place for that is Al's
call; the wording above assumes a fourth column, matching the shape section 11 uses.

## 10. Rejected alternative

**Record the Q7 resolution as a patch, 1.0.9-draft, on the grounds that no number
changes.** It is true that no number changes: every figure in this repository is already
USDC-denominated, so nothing recomputes.

Rejected for two reasons. `09-flags-and-bands.md` section 6 named settling Q7 as the
condition for **version 1.1** specifically, and a document that sets its own condition and
then meets it without moving to the version it named has made its own version history
unreadable. And the change is not only a unit: `02-pair-selection.md` section 2 adds a
band rule that spans pairs, which changes what a band MEANS even before it changes any
band, and a definition change is a minor bump by the rule in `types.go`.

The per-asset quote alternative was rejected by Al inside the methodology, and the
argument is in `02-pair-selection.md` section 1 rather than here. Repeating it in this
record would create the second home this record's own section 1 declines to create.

## 11. Amendment history

This record is append-only from its first commit. An amendment adds a row here and text
below the section it concerns, and no earlier sentence is edited or deleted.

| Date | Amendment |
| --- | --- |
| 5 September 2026 | Record created. Al's decision in section 1, taken in `02-pair-selection.md` section 1 and marked DECIDED there. Section 4 opens the code-constant divergence, which is handed to Al. Section 8 records two amendments refused as RED, with the text drafted in section 9 |
| 11 September 2026 | Section 12 added. Two consequences of section 1 reached the code and the contract six days after the decision, and both had been reporting the decision as still missing in the meantime |

## 12. Two consequences applied on 11 September 2026, and the six day lag is the finding

**Nothing in section 1 changed.** This section records where its consequences landed, and
it is written because the lag is the part worth keeping rather than the change.

**What was found.** Two places in the codebase were still describing the primary-pair
question as open, in prose, after this record had closed it:

1. `internal/api/assetid.go` refused to resolve an asset with more than one pair when
   `quote` was omitted, and its own comment gave the reason: "decision D-1, and
   `docs/methodology/02-pair-selection.md` is still a worksheet whose own checklist says
   no decisions are recorded in it yet". That sentence was true when written. Section 2 of
   that document now reads "The primary pair is USDC, always", so the refusal outlived its
   reason and went on reporting a made decision as missing.
2. `internal/api/api.go` withheld the two threshold unit keys, `manipulationCheapUnit` and
   `thinDepth5PctUnit`, from `GET /v1/methodology`, because with a per-pair quote there was
   no single unit to name. Q7 closing gave it one.

**What was applied.**

| Where | Change |
| --- | --- |
| `internal/domain/types.go` | `GlobalQuote()` added, returning the (code, issuer) identity. A function rather than a var, and in `domain` rather than in `api`, for the reasons in its own header |
| `internal/api/assetid.go` | an omitted `quote` resolves to the `GlobalQuote()` pair, matched with `Asset.Equal` so another issuer's USDC does not qualify |
| `internal/api/api.go` | both unit keys served, as the (code, issuer) identity and never the bare ticker |
| `docs/api/keel-openapi.yaml` | 1.5.0 to **1.5.1**. The `quote` parameter's description corrected, and the methodology example's two `XLM` units corrected. A patch, because only a description and an example moved; the thresholds map is open ended by design so added keys are not a schema change. DEC-003's freeze was checked first and the contract is NOT FROZEN |
| `docs/api/mocks/` | regenerated, `make api-mocks-check` passes |

**THE CONTRACT WAS WRONG IN A DIFFERENT WAY AND THAT IS WORTH RECORDING SEPARATELY.** Its
`quote` parameter had always described the primary pair as "the pair with the largest
combined depth at 10 percent". No such rule was ever adopted anywhere in
`docs/methodology/`, and it is a bad rule on its own terms: under it an asset's primary
pair, and therefore its headline band, could change because depth moved rather than
because risk did. So the implementation was not behind the contract here; the contract was
carrying a rule the methodology never held. It was corrected rather than implemented.

**Two assertions were INVERTED rather than deleted**, in `internal/api/api_test.go`:
`TestOmittedQuoteWithSeveralPairsSaysSoRatherThanChoosing` expected a 400 and now expects
the USDC pair, and the methodology test asserted the two keys were ABSENT and now pins
their value. Each keeps the old assertion quoted in a comment above the new one, because a
test of a deliberate gap that is quietly deleted takes the reason for the gap with it. Two
tests were added: an asset with several pairs and no USDC pair among them still refuses to
choose, which is the one case the ambiguity error still describes, and a USDC from a
different issuer does not match the primary.

**What this section does NOT do.** `ManipulationRatioLowPct` remains `1.0` in
`internal/domain/types.go` and in the contract's example, even though DEC-017 sets it to
`0.1`. That is DEC-017's own step 5 and its section 5 puts two of Al's steps before it:
the methodology text, and one hand computed verdict. Changing the constant alone would
have the API publish `0.1` while `09-flags-and-bands.md` section 6 still publishes `1.0`,
which is worse than the present state where both are the superseded value and agree.
