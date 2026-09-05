# Could Keel have warned about the Blend incident of February 2026?

**Status: DRAFT, and two of its sections are empty on purpose.** Sections 5 and 6
are the ones that answer the title, and they are filled from a measurement that was
still running when this structure was written. Nothing is written into them in
advance. See section 10 for what is outstanding and who owns it.

**Version:** draft, 5 September 2026
**Methodology version:** `1.0.8-draft`, the version the engine stamps on every
result quoted here. The methodology documents are at `1.1.0-draft` and describe a
multi-pair rule the engine does not implement yet; DEC-014 and DEC-015 are where
that gap is recorded. Nothing in this report depends on it: USTRY is measured
against USDC and USDC is the unit the thresholds are in.
**Asset:** `USTRY`, issuer `GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC`,
against `USDC`, issuer `GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN`.
The identity is fixed by `docs/decisions/DEC-001-ustry-identity.md` and an asset is
the pair (code, issuer), never the ticker.

---

## 1. What this report claims, and what it does not

**It claims** that the state of the USTRY/USDC order book in February 2026 can be
rebuilt from public Stellar data, that Keel's published methodology applied to that
state produces a risk band on each day of the month, and that the resulting series
either does or does not cross into `CRITICAL` before the exploit date. Which of
those two it is, is section 6.

**It does not claim** that anybody would have acted on the warning, that Keel
existed at the time, or that the thresholds it uses are calibrated. They are chosen,
and `docs/methodology/11-limitations.md` says so in those words.

**It especially does not claim to be free of hindsight.** Section 7 is about that
and it is not a formality: this report knows the date of the attack, and a backtest
that knows the outcome can find a signal in almost anything. What makes the claim
checkable rather than rhetorical is that every threshold used here was written down
before the series was computed, and section 9 says exactly how a reader can confirm
that from the repository's own history.

**If the series shows no clear signal before the exploit date, that is the finding
and it is reported as one.** The PRD says so first, at section 10: "If the backtest
does not show a clear signal, that is not a project failure but a finding that has
to be reported honestly. Reporting it as it is does far more for long term
credibility than tuning thresholds until the result looks good."

## 2. The incident, from the chain

| | |
|---|---|
| Date | 22 February 2026 |
| Ledger the manipulation executed in | **61340263**, closed 2026-02-22T00:10:21Z |
| What happened | the USTRY price was pushed up roughly 100 times through a thinly traded feed and the position was then used as collateral to borrow about $61 million in XLM |
| Where that is stated | `docs/context/Keel_PRD.md` section 1. The date correction from May to February is `docs/decisions/DEC-001-ustry-identity.md` section 1 |

The order book immediately before that trade, at the end of ledger **61340262**, is
in `testdata/fixtures/ustry_pre_exploit.md`. Every figure in it was computed by hand
in a spreadsheet before any implementation existed, and that file is in a directory
the engine's authors cannot write to. The two levels were the whole book:

```
Asks: [ { price_r: {266843207, 2500000}, amount: 1.2185312 } ]   price 106.7372828
Bids: [ { price_r: {1057, 1000},         amount: 0.0001000 } ]   price   1.0570000
Pools: []
```

**One ask and one bid, and 105 dollars of nothing between them.**

## 3. What Keel says about that state

Applying the methodology to the book above, at ledger 61340262:

| Quantity | Value | Where the rule is written |
|---|---|---|
| reference price `P0` | 53.8971414 | `03-reference-price.md` |
| `spreadPct` | 196.0777141 per cent | `03-reference-price.md` |
| depth at ±2, ±5, ±10 per cent, both sides | **0** on all six | `04-depth.md` |
| manipulation cost to move the price 50 per cent | **0**, and the target is **reachable** | `05-manipulation-cost.md` |
| `maxReachablePrice` | 106.7372828 | `05-manipulation-cost.md` section 5 |
| cost to reach it | **0** | |
| ratio of that price to the real price of 1.057 | **100.98** | |
| band | **CRITICAL** | `09-flags-and-bands.md` |
| flags | `ZERO_DEPTH_2PCT`, `THIN_DEPTH_5PCT`, `SPREAD_EXTREME`, `MANIPULATION_CHEAP` | `09-flags-and-bands.md` |

**The line that matters most is the cost of zero.** An attacker does not pay for the
trade that moves the price. They pay for the third-party liquidity they have to
consume on the way to it, and on this book there was none to consume: no ask was
cheaper than the target, so nothing had to be bought, and the one ask sitting at
106.74 was enough to make the target reachable. `05-manipulation-cost.md` section 1
is the definition and this report does not restate it in its own words.

`bandConfidence` is **partial**, not full. Six flags need supply data, trade history
or trustline distribution that a book snapshot cannot carry, and they are reported as
`unevaluated` rather than as clear. A metric that could not be assessed is never
counted as a passing metric; `09-flags-and-bands.md` section 2 is the rule.

## 4. Where the historical data comes from, and why it is not a measurement

Horizon serves no order book at a past ledger. It serves every operation and the
result of every operation for ever, and a book is what those operations left behind.
Keel rebuilds the book by replaying `manage_sell_offer` and `manage_buy_offer`
operations up to a target ledger and applying the trades that consumed them.

**Every figure in section 5 therefore carries `dataSource: offers-implied` and is a
reconstruction rather than a reading.** It is a stronger source than the trade stream
would be, because an offer proves liquidity that was *posted* while a trade proves
only liquidity that was *consumed*, but it is not the same thing as a snapshot and
this report does not present it as one.

**The method was checked against the hand-computed fixture before it was trusted.**
On 5 September 2026 the book at control ledger 61340262 was rebuilt this way and the
methodology run over it, and every quantity in section 3 came back identical to the
figures worked by hand. The reading is
`docs/evidences/2026-09-05-control-ledger-validation.md`, and the artefact and its
provenance sidecar are beside it.

**Three limits of the method, each counted on every run rather than assumed away:**

1. An offer whose owner never traded and is not resting today is not discovered.
2. An account walk that fails or hits its page cap loses that account's offers.
3. No AMM pool is reconstructed at all, so every figure here is order book only.

The first two make the rebuilt book **thinner** than the market was, which overstates
risk rather than understating it. That is the conservative direction and it is
principle P-2 in the PRD. The third is a genuine gap and section 8 carries it.

## 5. The book, day by day, through February 2026

> **EMPTY UNTIL THE SERIES LANDS.** This section is a table of one row per day, from
> 1 to 28 February, each row carrying the ledger sampled, the size posted on each
> side, the spread, the depth ladder, the manipulation cost at each rung, the flags
> and the band. It is generated by the command in section 9 and its raw form is the
> CSV named there. Nothing is written here by hand.
>
> The sample rule, fixed in code before the run: **the first trade at or after each
> UTC midnight**, and each row reports the instant actually sampled and how far it
> fell from midnight. First trade rather than nearest to midnight, so a row labelled
> with a day never describes the state the market was in the evening before.
>
> Two extra rows sit outside the daily grid and are marked as such: control ledger
> 61340262 and the incident ledger 61340263.

## 6. When the unsafe threshold was crossed

> **EMPTY UNTIL SECTION 5 IS FILLED, AND ITS CONCLUSION IS NOT CLAUDE'S TO WRITE.**
> The zone map gives the structure and the tables to Claude and every claim about
> what a number MEANS to Al.
>
> What goes here is one date, or the honest statement that there is not one:
>
> - the first day on which the band reached `CRITICAL`, and how many days before
>   22 February that is;
> - the first day each individual flag fired, because a band is a summary and the
>   flags are what a reader can check;
> - whether the crossing was a step or a drift, since a book that was already
>   dangerous on 1 February is a different finding from one that deteriorated.
>
> **If the band was already `CRITICAL` on the first day of the month, the honest
> headline is not "Keel would have warned eight days early". It is that this asset
> was never safe, and that a metric which is critical for the whole month tells a
> reader less than a metric that changes.** Which of those it is, the series decides.

## 7. Hindsight bias, named

This report knows the date of the attack. Three specific ways that could corrupt it,
and what is done about each.

**Choosing the asset.** USTRY was chosen because it was attacked. A method that
finds danger only in assets already known to have been attacked has demonstrated
nothing. What limits the damage here is that the method is not tuned to this asset:
the same engine ran over 64 active Stellar assets on 26 August 2026 with zero
failures, recorded in `docs/evidences/2026-08-26-scan-64-assets-stored.md`, and the
thresholds are the same for all of them.

**Choosing the thresholds.** The thresholds in `09-flags-and-bands.md` are chosen
rather than calibrated and that file says so. **They were written before this series
was computed**, and section 9 explains how a reader can verify that from the git
history rather than taking it on trust. Had any of them moved after seeing the
result, PRD section 10 requires this report to say so. None has.

**Finding a signal in the trade stream.** This one is a live example rather than a
hypothetical, and it is why the analysis in section 5 uses the book and not the
trades. A reading of the same month's trade stream on 26 August 2026 found exactly
one pre-exploit "signal": a dust trade on 10 February that nobody would have noticed
at the time and that only looks meaningful because the date of the attack is already
known. That reading is `docs/evidences/2026-08-26-ustry-february-trades-implied.md`
section 4.

**What the trade stream could not see, and why that is the whole argument.** USTRY
traded 13,547 times in February at a spread of a fraction of a per cent around 1.057.
Every one of those trades was small and every one stayed inside a price range where
liquidity existed. Nothing in what *traded* was unusual. What made USTRY dangerous
was what was *posted*: a single ask a hundred times above the bid with nothing in
between. A price feed sees the first. Keel is built to see the second.

## 8. Limitations

Each of these is in `docs/methodology/11-limitations.md` or in a decision record,
and is repeated here because a reader of the report should not have to go and find
them.

1. **No AMM pool is reconstructed at any historical ledger.** Section 5 is order book
   only. USTRY had a pool holding honest reserves at 1.0555 for twelve days spanning
   the attack, and it prevented nothing, which is limitation 1 of the methodology.
   Its absence from these figures does not change that conclusion and does bound
   what they measure.
2. **Resting liquidity is not executable liquidity.** An offer can be withdrawn
   instantly. Every depth figure describes what was posted at one instant.
3. **Path payments through intermediate assets are not counted**, so true effective
   liquidity may exceed what is reported.
4. **Centralised exchange liquidity is invisible.**
5. **Thresholds are chosen, not calibrated.**
6. **Order ownership cannot be known ahead of time**, so manipulation cost is always
   an upper bound on what an attacker actually pays.
7. **The collateral parameters in force at the time are not fully recoverable.** The
   Blend `c_factor` for USTRY in February 2026 could not be read from public
   unauthenticated sources; four routes were tried and each is recorded in
   `docs/evidences/2026-08-31-ustry-reserve-config-history.md` section 5. What is
   established is the sign and not the figure: it was above zero, because the
   incident transaction borrowed against a USTRY position.
8. **The reconstruction is a lower bound on the book.** Section 4 says why, and every
   row of section 5 carries the diagnostics that let a reader see how much of the
   book a given day's walk actually reached.

## 9. How to reproduce every number in this report

Nothing here requires a BigQuery account, an API key, or any registration. Public
Horizon and this repository are enough, which is NFR-10.

**The book at the control ledger, and the fixture it is checked against:**

```bash
go run ./cmd/keel bookseries \
  -pairs scripts/record-pairs.example.json \
  -also-ledger 61340262,61340263 \
  -trades-from-ledger 61300000 -since-ledger 61300000 -lookahead 5000 \
  -csv /tmp/control.csv
```

Compare against `testdata/fixtures/ustry_pre_exploit.md`. The reading of that
comparison is `docs/evidences/2026-09-05-control-ledger-validation.md`.

**The February series in section 5:**

```bash
go run ./cmd/keel bookseries \
  -pairs scripts/record-pairs.example.json \
  -from-trades docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-trades-2026-02-01_2026-03-01.csv \
  -also-ledger 61340262,61340263 \
  -trades-from-ledger 60987032 -since-ledger 60987032 -lookahead 5000 \
  -max-pages-per-account 60 \
  -csv <the CSV named in section 5>
```

It writes a provenance sidecar beside the CSV in the shape
`docs/decisions/DEC-010-backtest-refuses-window.md` requires. Read
`walks_truncated` and `walks_failed` in it before reading any row as a market that
emptied.

**The trade stream for the same month**, which is what section 7 contrasts against:
`docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-trades-2026-02-01_2026-03-01.csv`,
13,547 rows, every field as Horizon sent it.

**That the thresholds predate the series.** Every threshold is a constant in
`internal/domain.DefaultParams` and a row in `09-flags-and-bands.md`. Both are under
version control, so:

```bash
git log --follow -p docs/methodology/09-flags-and-bands.md | grep -n 'Absolute\|Pct'
git log -1 --format=%cI -- docs/methodology/09-flags-and-bands.md
git log -1 --format=%cI -- docs/report/blend-february-2026.md
```

The threshold values are older than this report. A reader who finds otherwise has
found a defect and should say so.

## 10. What is outstanding on this draft

| Item | Owner | Why it is not done |
|---|---|---|
| Section 5, the day-by-day table | Claude | the series was still running when this structure was written |
| Section 6, the conclusion | **Al** | the zone map gives every claim about what a number MEANS to Al. Claude fills the table under it |
| The headline sentence of section 1 | **Al** | same reason |
| Whether an AMM reserve series can be added | **Al**, then Claude | pool reserves at a past ledger are not reconstructed today, and whether that gap is closed or stated is a decision |
| Publication | **Al** | D3's fifth criterion is "The backtest report published openly" |

## 11. Version history

| Date | Change |
|---|---|
| 5 September 2026 | Structure drafted. Sections 2, 3, 4, 7, 8 and 9 written from evidence already in the repository. Sections 5 and 6 deliberately empty |
