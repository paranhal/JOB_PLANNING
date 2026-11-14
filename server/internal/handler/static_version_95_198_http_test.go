package handler

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

var weeklyLinkTagRe = regexp.MustCompile(`<script src="(/static/js/weekly-link\.js[^"]*)"`)

func scriptTags198(body, name string) []string {
	re := regexp.MustCompile(`<script src="(/static/js/` + regexp.QuoteMeta(name) + `[^"]*)"`)
	var out []string
	for _, m := range re.FindAllStringSubmatch(body, -1) {
		out = append(out, m[1])
	}
	return out
}

// 95-198: 주간보고 화면의 weekly-link.js 는 빌드 값이 붙은 주소로 로드돼야 한다.
func TestHTTP95_198_WeeklyLinkScriptVersioned(t *testing.T) {
	SetBuildInfo("95-197", "abc1234", "2026-11-14 22:00", time.Now())
	want := "/static/js/weekly-link.js?v=" + AssetQuery()
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")

	// (1) 연결된 업무가 있어 페이지 수준 연결 창을 쓰는 화면(company_weekly.html)
	v := seed197(t)
	body := v.do(t, "GET", w190+"&edit=0", nil, oa).Body.String()
	tags := scriptTags198(body, "weekly-link.js")
	if len(tags) != 1 || tags[0] != want {
		t.Fatalf("페이지 연결 창의 weekly-link.js 주소 %v (기대 %s)", tags, want)
	}

	// (2) 연결된 업무가 없고 미연결 업무만 있어 부분 템플릿(_weekly_unassigned)이 스크립트를 싣는 화면
	v2 := setup190(t, false)
	seed193Weekly(t, v2)
	body2 := v2.do(t, "GET", w190+"&edit=0", nil, oa).Body.String()
	tags2 := scriptTags198(body2, "weekly-link.js")
	if len(tags2) != 1 || tags2[0] != want {
		t.Fatalf("부분 템플릿의 weekly-link.js 주소 %v (기대 %s) — dict 호출이라 .AssetQuery 가 비면 안 된다", tags2, want)
	}
	if strings.Contains(body2, "weekly-link.js?v=\"") || strings.Contains(body2, "weekly-link.js?v=-") {
		t.Fatal("버전 값이 비어 있다")
	}
}

// 빌드 값이 바뀌면 주소도 바뀐다(캐시 무효화의 전제).
func TestHTTP95_198_ScriptQueryFollowsBuild(t *testing.T) {
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	v := seed197(t)
	SetBuildInfo("95-197", "abc1234", "2026-11-14 22:00", time.Now())
	q1 := scriptTags198(v.do(t, "GET", w190+"&edit=0", nil, oa).Body.String(), "weekly-link.js")
	SetBuildInfo("95-198", "def5678", "2026-11-15 07:00", time.Now())
	q2 := scriptTags198(v.do(t, "GET", w190+"&edit=0", nil, oa).Body.String(), "weekly-link.js")
	if len(q1) != 1 || len(q2) != 1 || q1[0] == q2[0] {
		t.Fatalf("빌드가 바뀌어도 주소가 같다 %v %v", q1, q2)
	}
}

// 업무 등록·작업 상세 화면의 work-group-task.js 도 빌드 값이 붙은 주소로 로드된다.
func TestHTTP95_198_WorkGroupTaskScriptVersioned(t *testing.T) {
	SetBuildInfo("95-198", "def5678", "2026-11-15 07:00", time.Now())
	want := "/static/js/work-group-task.js?v=" + AssetQuery()
	v := setup191(t)
	for _, path := range []string{"/workboard/register", "/workboard/tasks/" + v.progress} {
		body := v.do("GET", path, nil, v.admin).Body.String()
		tags := scriptTags198(body, "work-group-task.js")
		if len(tags) != 1 || tags[0] != want {
			t.Fatalf("%s: work-group-task.js 주소 %v (기대 %s)", path, tags, want)
		}
	}
}

// AS 조치 화면의 JS 3개도 빌드 값이 붙은 주소로 로드된다.
func TestHTTP95_198_ASActionScriptsVersioned(t *testing.T) {
	SetBuildInfo("95-198", "def5678", "2026-11-15 07:00", time.Now())
	e, db, _ := newAS151Server(t)
	insert151AS(t, db, "R198-a", "C151", "in_progress", "", "", false)
	body := doAS151Get(t, e, "/as/R198-a/action", as151OA(t, db)).Body.String()
	for _, name := range []string{"as_action_sets.js", "as_action_set_cause.js", "as_keywords.js"} {
		want := "/static/js/" + name + "?v=" + AssetQuery()
		if tags := scriptTags198(body, name); len(tags) != 1 || tags[0] != want {
			t.Fatalf("%s 주소 %v (기대 %s)", name, tags, want)
		}
	}
}

// 업무그룹·협업·주간회의 화면.
func TestHTTP95_198_OtherPagesScriptsVersioned(t *testing.T) {
	SetBuildInfo("95-198", "def5678", "2026-11-15 07:00", time.Now())
	e, db, h := newAS151Server(t)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/work-groups", h.WorkGroup.List)
	g.GET("/cooperations/inbox", h.Coop.ListInbox)
	g.GET("/meeting/weekly", h.Meeting.Weekly)
	ck := as151OA(t, db)
	for _, c := range []struct{ path, js string }{
		{"/work-groups", "work-group.js"},
		{"/cooperations/inbox", "coop.js"},
		{"/meeting/weekly", "weekly-edit.js"},
	} {
		r := doAS151Get(t, e, c.path, ck)
		if r.Code != http.StatusOK {
			t.Fatalf("%s %d", c.path, r.Code)
		}
		want := "/static/js/" + c.js + "?v=" + AssetQuery()
		if tags := scriptTags198(r.Body.String(), c.js); len(tags) != 1 || tags[0] != want {
			t.Fatalf("%s: %s 주소 %v (기대 %s)", c.path, c.js, tags, want)
		}
	}
}
