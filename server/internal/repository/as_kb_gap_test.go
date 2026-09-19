package repository

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestRecordKBGapIncrementsSameQuery(t *testing.T) {
	repo, _ := setupASSearch(t)
	g1, err := repo.RecordKBGap("  RFID  태그 인식 불량 ", "양기헌", "")
	if err != nil || g1 == nil || g1.SearchCount != 1 || g1.HitCount != 0 {
		t.Fatalf("first %+v err=%v", g1, err)
	}
	g2, err := repo.RecordKBGap("rfid 태그 인식 불량", "홍길동", "AS1")
	if err != nil || g2 == nil || g2.GapID != g1.GapID || g2.SearchCount != 2 {
		t.Fatalf("second %+v err=%v", g2, err)
	}
	open, err := repo.ListOpenKBGaps()
	if err != nil || len(open) != 1 || open[0].SearchCount != 2 {
		t.Fatalf("open %+v err=%v", open, err)
	}
	g3, err := repo.RecordKBGap("게시판 첨부 오류", "양기헌", "")
	if err != nil || g3 == nil {
		t.Fatal(err)
	}
	open, _ = repo.ListOpenKBGaps()
	if len(open) != 2 || open[0].QueryNorm != g2.QueryNorm {
		t.Fatalf("횟수 순이 아니다: %+v", open)
	}
	kb, err := repo.PublishKB(KBWrite{ActionText: "리더기 재시작 후 태그를 다시 등록", Symptom: "RFID 태그 인식 불량", AuthorName: "양기헌"})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ResolveKBGap(g1.GapID, kb.KBID); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.GetKBGap(g1.GapID)
	if got.ResolvedKBID != kb.KBID || strings.TrimSpace(got.ResolvedAt) == "" {
		t.Fatalf("resolve %+v", got)
	}
	open, _ = repo.ListOpenKBGaps()
	if len(open) != 1 || open[0].GapID != g3.GapID {
		t.Fatalf("남은 갭 %+v", open)
	}
}

func TestMissingActionGroupedByKeyword(t *testing.T) {
	repo, _ := setupASSearch(t)
	createAS(t, repo, "c1", "A1", "로그인 오류가 반복됨", time.Date(2026, 9, 1, 10, 0, 0, 0, time.Local))
	createAS(t, repo, "c1", "A1", "로그인 실패 메시지가 뜸", time.Date(2026, 9, 2, 10, 0, 0, 0, time.Local))
	createAS(t, repo, "c1", "A2", "출력 안 됨", time.Date(2026, 9, 3, 10, 0, 0, 0, time.Local))
	createAS(t, repo, "c1", "A2", "전혀다른고유증상XYZ", time.Date(2026, 9, 4, 10, 0, 0, 0, time.Local))
	rows, err := repo.ListMissingActionSymptoms()
	if err != nil || len(rows) < 4 {
		t.Fatalf("n=%d err=%v", len(rows), err)
	}
	dict, err := NewASKeywordRepo(repo.db).List(true)
	if err != nil {
		t.Fatal(err)
	}
	groups := GroupMissingActionByKeyword(dict, rows)
	if len(groups) < 2 {
		t.Fatalf("묶음 %+v", groups)
	}
	var login, printN, other int
	sum := 0
	for _, g := range groups {
		sum += g.Count
		switch {
		case g.Keyword == "로그인":
			login = g.Count
		case g.Keyword == "출력":
			printN = g.Count
		case g.Other:
			other = g.Count
		}
	}
	if login < 2 || printN < 1 || other < 1 {
		t.Fatalf("로그인=%d 출력=%d 그외=%d groups=%+v", login, printN, other, groups)
	}
	if sum != repo.MissingActionCount() {
		t.Fatalf("합=%d missing=%d", sum, repo.MissingActionCount())
	}
}

func TestGapGroupsUseKeywordLinksAndEmptyWhenIndexMissing(t *testing.T) {
	repo, _ := setupASSearch(t)
	createAS(t, repo, "c1", "A2", "홈페이지 접속이 안 됩니다", time.Date(2026, 7, 1, 10, 0, 0, 0, time.Local))
	createAS(t, repo, "c1", "A2", "전혀다른고유증상XYZ", time.Date(2026, 7, 2, 10, 0, 0, 0, time.Local))
	if _, err := repo.RebuildKeywordLinks(nil); err != nil {
		t.Fatal(err)
	}
	groups, err := repo.ListGapKeywordGroups()
	if err != nil {
		t.Fatal(err)
	}
	var home, other int
	for _, g := range groups {
		if g.KeywordID == "KW001" {
			home = g.Count
		}
		if g.Other {
			other = g.Count
		}
	}
	if home < 1 || other < 1 {
		t.Fatalf("groups home=%d other=%d %+v", home, other, groups)
	}
	_, total, err := repo.ListGapReceipts("KW001", false, "", "", "", 0, 20)
	if err != nil || total != home {
		t.Fatalf("목록 total=%d 묶음=%d err=%v", total, home, err)
	}
	if _, err := repo.db.Exec(`DELETE FROM as_keyword_links`); err != nil {
		t.Fatal(err)
	}
	empty, err := repo.ListGapKeywordGroups()
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range empty {
		if !g.Other && g.Count > 0 {
			t.Fatalf("색인 없이 키워드 묶음이 있다 %+v", empty)
		}
	}
	if repo.MissingActionCount() < 2 {
		t.Fatal("총계는 남아야 한다")
	}
}

func TestGapKindNoneAndShortSplit(t *testing.T) {
	repo, proc := setupASSearch(t)
	none := createAS(t, repo, "c1", "A2", "홈페이지 접속이 안 됩니다", time.Date(2026, 7, 1, 10, 0, 0, 0, time.Local))
	short := createAS(t, repo, "c1", "A2", "홈페이지 로그인이 안 됨", time.Date(2026, 7, 2, 10, 0, 0, 0, time.Local))
	emptyProc := createAS(t, repo, "c1", "A2", "홈페이지 배너가 안 보임", time.Date(2026, 7, 3, 10, 0, 0, 0, time.Local))
	long := createAS(t, repo, "c1", "A2", "홈페이지 팝업이 안 닫힘", time.Date(2026, 7, 4, 10, 0, 0, 0, time.Local))
	if err := proc.Create(&model.ASProcess{ASID: short.ASID, WorkContent: "완료", TimeSpent: 10}); err != nil {
		t.Fatal(err)
	}
	if err := proc.Create(&model.ASProcess{ASID: emptyProc.ASID, WorkContent: "   ", TimeSpent: 10}); err != nil {
		t.Fatal(err)
	}
	if err := proc.Create(&model.ASProcess{ASID: long.ASID, WorkContent: "1234567890", TimeSpent: 10}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RebuildKeywordLinks(nil); err != nil {
		t.Fatal(err)
	}

	rows, err := repo.ListMissingActionSymptoms()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]model.ASMissingAction{}
	for _, row := range rows {
		byID[row.ASID] = row
	}
	if _, ok := byID[long.ASID]; ok {
		t.Fatal("10자 이상 조치는 목록에 있으면 안 된다")
	}
	if byID[none.ASID].Kind != "none" {
		t.Fatalf("기록 없음 %+v", byID[none.ASID])
	}
	if byID[short.ASID].Kind != "short" || byID[short.ASID].Action != "완료" {
		t.Fatalf("10자 미만 %+v", byID[short.ASID])
	}
	if byID[emptyProc.ASID].Kind != "none" {
		t.Fatalf("빈 조치 행은 none 여야 한다 %+v", byID[emptyProc.ASID])
	}

	groups, err := repo.ListGapKeywordGroups()
	if err != nil {
		t.Fatal(err)
	}
	var home *model.ASGapSymptomGroup
	for i := range groups {
		if groups[i].KeywordID == "KW001" {
			home = &groups[i]
			break
		}
	}
	if home == nil || home.Count != 3 || home.NoneCount != 2 || home.ShortCount != 1 {
		t.Fatalf("묶음 숫자 %+v", home)
	}

	list, total, err := repo.ListGapReceipts("KW001", false, "", "", "", 0, 20)
	if err != nil || total != 3 {
		t.Fatalf("목록 total=%d err=%v", total, err)
	}
	got := map[string]GapReceiptRow{}
	for _, r := range list {
		got[r.ASID] = r
	}
	if got[none.ASID].Kind != "none" || got[none.ASID].ShortAction != "" {
		t.Fatalf("none 행 %+v", got[none.ASID])
	}
	if got[short.ASID].Kind != "short" || got[short.ASID].ShortAction != "완료" {
		t.Fatalf("short 행 %+v", got[short.ASID])
	}
}

func TestKBGapSchemaMigrationFile(t *testing.T) {
	up, err := os.ReadFile(filepath.Join("..", "..", "migrations", "045_as_kb_gaps.sql"))
	if err != nil || !strings.Contains(string(up), "as_kb_gaps") {
		t.Fatal("045 up")
	}
	down, err := os.ReadFile(filepath.Join("..", "..", "migrations", "045_as_kb_gaps.down.sql"))
	if err != nil || !strings.Contains(string(down), "DROP TABLE") {
		t.Fatal("045 down")
	}
}
