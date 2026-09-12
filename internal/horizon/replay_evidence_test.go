package horizon

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"
)

// This is an offline check of actual operation results, not a golden financial
// fixture. The source JSON and its hash are recorded in the evidence directory.
func TestFebruary22SixOffersAreCancelledAtLedger61340261(t *testing.T) {
	var ops []offerOperation
	for _, name := range []string{"bid-owner-before-sample-operations.json", "transition-20-operations.json"} {
		body, err := os.ReadFile("../../docs/evidences/track-b-2026-09-12/" + name)
		if err != nil {
			t.Fatal(err)
		}
		var page struct {
			Embedded struct {
				Records []operationRecord `json:"records"`
			} `json:"_embedded"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			t.Fatal(err)
		}
		for _, record := range page.Embedded.Records {
			if !record.TransactionSuccessful || !record.touchesPair(refOf(testUSTRY), refOf(testUSDC)) {
				continue
			}
			toid, err := strconv.ParseInt(record.PagingToken, 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			if TOIDLedger(toid) > 61340262 {
				continue
			}
			result, err := ParseManageOfferResult(record.Transaction.ResultXDR, TOIDOperationIndex(toid))
			if err != nil {
				t.Fatal(err)
			}
			submitted, err := strconv.ParseInt(record.OfferID, 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			ops = append(ops, offerOperation{TOID: toid, Ledger: TOIDLedger(toid), SubmittedOfferID: submitted, Result: result})
		}
	}
	before := replayOffers(ops, nil, 61340172)
	after := replayOffers(ops, nil, 61340262)
	for _, id := range []int64{1824767559, 1824767560, 1824767561, 1824767562, 1824767563, 1824767564} {
		deletedAt61340261 := false
		for _, op := range ops {
			if op.Ledger == 61340261 && op.SubmittedOfferID == id && op.Result.Effect == offerDeleted {
				deletedAt61340261 = true
				break
			}
		}
		if !deletedAt61340261 {
			t.Fatalf("offer %d has no deletion result at ledger 61340261", id)
		}
		o, ok := before[id]
		if !ok {
			t.Fatalf("offer %d was not created before the sample", id)
		}
		if _, ok := after[id]; ok {
			t.Fatalf("canceled offer %d survives at control ledger", id)
		}
		t.Logf("offer %d posted selling=%s amount=%s price=%d/%d; deleted at 61340261", id, o.Selling.AssetCode, o.Amount, o.PriceN, o.PriceD)
	}
	// Trades are deliberately omitted: posted sizes are not claimed as the
	// remaining balances. Cancellation is independent of intervening fills.
}
