"""Independent rational arithmetic for review, not the human Layer 1 oracle.

No backend imports. Inputs are controlled scenarios, not historical snapshots.
Run --check to compare the committed review sheet with this calculation.
"""
import argparse
import json
from fractions import Fraction as F
from pathlib import Path


def exact(value):
    return {"numerator": str(value.numerator), "denominator": str(value.denominator)}


def calculate(name, bid, ask, bid_amount, ask_amount):
    bid, ask, bid_amount, ask_amount = map(F, (bid, ask, bid_amount, ask_amount))
    mid = (bid + ask) / 2
    return {
        "name": name,
        "mid": exact(mid),
        "spreadPct": exact((ask - bid) / mid * 100),
        "depth": [
            {"delta": str(d), "buy": exact(ask * ask_amount if ask <= mid * (1 + F(d)) else F(0)),
             "sell": exact(bid * bid_amount if bid >= mid * (1 - F(d)) else F(0))}
            for d in ("0.02", "0.05", "0.10")
        ],
        "manipulation": [
            {"delta": d, "cost": exact(ask * ask_amount if ask < mid * (1 + F(d)) else F(0)),
             "reachable": ask >= mid * (1 + F(d))}
            for d in ("0.5", "1", "10", "100")
        ],
    }


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    result = {
        "status": "assistant-prepared independent arithmetic; awaiting human review",
        "scope": "synthetic book-only scenarios; no historical market claim",
        "cases": [calculate("normal", "99/100", "101/100", "1000000", "1000000"),
                  calculate("broken", "1057/1000", "266843207/2500000", "0.0001000", "1.2185312")],
        "incomplete": {"result": None, "reason": "missing offer history or unknown pool coverage; refuse persistence"},
    }
    path = Path(__file__).with_name("representative-calculations.json")
    body = json.dumps(result, indent=2) + "\n"
    if args.check:
        if path.read_text(encoding="utf-8") != body:
            raise SystemExit("review calculations differ; investigate, do not fit them to backend output")
        print("Independent rational calculations match the review sheet")
    else:
        path.write_text(body, encoding="utf-8")


if __name__ == "__main__":
    main()
