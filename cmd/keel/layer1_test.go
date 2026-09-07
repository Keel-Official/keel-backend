package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHandFiguresExist is the ordering rule's only enforceable half.
//
// It cannot prove a figure was worked out by hand, and layer1.go says so in its
// header. What it proves is that the file was on disk before this command would
// print anything, so the four states that are NOT that are refused: missing,
// empty, a directory, and the happy one.
func TestHandFiguresExist(t *testing.T) {
	dir := t.TempDir()

	missing := filepath.Join(dir, "not-written-yet.md")
	if err := handFiguresExist(missing); err == nil {
		t.Fatal("a missing hand recomputation was accepted; the ordering rule is not enforced")
	} else if !strings.Contains(err.Error(), "Write your hand figures first") {
		t.Errorf("the refusal does not say what to do about it: %v", err)
	}

	empty := filepath.Join(dir, "empty.md")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := handFiguresExist(empty); err == nil {
		t.Error("an empty file was accepted, which is a file created to satisfy the check")
	}

	if err := handFiguresExist(dir); err == nil {
		t.Error("a directory was accepted; the check must name the recomputation file itself")
	}

	written := filepath.Join(dir, "USTRY-64310828.md")
	if err := os.WriteFile(written, []byte("P0 = ...\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := handFiguresExist(written); err != nil {
		t.Errorf("a written recomputation was refused: %v", err)
	}
}

// TestSnapshotFromRecordingCarriesThePool guards the reason ParsePools was
// exported. Before it, anything reading a recording got a book and no pool and
// reported a combined figure that was silently order book only.
func TestSnapshotFromRecordingCarriesThePool(t *testing.T) {
	const rec = "../../recordings/samples/AQUA.GBNZILST-USDC.GA5ZSEJY/2026-08-26/64129587.json.gz"
	if _, err := os.Stat(rec); err != nil {
		t.Skipf("sample recording not present: %v", err)
	}

	snap, err := snapshotFromRecording(rec, "quote")
	if err != nil {
		t.Fatalf("reading a committed sample recording failed: %v", err)
	}
	if snap.LedgerSeq == 0 {
		t.Error("the snapshot carries no ledger sequence, so nothing it says is anchored")
	}
	if len(snap.Book.Bids) == 0 && len(snap.Book.Asks) == 0 {
		t.Error("the snapshot carries no book at all")
	}
	if snap.Source != "horizon" {
		t.Errorf("source = %q, want horizon: these are recorded Horizon bytes, not a reconstruction", snap.Source)
	}
}
