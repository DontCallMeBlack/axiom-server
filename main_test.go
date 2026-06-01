package main

import (
	"testing"
	"time"
)

func TestFinalizeDueAuctionsTransactional(t *testing.T) {
	// Prepare a fresh state
	mockAuctions = map[string]*Auction{}
	mockEscrow = map[string]int{}
	users = map[string]*User{}
	ledgerRecords = []LedgerRecord{}

	// Create auction that already ended
	auc := &Auction{
		ID:               "test-auc",
		ItemName:         "Test Item",
		ImageURL:         "/loot-images/test.jpg",
		HolderName:       "Chief",
		ClassRestriction: "all",
		EndTime:          time.Now().UTC().Add(-1 * time.Minute),
		HighestBid:       100,
		HighestBidder:    "BidderOne",
		IsActive:         true,
	}
	mockAuctions[auc.ID] = auc

	// Set escrow for winner
	mockEscrow["BidderOne"] = 100

	// Call finalizer
	finalizeDueAuctions(nil)

	// Check auction marked inactive
	if mockAuctions["test-auc"].IsActive {
		t.Fatalf("expected auction to be inactive after finalizer")
	}

	// Check escrow reduced to zero
	if mockEscrow["BidderOne"] != 0 {
		t.Fatalf("expected escrow to be 0 after settlement, got %d", mockEscrow["BidderOne"])
	}

	// Check ledger record created
	if len(ledgerRecords) == 0 {
		t.Fatalf("expected ledger record to be added")
	}
	if ledgerRecords[0].Amount != -100 {
		t.Fatalf("expected ledger amount -100, got %d", ledgerRecords[0].Amount)
	}
}

func TestPlaceBidAndFinalizeRace(t *testing.T) {
	// Reset
	mockAuctions = map[string]*Auction{}
	mockEscrow = map[string]int{}
	ledgerRecords = []LedgerRecord{}

	auc := &Auction{
		ID:               "race-auc",
		ItemName:         "Race Item",
		ImageURL:         "/loot-images/race.jpg",
		HolderName:       "Chief",
		ClassRestriction: "all",
		EndTime:          time.Now().UTC().Add(500 * time.Millisecond),
		HighestBid:       50,
		HighestBidder:    "Alice",
		IsActive:         true,
	}
	mockAuctions[auc.ID] = auc
	mockEscrow["Alice"] = 50
	mockEscrow["Bob"] = 0

	// Simulate Bob placing a higher bid concurrently
	go func() {
		// wait a bit and place bid
		time.Sleep(100 * time.Millisecond)
		// Build request struct and call handler logic by emulating placement
		// We'll directly manipulate as handlePlaceBid HTTP handler requires request objects; instead, perform same logic under stateMu
		stateMu.Lock()
		auction := mockAuctions["race-auc"]
		if auction.IsActive {
			// outbids Alice
			if auction.HighestBidder != "" && auction.HighestBidder != "Bob" {
				mockEscrow[auction.HighestBidder] -= auction.HighestBid
				if mockEscrow[auction.HighestBidder] < 0 {
					mockEscrow[auction.HighestBidder] = 0
				}
			}
			auction.HighestBid = 120
			auction.HighestBidder = "Bob"
			mockEscrow["Bob"] += 120
		}
		stateMu.Unlock()
	}()

	// Wait for auction to end
	time.Sleep(700 * time.Millisecond)
	finalizeDueAuctions(nil)

	// Auction should be inactive
	if mockAuctions["race-auc"].IsActive {
		t.Fatalf("expected auction to be inactive after finalizer")
	}

	// Bob should have paid 120 from escrow
	if mockEscrow["Bob"] != 0 {
		t.Fatalf("expected Bob escrow 0 after settlement, got %d", mockEscrow["Bob"])
	}

	// Alice's escrow should remain 0
	if mockEscrow["Alice"] != 0 {
		t.Fatalf("expected Alice escrow 0 after settlement, got %d", mockEscrow["Alice"])
	}

	// Ledger should include Bob's spend
	found := false
	for _, r := range ledgerRecords {
		if r.CharName == "Bob" && r.Amount == -120 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected ledger to include Bob's -120 spend")
	}
}
