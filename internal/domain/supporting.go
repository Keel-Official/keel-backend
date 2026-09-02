// The supporting metrics: FR-8, FR-9 and FR-10.
//
// ZONE: YELLOW, the same as the rest of internal/domain. Every function here is
// pure: it takes trades and balances that somebody else read, plus an anchor
// time, and returns arithmetic over them. No clock, no network, no ordering
// assumption about its input.
//
// THE ORDERING RULE, AND THE ORACLE THIS FILE WAS WRITTEN AGAINST. A function in
// this package may only be written after its expected values exist in a RED
// artifact, and that artifact is named out loud first. For this file the oracle is
// docs/methodology/07-supporting-metrics.md sections 1 to 4, together with the two
// trades CSV under docs/evidences/ that its figures were computed from. Those
// figures were produced by Al before this file existed: 14,478 genuine trades of
// 56,863 in August, 12,204 of 13,547 in February, a top-1 holder share of
// 91.5406 per cent, a monthly volume-to-supply ratio of 0.0548603 per cent. See
// DEC-008 and the reconstructed ordering-rule record.
//
// Nothing in this file may be adjusted to make those numbers come out. Where this
// code and section 1's table disagree, the disagreement is the finding and it is
// reported rather than tuned away. internal/conformance is where the comparison
// lives, for the same reason the golden fixture's comparison lives there.
//
// WHAT SECTION 1 SETTLES AND WHAT IT DOES NOT. It gives five exclusion conditions,
// their order, and the counts each one catches over two months. It does NOT name
// the statistic that condition 4 compares a pool fill against, only that the
// comparison is against "the contemporaneous order book" within plus or minus
// fifteen minutes, nor does it say what "dearer" means when a fill lands inside the
// spread.
//
// Those two gaps were closed by MEASUREMENT rather than by choosing, and the work
// is recorded at poolFillIsDearer and at median. Ten readings were enumerated and
// each was checked against two figures section 1 states and that were computed by
// hand before this file existed: August's 389 dearer / 57 cheaper / 150
// unevaluated, and February's condition 4 total of 8 trades and 0.7976176 USDC.
// Exactly one reading satisfies both. The first two attempts did not, and both are
// written down beside the one that did, because a rejected reading that looks more
// sophisticated than the accepted one is worth more as a record than as a deletion.
//
// This is the ordering rule doing the thing it exists for. The numbers came first,
// they were not adjusted, and where the prose was ambiguous they are what said
// which reading the prose meant.
//
// THE TWO FIFTEEN-MINUTE WINDOWS ARE NOT THE SAME WINDOW. Section 4 says so in as
// many words. Params.OracleWindow is how far back oracle volume is summed;
// GenuineRules.BookComparisonWindow is how far a pool fill may look for a
// contemporaneous book. They share a number today and they are not the same
// quantity, so they are two fields and a change to one must not move the other.

package domain

import (
	"errors"
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

// ---------------------------------------------------------------- Genuine trades

// GenuineState is the three-state result section 1 requires. Every trade resolves
// to exactly one of these and the metric reports the count and volume of each.
//
// Unevaluated is NOT a synonym for excluded and not a synonym for zero. It is
// reserved for one situation: a pool fill that passed all five conditions and has
// no order book within the comparison window to be judged against. Section 5 of
// 07-supporting-metrics.md and section 2 of 09-flags-and-bands.md both require
// that distinction to survive into the output.
type GenuineState string

// The three states.
const (
	GenuineStateGenuine     GenuineState = "genuine"
	GenuineStateExcluded    GenuineState = "excluded"
	GenuineStateUnevaluated GenuineState = "unevaluated"
)

// ExclusionCondition numbers the five conditions exactly as section 1 numbers
// them. The numbers are part of the output, not an implementation detail: the
// metric reports "how much volume was excluded and why", and a condition number is
// the "why" in the smallest form that stays checkable against the methodology.
type ExclusionCondition int

// The five conditions, in the order they are evaluated. ConditionNone is the zero
// value and means the trade was not excluded.
//
// THE ORDER IS LOAD-BEARING AND IS NOT AN OPTIMISATION. A trade stops at the first
// condition it meets. Section 1 works the consequence out in full: 133 of the 150
// August pool fills that lack a contemporaneous book are also dust, and because
// dust is tested first they resolve as Excluded rather than Unevaluated, which is
// why August has 17 unevaluated trades and not 150. Reversing the order would
// inflate the unknown bucket eightfold and hide sub-cent dust behind a label that
// is supposed to mean "could not measure".
const (
	ConditionNone            ExclusionCondition = 0
	ConditionSelfTrade       ExclusionCondition = 1
	ConditionDust            ExclusionCondition = 2
	ConditionIssuerLeg       ExclusionCondition = 3
	ConditionOffBookPoolFill ExclusionCondition = 4
	ConditionPriceOutlier    ExclusionCondition = 5
)

// String names each condition in the words section 1 uses for it, so a report can
// be read beside the methodology without a lookup table.
func (c ExclusionCondition) String() string {
	switch c {
	case ConditionNone:
		return "none"
	case ConditionSelfTrade:
		return "self-trade"
	case ConditionDust:
		return "dust"
	case ConditionIssuerLeg:
		return "issuer leg"
	case ConditionOffBookPoolFill:
		return "off-book pool fill"
	case ConditionPriceOutlier:
		return "price outlier"
	default:
		return "unknown"
	}
}

// GenuineRules holds the three constants section 1's conditions read. They live
// here rather than as literals for the reason P2-22 records about the crosscheck
// delay: the variable an experiment moves must be named in the code, or a run
// cannot say what it was run with.
type GenuineRules struct {
	// DustThreshold is condition 2, in the QUOTE asset. Section 1 sets it at 0.01
	// USDC. It removes three quarters of August's trades and 1.71 per cent of its
	// value, which is the whole argument for it: a market can be flooded with
	// sub-cent fills without moving real volume.
	DustThreshold decimal.Decimal

	// BookComparisonWindow is the plus-or-minus reach of condition 4. Fifteen
	// minutes. NOT Params.OracleWindow; see this file's header.
	BookComparisonWindow time.Duration

	// PriceOutlierFactor is condition 5. A price more than this multiple away from
	// the day's median in either direction is excluded. Section 1 sets it at 1.5
	// and it catches exactly one trade in February and none in August.
	PriceOutlierFactor decimal.Decimal
}

// DefaultGenuineRules returns the constants section 1 states for methodology
// 1.0.8-draft. Every value is CHOSEN and measured against the USTRY history, not
// calibrated across a body of assets.
func DefaultGenuineRules() GenuineRules {
	return GenuineRules{
		DustThreshold:        dec("0.01"),
		BookComparisonWindow: 15 * time.Minute,
		PriceOutlierFactor:   dec("1.5"),
	}
}

// TradeClassification is one trade's verdict. It carries the trade's identity
// rather than a copy of the trade, so a caller can join back to the record it came
// from and a reader can check any single verdict against the chain.
type TradeClassification struct {
	ID        string
	LedgerSeq uint32
	ClosedAt  time.Time

	State     GenuineState
	Condition ExclusionCondition

	// BaseAmount and CounterAmount are carried because the two volume figures are
	// denominated differently and both are required. Section 1 reports genuine
	// volume in the QUOTE asset; section 3's numerator takes the BASE leg of the
	// same trades so its ratio is dimensionless. Summing the wrong one is a
	// mistake the type system cannot catch, so both are here and named.
	BaseAmount    decimal.Decimal
	CounterAmount decimal.Decimal
}

// ClassifyTrades applies section 1's rule to every trade and returns one verdict
// per trade, in the order the trades were given.
//
// base is the asset whose issuer condition 3 tests. The issuer is taken from the
// asset rather than passed separately, because an asset is the pair (code, issuer)
// and splitting them is how a check comes to be made against a ticker.
//
// It allocates the daily medians once for the whole input rather than per trade.
// That is not only speed: a median recomputed per trade from a sliding subset is a
// different statistic, and two implementations of it would not agree.
func ClassifyTrades(trades []Trade, base Asset, r GenuineRules) []TradeClassification {
	medians := dailyOrderBookMedians(trades)
	bookPrices := orderBookPricesByTime(trades)

	out := make([]TradeClassification, 0, len(trades))
	for _, t := range trades {
		c := TradeClassification{
			ID:            t.ID,
			LedgerSeq:     t.LedgerSeq,
			ClosedAt:      t.ClosedAt,
			BaseAmount:    t.BaseAmount,
			CounterAmount: t.CounterAmount,
		}
		c.State, c.Condition = classifyOne(t, base, r, medians, bookPrices)
		out = append(out, c)
	}
	return out
}

// classifyOne walks the five conditions in order and stops at the first met.
//
// The switch is written as a sequence of early returns rather than a loop over
// predicates because condition 4 is not a predicate: it has three outcomes, not
// two, and the third one is what produces an Unevaluated trade. Folding it into a
// boolean list would lose exactly the state section 5 exists to preserve.
func classifyOne(
	t Trade,
	base Asset,
	r GenuineRules,
	medians map[string]decimal.Decimal,
	bookPrices []timedPrice,
) (GenuineState, ExclusionCondition) {
	// 1. Self-trade. Both sides are one account.
	//
	// Empty account IDs are not a match. A pool trade has no account on one side,
	// and treating two empty strings as the same account would classify every
	// pool-to-pool artefact as a self-trade.
	if t.BaseAccount != "" && t.BaseAccount == t.CounterAccount {
		return GenuineStateExcluded, ConditionSelfTrade
	}

	// 2. Dust. Counter notional below the threshold, strictly.
	//
	// Strictly below, so a trade exactly at 0.01 USDC is kept. Section 1 says
	// "below a dust threshold" and a boundary trade has to land in one bucket;
	// keeping it is the choice that does not silently widen the exclusion.
	if t.CounterAmount.LessThan(r.DustThreshold) {
		return GenuineStateExcluded, ConditionDust
	}

	// 3. Issuer leg. Either side is the base asset's issuer.
	//
	// The issuer holds unissued supply, so a trade against it is issuance or
	// redemption rather than a market. Section 2 records that this never fires on
	// USTRY in either month; it stays because an asset where it does fire needs
	// it, and a condition that costs nothing to check is not the place to save.
	if base.Issuer != "" && (t.BaseAccount == base.Issuer || t.CounterAccount == base.Issuer) {
		return GenuineStateExcluded, ConditionIssuerLeg
	}

	// 4. Off-book pool fill, and the only condition with three outcomes.
	if t.LiquidityPoolID != "" {
		ref, ok := contemporaneousBookPrice(bookPrices, t.ClosedAt, r.BookComparisonWindow)
		switch {
		case !ok:
			// No book within the window. The fill is not judged, and saying so is
			// the point: failing loud beats scoring it against an hour-stale
			// price. This is reached only after conditions 1 to 3 have passed,
			// which is why August has 17 of these and not 150.
			return GenuineStateUnevaluated, ConditionNone
		case poolFillIsDearer(t, ref):
			return GenuineStateExcluded, ConditionOffBookPoolFill
		}
		// A fill CHEAPER than the book is kept, deliberately. It is real price
		// improvement, and 55 of February's 58 pool trades were arbitrage closing
		// a twelve day gap. A rule that discarded them would remove the most
		// economically useful activity in the record.
	}

	// 5. Price outlier, against the day's median of ORDER-BOOK trades only.
	//
	// Pool fills do not enter the median, so this cannot circle back into
	// condition 4. A day with no order-book trades has no median, and the trade
	// PASSES rather than falling to Unevaluated: Unevaluated is reserved for a
	// pool fill that cannot be compared, not a catch-all for anything unmeasured.
	// Section 1 records that this case occurs zero times in August.
	if m, ok := medians[utcDay(t.ClosedAt)]; ok && priceIsOutlier(t.Price.Decimal(), m, r.PriceOutlierFactor) {
		return GenuineStateExcluded, ConditionPriceOutlier
	}

	return GenuineStateGenuine, ConditionNone
}

// timedPrice is one order-book trade reduced to what condition 4 needs of it.
type timedPrice struct {
	at    time.Time
	price decimal.Decimal
}

// orderBookPricesByTime returns every ORDER-BOOK trade's price, sorted by close
// time. Pool fills are excluded, because the thing condition 4 compares against
// is the book and a pool fill is not the book.
//
// Sorted so that the window lookup is a bounded scan rather than a full pass, and
// sorted explicitly rather than assuming the caller's order: the input arrives
// from a CSV, a database, or a Horizon page, and only one of those three promises
// an order.
func orderBookPricesByTime(trades []Trade) []timedPrice {
	out := make([]timedPrice, 0, len(trades))
	for _, t := range trades {
		if t.LiquidityPoolID != "" {
			continue
		}
		out = append(out, timedPrice{at: t.ClosedAt, price: t.Price.Decimal()})
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].at.Equal(out[j].at) {
			return out[i].at.Before(out[j].at)
		}
		return out[i].price.LessThan(out[j].price)
	})
	return out
}

// contemporaneousBookPrice is the median order-book price within plus or minus the
// window of at. The second return is false when no order-book trade falls inside
// it, which is the Unevaluated case.
//
// THE MEDIAN IS A READING OF SECTION 1 AND NOT A QUOTATION FROM IT. See the file
// header. The window is symmetric and CLOSED on both ends here, which is
// deliberately different from the half-open windows in sections 3 and 4: those two
// are aggregations anchored to an output ledger, where a boundary trade must land
// in exactly one window. This one is a neighbourhood around an instant, where the
// question is only whether a comparable book existed nearby, and excluding a trade
// that sits exactly fifteen minutes away would answer that question with no.
func contemporaneousBookPrice(prices []timedPrice, at time.Time, window time.Duration) (decimal.Decimal, bool) {
	lo := at.Add(-window)
	hi := at.Add(window)

	start := sort.Search(len(prices), func(i int) bool { return !prices[i].at.Before(lo) })
	var in []decimal.Decimal
	for i := start; i < len(prices) && !prices[i].at.After(hi); i++ {
		in = append(in, prices[i].price)
	}
	if len(in) == 0 {
		return decimal.Zero, false
	}
	return median(in), true
}

// poolFillIsDearer answers whether the pool fill's price was higher than the
// contemporaneous book, which is the direction condition 4 turns on: only a fill
// DEARER than the book is excluded, and a cheaper one is kept as real price
// improvement.
//
// DEARER IS A STATEMENT ABOUT THE PRICE LEVEL AND NOT ABOUT WHO WAS HURT, and that
// is the correction this function carries rather than the reading it was written
// with. Prices here are quote-per-base throughout, so "dearer" is simply a higher
// number: the base asset cost more than the book was charging for it at that
// moment. LiquidityPoolSide is not consulted.
//
// THE SIDE-AWARE READING WAS TRIED FIRST AND IS WRONG, and it is recorded here
// because it is the more sophisticated-looking of the two. That reading asked
// whether the fill was worse FOR THE ACCOUNT: a higher price when the pool supplies
// the base and the account is buying, a lower price when the account is selling. It
// is a coherent rule and it is not this one. Section 1's own pre-ordering figures
// settle it, and they were not used as a target until after both readings existed:
//
//	                          dearer  cheaper  unevaluated
//	07 section 1, August:        389       57          150
//	price > book (this code):    389       57          150
//	side-aware:                  419       27          150
//
// The unevaluated count is 150 under both, which is what says the plus-or-minus
// fifteen minute window and the median statistic were already right and only the
// comparison was wrong. Adjust the code to match the numbers, never the reverse.
func poolFillIsDearer(t Trade, book decimal.Decimal) bool {
	return t.Price.Decimal().GreaterThan(book)
}

// dailyOrderBookMedians is the median price per UTC calendar day, over ORDER-BOOK
// trades only, for condition 5.
//
// Keyed by the UTC date string rather than by time.Time, because two times in one
// day are different keys and the day is the unit section 1 names. A day with no
// order-book trades is absent from the map, which is what makes the "no median, so
// the trade passes" branch expressible.
func dailyOrderBookMedians(trades []Trade) map[string]decimal.Decimal {
	byDay := map[string][]decimal.Decimal{}
	for _, t := range trades {
		if t.LiquidityPoolID != "" {
			continue
		}
		d := utcDay(t.ClosedAt)
		byDay[d] = append(byDay[d], t.Price.Decimal())
	}

	// Sorted keys before iteration, non-negotiable rule 2. Nothing in this loop
	// depends on order today, and that is exactly when the rule gets dropped and
	// then costs something later.
	days := make([]string, 0, len(byDay))
	for d := range byDay {
		days = append(days, d)
	}
	sort.Strings(days)

	out := make(map[string]decimal.Decimal, len(days))
	for _, d := range days {
		out[d] = median(byDay[d])
	}
	return out
}

// utcDay is the calendar day of t in UTC, as YYYY-MM-DD.
//
// UTC and not local. A local day would make the same trade fall in different days
// on two machines, which breaks NFR-9 in the least visible way available.
func utcDay(t time.Time) string { return t.UTC().Format("2006-01-02") }

// median sorts a copy and returns the middle value, taking the UPPER of the two
// middles on an even count.
//
// THE EVEN-COUNT CONVENTION IS NOT A DETAIL AND IT WAS DERIVED, NOT CHOSEN. The
// obvious implementation averages the two middles, and that was the first one
// written here. It is wrong for this methodology, and the way that was established
// is worth recording, because the sentence in section 1 does not settle it and no
// amount of reading it again would have.
//
// Section 1 states two figures that bear on condition 4, and they were computed by
// hand from the two evidence CSV before this file existed. Ten readings were
// enumerated, five reference statistics crossed with two comparison directions,
// and each was measured against BOTH figures:
//
//	                                  August 389/57/150   February 8 / 0.7976176
//	median, average of two middles           yes                    no  (9)
//	median, LOWER of two middles             yes                    no  (9)
//	median, UPPER of two middles             yes                    YES
//	max in window                            no  (380)              yes
//	min in window                            no  (394)              no
//	  ... each also with a side-aware comparison, all ten failing August
//
// Exactly one of the ten satisfies both. August alone does not discriminate: all
// three median conventions give 389/57/150, because the fills it separates are
// nowhere near a midpoint. February discriminates on a SINGLE pool fill, at
// 2026-02-24T00:40:15Z, whose window holds 64 order-book prints split exactly 32
// at 1.0585791463 and 32 at 1.0576268536, and whose own price of 1.0583315435
// sits between them. Averaging invents a reference of 1.0581030 that never traded
// and calls the fill dearer; the upper middle is a price that actually traded and
// calls it cheaper, which is what section 1's count of 8 requires.
//
// So the convention has a property worth stating beyond fitting a number: the
// reference is always a price that was actually paid, never one interpolated
// between two that were. Averaging inside a spread compares a fill against a
// midpoint no counterparty ever offered.
//
// It applies to condition 5's daily median too, since both go through here. That is
// deliberate: two medians in one document computed two ways is how two numbers that
// should agree begin to disagree.
func median(in []decimal.Decimal) decimal.Decimal {
	if len(in) == 0 {
		return decimal.Zero
	}
	s := make([]decimal.Decimal, len(in))
	copy(s, in)
	sort.Slice(s, func(i, j int) bool { return s[i].LessThan(s[j]) })

	// len/2 is the upper of the two middles on an even count and the true middle
	// on an odd one, so one expression serves both and there is no branch to get
	// backwards.
	return s[len(s)/2]
}

// priceIsOutlier reports whether price is more than factor away from median in
// either direction.
//
// Both directions, and the test is on the RATIO rather than on a difference,
// because "1.5x from the median" is a multiplicative statement. The incident trade
// is the specimen: 106.737283 against a daily median near 1.057427 is a ratio
// close to 101, and it is the only trade condition 5 catches in February.
//
// A median of zero has no meaningful ratio, so nothing is an outlier against it.
func priceIsOutlier(price, med, factor decimal.Decimal) bool {
	if med.LessThanOrEqual(decimal.Zero) || price.LessThanOrEqual(decimal.Zero) {
		return false
	}
	if price.GreaterThan(med.Mul(factor)) {
		return true
	}
	return price.Mul(factor).LessThan(med)
}

// ---------------------------------------------------------------- Summary

// ConditionTotal is one condition's tally.
type ConditionTotal struct {
	Condition ExclusionCondition
	Trades    int
	BaseVol   decimal.Decimal
	QuoteVol  decimal.Decimal
}

// GenuineSummary is section 1's Outputs block: the count and volume of each state,
// with the excluded volume broken down per condition.
//
// Both volume units are carried throughout. Section 1 quotes its figures in the
// quote asset and section 3 needs the base leg of the same set, and a summary that
// held one of them would send the second caller back to the classifications to
// re-sum, which is where the two figures start to drift apart.
type GenuineSummary struct {
	Total int

	GenuineTrades   int
	GenuineBaseVol  decimal.Decimal
	GenuineQuoteVol decimal.Decimal

	ExcludedTrades   int
	ExcludedBaseVol  decimal.Decimal
	ExcludedQuoteVol decimal.Decimal

	UnevaluatedTrades   int
	UnevaluatedBaseVol  decimal.Decimal
	UnevaluatedQuoteVol decimal.Decimal

	// ByCondition is ordered by condition number, always five entries, including
	// the ones that caught nothing. A condition that fires zero times is a result
	// and not an absence: section 1 reports four zeroes for August and they are
	// what shows the exclusion is entirely dust.
	ByCondition []ConditionTotal

	TotalBaseVol  decimal.Decimal
	TotalQuoteVol decimal.Decimal
}

// ExcludedPct is excluded quote volume as a percentage of total quote volume, which
// is the figure SupportingMetrics.TradesExcludedPct carries.
//
// Nil when there is no volume to take a percentage of. Zero volume gives no
// percentage rather than zero per cent, for the reason section 5 states: an asset
// with no data must not read the same as one checked and found clean.
func (s GenuineSummary) ExcludedPct() *decimal.Decimal {
	if s.TotalQuoteVol.LessThanOrEqual(decimal.Zero) {
		return nil
	}
	p := s.ExcludedQuoteVol.DivRound(s.TotalQuoteVol, Precision).Mul(dec("100"))
	return &p
}

// SummariseGenuine tallies a set of classifications.
func SummariseGenuine(cs []TradeClassification) GenuineSummary {
	s := GenuineSummary{Total: len(cs)}

	byCond := map[ExclusionCondition]*ConditionTotal{}
	for _, c := range []ExclusionCondition{
		ConditionSelfTrade, ConditionDust, ConditionIssuerLeg,
		ConditionOffBookPoolFill, ConditionPriceOutlier,
	} {
		byCond[c] = &ConditionTotal{Condition: c}
	}

	for _, c := range cs {
		s.TotalBaseVol = s.TotalBaseVol.Add(c.BaseAmount)
		s.TotalQuoteVol = s.TotalQuoteVol.Add(c.CounterAmount)

		switch c.State {
		case GenuineStateGenuine:
			s.GenuineTrades++
			s.GenuineBaseVol = s.GenuineBaseVol.Add(c.BaseAmount)
			s.GenuineQuoteVol = s.GenuineQuoteVol.Add(c.CounterAmount)
		case GenuineStateExcluded:
			s.ExcludedTrades++
			s.ExcludedBaseVol = s.ExcludedBaseVol.Add(c.BaseAmount)
			s.ExcludedQuoteVol = s.ExcludedQuoteVol.Add(c.CounterAmount)
			if ct, ok := byCond[c.Condition]; ok {
				ct.Trades++
				ct.BaseVol = ct.BaseVol.Add(c.BaseAmount)
				ct.QuoteVol = ct.QuoteVol.Add(c.CounterAmount)
			}
		case GenuineStateUnevaluated:
			s.UnevaluatedTrades++
			s.UnevaluatedBaseVol = s.UnevaluatedBaseVol.Add(c.BaseAmount)
			s.UnevaluatedQuoteVol = s.UnevaluatedQuoteVol.Add(c.CounterAmount)
		}
	}

	// Fixed order, by condition number, so the output is comparable with section
	// 1's table row for row and identical between runs.
	for _, c := range []ExclusionCondition{
		ConditionSelfTrade, ConditionDust, ConditionIssuerLeg,
		ConditionOffBookPoolFill, ConditionPriceOutlier,
	} {
		s.ByCondition = append(s.ByCondition, *byCond[c])
	}
	return s
}

// ---------------------------------------------------------------- FR-10

// LastGenuineTrade is the most recent genuine trade at or before the anchor, as a
// reference rather than a copy.
//
// Nil means UNEVALUATED and never "none recently". Section 1: the metric is
// unevaluated when no genuine trade exists in the fetched history, because there
// is no timestamp to measure from. That is a different statement from a large but
// finite gap, which is a measured value and is the very signal the metric exists to
// surface.
//
// Trades after the anchor are ignored rather than treated as an error. A caller
// replaying a past ledger legitimately holds trades from after it, and silently
// reading one of them would produce a negative staleness.
func LastGenuineTrade(cs []TradeClassification, anchor time.Time) *TradeRef {
	var best *TradeClassification
	for i := range cs {
		c := &cs[i]
		if c.State != GenuineStateGenuine || c.ClosedAt.After(anchor) {
			continue
		}
		if best == nil || c.ClosedAt.After(best.ClosedAt) ||
			(c.ClosedAt.Equal(best.ClosedAt) && c.LedgerSeq > best.LedgerSeq) {
			best = c
		}
	}
	if best == nil {
		return nil
	}
	return &TradeRef{LedgerSeq: best.LedgerSeq, At: best.ClosedAt}
}

// TimeSinceLastGenuineTrade is the anchor minus the last genuine trade's close
// time. Nil when there is no genuine trade to measure from, for the reason above.
//
// The anchor is the output ledger's close time and never the wall clock. This
// package cannot call time.Now, which is the architecture test enforcing what
// NFR-9 requires rather than trusting it.
func TimeSinceLastGenuineTrade(cs []TradeClassification, anchor time.Time) *time.Duration {
	ref := LastGenuineTrade(cs, anchor)
	if ref == nil {
		return nil
	}
	d := anchor.Sub(ref.At)
	return &d
}

// ---------------------------------------------------------------- Windows

// InWindow reports whether t falls in the half-open window ending at anchor:
// anchor-w < t <= anchor.
//
// The far edge is OPEN and the recent edge is CLOSED, which sections 3 and 4 both
// fix explicitly and for one reason: a trade sitting exactly on an edge must land
// in exactly one window, or two correct implementations return different numbers
// for the same ledger. The recent edge is the closed one so that the output
// ledger's own trades count.
func InWindow(t, anchor time.Time, w time.Duration) bool {
	return t.After(anchor.Add(-w)) && !t.After(anchor)
}

// GenuineVolumeInWindow sums the genuine volume inside the half-open window ending
// at anchor. Both units are returned, because section 1 and section 4 quote the
// quote asset and section 3's numerator needs the base leg of the same trades.
//
// The second return is the number of RECORDED trades in the window, genuine or
// not, and it is not decoration. Section 4 requires it: a window with recorded
// trades but no genuine volume is an active market full of fake prints, and a
// window with no trades at all is a silent one. Both are a measured zero and they
// carry different risk into the MR term, so the count is what separates them.
func GenuineVolumeInWindow(cs []TradeClassification, anchor time.Time, w time.Duration) (base, quote decimal.Decimal, recorded int) {
	for _, c := range cs {
		if !InWindow(c.ClosedAt, anchor, w) {
			continue
		}
		recorded++
		if c.State != GenuineStateGenuine {
			continue
		}
		base = base.Add(c.BaseAmount)
		quote = quote.Add(c.CounterAmount)
	}
	return base, quote, recorded
}

// ---------------------------------------------------------------- FR-8

// HolderBalance is one trustline reduced to what section 2 measures over.
type HolderBalance struct {
	AccountID string
	Balance   decimal.Decimal
}

// HolderExclusions is section 2's explicit exclusion list.
//
// EXPLICIT AND ASSET-SPECIFIC ON PURPOSE. Section 2 chooses an auditable list over
// pool-detection heuristics, and states the cost in as many words: run unchanged
// against another asset this returns wrong numbers with no warning, because that
// asset's pools are not on the list. Applying it to a new asset means editing the
// list first. A list can be audited line by line; a heuristic guessing which
// accounts are pools cannot.
type HolderExclusions struct {
	// Issuer holds unissued supply and is not a holder.
	Issuer string

	// Addresses are the pool and contract positions to drop. For USTRY these are
	// the AMM pool ID and the Blend V2 contract, and section 2 records that
	// NEITHER has ever appeared in /accounts?asset=: all 875 trustlines in the 31
	// August pull were 56-character G addresses. So this exclusion is a no-op
	// against that endpoint and pool-held supply is out of the denominator by
	// construction rather than by subtraction.
	//
	// It stays wired anyway. It is a cheap guard, an asset whose pool position did
	// surface would need it, and a guard removed because it never fired is a guard
	// removed for the wrong reason.
	Addresses []string
}

// HolderStats is section 2's three measures plus the population they came from.
type HolderStats struct {
	// Population is the number of accounts left after the three removals. 263 for
	// the 31 August USTRY pull, from 875 trustlines.
	Population int

	// CirculatingSupply is the summed balance of that population, and it is the
	// SAME number section 3 divides by. One field, one set, which is what
	// checklist item 5 of 07-supporting-metrics.md verifies.
	CirculatingSupply decimal.Decimal

	Top1Pct  decimal.Decimal
	Top10Pct decimal.Decimal

	// HHI is the sum of squared PERCENTAGE shares, so it runs to 10,000 for a
	// single holder rather than to 1. Section 2 reports 8,410.8452 for USTRY and
	// reads it against the 2,500 that conventionally marks a highly concentrated
	// market, so the scale is part of the definition and not a presentation
	// choice.
	HHI decimal.Decimal

	// ZeroBalanceDropped and ExcludedDropped record what the filters removed, so a
	// reader can reconstruct the population from the raw pull. 612 and 0 for the
	// 31 August pull.
	ZeroBalanceDropped int
	ExcludedDropped    int
}

// ErrHolderSetEmpty is returned when nothing survives the filters. It is an error
// and not a zeroed result, because section 3 has to tell a zero denominator apart
// from an unavailable one and a struct full of zeroes cannot carry that.
var ErrHolderSetEmpty = errors.New("domain: no holder remains after exclusions, so circulating supply is zero and concentration is undefined")

// ErrHolderSetTruncated is returned when the caller could not page the full
// trustline set.
//
// Section 2 requires all three measures to be UNEVALUATED in that case and never
// partial, and the reason is specific to the endpoint: /accounts?asset= paginates
// by account ID, not by balance, so a partial fetch is an alphabetical slice
// rather than the largest holders. A top-1 share computed from it is the largest
// holder among whoever happened to be fetched, which is a guess in the shape of a
// measurement.
var ErrHolderSetTruncated = errors.New("domain: the trustline set was truncated, so holder concentration is unevaluated rather than partial")

// HolderConcentration computes section 2's three measures.
//
// truncated must be the caller's answer to "did I page the whole set", and it is a
// parameter rather than something inferred here because this package cannot see
// the endpoint. Passing false when the set was short is how a guess becomes a
// measurement, so the field it comes from is HolderObservation.Truncated() and
// nothing else.
func HolderConcentration(holders []HolderBalance, excl HolderExclusions, truncated bool) (HolderStats, error) {
	if truncated {
		return HolderStats{}, ErrHolderSetTruncated
	}

	drop := make(map[string]struct{}, len(excl.Addresses)+1)
	if excl.Issuer != "" {
		drop[excl.Issuer] = struct{}{}
	}
	for _, a := range excl.Addresses {
		if a != "" {
			drop[a] = struct{}{}
		}
	}

	var stats HolderStats
	kept := make([]decimal.Decimal, 0, len(holders))
	for _, h := range holders {
		if _, ok := drop[h.AccountID]; ok {
			stats.ExcludedDropped++
			continue
		}
		// Zero AND negative. A negative trustline balance is not a thing Horizon
		// serves, and if one ever arrives it must not enter a denominator.
		if h.Balance.LessThanOrEqual(decimal.Zero) {
			stats.ZeroBalanceDropped++
			continue
		}
		kept = append(kept, h.Balance)
		stats.CirculatingSupply = stats.CirculatingSupply.Add(h.Balance)
	}

	if len(kept) == 0 || stats.CirculatingSupply.LessThanOrEqual(decimal.Zero) {
		return HolderStats{}, ErrHolderSetEmpty
	}
	stats.Population = len(kept)

	// Descending by balance. Sorted rather than scanned for a maximum because the
	// top-10 share needs the order anyway, and two passes over one sort is
	// cheaper to read than a partial selection.
	sort.Slice(kept, func(i, j int) bool { return kept[i].GreaterThan(kept[j]) })

	hundred := dec("100")
	stats.Top1Pct = kept[0].DivRound(stats.CirculatingSupply, Precision).Mul(hundred)

	top10 := decimal.Zero
	for i := 0; i < len(kept) && i < 10; i++ {
		top10 = top10.Add(kept[i])
	}
	stats.Top10Pct = top10.DivRound(stats.CirculatingSupply, Precision).Mul(hundred)

	// HHI over PERCENTAGE shares. Squaring the share as a percentage rather than
	// as a fraction is what puts the result on the 0 to 10,000 scale the 2,500
	// benchmark is stated against.
	for _, b := range kept {
		share := b.DivRound(stats.CirculatingSupply, Precision).Mul(hundred)
		stats.HHI = stats.HHI.Add(share.Mul(share))
	}

	return stats, nil
}

// ---------------------------------------------------------------- FR-9

// VolumeToSupplyUnevaluated says which of section 3's three causes made a ratio
// unevaluated. They are kept apart because they are three different failures with
// three different fixes, and collapsing them tells an operator that something is
// wrong without saying what.
type VolumeToSupplyUnevaluated string

// The three causes, and none of them is "the numerator was zero". A window with
// recorded trades and no genuine volume is a MEASURED ZERO.
const (
	VolumeToSupplyOK VolumeToSupplyUnevaluated = ""

	// VolumeToSupplyNoDenominator: section 2 is unevaluated, so circulating
	// supply is unknown.
	VolumeToSupplyNoDenominator VolumeToSupplyUnevaluated = "denominator unavailable"

	// VolumeToSupplyZeroDenominator: the holder population is empty, so supply is
	// zero and the ratio is undefined rather than zero.
	VolumeToSupplyZeroDenominator VolumeToSupplyUnevaluated = "denominator zero"

	// VolumeToSupplyNoNumerator: the trade recording does not cover the window,
	// so genuine volume cannot be computed for it.
	VolumeToSupplyNoNumerator VolumeToSupplyUnevaluated = "numerator unavailable"
)

// VolumeToSupply is genuine volume divided by circulating supply, BOTH IN THE BASE
// ASSET, returned as a fraction.
//
// Both sides in the base asset is the whole point and it is not a convenience.
// August's volume-weighted price was 1.072817 USDC per USTRY and that number moves,
// so a quote-denominated numerator over a base-denominated denominator would report
// change when neither activity nor supply changed. In August the price barely
// drifted and the choice hardly shows; on a period like 22 February it would swing
// the ratio on price alone, which is when the discipline earns its keep.
//
// covered is the caller's answer to "does the trade recording reach the whole
// window". It cannot be inferred from an empty result: no trades in the window and
// no data for the window produce the same empty sum and are opposite findings.
func VolumeToSupply(genuineBaseVolume, circulatingSupply decimal.Decimal, supplyKnown, covered bool) (*decimal.Decimal, VolumeToSupplyUnevaluated) {
	switch {
	case !supplyKnown:
		return nil, VolumeToSupplyNoDenominator
	case circulatingSupply.LessThanOrEqual(decimal.Zero):
		return nil, VolumeToSupplyZeroDenominator
	case !covered:
		return nil, VolumeToSupplyNoNumerator
	}
	r := genuineBaseVolume.DivRound(circulatingSupply, Precision)
	return &r, VolumeToSupplyOK
}

// ---------------------------------------------------------------- Assembly

// SupportingInput is everything the three supporting metrics need that a Snapshot
// does not carry. It is a struct rather than eight parameters so that adding an
// input later cannot silently reorder the existing ones at a call site, which is
// the same reason flagInput is a struct.
//
// EVERY "IS IT AVAILABLE" QUESTION IS AN EXPLICIT FIELD AND NEVER INFERRED FROM AN
// EMPTY SLICE. No trades in the window and no data for the window produce the same
// empty sum and are opposite findings; section 3 requires them kept apart and a nil
// slice cannot do it.
type SupportingInput struct {
	// Anchor is the output ledger's close time. Every window is measured backward
	// from it and never from a clock, which is what NFR-9 requires and what the
	// architecture test enforces by banning time.Now from this package.
	Anchor time.Time

	// Base is the asset whose issuer condition 3 tests and whose supply the ratio
	// divides by.
	Base Asset

	Trades []Trade

	// TradesCover says whether the recording reaches the whole of each window. It
	// is the caller's answer and cannot be derived here.
	TradesCover bool

	Holders          []HolderBalance
	HoldersTruncated bool
	HoldersKnown     bool
	Exclusions       HolderExclusions

	Rules        GenuineRules
	OracleWindow time.Duration
}

// SupportingDetail is what SupportingMetrics has no room for: the per-condition
// breakdown, the three unevaluated causes, and the recorded trade count in the
// oracle window.
//
// It exists because SupportingMetrics is the API-shaped type and the API does not
// carry all of this, while an operator reading a run does need it. Section 4 is
// explicit that the recorded count must survive: a window with recorded trades and
// no genuine volume is an active market full of fake prints, and a window with no
// trades at all is a silent one. Both are a measured zero.
type SupportingDetail struct {
	Genuine GenuineSummary

	VolumeToSupplyD1Why  VolumeToSupplyUnevaluated
	VolumeToSupplyD7Why  VolumeToSupplyUnevaluated
	VolumeToSupplyD30Why VolumeToSupplyUnevaluated

	OracleWindowRecorded int

	// HolderErr is why holder concentration is unevaluated, nil when it is not.
	HolderErr error

	// CirculatingSupply is carried so the ratio's denominator can be read beside
	// the ratio. Zero when holder concentration is unevaluated.
	CirculatingSupply decimal.Decimal
}

// The three volume-to-supply windows, in the order SupportingMetrics names them.
const (
	windowD1  = 24 * time.Hour
	windowD7  = 7 * 24 * time.Hour
	windowD30 = 30 * 24 * time.Hour
)

// ComputeSupporting assembles FR-8, FR-9 and FR-10 into the output shape.
//
// It returns a value and a detail rather than an error, because none of the
// failures here is an error: every one of them is an UNEVALUATED metric, which is a
// result the contract has a shape for. Returning an error would collapse "the
// trustline set could not be paged" into the same channel as "the input was
// malformed", and section 5 exists to keep those apart.
func ComputeSupporting(in SupportingInput) (SupportingMetrics, SupportingDetail) {
	var out SupportingMetrics
	var det SupportingDetail

	// FR-8, and it comes first because the ratio in FR-9 divides by its
	// denominator.
	supplyKnown := false
	if in.HoldersKnown {
		stats, err := HolderConcentration(in.Holders, in.Exclusions, in.HoldersTruncated)
		if err != nil {
			det.HolderErr = err
		} else {
			top1, top10, hhi := stats.Top1Pct, stats.Top10Pct, stats.HHI
			out.HolderTop1Pct, out.HolderTop10Pct, out.HolderHHI = &top1, &top10, &hhi
			det.CirculatingSupply = stats.CirculatingSupply
			supplyKnown = true
		}
	} else {
		det.HolderErr = ErrHolderSetTruncated
	}

	// FR-10 and the numerator of FR-9, from one classification of one trade set.
	// Three sections share one genuine definition, which is section 4's own
	// requirement: two different genuine definitions in one document is how
	// numbers begin to contradict each other.
	cs := ClassifyTrades(in.Trades, in.Base, in.Rules)
	det.Genuine = SummariseGenuine(cs)
	out.TradesExcludedPct = det.Genuine.ExcludedPct()
	out.LastGenuineTrade = LastGenuineTrade(cs, in.Anchor)

	// FR-9, three windows.
	for _, w := range []struct {
		d   time.Duration
		dst **decimal.Decimal
		why *VolumeToSupplyUnevaluated
	}{
		{windowD1, &out.VolumeToSupplyD1, &det.VolumeToSupplyD1Why},
		{windowD7, &out.VolumeToSupplyD7, &det.VolumeToSupplyD7Why},
		{windowD30, &out.VolumeToSupplyD30, &det.VolumeToSupplyD30Why},
	} {
		base, _, _ := GenuineVolumeInWindow(cs, in.Anchor, w.d)
		ratio, why := VolumeToSupply(base, det.CirculatingSupply, supplyKnown, in.TradesCover)
		*w.dst, *w.why = ratio, why
	}

	// Section 4. The quote asset, because it feeds the MR term and is compared
	// against a manipulation cost denominated the same way.
	if in.TradesCover {
		_, quote, recorded := GenuineVolumeInWindow(cs, in.Anchor, in.OracleWindow)
		out.GenuineVolumeInWindow = &quote
		det.OracleWindowRecorded = recorded
	}

	return out, det
}
