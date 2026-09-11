# Tugas B: the February evidence

**You are holding one of two work tracks.** The other one is `tugas-a.md`. They were
split so that two people can work at the same time without ever editing the same file
and without waiting for each other. If you read only this file you will still know
exactly what to do.

**Both files are scored against the same source:** the Statement of Work, the document
the client is paying against. Every work item below quotes the SOW sentence it serves,
so you can always answer "why am I doing this".

---

## 1. Sixty seconds on what Keel is

An oracle answers **"what is the price"**. Keel answers **"what volume can that price
actually support"**.

In February 2026 an attacker pushed the price of a Stellar asset called USTRY up 100
times using a market so thin that moving it cost almost nothing, borrowed 61 million
dollars of XLM against the inflated collateral, and the ecosystem lost 10 million.
The oracle was not wrong about the price. Nobody had a number for **how much real
liquidity stood behind that price**. Keel produces that number.

**Your track is the part that proves the number would have helped.** The SOW's central
promise is a backtest: rebuild that market day by day from public Stellar data, apply
Keel's published methodology, and say whether it would have called the asset unsafe
before the exploit. That report is the single most reviewable artefact the client gets,
and right now the two sections that answer its own title are empty.

**Keel never signs or submits a transaction.** There is no signing code anywhere in the
repository and there never will be. It only reads.

---

## 2. Which half is yours

The two tracks are split by **what the work is for**, and that happens to also split
cleanly by file, which is what keeps you out of each other's way.

| | Tugas A, the other file | **Tugas B, yours** |
|---|---|---|
| One sentence | Make Keel something a stranger can open and use in five minutes | Make Keel's claim about February 2026 true and checkable |
| The question it answers | "Can I use this today?" | "Was this ever right?" |
| Its shape | a running service | a document with evidence under it |

**Yours in one line:** the reconstruction of February 2026 stops producing books that
could not have existed, those results get stored so the API can serve a past ledger, the
report's empty sections get filled with what the data actually says, and the client is
told about a date correction nobody has told them about yet.

Your estimated load is **about 36 hours**, which is roughly the 8 days remaining at the
SOW's own pace of 4.6 hours per builder per day. Track A was re-costed to about 49 on
10 September 2026 and names its own split point, so if days run short, that side sheds
work before this one does.

---

## 3. Getting running, ten minutes

You need Docker and Go 1.23. Nothing else.

```bash
git clone https://github.com/Keel-Official/keel-backend.git
cd keel-backend

make up                 # Postgres in Docker, on localhost:5433 (not 5432, see below)
make migrate            # apply the schema. This is the ONLY way migrations are applied
make ci                 # vet, architecture tests, 339 tests with -race, linter. Must be green
make conformance        # the engine against the hand computed golden fixture
```

Then run the two commands your track lives in. Both read only from public Horizon and
neither needs a database:

```bash
# rebuild the order book at ONE past ledger and print it
make replay PAIRS=scripts/record-pairs.example.json LEDGER=61340262

# rebuild it once per day across a month, into a CSV
make bookseries PAIRS=scripts/record-pairs.example.json \
  CSV=/tmp/try.csv FROM_TRADES=docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-trades-2026-02-01_2026-03-01.csv
```

**Be warned before you run `bookseries`:** the last full run took **1 hour 44 minutes**
and 1,880 Horizon requests. Public Horizon allows about 3,600 per hour per IP. Do not
start a second one in parallel, and read section 6 below before running one at all.

**The one trap that has already cost this project a day.** Postgres is published on host
port **5433**, not 5432. If you already have a Postgres on your machine it owns 5432, and
a DSN pointing there will reach the wrong server and fail with `role "keel" does not
exist` rather than with a connection error.

Useful reading, in this order: `README.md`, then `CLAUDE.md` (the working rules), then
`docs/methodology/10-validation.md`, then `docs/report/blend-february-2026.md` which is
the thing you are finishing.

---

## 4. Rules you cannot break

These are not style preferences. Each one exists because breaking it already caused a
real failure here.

1. **Never use `float64` for money.** Every monetary value uses
   `github.com/shopspring/decimal`. An architecture test fails the build otherwise.
2. **Every output carries `LedgerSeq` and `MethodologyVersion`.** A number without the
   ledger it came from and the method that produced it is not a result, it is a rumour.
3. **Sort map keys before iterating.** Two runs of the same input must produce
   byte-identical output.
4. **Read prices from the `price_r` field**, the `n/d` fraction, never from the `price`
   string. The string is rounded.
5. **Never edit the two locked directories under `testdata/`.** They hold numbers computed
   BY HAND before the code existed, and they are the only reason to believe the engine is
   checked against something independent of itself. **If the code and those numbers
   disagree, the disagreement IS the finding: report it, correct the code, and never touch
   the numbers.** Reading them is not only allowed, it is the job. Both are RED zones: only
   Al writes there.
6. **`docs/methodology/` is Al's for definitions.** You may restructure, cross-reference
   and check it. You may not invent a definition. A change there bumps the methodology
   version across the whole set, which is DEC-014's one-version rule.
7. **`docs/report/` gives you the structure, the tables and the reproduction steps. Every
   claim about what a number MEANS is Al's.** This is the rule that most shapes your work,
   so it is spelled out again in item B3.
8. **Decision records are amended, never reversed by deletion.** A reversal is recorded as
   a reversal. You draft and amend; you do not create or reverse a decision.
9. **English everywhere**, and Conventional Commits: `fix(scope): imperative lowercase`,
   72 characters max on the first line, body explaining the WHY.

---

## 5. What is already done, so you do not redo it

| Already working | Where |
|---|---|
| Rebuilding a past order book from the operations that posted it | `keel replay`, `internal/horizon/replay.go` |
| Rebuilding it once a day across a month | `keel bookseries`, `internal/horizon/series.go` |
| Layer 3 cross-validation, Horizon against rebuilt history | `keel crosscheck`. 79 ledgers, 360 comparisons, **0 mismatches** |
| The hand computed golden fixture for the pre-exploit book | the fixture file named in `internal/conformance/fixture.go` |
| A month of rebuilt book state, twice, the second run much better | `docs/evidences/*bookseries*cap400.csv` |
| The report's structure, ten sections, definitions quoted rather than restated | `docs/report/blend-february-2026.md` |
| Seventeen decision records | `docs/decisions/` |

**The single most important thing already proven, because it is what makes your work
trustworthy at all:** the reconstruction reproduces the hand computed golden fixture
exactly at the control ledger. Ask amount 1.2185315 against the fixture's 1.2185312, and
1.1684312 against 1.1684309, both differing by the same 0.0000003 of dust, and the change
across the manipulation trade is 0.0501003 in both, which is the exploit's USTRY volume to
the last decimal. A number computed by hand before the code existed, met from the other
direction by a fold over 800,000 operations. That is your foundation.

Current score against the SOW: Deliverable 1 about 89%, Deliverable 2 about 59%,
Deliverable 3 about 38%. Weeks 1 and 2 are complete. Your track is most of what is left of
Deliverable 2.

---

## 6. Your work items

Items marked **AL ONLY** need a judgement the client is paying a person to make, or a
directory only Al may write in. Do not attempt those; prepare everything around them and
hand them over.

Do B1 first. Everything else in this track reads better once the data underneath it is
sound.

---

### B1. Stop producing books that could not have existed

**SOW, Week 3:** *"Run the Blend incident backtest on the May 2026 ledgers."* **SOW,
Deliverable 2:** *"An open, reproducible report will reconstruct USTRY liquidity metrics
... and identify when the unsafe threshold was crossed relative to the exploit."*

**Start by understanding what the tool does**, because the defect only makes sense once
you do. Horizon will serve you the order book **as it is right now**. It will not serve
you the order book as it was in February. So `bookseries` walks backwards through every
account's operation history, finds every offer that was created, modified or removed, and
folds them back to reconstruct the book at a past moment. It is a genuinely hard thing to
do and it mostly works.

**Where it does not.** Look at the current output:

```bash
python3 - <<'EOF'
import csv
r=list(csv.DictReader(open("docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-bookseries-2026-02-01_2026-03-01-cap400.csv")))
for x in r:
    print(x['day'] or x['target_ledger'], x['best_bid'][:9], x['best_ask'][:9],
          "CROSSED" if x['best_bid'] and x['best_ask'] and float(x['best_bid'])>float(x['best_ask']) else "",
          x['band'])
EOF
```

Seven of the thirty rows come back **CROSSED**: the best bid is higher than the best ask.
The Stellar matching engine makes that impossible, because such offers would have traded
against each other instantly. So those rows describe a market that never existed.

The cause is visible in the data itself and you do not have to guess at it. Every row
reports `fold_complete: false` and `missing_offer_ids: 3`. The sidecar file beside the CSV
reports `walks_truncated: 36` and `walks_failed: 5`. The fold is telling you that it never
resolved the operations that would have removed some offers. One dust ask of 0.0000001
USTRY sits unmoved at the same price from 9 February to 28 February while the bids climb
past it.

**Read this first, it is the whole history in two files:**
`docs/evidences/2026-09-08-february-book-series.md` section 6 prices the fix, and
`git show a9e28e3` is the previous attempt at it: one page cap served both the 213 accounts
that hold no offers and the 2 that hold the entire book, so it was set small for the many
and was then too small for the few that mattered. Splitting it into two caps took
`missing_offer_ids` from 203 down to 3 and removed several other artefacts. **Three
missing offers is not zero.**

**Done when:** either `fold_complete` reads true and no row is crossed, or the residual is
characterised precisely enough that a reader knows which rows to trust and why. **Both of
those are acceptable outcomes.** What is not acceptable is publishing a crossed row without
saying it is impossible.

A truncated or failed walk loses offers, and **a lost offer reads as a thinner book rather
than as an error.** That is the direction that misleads: it makes an asset look more
dangerous than it was. Keep that asymmetry in mind whenever you are tempted to accept a
partial result.

**Files:** `internal/horizon/series.go`, `rewind.go`, `offerxdr.go`, and
`cmd/keel/bookseries.go`.

**Estimate: 12 hours.**

---

### B2. Resolve the discontinuity of 22 February

**Why this is a separate item from B1**, even though it looks like the same bug: it may not
be a bug at all, and telling those two apart is the finding.

Two rows of the same run, produced by the same fold, ninety ledgers apart, which is about
seven and a half minutes:

| Ledger | When | Resting offers | Depth at 2% (buy side) | Band |
|---|---|---|---|---|
| 61340172 | 22 Feb, daily sample | 23 | **227,479 USDC** | LOW |
| 61340262 | 22 Feb, control ledger | 17 | **0.0000001 USDC** | CRITICAL |

Six orders of magnitude in seven minutes. There are exactly two explanations and they lead
to opposite conclusions:

1. **The market really did empty minutes before the exploit.** If so, this is the most
   valuable finding in the whole project, and it is bad news for the product as specified:
   a daily sampling cadence would never have warned anybody. That belongs in the report in
   plain words, not buried.
2. **The fold loses the offers that hold the daily rows up.** If so, every daily row is a
   lower bound and the report cannot cite any of them.

**Two things make explanation 1 credible and you must weigh them.** The control ledger's
figures match the hand computed golden fixture exactly, and that fixture was worked out by
hand before any of this code existed. And the pre-exploit book being nearly empty is what
the incident *is*.

**Done when:** one of the two explanations is supported by evidence, written up in a new
file under `docs/evidences/` naming the ledgers and the operations involved, and the other
is ruled out rather than left open. `keel replay` at individual ledgers between 61340172
and 61340262 is your instrument: it prints one book at one ledger and is much cheaper than
a month-long run.

**AL ONLY:** deciding what the resolved answer MEANS for the product's claim. Establishing
which of the two is true is engineering and is yours.

**Estimate: 6 hours.** Combined with B1 if the cause turns out to be shared.

---

### B3. Fill sections 5 and 6 of the report

**SOW, Deliverable 2:** *"An open, reproducible report will reconstruct USTRY liquidity
metrics across May 2026 and identify when the unsafe threshold was crossed relative to the
exploit."* **Evidence:** *"Live API URL, open backtest report, raw data, and calculation
code."*

Open `docs/report/blend-february-2026.md`. Ten sections. Sections 5 and 6 are marked empty
on purpose and **they are the two that answer the report's own title.** Section 5 is a table
of one row per day. Section 6 is the date the asset first crossed into CRITICAL, or the
honest statement that there is not one.

**This is where the ownership rule bites, so read it carefully.** The zone rules give you
the structure, the tables and the reproduction steps. **Every claim about what a number
MEANS is Al's.** In practice:

- You generate section 5's table from the CSV. That is yours entirely.
- You list, for section 6, the first day each flag fired and the first day the band reached
  CRITICAL. Those are facts read off a table. Yours.
- Whether the answer is "Keel would have warned N days early" or "this asset was never
  safe, and a metric that is critical all month tells a reader less than one that changes"
  is **Al's sentence to write**. The report already says so in its own placeholder, and it
  already names the trap: if the band was CRITICAL on 1 February, the honest headline is
  the second one, not the first.

**Do not make the numbers agree with a headline.** The report's section 7 is about
hindsight bias and it is not a formality: this analysis knows the date of the attack, and a
backtest that knows the outcome can find a signal in almost anything.

Also: **every number in the report must cite an evidence file that already exists**, and the
report **quotes** `docs/methodology/` rather than restating a definition in its own words. A
definition with two homes drifts, and this one would drift in front of the client.

**Blocked on:** B1 and B2. That is a dependency inside your own track, not across tracks,
so it does not stall anybody else.

**Estimate: 8 hours.**

---

### B4. Store the reconstruction so the API can serve a past ledger

**SOW, Deliverable 2:** *"...and historical metrics through the ledger query parameter."*

**Why.** This one is a satisfying find. The API's historical path is complete and correct:

```
internal/api/api.go:344
    m, err := s.cfg.Reader.MetricsAtLedger(ctx, pair.ID, uint32(ledger),
        domain.MethodologyVersion, domain.DataSourceOffersImplied)
```

It asks the database for a stored result at that ledger, rebuilt from offer operations. The
store has the function to write one, `store.SaveMetrics`. **And nothing has ever called it
with a reconstructed result.** Its only caller is the live scanner. `bookseries` writes a
CSV, `replay` prints JSON. So the endpoint has a reader and no writer, and a request for a
past ledger can only ever answer 404.

**Done when:** `keel replay` (or `bookseries`) can persist its computed result with
`dataSource: offers-implied`, and after running it, `GET /v1/asset/{id}/depth?ledger=61340262`
returns that result instead of a 404. Add a flag rather than always writing: a tool that
silently writes to a database is a tool people stop trusting.

**Rule 1 applies with force here:** the stored row carries its `LedgerSeq` and its
`MethodologyVersion`. Rows already stored under an older methodology version keep their
label and stay reproducible. Never relabel an old row.

**Files:** `cmd/keel/replay.go`, and possibly `cmd/keel/bookseries.go`. **Call
`store.SaveMetrics`, do not modify `internal/store/`**, which belongs to Track A. If you
find you genuinely need a change in there, write it down and hand it over.

**Estimate: 6 hours.**

---

### B5. Close the two methodology gaps

**SOW, Deliverable 1:** *"The methodology will be documented and reproducible."*
**Evidence:** *"Public repository link and methodology document."*

Twelve documents exist and eleven are complete. Two things are open:

1. `docs/methodology/06-oracle-resilience.md` still reads **"Status: partial"**, and the
   VWAP window length is an assumption rather than a documented choice.
2. The methodology documents are at version `1.1.0-draft` while the code stamps
   `1.0.8-draft`. That is deliberate and recorded in DEC-014 and DEC-015, and the README of
   the methodology explains it, but it is the kind of thing a reviewer notices and asks
   about. Either the gap closes or the explanation gets stated where a reviewer will meet it
   first.

**AL ONLY** for the definitions themselves. **Yours:** the cross-references, the check
that no document contradicts another, and drafting the amendment for Al to approve.

**DO NOT PERFORM THE VERSION BUMP, and this changed on 10 September 2026.** Track A's new
item A7 also changes this directory, in `09-flags-and-bands.md` sections 4 and 6, and
DEC-014's one-version rule moves all twelve headers on any single change. Two people
bumping the same set produces two different version numbers and a set that disagrees with
itself, which is the exact failure DEC-014 was written after. So the bump has one owner and
it is A7. You draft your text, and it rides A7's bump. Item 2 above, the gap between the
documents at `1.1.0-draft` and the code at `1.0.8-draft`, is closed by that same bump
rather than by you.

**Estimate: 4 hours**, down slightly because the bookkeeping moved to A7.

---

### B6. Five hand recomputations. **AL ONLY**

**SOW, Deliverable 1, and this one is subtler than it looks.**

There are five worksheets waiting in the locked Layer 1 directory, one per asset, each
containing the raw order book transcribed and **no computed values at all**. The job is to
work the methodology by hand, on paper or in a spreadsheet, and write down what the answer
should be.

**Why a human must do this, and why it cannot be automated.** Since one of the engine's core
files stopped being written by only one person, the numbers in the locked directories are the
only structural reason to believe the implementation is checked against figures derived
independently of it. A recomputation produced by the thing it is meant to test is not
evidence, it is a restatement. That is why the directory is RED and why nobody else may put
a number in it.

**Do not skip the ordering rule.** A figure there must exist **before** the code that
satisfies it. No mechanism can enforce that, because no permission system can tell whether a
number was computed before or after the code it happens to agree with. It is still the rule.

That directory's own `README.md` is the full procedure, including one trap worth knowing
before you start: the checker reads these files with `grep`, so an `.xlsx` or `.numbers` file
is a zip archive whose text is compressed and matches nothing. It will be reported as
identifying no asset and no ledger, which looks exactly like an empty file. **Export to CSV
or Markdown.**

Check where you stand at any time with:

```bash
make manual-check
```

**Estimate: 5 hours.**

---

### B7. Tell the client about the date. **AL ONLY**

**Why this is on a work list at all.** The SOW says the attack happened on **20 May 2026**
and asks for USTRY metrics "across May 2026". `DEC-001` section 8 establishes from the
primary source that it was **February 2026**, and every artefact in this repository is
February: the golden fixture, the book series, the report's own filename.

The repository is right and it is internally consistent. **The client has not been told.**
DEC-001 section 6 item 2 has had no record of being done since August. So a reviewer holding
the SOW will open `blend-february-2026.md` and read a different month than the one they were
promised, with no explanation attached.

There is a second item that travels with it. Week 1's verification of the SOW's own USDY
figures did not confirm them, it corrected them: supply about 467.5 million units against
"approximately $109.58 million", 2,714 authorized trustlines against 684 holders, and about
66 USDC of 24-hour volume against $191. That strengthens the SOW's argument rather than
weakening it. But `/assets?asset_code=USDY` returns **37 different issuers** and the SOW
names none of them, so which asset that sentence is about is a question only the client can
answer.

`docs/internal/ambassador-note-2026-09-07.md` is a draft of the message. It is local and not
in the repository.

**Estimate: 1 hour, and it is the highest ratio of value to effort on either list.**

---

## 7. The boundary with Track A

**Files Track A owns. Do not edit them.** If you need a change in one, write it down and
hand it over rather than reaching across.

```
cmd/keel/scan.go, serve.go       internal/api/         internal/store/
internal/domain/                 migrations/           configs/
Dockerfile, docker-compose*.yml, Caddyfile
.github/workflows/deploy.yml     scripts/deploy/       docs/api/    README.md
```

**Files you own. Track A will not edit them.**

```
internal/horizon/series.go, rewind.go, replay.go, offerxdr.go, trades.go
cmd/keel/bookseries.go, replay.go, crosscheck.go, divergence.go
docs/report/          docs/evidences/       docs/methodology/
docs/decisions/       testdata/
```

**Three places the two tracks touch, and how each is handled so neither blocks:**

1. **`store.SaveMetrics`.** You call it in B4. Track A owns it. Call it, do not change it.
2. **`internal/domain` compute functions.** You call them through the series and replay
   paths. Track A owns that package. Call, do not edit.
3. **The `-historical` flag.** Track A's, in the deploy configuration. It stays off until
   your B4 rows exist, and with it off the API returns a truthful `503
   HISTORICAL_UNAVAILABLE` rather than something wrong. So **you are not blocking their
   deployment and they are not blocking your work.** When B4 lands, tell them and they flip
   one line.
4. **`docs/methodology/`, and this one is a CARVE-OUT rather than a shared file.** Added
   10 September 2026. Track A gained an item, A7, that implements
   `MANIPULATION_RATIO_LOW` under DEC-017, and it edits `09-flags-and-bands.md`
   **sections 4 and 6 only**. Everything else in that directory is yours, including all of
   `06-oracle-resilience.md`.
   **AND THE VERSION BUMP IS THEIRS, NOT YOURS.** DEC-014's one-version rule means a
   change in any one file moves all twelve headers, so that operation must have exactly one
   owner or two people will do it twice and disagree about the number. B5 therefore drafts
   its text and hands it to A7's single bump. See B5.

Nothing else. No item in your list waits on any item in theirs.

---

## 8. Definition of done for Track B

```
[ ] B1  no crossed row survives, or every residual is characterised and labelled
[ ] B1  fold_complete true, or the exact reason it is not, per row
[ ] B2  the 22 February discontinuity has one supported explanation and one ruled out
[ ] B3  report section 5 filled from the CSV, one row per day
[ ] B3  report section 6 filled, and AL has written the sentence about what it means
[ ] B4  a reconstructed result can be stored, and ?ledger= returns it instead of a 404
[ ] B5  06-oracle-resilience.md no longer says partial, versions reconciled or explained
[ ] B6  AL: five hand recomputations, make manual-check counts 5 of 5
[ ] B7  AL: the Ambassador is told about February versus May, and asked which USDY issuer
[ ] make ci and make conformance green on every commit
```

When all of these are true, the SOW's Deliverable 2 evidence line, "open backtest report,
raw data, and calculation code", is satisfied, and the report answers the question in its own
title.

---

## 9. Glossary

| Term | What it means here |
|---|---|
| **SDEX** | Stellar Decentralized Exchange. The on-chain order book |
| **AMM** | Automated Market Maker. Liquidity pools, an alternative venue to the order book |
| **Depth at 2%** | how much you can trade before the price moves 2% away from the reference price |
| **Manipulation cost** | what an attacker would have to spend to move the price by a given amount |
| **Band** | the overall risk verdict for an asset: LOW, MEDIUM, HIGH, CRITICAL |
| **Flag** | a specific named condition, for example `MANIPULATION_CHEAP`. Each has a tier |
| **`bandConfidence`** | `full` or `partial`. `partial` means at least one important flag could not be evaluated |
| **CROSSED book** | best bid above best ask. Impossible on a real exchange, so a sign the reconstruction is wrong |
| **The fold** | walking an account's operation history backwards and replaying offer changes to rebuild a past book |
| **`offers-implied`** | a book reconstructed that way, rather than read live. It is a labelled data source, not a synonym for "real" |
| **Control ledger** | 61340262, the ledger immediately before the manipulation trade. The state the golden fixture describes |
| **Golden fixture** | the hand computed pre-exploit book, loaded by `internal/conformance/fixture.go`. Its numbers were computed before any code existed. The code is corrected to match it, never the reverse |
| **Layer 1 / 2 / 3** | the three validation layers in `docs/methodology/10-validation.md`. Layer 1 is hand recomputation, Layer 3 is Horizon against rebuilt history |
| **Horizon** | Stellar's public HTTP API. Keel's only data source |
| **Hubble** | Stellar's BigQuery dataset. Deliberately deferred, see DEC-002 |
| **Zone** | GREEN, YELLOW or RED. Who may write in a directory. The table is in `CLAUDE.md` |
| **DEC-nnn** | an architecture decision record in `docs/decisions/`. They are amended, never quietly reversed |
