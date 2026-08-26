package domain

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// The 22 February 2026 manipulation, as the only worked example this file has.
//
// Every figure below predates this test. The two prices are the exact price
// fractions Horizon serves for the two consecutive trades, the notional is the
// headline number of docs/methodology/10-validation.md section 7, and the ratio
// of about 101 is recomputed independently by scripts/audit-verification.sh.
// Nothing here was produced by running the code it tests.
const (
	// The last honest trade before the manipulation, 2026-02-22T00:06:31Z, in
	// operation 263454256009383937.
	honestN, honestD = 2125646195, 2010206197
	// The manipulating trade, 2026-02-22T00:10:21Z, ledger 61340263, alone in
	// operation 263454423513071617.
	attackN, attackD = 266843207, 2500000
)

func fill(op string, idx, sec int, n, d int64, base, counter string) Trade {
	return Trade{
		ID:            op + "-" + itoa(idx),
		OperationID:   op,
		FillIndex:     idx,
		ClosedAt:      time.Date(2026, 2, 22, 0, 0, sec, 0, time.UTC),
		Type:          "orderbook",
		Price:         Price{N: n, D: d},
		BaseAmount:    dec(base),
		CounterAmount: dec(counter),
	}
}

func itoa(n int) string { return decimal.NewFromInt(int64(n)).String() }

// honestThenAttack is the real pair of operations, three minutes fifty apart.
func honestThenAttack() []Trade {
	return []Trade{
		fill("263454256009383937", 1, 391, honestN, honestD, "0.0273371", "0.0289069"),
		fill("263454423513071617", 0, 621, attackN, attackD, "0.0501003", "5.3475699"),
	}
}

func TestTheManipulationIsABetweenLegsBoundAndSaysSo(t *testing.T) {
	bounds := TradeImpliedDepthBounds(honestThenAttack())
	if len(bounds) != 1 {
		t.Fatalf("want 1 bound, got %d", len(bounds))
	}
	b := bounds[0]

	// It spans two operations, so its cause is assumed rather than shown. A
	// caller that reports this number without that label is claiming more than
	// the chain does.
	if b.Kind != BoundBetweenLegs {
		t.Errorf("kind = %q, want %q", b.Kind, BoundBetweenLegs)
	}

	// The bound is the notional the attacker actually paid. This is the sentence
	// Deliverable 2 rests on: over that whole price range depth was at most
	// 5.3475699 USDC, read off the chain rather than reconstructed.
	if want := dec("5.3475699"); !b.Bound.Equal(want) {
		t.Errorf("bound = %s, want %s", b.Bound, want)
	}

	// P_after / P_before is about 100.94, which is the "100x" of the incident
	// report computed from the exact fractions rather than from the rounded
	// 1.057 that document quotes. δ is that ratio minus one.
	if got := b.Delta.Add(one); got.LessThan(dec("100.94")) || got.GreaterThan(dec("100.95")) {
		t.Errorf("price ratio = %s, want about 100.94", got)
	}

	// Three minutes fifty is exactly the quantity that makes the caveat real, so
	// it has to reach the caller rather than being dropped.
	if want := 3*time.Minute + 50*time.Second; b.Elapsed != want {
		t.Errorf("elapsed = %s, want %s", b.Elapsed, want)
	}
}

func TestFillsOfOneLegGiveACausalBoundWorthTheWholeLeg(t *testing.T) {
	// One taker walking a book from 1.00 to 1.20, in three fills totalling
	// 6 USDC. The engine fills best price first, so this span was crossed BY
	// this operation and needs no assumption about the gap.
	trades := []Trade{
		fill("900", 0, 10, 100, 100, "1", "1"),
		fill("900", 1, 10, 110, 100, "2", "2.2"),
		fill("900", 2, 10, 120, 100, "3", "3.6"),
	}
	bounds := TradeImpliedDepthBounds(trades)
	if len(bounds) != 1 {
		t.Fatalf("want 1 bound, got %d", len(bounds))
	}
	b := bounds[0]
	if b.Kind != BoundWithinLeg {
		t.Errorf("kind = %q, want %q", b.Kind, BoundWithinLeg)
	}
	// δ = 1.20 / 1.00 - 1.
	if want := dec("0.2"); !b.Delta.Equal(want) {
		t.Errorf("delta = %s, want %s", b.Delta, want)
	}
	// The bound is the WHOLE operation, 1 + 2.2 + 3.6, and not the last fill.
	// Using the last fill alone would claim the book was thinner than it was.
	if want := dec("6.8"); !b.Bound.Equal(want) {
		t.Errorf("bound = %s, want %s", b.Bound, want)
	}
	if b.From.ID != "900-0" || b.To.ID != "900-2" {
		t.Errorf("endpoints = %s -> %s, want the first and last fill", b.From.ID, b.To.ID)
	}
}

func TestAPathPaymentThroughTheBaseAssetIsTwoLegsAndNotOneWalk(t *testing.T) {
	// Operation 263504030385864713, 2026-02-22T18:48:02Z, ledger 61351813:
	// path_payment_strict_send USDC -> USTRY -> USDC. It BUYS 0.890217 USTRY on
	// the order book at 1.1233215 and SELLS the same 0.890217 to the pool at
	// 0.9953083, for a round trip that returned 0.8860404 of the 1 USDC sent.
	//
	// Read as one walk this is an 11.4 percent span for 1.886 USDC and it was the
	// tightest causal bound in the whole of February 2026. It is not a bound at
	// all: the two prices are two venues at one instant, and no liquidity was
	// crossed between them. Direction is what tells them apart.
	buy := fill("263504030385864713", 0, 10, 11233215, 10000000, "0.890217", "1")
	buy.BaseIsSeller = false
	sell := fill("263504030385864713", 1, 10, 9953083, 10000000, "0.890217", "0.8860404")
	sell.BaseIsSeller = true
	sell.Type = "liquidity_pool"

	for _, b := range TradeImpliedDepthBounds([]Trade{buy, sell}) {
		if b.Kind == BoundWithinLeg {
			t.Errorf("a causal bound of %s at delta %s was claimed across a direction change",
				b.Bound, b.Delta)
		}
	}
}

func TestASingleFillLegCrossesNothingOnItsOwn(t *testing.T) {
	trades := []Trade{fill("900", 0, 10, 100, 100, "1", "1")}
	if got := TradeImpliedDepthBounds(trades); len(got) != 0 {
		t.Errorf("want no bound from one fill, got %d", len(got))
	}
}

func TestBothKindsAreEmittedAndCanBeSeparated(t *testing.T) {
	trades := []Trade{
		fill("900", 0, 10, 100, 100, "1", "1"),
		fill("900", 1, 10, 110, 100, "2", "2.2"),
		fill("901", 0, 20, 200, 100, "1", "2"),
		fill("901", 1, 20, 220, 100, "1", "2.2"),
	}
	bounds := TradeImpliedDepthBounds(trades)
	within := BoundsOfKind(bounds, BoundWithinLeg)
	across := BoundsOfKind(bounds, BoundBetweenLegs)

	if len(within) != 2 {
		t.Errorf("want 2 within-leg bounds, got %d", len(within))
	}
	if len(across) != 1 {
		t.Errorf("want 1 between-legs bound, got %d", len(across))
	}
	if len(within)+len(across) != len(bounds) {
		t.Errorf("the two kinds do not partition the result")
	}
	// The across bound spans 901's first fill against 900's last, and is worth
	// the arriving operation, 2 + 2.2.
	if len(across) == 1 {
		if want := dec("4.2"); !across[0].Bound.Equal(want) {
			t.Errorf("across bound = %s, want %s", across[0].Bound, want)
		}
		if across[0].From.ID != "900-1" || across[0].To.ID != "901-0" {
			t.Errorf("across endpoints = %s -> %s", across[0].From.ID, across[0].To.ID)
		}
	}
}

func TestABoundOnlyReachesRungsAtOrBelowItsOwnDelta(t *testing.T) {
	bounds := TradeImpliedDepthBounds(honestThenAttack())

	// δ is about 99.94, so it bounds every rung below it.
	for _, rung := range []string{"0.02", "0.05", "0.10", "0.5", "1"} {
		got, ok := TightestBoundAtLeast(bounds, dec(rung))
		if !ok {
			t.Fatalf("rung %s: no bound offered, want one", rung)
		}
		if want := dec("5.3475699"); !got.Equal(want) {
			t.Errorf("rung %s: bound = %s, want %s", rung, got, want)
		}
	}

	// It says nothing about a rung ABOVE its own δ, and must not pretend to.
	if _, ok := TightestBoundAtLeast(bounds, dec("200")); ok {
		t.Error("a bound observed at delta 99.94 was offered for the 200 rung")
	}
}

func TestTheOrderTradesArriveInDoesNotChangeTheResult(t *testing.T) {
	forward := []Trade{
		fill("900", 0, 10, 100, 100, "1", "1"),
		fill("900", 1, 10, 110, 100, "2", "2.2"),
		fill("901", 0, 20, 200, 100, "1", "2"),
	}
	reversed := []Trade{forward[2], forward[1], forward[0]}

	a := TradeImpliedDepthBounds(forward)
	b := TradeImpliedDepthBounds(reversed)
	if len(a) != len(b) {
		t.Fatalf("lengths differ: %d and %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Kind != b[i].Kind || !a[i].Delta.Equal(b[i].Delta) ||
			!a[i].Bound.Equal(b[i].Bound) || a[i].To.ID != b[i].To.ID {
			t.Errorf("row %d differs between input orderings", i)
		}
	}
}

func TestALegThatMovedNothingProducesNoBound(t *testing.T) {
	same := []Trade{
		fill("900", 0, 100, honestN, honestD, "1", "1.0574269"),
		fill("901", 0, 200, honestN, honestD, "2", "2.1148539"),
	}
	if got := TradeImpliedDepthBounds(same); len(got) != 0 {
		t.Errorf("want no bound from two operations at one price, got %d", len(got))
	}
}

func TestAnUnusablePriceIsDroppedRatherThanBoundingAtZero(t *testing.T) {
	trades := []Trade{
		fill("900", 0, 100, honestN, honestD, "0.02", "0.0211"),
		{ID: "901-0", OperationID: "901", ClosedAt: time.Date(2026, 2, 22, 0, 2, 0, 0, time.UTC),
			Price: Price{N: 0, D: 0}, CounterAmount: decimal.New(1, 0)},
		fill("902", 0, 300, attackN, attackD, "0.05", "5.3475699"),
	}
	bounds := TradeImpliedDepthBounds(trades)
	if len(bounds) != 1 {
		t.Fatalf("want 1 bound across the unusable record, got %d", len(bounds))
	}
	if bounds[0].From.ID != "900-0" || bounds[0].To.ID != "902-0" {
		t.Errorf("the unusable record was not dropped: %s -> %s", bounds[0].From.ID, bounds[0].To.ID)
	}
}

func TestATradeWithNoOperationIdGroupsWithNothing(t *testing.T) {
	// Two fills that would be one operation if their tokens had parsed. With no
	// operation id they are two operations, which withholds a causal bound
	// rather than inventing one.
	trades := []Trade{
		{ID: "?", ClosedAt: time.Date(2026, 2, 22, 0, 0, 10, 0, time.UTC),
			Price: Price{N: 100, D: 100}, CounterAmount: dec("1")},
		{ID: "?", ClosedAt: time.Date(2026, 2, 22, 0, 0, 10, 0, time.UTC),
			Price: Price{N: 110, D: 100}, CounterAmount: dec("2.2")},
	}
	for _, b := range TradeImpliedDepthBounds(trades) {
		if b.Kind == BoundWithinLeg {
			t.Error("a causal bound was manufactured from two ungrouped trades")
		}
	}
}

func TestNoTradesIsNotAnError(t *testing.T) {
	if got := TradeImpliedDepthBounds(nil); len(got) != 0 {
		t.Errorf("want no bounds from no trades, got %d", len(got))
	}
	if _, ok := TightestBoundAtLeast(nil, dec("0.05")); ok {
		t.Error("a bound was offered where no trade was observed")
	}
}
