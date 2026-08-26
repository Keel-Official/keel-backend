# The recordings that go into git

Sixty raw Horizon readings, one per pair, all at ledger **64129592**, taken on
**26 August 2026** with `keel record` schema 2.

```bash
go run ./cmd/keel record -pairs configs/demonstration-set.json -out recordings/samples -once
```

`migrations/0001_core.sql` promises exactly this: the raw stream stays out of git,
and sixty recordings go in as evidence. `docs/methodology/10-validation.md` section
3 names this directory as where the Layer 3 evidence lives.

## What one file holds

Schema 2 and nothing else: the order book response and the liquidity pool response
for one pair, each with the exact URL requested, the HTTP status, the body verbatim
as a string, and that body's sha256. It parses nothing and converts nothing. A
non-2xx response and an empty pool list are both recorded and kept, because a
recording that makes a judgement can be wrong in a way a recording of bytes cannot.

That property is the reason schema 2 replaced schema 1. Schema 1 also stored parsed
conclusions, and the parsed half is the half that had to be revised when an order
book bid amount turned out to be denominated in the quote asset rather than the
base.

## What this is evidence OF, and what it is not

It is the **live half** of Layer 3. The comparison Layer 3 defines is a live Horizon
reading of a ledger against a reconstruction of that same ledger, and only the live
half has to be taken while the ledger is current. That half is here.

**The historical half does not exist yet and the results table in
`10-validation.md` section 3 is still empty.** It needs Hubble, which DEC-002
defers, which is handoff item B-8 and is an open decision. Nothing here should be
read as a completed cross-validation.

Four of the sixty straddled a ledger boundary, meaning the two requests in a tick
landed either side of a ledger close. Each file records `ledger_before` and
`ledger_after` so the reader can see which, rather than the recorder silently
picking one.

## Why one ledger and sixty pairs, rather than eight pairs over many hours

Layer 3 asks for at least fifty PAIRS reported. Sixty pairs at one ledger satisfies
that shape directly, and it covers the whole liquidity range in
`configs/demonstration-set.json`, from XLM down to assets whose only venue is a
pool.

The other shape exists too and it is not in git. `.github/workflows/record.yml`
records the eight pairs of `configs/recorder-pairs.json` every hour onto the orphan
`recordings` branch, which shares no history with `main`. That archive is the time
series: the same pairs at many ledgers. Two weeks of it is thousands of files,
which is the reason it is not here.

So: this directory is broad and shallow, the orphan branch is narrow and deep, and
Layer 3 can be run against either once the historical half exists.
