package repository

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestASKeywordSeedAndNoGreetingsInCandidates(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "kw.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"c1", "기관", "기관"); err != nil {
		t.Fatal(err)
	}
	repo := NewASRepo(db)
	kw := NewASKeywordRepo(db)
	as := &model.ASReceipt{
		CustomerID: "c1",
		Symptom:    "안녕하세요 감사합니다 부탁드립니다. https://www.example.kr/go 041-123-4567 무인예약이 안 되고 팝업 오류가 납니다.",
		ReceiptDatetime: time.Now(), Urgency: "normal", Priority: "normal",
	}
	if err := repo.Create(as); err != nil {
		t.Fatal(err)
	}

	all, err := kw.List(true)
	if err != nil || len(all) < 30 {
		t.Fatalf("시드 키워드 n=%d err=%v", len(all), err)
	}
	hasPopup := false
	for _, k := range all {
		if k.Keyword == "팝업" {
			hasPopup = true
		}
	}
	if !hasPopup {
		t.Fatal("초기 표의 팝업이 있어야 한다")
	}

	cands, err := kw.FrequencyCandidates(80)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cands {
		if c.Word == "안녕하세요" || c.Word == "감사합니다" || c.Word == "부탁드립니다" {
			t.Fatalf("불용어가 후보에 나왔다: %s", c.Word)
		}
		if c.Word == "kr" || c.Word == "go" || c.Word == "www" || c.Word == "041" {
			t.Fatalf("URL·숫자 조각이 후보에 나왔다: %s", c.Word)
		}
	}

	sug, err := kw.Suggest(as.Symptom)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, k := range sug {
		got[k.Keyword] = true
		if k.Keyword == "안녕하세요" || k.Keyword == "감사합니다" {
			t.Fatal("제안에도 인사말이 나오면 안 된다")
		}
	}
	if !got["팝업"] || !got["오류"] || !got["무인예약"] {
		t.Fatalf("사전 대조 후보: %+v", sug)
	}
}

func TestASKeywordLinksOnlyWhenChecked(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "kw2.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"c1", "기관", "기관"); err != nil {
		t.Fatal(err)
	}
	repo := NewASRepo(db)
	kw := NewASKeywordRepo(db)
	as := &model.ASReceipt{CustomerID: "c1", Symptom: "팝업 오류", ReceiptDatetime: time.Now()}
	if err := repo.Create(as); err != nil {
		t.Fatal(err)
	}
	if err := kw.ReplaceLinks(as.ASID, model.KWFieldSymptom, nil, []string{"KW015"}); err != nil {
		t.Fatal(err)
	}
	links, _ := kw.LinksByAS(as.ASID)
	if len(links) != 0 {
		t.Fatalf("체크 없이 붙으면 안 됨 n=%d", len(links))
	}
	if err := kw.ReplaceLinks(as.ASID, model.KWFieldSymptom, []string{"KW015", "KW012"}, []string{"KW015", "KW012"}); err != nil {
		t.Fatal(err)
	}
	links, _ = kw.LinksByAS(as.ASID)
	if len(links) != 2 {
		t.Fatalf("체크한 것만 n=%d", len(links))
	}
	if links[0].Source != model.KWSourceAuto && links[1].Source != model.KWSourceAuto {
		t.Fatalf("제안 체크는 auto: %+v", links)
	}

	hits, total, err := repo.SearchAS(model.ASSearchFilter{KeywordID: "KW015", PageSize: 20})
	if err != nil || total != 1 || len(hits) != 1 || hits[0].ASID != as.ASID {
		t.Fatalf("모아보기 total=%d n=%d err=%v", total, len(hits), err)
	}
}

func TestASKeywordRejectsStopword(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "kw3.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	kw := NewASKeywordRepo(db)
	err = kw.Create(&model.ASKeyword{Keyword: "안녕하세요", Group: model.KWGroupSymptom})
	if err == nil || !strings.Contains(err.Error(), "stopword") {
		t.Fatalf("인사말은 사전에 못 넣는다: %v", err)
	}
}
