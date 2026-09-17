package repository

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
