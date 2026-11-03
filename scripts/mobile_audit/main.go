package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/golang-jwt/jwt/v5"
)

//go:embed probe.js
var probeJS string

type rowResult struct {
	Grade     string  `json:"grade"`
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	Path      string  `json:"path"`
	Viewport  string  `json:"viewport"`
	Shot      string  `json:"shot"`
	URL       string  `json:"url"`
	Skipped   string  `json:"skipped,omitempty"`
	Issues    []Issue `json:"issues"`
	ScrollW   int     `json:"scroll_w"`
	InnerW    int     `json:"inner_w"`
	TouchN    int     `json:"touch_short"`
	TableClip int     `json:"table_clip"`
}

func main() {
	base := flag.String("base", envOr("MOBILE_AUDIT_BASE", "http://127.0.0.1:8080"), "서버 주소")
	user := flag.String("user", os.Getenv("MOBILE_AUDIT_USER"), "로그인 아이디(폼)")
	pass := flag.String("pass", os.Getenv("MOBILE_AUDIT_PASS"), "로그인 비밀번호")
	jwtSecret := flag.String("jwt-secret", envOr("JWT_SECRET", "cs-system-jwt-secret-2026"), "세션 쿠키용 JWT 비밀(서버 handler.go 와 같음)")
	uid := flag.String("user-id", envOr("MOBILE_AUDIT_USER_ID", "USR-1776066384466512006"), "JWT user_id")
	urole := flag.String("role", envOr("MOBILE_AUDIT_ROLE", "vision_admin"), "JWT role")
	uname := flag.String("name", envOr("MOBILE_AUDIT_NAME", "비젼관리자"), "JWT name")
	uorg := flag.String("org", envOr("MOBILE_AUDIT_ORG", "O01"), "JWT org_id")
	unameLogin := flag.String("username", envOr("MOBILE_AUDIT_USERNAME", "vsadmin"), "JWT username")
	grade := flag.String("grade", "A", "캡처할 등급(A). ① 단계는 A만")
	outDir := flag.String("out", "", "스크린샷 폴더")
	report := flag.String("report", "", "실측 보고서 md")
	screens := flag.String("screens", defaultCatalogPath(), "screens.json")
	flag.Parse()

	cat, err := loadCatalog(*screens)
	if err != nil {
		fmt.Fprintf(os.Stderr, "분류표: %v\n", err)
		os.Exit(1)
	}
	root, err := repoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if *outDir == "" {
		*outDir = filepath.Join(root, "docs", "리뷰", "screenshots", "95-69")
	}
	if *report == "" {
		*report = filepath.Join(root, "docs", "리뷰", "95-69단계_모바일_화면_점검.md")
	}
	if err := os.MkdirAll(*outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath()),
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("hide-scrollbars", false),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
	)
	alloc, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()
	ctx, cancel := chromedp.NewContext(alloc)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 12*time.Minute)
	defer cancel()

	needLogin := false
	for _, s := range cat.screens(*grade) {
		if needsAuth(s) {
			needLogin = true
			break
		}
	}
	logged := false
	if needLogin && *user != "" && *pass != "" {
		if err := login(ctx, *base, *user, *pass); err != nil {
			fmt.Fprintf(os.Stderr, "폼 로그인 실패: %v\n", err)
		} else {
			logged = true
			dismissBriefing(ctx)
		}
	}
	if needLogin && !logged {
		tok, err := mintAuditJWT(*jwtSecret, *uid, *unameLogin, *urole, *uname, *uorg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "JWT: %v\n", err)
		} else if err := setSessionCookie(ctx, *base, tok); err != nil {
			fmt.Fprintf(os.Stderr, "쿠키: %v\n", err)
		} else {
			logged = true
			_ = chromedp.Run(ctx, chromedp.Navigate(strings.TrimRight(*base, "/")+"/"))
			dismissBriefing(ctx)
		}
	}

	var rows []rowResult
	list := cat.screens(*grade)
	for _, vp := range cat.Viewports {
		if err := chromedp.Run(ctx, chromedp.EmulateViewport(int64(vp.Width), int64(vp.Height))); err != nil {
			fmt.Fprintf(os.Stderr, "뷰포트 %s: %v\n", vp.Name, err)
			continue
		}
		for _, sc := range list {
			path := sc.Path
			skip := ""
			if sc.Resolve != "" {
				if !logged {
					skip = "로그인 필요(동적 상세)"
				} else {
					got, err := resolvePath(ctx, *base, sc.Resolve)
					if err != nil || got == "" {
						skip = "대상 건 없음"
					} else {
						path = got
					}
				}
			} else if needsAuth(sc) && !logged {
				skip = "로그인 실패·계정 없음"
			}
			r := captureOne(ctx, *base, *outDir, *grade, sc, path, vp, cat.MinTouch, skip)
			rows = append(rows, r)
			fmt.Printf("%s %s %s issues=%d %s\n", vp.Name, sc.ID, path, len(r.Issues), skip)
		}
	}

	if err := writeReport(*report, cat, *base, *grade, rows, *outDir); err != nil {
		fmt.Fprintf(os.Stderr, "보고서: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("보고서:", *report)
	fmt.Println(gradeSummary(cat))
}

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func chromePath() string {
	if p := os.Getenv("CHROME_PATH"); p != "" {
		return p
	}
	mac := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	if _, err := os.Stat(mac); err == nil {
		return mac
	}
	return "google-chrome"
}

func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for d := wd; d != "/" && d != "."; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "VERSION")); err == nil {
			if _, err := os.Stat(filepath.Join(d, "scripts", "mobile_audit")); err == nil {
				return d, nil
			}
		}
	}
	return wd, nil
}

func mintAuditJWT(secret, userID, username, role, name, org string) (string, error) {
	claims := jwt.MapClaims{
		"user_id":     userID,
		"username":    username,
		"role":        role,
		"name":        name,
		"org_id":      org,
		"view_org_id": org,
		"permissions": "",
		"is_readonly": false,
		"is_test":     false,
		"profile_ok":  true,
		"verified_at": time.Now().Unix(),
		"exp":         time.Now().Add(24 * time.Hour).Unix(),
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString([]byte(secret))
}

func setSessionCookie(ctx context.Context, base, token string) error {
	u := strings.TrimRight(base, "/")
	return chromedp.Run(ctx,
		chromedp.Navigate(u+"/login"),
		network.SetCookie("token", token).WithPath("/").WithHTTPOnly(true).WithURL(u+"/"),
	)
}

func login(ctx context.Context, base, user, pass string) error {
	var loc string
	err := chromedp.Run(ctx,
		chromedp.Navigate(strings.TrimRight(base, "/")+"/login"),
		chromedp.WaitVisible(`input[name="username"]`, chromedp.ByQuery),
		chromedp.SetValue(`input[name="username"]`, user, chromedp.ByQuery),
		chromedp.SetValue(`input[name="password"]`, pass, chromedp.ByQuery),
		chromedp.Click(`form[action="/login"] button[type="submit"]`, chromedp.ByQuery),
		chromedp.Sleep(800*time.Millisecond),
		chromedp.Location(&loc),
	)
	if err != nil {
		return err
	}
	if strings.Contains(loc, "/login") {
		return fmt.Errorf("로그인 화면에 머무름")
	}
	return nil
}

func dismissBriefing(ctx context.Context) {
	_ = chromedp.Run(ctx,
		chromedp.Sleep(400*time.Millisecond),
		chromedp.Evaluate(`(function(){
			var x = document.querySelector('#login-briefing-modal .brief-x');
			if (x) x.click();
		})()`, nil),
	)
}

func resolvePath(ctx context.Context, base, kind string) (string, error) {
	list := strings.TrimRight(base, "/") + "/as"
	sel := `a[href*="/action"]`
	if kind == "mnt_action" {
		list = strings.TrimRight(base, "/") + "/maintenance"
	}
	var href string
	err := chromedp.Run(ctx,
		chromedp.Navigate(list),
		chromedp.Sleep(600*time.Millisecond),
		chromedp.Evaluate(`(function(){
			var as = document.querySelector('a[href*="/as/"][href*="/action"]');
			if (as) return as.getAttribute('href');
			var m = document.querySelector('a[href*="/maintenance/visits/"][href*="/action"]');
			if (m) return m.getAttribute('href');
			var any = document.querySelector('`+sel+`');
			return any ? any.getAttribute('href') : '';
		})()`, &href),
	)
	if err != nil {
		return "", err
	}
	if href == "" {
		return "", fmt.Errorf("없음")
	}
	if strings.HasPrefix(href, "http") {
		return href, nil
	}
	if !strings.HasPrefix(href, "/") {
		href = "/" + href
	}
	return href, nil
}

func captureOne(ctx context.Context, base, outDir, grade string, sc Screen, path string, vp Viewport, minTouch int, skip string) rowResult {
	r := rowResult{Grade: grade, ID: sc.ID, Title: sc.Title, Path: path, Viewport: vp.Name}
	if skip != "" {
		r.Skipped = skip
		return r
	}
	url := path
	if strings.HasPrefix(path, "/") {
		url = strings.TrimRight(base, "/") + path
	}
	r.URL = url
	var buf []byte
	var m PageMetrics
	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.Sleep(700*time.Millisecond),
		chromedp.Evaluate(`(function(){ var x=document.querySelector('#login-briefing-modal .brief-x'); if(x) x.click(); })()`, nil),
		chromedp.Evaluate(probeJS, &m),
		chromedp.FullScreenshot(&buf, 90),
	)
	if err != nil {
		r.Skipped = err.Error()
		return r
	}
	r.Issues = findIssues(m, minTouch)
	r.ScrollW = int(m.ScrollWidth)
	r.InnerW = int(m.InnerWidth)
	for _, t := range m.Targets {
		if t.Short > 0 && t.Short < float64(minTouch) {
			r.TouchN++
		}
	}
	for _, t := range m.Tables {
		if t.Clipped {
			r.TableClip++
		}
	}
	name := fmt.Sprintf("%s-%s-%s.png", vp.Name, sc.ID, safeFile(sc.Title))
	shot := filepath.Join(outDir, name)
	if err := os.WriteFile(shot, buf, 0644); err == nil {
		r.Shot = shot
	}
	return r
}

func safeFile(s string) string {
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.Map(func(r rune) rune {
		if r < 32 {
			return -1
		}
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '-'
		}
		return r
	}, s)
	return s
}

func writeReport(path string, cat *Catalog, base, grade string, rows []rowResult, outDir string) error {
	var b strings.Builder
	b.WriteString("# 95-69단계 — 모바일 화면 점검 (① 실측)\n\n")
	b.WriteString("기준: `claude/커서_작업지시서_95-69_모바일_화면_점검_보완.md` (저장소에 없음, 채팅 요점). ")
	b.WriteString("이 단계는 화면 코드를 고치지 않았다. 폭 390·360, A등급만 캡처.\n\n")
	b.WriteString(fmt.Sprintf("서버: `%s`  시각: %s\n\n", base, time.Now().Format("2006-01-02 15:04")))
	b.WriteString("## 화면 분류 (확인 요청)\n\n")
	for _, g := range []string{"A", "B", "C"} {
		block := cat.Grades[g]
		b.WriteString(fmt.Sprintf("### %s — %s (%d)\n\n%s\n\n", g, block.Label, len(block.Screens), block.Note))
		b.WriteString("| id | 경로 | 화면 |\n|----|------|------|\n")
		for _, s := range block.Screens {
			b.WriteString(fmt.Sprintf("| `%s` | `%s` | %s |\n", s.ID, s.Path, s.Title))
		}
		b.WriteString("\n")
	}
	b.WriteString("## 실측 결과 (A, 390·360)\n\n")
	b.WriteString("점검: 문서 가로 스크롤, 잘린 표(스크롤 부모 없음), 44px 미만 터치, 고정 영역 겹침.\n\n")
	b.WriteString("| 폭 | 화면 | 경로 | 가로스크롤 | 잘린표 | 터치<44 | 고정겹침 | 캡처 |\n|----|------|------|------------|--------|---------|----------|------|\n")
	for _, r := range rows {
		if r.Skipped != "" {
			b.WriteString(fmt.Sprintf("| %s | %s | `%s` | — | — | — | — | 건너뜀 (%s) |\n", r.Viewport, r.Title, r.Path, r.Skipped))
			continue
		}
		hs, tc, th, fo := "없음", "0", "0", "없음"
		for _, is := range r.Issues {
			switch is.Kind {
			case "hscroll":
				hs = fmt.Sprintf("%dpx", is.ExtraPx)
			case "table_clip":
				tc = fmt.Sprintf("%d", is.Count)
			case "touch":
				th = fmt.Sprintf("%d", is.Count)
			case "fixed_overlap":
				fo = fmt.Sprintf("%d", is.Count)
			}
		}
		shot := ""
		if r.Shot != "" {
			rel, err := filepath.Rel(filepath.Dir(path), r.Shot)
			if err != nil {
				rel = r.Shot
			}
			shot = fmt.Sprintf("![](%s)", filepath.ToSlash(rel))
		}
		b.WriteString(fmt.Sprintf("| %s | %s | `%s` | %s | %s | %s | %s | %s |\n", r.Viewport, r.Title, r.Path, hs, tc, th, fo, shot))
	}
	b.WriteString("\n분류를 확인한 뒤 ② 단계(화면 수정)로 진행하면 된다.\n")
	_ = outDir
	return os.WriteFile(path, []byte(b.String()), 0644)
}
