#!/usr/bin/env python3
"""Measure one account's offer-ladder absence windows over a ledger range.

WHY THIS FILE EXISTS. `docs/evidences/2026-09-14-maker-withdrawal-cadence-february.md`
established that the USTRY/USDC market maker deleted and re-posted its whole ladder at
00:10 UTC on all 28 days of February 2026. That document measures ONE account, and the
sentence the backtest report wants to build on it -- that a once-a-day depth figure
inherits the blind spot -- is wider than one account can carry. This script is the same
measurement pointed at other makers, so the generalisation is measured rather than
assumed.

METHOD, and it is a transcription of that document's sections 1 and 8 rather than a new
one. One forward walk of the account's own operation stream. Offer operations are grouped
by (transaction, pair). A group is a DELETE-ALL when every amount is 0, which is what
deletes an offer on manage_buy_offer and manage_sell_offer, and a POST when no amount is
0. An absence window runs from a delete-all to the next post on the same pair, and its
duration is the difference of the two created_at values.

NOTHING HERE PASSES THROUGH internal/domain. Every figure is a count or a subtraction over
Horizon operation records, so no threshold this project chose can move any number this
script prints. It also does not use internal/horizon's fold: it reads the operation stream
directly, so a defect in the reconstruction cannot reach these numbers.

Usage:
    python3 cadence.py walk    <ACCOUNT> <LO_LEDGER> <HI_LEDGER> <PREFIX>
    python3 cadence.py windows <PREFIX> [PAIR]
"""

import collections
import csv
import datetime as dt
import json
import statistics
import sys
import time
import urllib.request

HORIZON = "https://horizon.stellar.org"

# create_passive_sell_offer is included because it is an offer operation and a maker may
# use it. It is listed rather than assumed absent: a maker that used it and was not read
# would read as a book that never emptied, which is the wrong direction to be wrong in.
OFFER_OPS = {"manage_buy_offer", "manage_sell_offer", "create_passive_sell_offer"}


def get(url, attempts=5):
    """One GET with backoff. Public Horizon serves deep operation pages slowly and
    occasionally refuses one; a walk that dies on a single refusal reports a short month
    as a quiet month, so the retry is part of the measurement rather than a convenience."""
    for attempt in range(attempts):
        try:
            with urllib.request.urlopen(url, timeout=60) as response:
                return json.load(response)
        except Exception:
            if attempt == attempts - 1:
                raise
            time.sleep(2 * (attempt + 1))


def asset_label(record, side):
    kind = record.get(f"{side}_asset_type")
    if kind == "native":
        return "XLM"
    code = record.get(f"{side}_asset_code")
    issuer = record.get(f"{side}_asset_issuer")
    return f"{code}:{issuer[:8]}" if code else "?"


def pair_of(record):
    """The pair as an unordered label. An offer names a selling and a buying asset, and
    the two sides of one book arrive with those two fields swapped, so sorting them is
    what makes a bid and an ask land on the same pair."""
    return "/".join(sorted([asset_label(record, "selling"), asset_label(record, "buying")]))


def walk(account, lo, hi, prefix):
    """Page the account's operations from ledger `lo` and stop past ledger `hi`.

    The ledger of an operation is read from its paging_token rather than from a separate
    request: a TOID is `ledger << 32 | txorder << 12 | opindex`, so the sequence is the
    top 32 bits and costs nothing."""
    url = f"{HORIZON}/accounts/{account}/operations?cursor={lo * 4294967296}&order=asc&limit=200"
    pages = records = 0
    ops = []
    first_at = last_at = None
    ledger = lo
    while url:
        page = get(url)
        page_records = page["_embedded"]["records"]
        if not page_records:
            break
        pages += 1
        past_end = False
        for record in page_records:
            ledger = int(record["paging_token"]) >> 32
            if ledger > hi:
                past_end = True
                break
            records += 1
            if first_at is None:
                first_at = record["created_at"]
            last_at = record["created_at"]
            if record["type"] in OFFER_OPS:
                ops.append({
                    "ledger": ledger,
                    "at": record["created_at"],
                    "tx": record["transaction_hash"],
                    "type": record["type"],
                    "amount": record.get("amount", "0"),
                    "offer_id": record.get("offer_id"),
                    "pair": pair_of(record),
                })
        if past_end:
            break
        url = page["_links"]["next"]["href"]
        if pages % 25 == 0:
            print(f"  ... {pages} pages, {records} records, ledger {ledger}", flush=True)

    meta = {
        "account": account, "ledger_lo": lo, "ledger_hi": hi,
        "pages": pages, "records": records, "offer_ops": len(ops),
        "first_record_at": first_at, "last_record_at": last_at,
        "by_pair": dict(collections.Counter(o["pair"] for o in ops)),
    }
    with open(f"{prefix}-ops.json", "w") as handle:
        json.dump({"meta": meta, "ops": ops}, handle)
    print(json.dumps(meta, indent=1))
    return meta, ops


def groups_and_windows(ops, pair):
    """Classify each (transaction, pair) group, then pair each delete-all with the next
    post.

    TWO DESIGN DECISIONS ARE VISIBLE HERE AND BOTH MAKE THE ABSENCE SHORTER RATHER THAN
    LONGER, which is the safe direction for a document arguing that books empty.

    1. A second delete-all arriving while a window is already open is ignored rather than
       opening a second window. The maker is already absent; counting it twice would
       double-count the same absence.
    2. A group that mixes zero and non-zero amounts is classified `mixed` and opens
       nothing. The USTRY maker produced none of these; a maker that produces many is
       editing its ladder rather than replacing it, and that is reported rather than
       forced into the delete/post shape."""
    by_transaction = collections.OrderedDict()
    for op in ops:
        if op["pair"] != pair:
            continue
        by_transaction.setdefault(op["tx"], []).append(op)

    groups = []
    for transaction, members in by_transaction.items():
        amounts = [float(m["amount"]) for m in members]
        if all(a == 0 for a in amounts):
            kind = "delete"
        elif all(a != 0 for a in amounts):
            kind = "post"
        else:
            kind = "mixed"
        groups.append({"tx": transaction, "kind": kind, "n": len(members),
                       "at": members[0]["at"], "ledger": members[0]["ledger"]})
    groups.sort(key=lambda g: (g["ledger"], g["at"]))

    windows = []
    opened = None
    for group in groups:
        if group["kind"] == "delete" and opened is None:
            opened = group
        elif group["kind"] == "post" and opened is not None:
            start = dt.datetime.fromisoformat(opened["at"].replace("Z", "+00:00"))
            end = dt.datetime.fromisoformat(group["at"].replace("Z", "+00:00"))
            windows.append({
                "open_at": opened["at"], "close_at": group["at"],
                "open_ledger": opened["ledger"], "close_ledger": group["ledger"],
                "seconds": (end - start).total_seconds(),
            })
            opened = None
    return groups, windows, opened


def cadence(windows):
    """The modal minute of day at which a window opens, and how many distinct days carry
    one within a minute of it.

    THE BAND IS CENTRED ON THE MODAL MINUTE and then widened by one minute each way.
    Scanning instead for the widest three-minute sum puts the centre off the mode when
    every opening sits inside one minute, which is what the first run of this script
    printed before it was corrected."""
    if not windows:
        return None
    minutes = collections.Counter()
    for window in windows:
        opened = dt.datetime.fromisoformat(window["open_at"].replace("Z", "+00:00"))
        minutes[opened.hour * 60 + opened.minute] += 1
    centre = max(minutes, key=lambda m: (minutes[m], -m))
    in_band, days = [], set()
    for window in windows:
        opened = dt.datetime.fromisoformat(window["open_at"].replace("Z", "+00:00"))
        if abs((opened.hour * 60 + opened.minute) - centre) <= 1:
            in_band.append(window)
            days.add(opened.date())
    return {"band_centre": f"{centre // 60:02d}:{centre % 60:02d}",
            "windows_in_band": len(in_band), "distinct_days": len(days), "in_band": in_band}


def report(prefix, pair=None, month_seconds=28 * 24 * 3600):
    with open(f"{prefix}-ops.json") as handle:
        loaded = json.load(handle)
    meta, ops = loaded["meta"], loaded["ops"]
    by_pair = collections.Counter(o["pair"] for o in ops)
    if not by_pair:
        print(f"{prefix}: no offer operations in the window")
        return None
    if pair is None:
        pair, _ = by_pair.most_common(1)[0]

    groups, windows, still_open = groups_and_windows(ops, pair)
    kinds = collections.Counter(g["kind"] for g in groups)
    seconds = [w["seconds"] for w in windows]
    band = cadence(windows)

    print(f"\n=== {prefix}: {pair} ===")
    print(f"account   {meta['account']}")
    print(f"walk      {meta['pages']} pages, {meta['records']} records, "
          f"ledgers {meta['ledger_lo']}..{meta['ledger_hi']}")
    print(f"first/last {meta['first_record_at']} .. {meta['last_record_at']}")
    print(f"offer ops {by_pair[pair]} on this pair of {meta['offer_ops']} on the account")
    print(f"groups    {kinds.get('delete', 0)} delete-all, {kinds.get('post', 0)} post, "
          f"{kinds.get('mixed', 0)} mixed")
    if seconds:
        print(f"windows   {len(windows)}  shortest {min(seconds):.0f}s  "
              f"median {statistics.median(seconds):.0f}s  longest {max(seconds):.0f}s")
        print(f"absent    {sum(seconds):.0f}s = {sum(seconds) / month_seconds * 100:.4f}% "
              f"of the walked span")
    else:
        print("windows   0")
    if still_open is not None:
        print(f"UNCLOSED  a delete-all at {still_open['at']} (ledger {still_open['ledger']}) "
              f"has no post before the end of the walk; it is NOT counted above")
    if band:
        print(f"cadence   modal band {band['band_centre']} UTC +/-1min: "
              f"{band['windows_in_band']} windows on {band['distinct_days']} distinct days")

    with open(f"{prefix}-windows.csv", "w", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=[
            "open_at", "close_at", "open_ledger", "close_ledger", "seconds"])
        writer.writeheader()
        writer.writerows(windows)
    return {
        "prefix": prefix, "account": meta["account"], "pair": pair,
        "pages": meta["pages"], "records": meta["records"],
        "offer_ops_on_pair": by_pair[pair], "offer_ops_total": meta["offer_ops"],
        "first_record_at": meta["first_record_at"], "last_record_at": meta["last_record_at"],
        "deletes": kinds.get("delete", 0), "posts": kinds.get("post", 0),
        "mixed": kinds.get("mixed", 0), "windows": len(windows),
        "shortest": min(seconds) if seconds else None,
        "median": statistics.median(seconds) if seconds else None,
        "longest": max(seconds) if seconds else None,
        "total_absent": sum(seconds) if seconds else 0,
        "pct_of_span": (sum(seconds) / month_seconds * 100) if seconds else 0.0,
        "unclosed_delete_at": still_open["at"] if still_open else None,
        "cadence": {k: v for k, v in band.items() if k != "in_band"} if band else None,
    }


if __name__ == "__main__":
    if len(sys.argv) < 2:
        print(__doc__)
        sys.exit(2)
    command = sys.argv[1]
    if command == "walk":
        walk(sys.argv[2], int(sys.argv[3]), int(sys.argv[4]), sys.argv[5])
    elif command == "windows":
        report(sys.argv[2], sys.argv[3] if len(sys.argv) > 3 else None)
    else:
        print(__doc__)
        sys.exit(2)
