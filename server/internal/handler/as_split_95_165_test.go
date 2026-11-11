package handler

import (
	"net/http"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func Test95_165D_SplitCardKeepsPaletteKey(t *testing.T) {
	pal := model.WBCard{Kind: model.WBSourceAS, RefID: "AS-1", Category: model.WBSourceAS, SourceNumber: "R1"}
	primary := model.WBCard{Kind: "task", RefID: "WT-1", Category: model.WBSourceAS, SourceHref: "/as/AS-1/action", TaskID: "WT-1", SourceRole: ""}
	split := model.WBCard{Kind: "task", RefID: "WT-2", Category: model.WBSourceAS, SourceHref: "/as/AS-1/action", TaskID: "WT-2", SourceRole: "asset:A2"}
	if cardDedupKey(pal) != "as:AS-1" || cardDedupKey(primary) != "as:AS-1" {
		t.Fatalf("원 업무 키 pal=%s primary=%s", cardDedupKey(pal), cardDedupKey(primary))
	}
	if cardDedupKey(split) != "as:AS-1|asset:A2" {
		t.Fatalf("분리 키 %s", cardDedupKey(split))
	}
	if taskDedupKey(model.WorkTask{SourceType: model.WBSourceAS, SourceID: "AS-1"}) != "as:AS-1" {
		t.Fatal("원 업무 task 키")
	}
	if taskDedupKey(model.WorkTask{SourceType: model.WBSourceAS, SourceID: "AS-1", SourceRole: "asset:A2"}) != "as:AS-1|asset:A2" {
		t.Fatal("분리 task 키")
	}
	e, _, _, _ := new45DServer(t)
	page := doGet(t, e, "/as/new")
	if page.Code != http.StatusOK || strings.Contains(page.Body.String(), "split_mode") {
		t.Fatalf("신규 접수 %d", page.Code)
	}
}
