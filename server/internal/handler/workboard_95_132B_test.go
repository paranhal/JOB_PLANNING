package handler

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var (
	htmlOpenTagRe     = regexp.MustCompile(`(?s)<[a-zA-Z][^>]*>`)
	htmlClassAttrRe   = regexp.MustCompile(`(?i)\bclass="([^"]*)"`)
	htmlXShowRe       = regexp.MustCompile(`(?i)\bx-show=`)
	htmlHiddenTokenRe = regexp.MustCompile(`(?:^|\s)hidden(?:\s|$)`)
)

func assertNewWorkDialogMarkup(t *testing.T, body, where string) {
	t.Helper()
	idx := strings.Index(body, `data-new-work-dialog`)
	if idx < 0 {
		t.Fatalf("%s: 업무 등록 다이얼로그 없음", where)
	}
	start := strings.LastIndex(body[:idx], "<div")
	end := strings.Index(body[idx:], ">")
	if start < 0 || end < 0 {
		t.Fatalf("%s: 다이얼로그 태그를 못 자름", where)
	}
	tag := body[start : idx+end+1]
	if !strings.Contains(tag, `x-show="newWorkOpen"`) {
		t.Fatalf("%s: x-show 없음: %s", where, tag)
	}
	if !strings.Contains(tag, "x-cloak") {
		t.Fatalf("%s: x-cloak 없음: %s", where, tag)
	}
	if !strings.Contains(tag, `@click.self="closeNewWork()"`) {
		t.Fatalf("%s: 바깥 클릭 없음: %s", where, tag)
	}
	if m := htmlClassAttrRe.FindStringSubmatch(tag); len(m) > 1 && htmlHiddenTokenRe.MatchString(m[1]) {
		t.Fatalf("%s: x-show와 hidden 동시: %s", where, tag)
	}
}

func Test95_132B_T1_ScheduleDayModalMarkup(t *testing.T) {
	e, _, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "132b1.db"))
	ck := jwtCookieUser(t, me)
	today := time.Now().Format(dateLayout)
	body := doGetWith(t, e, "/workboard/board?view=sched&range=day&date="+today+"&auto=0", ck).Body.String()
	assertNewWorkDialogMarkup(t, body, "일정표 일")
	if !strings.Contains(body, `data-new-work`) || !strings.Contains(body, "+ 업무 등록") {
		t.Fatal("일정표 일 등록 버튼 없음")
	}
}

func Test95_132B_T2_KanbanModalMarkup(t *testing.T) {
	e, _, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "132b2.db"))
	ck := jwtCookieUser(t, me)
	today := time.Now().Format(dateLayout)
	body := doGetWith(t, e, "/workboard/board?view=kanban&date="+today+"&auto=0", ck).Body.String()
	assertNewWorkDialogMarkup(t, body, "칸반")
}

func Test95_132B_T3_WeekAndLegacyKanbanModalMarkup(t *testing.T) {
	e, _, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "132b3.db"))
	ck := jwtCookieUser(t, me)
	today := time.Now().Format(dateLayout)
	week := doGetWith(t, e, "/workboard/board?view=week&date="+today+"&auto=0", ck).Body.String()
	assertNewWorkDialogMarkup(t, week, "일정표 주")
	legacy := doGetWith(t, e, "/workboard/kanban", ck).Body.String()
	if strings.Contains(legacy, `data-new-work-dialog`) {
		assertNewWorkDialogMarkup(t, legacy, "예전 칸반")
	}
}

func Test95_132B_T4_TemplateForbidsXShowHidden(t *testing.T) {
	root := findTemplateRoot(t)
	var hits []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		for _, tag := range htmlOpenTagRe.FindAllString(string(b), -1) {
			if !htmlXShowRe.MatchString(tag) {
				continue
			}
			m := htmlClassAttrRe.FindStringSubmatch(tag)
			if len(m) < 2 || !htmlHiddenTokenRe.MatchString(m[1]) {
				continue
			}
			hits = append(hits, filepath.ToSlash(rel)+": "+strings.TrimSpace(tag))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) > 0 {
		t.Fatalf("x-show와 class hidden 동시 사용:\n%s", strings.Join(hits, "\n"))
	}
}

func Test95_132B_T5_SummaryHasStartLinkNotRegister(t *testing.T) {
	e, _, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "132b5.db"))
	ck := jwtCookieUser(t, me)
	today := time.Now().Format(dateLayout)
	body := doGetWith(t, e, "/workboard/board?view=summary&date="+today+"&auto=0", ck).Body.String()
	if !strings.Contains(body, "+ 업무 시작") || strings.Contains(body, "+ 업무 등록") {
		t.Fatal("요약은 업무 시작 링크만")
	}
	assertNewWorkDialogMarkup(t, body, "요약(끼워넣기용 모달)")
}
