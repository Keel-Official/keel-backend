// Why each absent supporting figure is absent, as `scan` records it.
//
// The four cases the dashboard used to show as one dash are the ones tested here:
// a pair above the DEC-019 threshold, a walk that spent its page bound, a search
// that covered its whole window and found nothing, and a holder half that is
// missing under a trade half that is not.

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/store"
)

func TestTradeNotesNameTheThresholdForAPairAboveIt(t *testing.T) {
	r := store.TradeReading{
		Scope:                   store.ScopeLastGenuineOnly,
		VolumeUnevaluatedReason: "above the DEC-019 threshold of 20000 trades in 30 days: 666311, counted on Horizon on 26 August 2026",
		LastGenuine:             &domain.TradeRef{LedgerSeq: 1, At: time.Unix(0, 0)},
	}
	excluded, volume, last := tradeNotes(r, decimal.Zero, false)
	if !strings.Contains(excluded, "above the DEC-019 threshold") || excluded != volume {
		t.Errorf("excluded %q, volume %q; want both to carry the stored threshold reason", excluded, volume)
	}
	if last != "" {
		t.Errorf("a found trade got a note: %q", last)
	}
}

func TestTradeNotesSeparateAPageBoundFromAMeasuredAbsence(t *testing.T) {
	capped := store.TradeReading{Scope: store.ScopeLastGenuineOnly, DaysWalked: 11, Pages: 400}
	_, _, last := tradeNotes(capped, decimal.Zero, false)
	if !strings.HasPrefix(last, "not checked") || !strings.Contains(last, "400 page(s)") {
		t.Errorf("page bound note %q; want it to say not checked and name the pages", last)
	}

	anchorDay := store.TradeReading{Scope: store.ScopeLastGenuineOnly, Pages: 400}
	if _, _, last := tradeNotes(anchorDay, decimal.Zero, false); !strings.Contains(last, "never reached a whole day") {
		t.Errorf("anchor day note %q; want it to say no whole day was reached", last)
	}

	measured := store.TradeReading{Scope: store.ScopeFullWindow, DaysWalked: 30, BoundReached: true}
	if _, _, last := tradeNotes(measured, decimal.Zero, false); last != "no genuine trade in the 30 whole day(s) searched" {
		t.Errorf("measured absence note %q", last)
	}

	exhausted := store.TradeReading{Scope: store.ScopeFullWindow, Exhausted: true}
	if _, _, last := tradeNotes(exhausted, decimal.Zero, false); !strings.Contains(last, "whole trade history") {
		t.Errorf("exhausted note %q", last)
	}
}

func TestTradeNotesBlameTheDenominatorWhenTheWindowWasCovered(t *testing.T) {
	d30 := decimal.RequireFromString("12.5")
	r := store.TradeReading{
		Scope:             store.ScopeFullWindow,
		GenuineBaseD30:    &d30,
		TradesExcludedPct: pct("3.1"),
		LastGenuine:       &domain.TradeRef{LedgerSeq: 1, At: time.Unix(0, 0)},
	}
	excluded, volume, _ := tradeNotes(r, decimal.Zero, false)
	if excluded != "" {
		t.Errorf("a present exclusion share got a note: %q", excluded)
	}
	if !strings.Contains(volume, "circulating supply is unknown") {
		t.Errorf("volume note %q; want the missing denominator named", volume)
	}
	if _, volume, _ := tradeNotes(r, decimal.RequireFromString("1000"), true); volume != "" {
		t.Errorf("a computable ratio got a note: %q", volume)
	}
}

func TestWithNotesExplainsAnAssetWithNeitherCache(t *testing.T) {
	sup := withNotes(nil, "no holder reading has been taken for this asset yet", "no trade reading has been taken for this pair yet")
	if sup == nil {
		t.Fatal("no struct to carry the notes")
	}
	n := sup.Notes
	if n.Holders == "" || n.TradesExcludedPct == "" || n.VolumeToSupply == "" || n.LastGenuineTrade == "" {
		t.Errorf("an absent figure has no reason: %+v", n)
	}
	// A struct carrying only notes must evaluate the flags exactly as nil does.
	if sup.HolderTop1Pct != nil || sup.CirculatingSupply != nil || sup.LastGenuineTrade != nil || sup.GenuineSearchWindow != nil {
		t.Errorf("withNotes set a figure: %+v", sup)
	}
}

func TestWithNotesClearsTheNoteOfAPresentFigureAndHidesDatabaseErrors(t *testing.T) {
	sup := &domain.SupportingMetrics{
		HolderTop1Pct: pct("40"),
		Notes:         domain.SupportingNotes{Holders: "stale"},
	}
	sup = withNotes(sup, "", tradeUnreadable+": pq: connection refused")
	if sup.Notes.Holders != "" {
		t.Errorf("a present holder figure kept a note: %q", sup.Notes.Holders)
	}
	if sup.Notes.LastGenuineTrade != tradeUnreadable {
		t.Errorf("last trade note %q; want the reason without the driver message", sup.Notes.LastGenuineTrade)
	}
}
