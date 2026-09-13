package repository

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestKBNeverUpdatesProcessWorkContent(t *testing.T) {
	src, err := os.ReadFile("as_kb.go")
	if err != nil {
		t.Fatal(err)
	}
	low := strings.ToLower(string(src))
	if strings.Contains(low, "update as_processes") {
		t.Fatal("지식 저장소가 as_processes 를 UPDATE 하면 안 된다")
	}
	if strings.Contains(low, "work_content=") || strings.Contains(low, "set work_content") {
		t.Fatal("지식 코드가 work_content 를 바꾸면 안 된다")
	}

	repo, proc := setupASSearch(t)
	as := createAS(t, repo, "c1", "A1", "예약대출기 투입 목록이 안 나옴", time.Date(2026, 8, 12, 10, 0, 0, 0, time.Local))
	if err := proc.Create(&model.ASProcess{ASID: as.ASID, Worker: "태자운", WorkContent: "서비스 재시작 후 정상", TimeSpent: 20}); err != nil {
		t.Fatal(err)
	}
	before := repo.ProcessWorkSnapshot(as.ASID)
	if before == "" {
		t.Fatal("원본 스냅샷이 비었다")
	}
	pub, err := repo.PublishKB(KBWrite{
		ASID: as.ASID, ActionText: "방화벽 예외를 등록해야 근본 해결",
		AuthorName: "양기헌",
	})
	if err != nil || pub == nil {
		t.Fatalf("publish: %v", err)
	}
	if pub.Origin != model.KBOriginRevised {
		t.Fatalf("origin=%s", pub.Origin)
	}
	if repo.ProcessWorkSnapshot(as.ASID) != before {
		t.Fatal("지식을 쓴 뒤 as_processes 가 바뀌었다")
	}
	rev, err := repo.ReviseKB(pub.KBID, KBWrite{
		ActionText: "방화벽 + 서비스 재시작", AuthorName: "양기헌", ChangeNote: "재시작도 필요",
	})
	if err != nil || rev == nil {
		t.Fatalf("revise: %v", err)
	}
	if rev.Rev != 2 || rev.PrevKBID != pub.KBID || !rev.IsCurrent {
		t.Fatalf("새 rev: %+v", rev)
	}
	old, err := repo.GetKB(pub.KBID)
	if err != nil || old.IsCurrent {
		t.Fatalf("옛 행 is_current: %+v err=%v", old, err)
	}
	if repo.ProcessWorkSnapshot(as.ASID) != before {
		t.Fatal("지식을 고친 뒤 as_processes 가 바뀌었다")
	}
}

func TestSearchKnowledgeMergesLayers(t *testing.T) {
	repo, proc := setupASSearch(t)
	orig := createAS(t, repo, "c1", "A1", "예약대출기 투입 목록이 안 나옴", time.Date(2026, 8, 1, 10, 0, 0, 0, time.Local))
	if err := proc.Create(&model.ASProcess{ASID: orig.ASID, Worker: "", WorkContent: "서비스 재시작 후 정상", TimeSpent: 20}); err != nil {
		t.Fatal(err)
	}
	plain := createAS(t, repo, "c1", "A2", "예약대출기 전원이 나감", time.Date(2026, 8, 2, 10, 0, 0, 0, time.Local))
	if err := proc.Create(&model.ASProcess{ASID: plain.ASID, Worker: "홍길동", WorkContent: "멀티탭 교체 후 정상동작함", TimeSpent: 15}); err != nil {
		t.Fatal(err)
	}
	oldRev, err := repo.PublishKB(KBWrite{ASID: orig.ASID, ActionText: "서비스 재시작 후 정상", AuthorName: "태자운"})
	if err != nil {
		t.Fatal(err)
	}
	cur, err := repo.ReviseKB(oldRev.KBID, KBWrite{ActionText: "방화벽 예외를 등록해야 근본 해결", AuthorName: "양기헌", ChangeNote: "재발"})
	if err != nil {
		t.Fatal(err)
	}

	hits, total, err := repo.SearchKnowledge(model.ASSearchFilter{Query: "예약대출기", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if total < 2 {
		t.Fatalf("total=%d", total)
	}
	var kbHit, plainHit *model.ASSearchHit
	for i := range hits {
		if hits[i].ASID == orig.ASID {
			cp := hits[i]
			kbHit = &cp
		}
		if hits[i].ASID == plain.ASID {
			cp := hits[i]
			plainHit = &cp
		}
		if hits[i].KBID == oldRev.KBID && hits[i].IsKnowledge {
			t.Fatal("is_current=0 이 검색에 나왔다")
		}
		if hits[i].AuthorName == "" {
			t.Fatalf("작성자 빈칸: %+v", hits[i])
		}
	}
	if kbHit == nil || !kbHit.IsKnowledge || kbHit.KBID != cur.KBID {
		t.Fatalf("지식 카드가 원본을 대체해야 한다: %+v", kbHit)
	}
	if kbHit.Origin != model.KBOriginRevised {
		t.Fatalf("origin=%s", kbHit.Origin)
	}
	if kbHit.OriginalAction == "" {
		t.Fatal("revised 원본 조치가 없다")
	}
	if kbHit.AuthorName != "양기헌" || kbHit.SourceName != model.KBAuthorUnknown {
		t.Fatalf("정정 작성자=%s 원본=%s", kbHit.AuthorName, kbHit.SourceName)
	}
	if plainHit == nil || plainHit.IsKnowledge {
		t.Fatal("지식 없는 건은 원본 카드")
	}
	if plainHit.AuthorName != "홍길동" {
		t.Fatalf("원본 작성자=%s", plainHit.AuthorName)
	}

	hist := repo.KBHistory(cur.KBID)
	if len(hist) != 1 || hist[0].KBID != oldRev.KBID {
		t.Fatalf("이력: %+v", hist)
	}
}

func TestKBSchemaMigrationFile(t *testing.T) {
	up, err := os.ReadFile(filepath.Join("..", "..", "migrations", "044_as_kb_entries.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(up), "as_kb_entries") {
		t.Fatal("044 up")
	}
	down, err := os.ReadFile(filepath.Join("..", "..", "migrations", "044_as_kb_entries.down.sql"))
	if err != nil || !strings.Contains(string(down), "DROP TABLE") {
		t.Fatal("044 down")
	}
}
