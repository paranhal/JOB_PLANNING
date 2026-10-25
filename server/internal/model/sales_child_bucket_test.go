package model

import "testing"

func TestSalesChildBucketAndDecided(t *testing.T) {
	cases := []struct {
		p    SalesProject
		buck string
		dec  bool
	}{
		{SalesProject{Stage: SalesStage4Discover}, SalesChildProgress, false},
		{SalesProject{Stage: SalesStage4Propose}, SalesChildProgress, false},
		{SalesProject{Stage: SalesStage4Bid, BidStatus: SalesBidPending}, SalesChildProgress, false},
		{SalesProject{Stage: SalesStage4Bid, BidStatus: SalesBidWon}, SalesChildWon, true},
		{SalesProject{Stage: SalesStage4Bid, BidStatus: SalesBidNegotiating}, SalesChildWon, true},
		{SalesProject{Stage: SalesStage4Closed, CloseReason: SalesCloseContracted}, SalesChildContract, true},
		{SalesProject{Stage: SalesStage4Closed, CloseReason: SalesCloseLost}, SalesChildLost, true},
		{SalesProject{Stage: SalesStage4Closed, CloseReason: SalesCloseDropped}, SalesChildDropped, true},
		{SalesProject{Stage: SalesStage4Discover, Status: SalesStatusDormant}, SalesChildProgress, false},
	}
	for i, tc := range cases {
		if got := SalesChildBucket(tc.p); got != tc.buck {
			t.Errorf("%d bucket %s want %s", i, got, tc.buck)
		}
		if got := SalesChildDecided(tc.p); got != tc.dec {
			t.Errorf("%d decided %v want %v", i, got, tc.dec)
		}
	}
}
