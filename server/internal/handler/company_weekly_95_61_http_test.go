package handler

import (
	"bytes"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func readVendorTailwindCSS(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(findStaticRoot(t), "vendor", "tailwind.css"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func cssHasHiddenImportant(css string) bool {
	compact := strings.ReplaceAll(strings.ReplaceAll(css, " ", ""), "\n", "")
	return strings.Contains(compact, "[hidden]{display:none!important}")
}

func extractWeeklyLinkModalTag(src string) string {
	re := regexp.MustCompile(`(?s)<div[^>]*id="weekly-link-modal"[^>]*>`)
	m := re.FindString(src)
	if m != "" {
		return m
	}
	re2 := regexp.MustCompile(`(?s)<div[^>]*id='weekly-link-modal'[^>]*>`)
	return re2.FindString(src)
}

func modalTagHasHiddenAttr(tag string) bool {
	return regexp.MustCompile(`(?i)(?:^|\s)hidden(?:\s|=|>|$)`).MatchString(tag) &&
		!strings.Contains(tag, `type="hidden"`)
}

func modalTagHasFlexClass(tag string) bool {
	return regexp.MustCompile(`class="[^"]*\bflex\b`).MatchString(tag) ||
		regexp.MustCompile(`class='[^']*\bflex\b`).MatchString(tag)
}

// Test95_61_BundleHiddenImportantRepro 수정 전 실패 → CSS 반영 후 통과. 번들에 [hidden]{display:none !important}.
func Test95_61_BundleHiddenImportantRepro(t *testing.T) {
	css := readVendorTailwindCSS(t)
	if !cssHasHiddenImportant(css) {
		t.Fatal("번들에 [hidden]{display:none !important} 없음. hidden+flex 요소가 .flex 때문에 숨겨지지 않는다")
	}
}

func Test95_61_WeeklyLinkModalKeepsHiddenAndFlex(t *testing.T) {
	root := findTemplateRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "stats", "_weekly_unassigned.html"))
	if err != nil {
		t.Fatal(err)
	}
	tag := extractWeeklyLinkModalTag(string(b))
	if tag == "" {
		t.Fatal("weekly-link-modal 태그 없음")
	}
	if !modalTagHasHiddenAttr(tag) {
		t.Fatalf("팝업에 hidden 속성 없음: %s", tag)
	}
	if !modalTagHasFlexClass(tag) {
		t.Fatalf("팝업에 flex 클래스 없음(레이아웃 유지): %s", tag)
	}
}

var hiddenAttrDisplayClassRe = regexp.MustCompile(`(?is)<([a-z][a-z0-9]*)\b([^>]*)>`)

var displayUtilRe = regexp.MustCompile(`\b(?:flex|inline-flex|grid|inline-grid|block|inline-block|inline|table|table-cell|table-row|contents|flow-root)\b`)

var htmlQuotedAttrRe = regexp.MustCompile(`(?i)\b(?:class|type|id|name|value|role|style|aria-[a-z-]+|data-[a-z0-9_-]+)\s*=\s*("[^"]*"|'[^']*')`)

func tagHasBooleanHidden(attrs string) bool {
	stripped := htmlQuotedAttrRe.ReplaceAllString(attrs, " ")
	return regexp.MustCompile(`(?:^|[\s/])hidden(?:[\s/>]|$)`).MatchString(stripped)
}

type hiddenDisplayHit struct {
	File  string
	Tag   string
	Utils []string
}

func scanHiddenPlusDisplay(t *testing.T) []hiddenDisplayHit {
	t.Helper()
	root := findTemplateRoot(t)
	var hits []hiddenDisplayHit
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		for _, m := range hiddenAttrDisplayClassRe.FindAllSubmatch(b, -1) {
			attrs := string(m[2])
			if !tagHasBooleanHidden(attrs) {
				continue
			}
			cls := ""
			if cm := regexp.MustCompile(`(?i)\bclass\s*=\s*"([^"]*)"`).FindStringSubmatch(attrs); len(cm) == 2 {
				cls = cm[1]
			} else if cm := regexp.MustCompile(`(?i)\bclass\s*=\s*'([^']*)'`).FindStringSubmatch(attrs); len(cm) == 2 {
				cls = cm[1]
			}
			utils := displayUtilRe.FindAllString(cls, -1)
			if len(utils) == 0 {
				continue
			}
			hits = append(hits, hiddenDisplayHit{File: rel, Tag: strings.TrimSpace(string(m[0])), Utils: uniqStrings(utils)})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hits
}

func uniqStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func Test95_61_HiddenPlusDisplayClassInventory(t *testing.T) {
	hits := scanHiddenPlusDisplay(t)
	var b strings.Builder
	b.WriteString("file\tutils\ttag\n")
	sawModal := false
	for _, h := range hits {
		if strings.Contains(h.Tag, "weekly-link-modal") {
			sawModal = true
		}
		t.Logf("%s | %s | %s", h.File, strings.Join(h.Utils, ","), h.Tag)
		b.WriteString(h.File + "\t" + strings.Join(h.Utils, ",") + "\t" + h.Tag + "\n")
	}
	if !sawModal {
		t.Fatal("weekly-link-modal 이 hidden+flex 목록에 없다")
	}
	css := readVendorTailwindCSS(t)
	if !cssHasHiddenImportant(css) {
		t.Fatalf("hidden+display 패턴 %d건이 있는데 번들 !important 규칙이 없다\n%s", len(hits), b.String())
	}
}

func Test95_61_WeeklyLinkJSCloseUsesHidden(t *testing.T) {
	jsb, err := os.ReadFile(filepath.Join(findStaticRoot(t), "js", "weekly-link.js"))
	if err != nil {
		t.Fatal(err)
	}
	js := string(jsb)
	if !strings.Contains(js, "m.hidden = true") {
		t.Fatal("closeModal 이 hidden 속성을 쓰지 않는다")
	}
	if !strings.Contains(js, "m.hidden = false") {
		t.Fatal("open 이 hidden 을 끄지 않는다")
	}
	if !strings.Contains(js, `e.key === "Escape"`) {
		t.Fatal("ESC 닫기 없음")
	}
	if !strings.Contains(js, "data-weekly-link-close") {
		t.Fatal("취소(닫기) 핸들러 없음")
	}
	if !strings.Contains(js, "data-weekly-link-submit") && !strings.Contains(js, "weekly-link-submit") {
		t.Log("제출 버튼 훅은 템플릿 name/data 로 확인")
	}
}

func mountWeekly61(e *echo.Echo, h *Handler) {
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/stats/company-weekly", h.Stats.ShowWeeklyReport)
	g.GET("/stats/reports", h.Stats.Reports)
	g.POST("/stats/company-weekly", h.Stats.CreateCompanyWeekly)
	g.POST("/stats/company-weekly/tasks/project", h.Stats.LinkWeeklyTasksProject)
}

func Test95_61_PopupStartsClosedOnThreeScreens(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "cw61.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	seedWeekly32(t, db)
	techID := userIDByName(t, db, "si-tech")

	thisMon := parseWeekMonday("", time.Now())
	prevMon := thisMon.AddDate(0, 0, -7)
	prevDay := prevMon.AddDate(0, 0, 2).Format("2006-01-02") + " 12:00:00"
	thisDay := thisMon.AddDate(0, 0, 2).Format("2006-01-02") + " 12:00:00"
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, complete_datetime, status,
		assigned_to, assigned_user_id, org_id, symptom, created_at, updated_at)
		VALUES ('AS-PREV61','AS-PREV61','C32',?,?, 'completed', '김기술', ?, 'O03', '게이트', datetime('now'), datetime('now'))`,
		prevDay, prevDay, techID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, complete_datetime, status,
		assigned_to, assigned_user_id, org_id, symptom, created_at, updated_at)
		VALUES ('AS-THIS61','AS-THIS61','C32',?,?, 'completed', '김기술', ?, 'O03', '게이트', datetime('now'), datetime('now'))`,
		thisDay, thisDay, techID); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	h.Stats.dataDir = t.TempDir()
	mountWeekly61(e, h)
	ck := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")

	assertModalClosed := func(t *testing.T, name, body string) {
		t.Helper()
		tag := extractWeeklyLinkModalTag(body)
		if tag == "" {
			t.Fatalf("%s: 팝업 마크업 없음", name)
		}
		if !modalTagHasHiddenAttr(tag) {
			t.Fatalf("%s: 처음부터 hidden 이 없다: %s", name, tag)
		}
		if !modalTagHasFlexClass(tag) {
			t.Fatalf("%s: flex 없음: %s", name, tag)
		}
		if !strings.Contains(body, "data-weekly-link-close") {
			t.Fatalf("%s: 취소 버튼 없음", name)
		}
		if !strings.Contains(body, "data-weekly-link-submit") && !strings.Contains(body, `type="submit"`) {
			t.Fatalf("%s: 연결 제출 없음", name)
		}
		if !strings.Contains(body, "/static/js/weekly-link.js") {
			t.Fatalf("%s: weekly-link.js 없음", name)
		}
	}

	get := func(path string) string {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(ck)
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s → %d", path, rec.Code)
		}
		return rec.Body.String()
	}

	assertModalClosed(t, "회사 주간 보고 전주", get("/stats/company-weekly?week="+prevMon.Format("2006-01-02")+"&org=O03"))
	assertModalClosed(t, "회사 주간 보고 이번 주", get("/stats/company-weekly?week="+thisMon.Format("2006-01-02")+"&org=O03"))
	assertModalClosed(t, "보고서 메뉴 전주", get("/stats/reports?company_date="+companyWeeklyAnchor(prevMon).Format("2006-01-02")))
	assertModalClosed(t, "보고서 메뉴 이번 주", get("/stats/reports?company_date="+companyWeeklyAnchor(thisMon).Format("2006-01-02")))

	src := mustCompanyWeeklyFixture(t)
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("company_date", companyWeeklyAnchor(prevMon).Format("2006-01-02"))
	_ = w.WriteField("org", "O03")
	fw, err := w.CreateFormFile("file", "weekly.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(src); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	post := httptest.NewRecorder()
	preq := httptest.NewRequest(http.MethodPost, "/stats/company-weekly", &buf)
	preq.Header.Set(echo.HeaderContentType, w.FormDataContentType())
	preq.AddCookie(ck)
	e.ServeHTTP(post, preq)
	if post.Code != http.StatusOK && post.Code != http.StatusBadRequest {
		t.Fatalf("결과 화면 %d body=%s", post.Code, post.Body.String()[:min(400, post.Body.Len())])
	}
	assertModalClosed(t, "결과 화면", post.Body.String())

	root := findTemplateRoot(t)
	for _, name := range []string{"stats/company_weekly.html", "stats/reports.html", "stats/company_weekly_result.html"} {
		src := string(mustRead(t, filepath.Join(root, name)))
		if !strings.Contains(src, `{{template "weekly_unassigned"`) {
			t.Fatalf("%s 가 weekly_unassigned 를 쓰지 않는다", name)
		}
	}
}
