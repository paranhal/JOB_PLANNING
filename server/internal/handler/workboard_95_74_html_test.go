package handler

import (
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

var (
	leakedVisibleRe = regexp.MustCompile(`(?i)data-require-assignee|@submit\s*=`)
	openOccTextRe   = regexp.MustCompile(`\bopenOcc\b`)
	srcAttrLeakRe   = regexp.MustCompile(`(?m)">[ \t]*\n[ \t]*(?:\{\{if[^}]+\}\})?(?:data-require-assignee=|@[A-Za-z][\w:.-]*=)`)
)

func visibleLeakedAttrSnippets(body string) []string {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return []string{"parse: " + err.Error()}
	}
	var out []string
	var walk func(*html.Node, bool)
	walk = func(n *html.Node, skip bool) {
		if n.Type == html.ElementNode {
			d := strings.ToLower(n.Data)
			if d == "script" || d == "style" {
				skip = true
			}
		}
		if n.Type == html.TextNode && !skip {
			t := strings.TrimSpace(n.Data)
			if t == "" {
				return
			}
			if leakedVisibleRe.MatchString(t) || openOccTextRe.MatchString(t) {
				if len(t) > 160 {
					t = t[:160]
				}
				out = append(out, t)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, skip)
		}
	}
	walk(doc, false)
	return out
}

func htmlFormHasAttr(body, classNeedle, key string) bool {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return false
	}
	var found bool
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found {
			return
		}
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "form") {
			class := ""
			has := false
			for _, a := range n.Attr {
				if strings.EqualFold(a.Key, "class") {
					class = a.Val
				}
				if a.Key == key || strings.EqualFold(a.Key, key) {
					has = true
				}
			}
			if (classNeedle == "" || strings.Contains(class, classNeedle)) && has {
				found = true
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return found
}

func Test95_74_EditFormAttrsNotLeaked(t *testing.T) {
	e, db, _, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d74-edit.db"))
	users := repository.NewUserRepo(db)
	oa, err := users.GetByUsername("oa01")
	if err != nil || oa == nil {
		t.Fatal("oa01")
	}
	day := time.Now().AddDate(0, 0, 4).Format(dateLayout)
	mustCreateBoardTask(t, repo, "계획수정속성", day, "09:00", "09:30", model.OrgIDLibrary, oa.FullName, oa.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	var id string
	tasks, _ := repo.ListTasks()
	for _, tk := range tasks {
		if tk.Title == "계획수정속성" {
			id = tk.TaskID
			break
		}
	}
	if id == "" {
		t.Fatal("업무 없음")
	}
	rec := doGetWith(t, e, "/workboard/tasks/"+id+"/edit", jwtCookieUser(t, oa))
	if rec.Code != http.StatusOK {
		t.Fatalf("edit %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if leaks := visibleLeakedAttrSnippets(body); len(leaks) > 0 {
		t.Fatalf("본문에 속성 문자열이 샌다: %v", leaks)
	}
	if !htmlFormHasAttr(body, "js-keep-modal", "data-require-assignee") {
		t.Fatal("폼에 data-require-assignee 속성이 없다")
	}
	if !htmlFormHasAttr(body, "js-keep-modal", "@submit") {
		t.Fatal("폼에 @submit 속성이 없다")
	}
	if !strings.Contains(body, "js-keep-modal") || !strings.Contains(body, "id=\"overlap-resolve\"") {
		t.Fatal("keep-modal 또는 겹침 모달이 없다")
	}

	outDir := reviewShotDir95_74(t)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "task-edit.html"), rec.Body.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func reviewShotDir95_74(t *testing.T) string {
	t.Helper()
	return filepath.Clean(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-74"))
}

func Test95_74_RenderedPagesNoLeakedAttrs(t *testing.T) {
	e, db, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d74-pages.db"))
	h := New(db)
	e.GET("/workboard/tasks/:id", h.Workboard.ShowTask)
	users := repository.NewUserRepo(db)
	oa, _ := users.GetByUsername("oa01")
	day := time.Now().AddDate(0, 0, 5).Format(dateLayout)
	mustCreateBoardTask(t, repo, "렌더점검", day, "10:00", "10:30", model.OrgIDLibrary, oa.FullName, oa.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	var id string
	tasks, _ := repo.ListTasks()
	for _, tk := range tasks {
		if tk.Title == "렌더점검" {
			id = tk.TaskID
			break
		}
	}
	ck := jwtCookieUser(t, oa)
	pages := []struct {
		name string
		path string
	}{
		{"task_show 수정", "/workboard/tasks/" + id + "/edit"},
		{"task_show 조치", "/workboard/tasks/" + id},
		{"board_list", "/workboard/board?view=day&date=" + day},
		{"new_work as", "/workboard/new-work/form?kind=as&date=" + day},
		{"new_work maintenance", "/workboard/new-work/form?kind=maintenance&date=" + day},
		{"new_work project_work", "/workboard/new-work/form?kind=project_work&date=" + day},
		{"new_work admin", "/workboard/new-work/form?kind=admin&date=" + day},
		{"new_work sales", "/workboard/new-work/form?kind=sales&date=" + day},
	}
	for _, p := range pages {
		rec := doGetWith(t, e, p.path, ck)
		if rec.Code != http.StatusOK {
			t.Errorf("%s → %d", p.name, rec.Code)
			continue
		}
		if leaks := visibleLeakedAttrSnippets(rec.Body.String()); len(leaks) > 0 {
			t.Errorf("%s 본문 누출: %v", p.name, leaks)
		}
	}
	tech := doGetWith(t, e, "/workboard/tasks/"+id, jwtCookieUser(t, me))
	if tech.Code == http.StatusOK {
		if leaks := visibleLeakedAttrSnippets(tech.Body.String()); len(leaks) > 0 {
			t.Errorf("task_show 읽기(기술) 본문 누출: %v", leaks)
		}
	}
}

func Test95_74_TemplateSourceNoAttrLeak(t *testing.T) {
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
		if loc := srcAttrLeakRe.FindIndex(b); loc != nil {
			rel, _ := filepath.Rel(root, path)
			snip := string(b[loc[0]:loc[1]])
			hits = append(hits, filepath.ToSlash(rel)+": "+strings.TrimSpace(snip))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) > 0 {
		t.Fatalf("여는 태그 밖으로 속성이 샌다:\n%s", strings.Join(hits, "\n"))
	}
}

func Test95_74_EditFormBrowserConfirmAndAssignee(t *testing.T) {
	skipIfNoPlaywrightBrowser(t)
	cmd := exec.Command("node", "95_74_edit_browser_test.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("edit 브라우저 테스트 실패: %v\n%s", err, out)
	}
}

func Test95_74_EditTopScreenshot(t *testing.T) {
	skipIfNoPlaywrightBrowser(t)
	src := filepath.Join(reviewShotDir95_74(t), "task-edit.html")
	if _, err := os.Stat(src); err != nil {
		Test95_74_EditFormAttrsNotLeaked(t)
	}
	htmlAbs, err := filepath.Abs(src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(htmlAbs); err != nil {
		t.Fatal("계획 수정 HTML 덤프 없음")
	}
	pngAbs, err := filepath.Abs(filepath.Join(reviewShotDir95_74(t), "task-edit-top.png"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", "95_74_edit_screenshot.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"EDIT_HTML="+htmlAbs,
		"EDIT_PNG="+pngAbs,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("스크린샷 실패: %v\n%s", err, out)
	}
}
