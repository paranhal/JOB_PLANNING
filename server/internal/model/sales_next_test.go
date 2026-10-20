package model

import "testing"

func TestSalesNextActionTable(t *testing.T) {
	id := "S-1"
	cases := []struct {
		name, stage, bid, close, status, wp string
		write                               bool
		kind, label                         string
		enabled, lost                       bool
	}{
		{"발굴", SalesStage4Discover, "", "", SalesStatusActive, "", true, SalesActionWin, "수주", true, false},
		{"제안", SalesStage4Propose, "", "", SalesStatusActive, "", true, SalesActionWin, "수주", true, false},
		{"입찰대기", SalesStage4Bid, SalesBidPending, "", SalesStatusActive, "", true, SalesActionWin, "수주", true, true},
		{"입찰빈칸", SalesStage4Bid, "", "", SalesStatusActive, "", true, SalesActionWin, "수주", true, true},
		{"수주", SalesStage4Bid, SalesBidWon, "", SalesStatusActive, "", true, SalesActionConvert, "계약으로 전환", true, false},
		{"협상", SalesStage4Bid, SalesBidNegotiating, "", SalesStatusActive, "", true, SalesActionConvert, "계약으로 전환", true, false},
		{"계약미전환", SalesStage4Closed, "", SalesCloseContracted, SalesStatusContracted, "", true, SalesActionPromote, "계약으로 전환", true, false},
		{"전환됨", SalesStage4Closed, "", SalesCloseContracted, SalesStatusPromoted, "WP-1", true, SalesActionView, "사업 보기", true, false},
		{"실주", SalesStage4Closed, "", SalesCloseLost, SalesStatusLost, "", true, "", "", false, false},
		{"중단", SalesStage4Closed, "", SalesCloseDropped, SalesStatusDropped, "", true, "", "", false, false},
		{"휴면", SalesStage4Discover, "", "", SalesStatusDormant, "", true, "", "", false, false},
		{"발굴읽기", SalesStage4Discover, "", "", SalesStatusActive, "", false, SalesActionWin, "수주", false, false},
		{"수주읽기", SalesStage4Bid, SalesBidWon, "", SalesStatusActive, "", false, SalesActionConvert, "계약으로 전환", false, false},
		{"미전환읽기", SalesStage4Closed, "", SalesCloseContracted, SalesStatusContracted, "", false, SalesActionPromote, "계약으로 전환", false, false},
	}
	for _, tc := range cases {
		p := &SalesProject{
			SalesID: id, Stage: tc.stage, BidStatus: tc.bid, CloseReason: tc.close, Status: tc.status,
		}
		got := SalesNextActionOf(p, tc.wp, tc.write)
		if got.Kind != tc.kind || got.Label != tc.label || got.Enabled != tc.enabled || got.ShowLost != tc.lost {
			t.Fatalf("%s: got kind=%s label=%q enabled=%v lost=%v", tc.name, got.Kind, got.Label, got.Enabled, got.ShowLost)
		}
		if tc.kind == SalesActionPromote && tc.write && got.Href != "/sales/"+id+"/promote" {
			t.Fatalf("%s href=%s", tc.name, got.Href)
		}
		if tc.kind == SalesActionView && tc.write && got.Href != "/projects/"+tc.wp {
			t.Fatalf("%s href=%s", tc.name, got.Href)
		}
		if !tc.write && (got.Href != "" || got.ShowLost) {
			t.Fatalf("%s 읽기인데 href/실주가 남음 %+v", tc.name, got)
		}
	}
}
