package handler

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func Test95_142_BoardDoneBadgeAndNoBoardDanger(t *testing.T) {
	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d142.db"))
	ck := jwtCookieUser(t, me)
	day := time.Now().AddDate(0, 0, 3).Format(dateLayout)
	mustCreateBoardTask(t, repo, "완료142", day, "07:00", "08:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskComplete, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "예정142", day, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)

	week := doGetWith(t, e, "/workboard/board?view=sched&range=week&date="+day+"&auto=0&show=plan,act&mine=0", ck).Body.String()
	doneAt := strings.Index(week, "완료142")
	planAt := strings.Index(week, "예정142")
	if doneAt < 0 || planAt < 0 {
		t.Fatalf("주 보기 카드 없음 done=%d plan=%d", doneAt, planAt)
	}
	doneChunk := sliceAround(week, doneAt, 200, 1800)
	planChunk := sliceAround(week, planAt, 200, 1800)
	if !strings.Contains(doneChunk, "board-done-badge") || !strings.Contains(doneChunk, "board-done-text") {
		t.Fatalf("완료 카드 클래스 없음 %s", doneChunk)
	}
	if strings.Contains(doneChunk, "작업완료") {
		t.Fatal("완료 카드에 작업완료")
	}
	if strings.Contains(planChunk, "board-done-badge") || strings.Contains(planChunk, "작업완료") {
		t.Fatal("예정 카드에 완료 표시")
	}

	month := doGetWith(t, e, "/workboard/board?view=sched&range=month&date="+day+"&auto=0&show=plan,act&mine=0", ck).Body.String()
	mDone := strings.Index(month, "완료142")
	if mDone < 0 {
		t.Fatal("월 칩 완료 없음")
	}
	mChunk := sliceAround(month, mDone, 200, 800)
	if !strings.Contains(mChunk, "board-done-text") || !strings.Contains(mChunk, "data-month-chip") {
		t.Fatalf("월 칩 완료 클래스 없음 %s", mChunk)
	}

	css := string(mustRead(t, filepath.Join(findTemplateRoot(t), "..", "css", "input.css")))
	i := strings.Index(css, ".board-t-as")
	if i < 0 {
		t.Fatal("board-t-as 없음")
	}
	asBlock := css[i:]
	if j := strings.Index(asBlock[1:], ".board-"); j >= 0 {
		asBlock = asBlock[:j+1]
	}
	if strings.Contains(asBlock, "217 83 43") || strings.Contains(asBlock, "217,83,43") {
		t.Fatal("board-t-as 옛 빨강")
	}
	if !strings.Contains(asBlock, "161 98 7") {
		t.Fatal("board-t-as 황갈색 없음")
	}

	root := filepath.Join(findTemplateRoot(t), "workboard")
	var leaked []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		base := filepath.Base(path)
		if !strings.HasPrefix(base, "board") && !strings.HasPrefix(base, "_board") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), "text-danger") {
			leaked = append(leaked, base)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(leaked) > 0 {
		t.Fatalf("보드 템플릿 text-danger: %s", strings.Join(leaked, ","))
	}
}

func sliceAround(s string, at, before, after int) string {
	if at < 0 {
		return ""
	}
	a := at - before
	if a < 0 {
		a = 0
	}
	b := at + after
	if b > len(s) {
		b = len(s)
	}
	return s[a:b]
}
