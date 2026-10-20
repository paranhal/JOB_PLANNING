package model

import (
	"strings"
	"testing"
)

func TestSalesActTypeStageTable(t *testing.T) {
	types := []string{
		SalesActTypeResearch, SalesActTypeMaterial, SalesActTypeQuote,
		SalesActTypeRequirement, SalesActTypeRFPReceived, SalesActTypeProposal, SalesActTypeOther,
	}
	stages := []string{SalesStage4Discover, SalesStage4Propose, SalesStage4Bid, SalesStage4Closed}
	for _, typ := range types {
		st := SalesActTypeStage(typ)
		switch typ {
		case SalesActTypeResearch:
			if st != SalesStage4Discover {
				t.Fatalf("%s → %s", typ, st)
			}
		case SalesActTypeMaterial, SalesActTypeQuote, SalesActTypeRequirement, SalesActTypeRFPReceived:
			if st != SalesStage4Propose {
				t.Fatalf("%s → %s", typ, st)
			}
		case SalesActTypeProposal:
			if st != SalesStage4Bid {
				t.Fatalf("%s → %s", typ, st)
			}
		default:
			if st != "" {
				t.Fatalf("%s 기타가 아님 %s", typ, st)
			}
		}
		for _, stage := range stages {
			_ = SalesActTypesForStage(stage)
		}
	}
	if SalesActTypeStage("unknown") != "" {
		t.Fatal("알 수 없는 값은 기타")
	}
	if got := SalesActTypesForStage(SalesStage4Discover); len(got) != 1 || got[0] != SalesActTypeResearch {
		t.Fatalf("발굴 %+v", got)
	}
	if got := SalesActTypesForStage("nope"); len(got) != 1 || got[0] != SalesActTypeOther {
		t.Fatalf("기타 %+v", got)
	}
}

func TestSalesActStageMismatchWarning(t *testing.T) {
	w := SalesActStageMismatchWarning(SalesStage4Propose, SalesActTypeResearch)
	if w == "" || !strings.Contains(w, "제안") || !strings.Contains(w, "정보 수집") || !strings.Contains(w, "발굴") {
		t.Fatalf("경고=%q", w)
	}
	if SalesActStageMismatchWarning(SalesStage4Discover, SalesActTypeResearch) != "" {
		t.Fatal("같은 단계는 경고 없음")
	}
}
