package repository

import (
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestListGapReceiptsPagingFiltersAndKBExclude(t *testing.T) {
	repo, proc := setupASSearch(t)
	homeC1 := make([]*model.ASReceipt, 0, 25)
	for i := 0; i < 22; i++ {
		as := createAS(t, repo, "c1", "A1", "홈페이지 접속이 안 됩니다", time.Date(2026, 7, 1+i, 10, 0, 0, 0, time.Local))
		homeC1 = append(homeC1, as)
	}
	c2a := createAS(t, repo, "c2", "B1", "홈페이지 로그인이 안 됨", time.Date(2026, 8, 1, 10, 0, 0, 0, time.Local))
	c2b := createAS(t, repo, "c2", "B1", "홈페이지 배너가 안 보임", time.Date(2026, 8, 2, 10, 0, 0, 0, time.Local))
	other := createAS(t, repo, "c1", "A2", "전혀다른고유증상XYZ", time.Date(2026, 9, 1, 10, 0, 0, 0, time.Local))
	if err := proc.Create(&model.ASProcess{ASID: homeC1[0].ASID, WorkContent: "짧음", TimeSpent: 30}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RebuildKeywordLinks(nil); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	page1, total, err := repo.ListGapReceipts("KW001", false, "", "receipt_date", "desc", 0, 20)
	dur := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if dur > 200*time.Millisecond {
		t.Fatalf("응답 %s > 200ms", dur)
	}
	if total != 24 {
		t.Fatalf("홈페이지 총계=%d want 24", total)
	}
	if len(page1) != 20 {
		t.Fatalf("1페이지 n=%d want 20", len(page1))
	}
	page2, total2, err := repo.ListGapReceipts("KW001", false, "", "receipt_date", "desc", 20, 20)
	if err != nil || total2 != 24 || len(page2) != 4 {
		t.Fatalf("offset=20 n=%d total=%d err=%v", len(page2), total2, err)
	}
	seen := map[string]bool{}
	for _, r := range page1 {
		seen[r.ASID] = true
	}
	for _, r := range page2 {
		if seen[r.ASID] {
			t.Fatalf("1페이지와 겹친다 %s", r.ASID)
		}
	}
	var shortN int
	for _, r := range append(append([]GapReceiptRow{}, page1...), page2...) {
		if r.ASID == homeC1[0].ASID {
			if r.Kind != "short" || r.ShortAction != "짧음" {
				t.Fatalf("10자 미만 행 %+v", r)
			}
			shortN++
		}
	}
	if shortN != 1 {
		t.Fatal("짧은 조치 행이 없다")
	}

	if _, err := repo.PublishKB(KBWrite{ASID: c2a.ASID, ActionText: "WAS 세션 시간을 늘린 뒤 다시 로그인하게 안내", AuthorName: "양기헌"}); err != nil {
		t.Fatal(err)
	}
	_, after, err := repo.ListGapReceipts("KW001", false, "", "", "", 0, 20)
	if err != nil || after != 23 {
		t.Fatalf("지식 있는 건 제외 total=%d err=%v", after, err)
	}
	pageKB, _, _ := repo.ListGapReceipts("KW001", false, "", "", "", 0, 20)
	for _, r := range pageKB {
		if r.ASID == c2a.ASID {
			t.Fatal("as_kb_entries 있는 건이 목록에 있다")
		}
	}

	siteRows, siteTotal, err := repo.ListGapReceipts("KW001", false, "c2", "", "", 0, 20)
	if err != nil || siteTotal != 1 || len(siteRows) != 1 || siteRows[0].ASID != c2b.ASID {
		t.Fatalf("사이트 필터 n=%d total=%d err=%v", len(siteRows), siteTotal, err)
	}

	others, otherTotal, err := repo.ListGapReceipts("", true, "", "", "", 0, 20)
	if err != nil || otherTotal < 1 {
		t.Fatalf("그 외 total=%d err=%v", otherTotal, err)
	}
	foundOther := false
	for _, r := range others {
		if r.ASID == other.ASID {
			foundOther = true
		}
		if r.ASID == homeC1[1].ASID {
			t.Fatal("링크 있는 건이 그 외에 있다")
		}
	}
	if !foundOther {
		t.Fatal("그 외에 링크 없는 건이 없다")
	}
}
