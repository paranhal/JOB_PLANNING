package handler

import (
	"html"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-218 ③ 저장 멱등 시험(게이트): 같은 보고서를 몇 번 저장해도 줄이 늘지 않고, 수정 없이 저장하면 문서가 바뀌지 않는다.

var re218TA = regexp.MustCompile(`(?s)<textarea[^>]*name="((?:done|plan)_[^"]+)"[^>]*>(.*?)</textarea>`)

func form218(body string) url.Values {
	f := url.Values{"week": {"2026-10-19"}, "org": {"O03"}, "edit": {"1"}}
	for _, m := range re218TA.FindAllStringSubmatch(body, -1) {
		f.Set(m[1], strings.TrimLeft(html.UnescapeString(m[2]), "\n"))
	}
	return f
}

func lines218(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, strings.TrimSpace(l))
		}
	}
	return out
}

func doc218(t *testing.T, v env190) string {
	t.Helper()
	var js string
	_ = v.db.QueryRow(`SELECT COALESCE(content_json,'') FROM company_weekly_reports WHERE org_id='O03' ORDER BY week_anchor DESC LIMIT 1`).Scan(&js)
	return js
}

func setup218(t *testing.T) env190 { return setup218Plan(t, true) }

func setup218Plan(t *testing.T, withPlan bool) env190 {
	v := setup205(t)
	for i, ti := range []string{"나라장터 물품(KLAS, RFID) 납품 목록 업데이트", "주간통계산출 및 업무보고자료 업로드", "주간통계산출 및 주간업무보고자료 업로드"} {
		for _, q := range []struct {
			sql string
			a   []interface{}
		}{
			{`INSERT INTO work_tasks(task_id,work_type,title,status,org_id,work_date,complete_date,activity_kind) VALUES (?,?,?,?,?,?,?,?)`, []interface{}{"BD" + string(rune('0'+i)), "admin", ti, "complete", "O03", "2026-10-20", "2026-10-20", "biz_support"}},
			{`INSERT INTO work_tasks(task_id,work_type,title,status,org_id,work_date,activity_kind) VALUES (?,?,?,?,?,?,?)`, []interface{}{"BP" + string(rune('0'+i)), "admin", ti + "(계획)", "open", "O03", "2026-10-27", "biz_support"}},
		} {
			if !withPlan && strings.HasPrefix(q.sql, "INSERT INTO work_tasks(task_id,work_type,title,status,org_id,work_date,activity_kind)") {
				continue
			}
			if _, err := v.db.Exec(q.sql, q.a...); err != nil {
				t.Fatal(err)
			}
		}
	}
	return v
}

func TestWeeklySaveIdempotent(t *testing.T) {
	type edit struct {
		name string
		fn   func(rk string, f url.Values)
		want func(before []string, after []string) bool
	}
	edits := []edit{
		{"수정 없이", func(rk string, f url.Values) {}, func(b, a []string) bool { return len(a) == len(b) }},
		{"번호만 붙임", func(rk string, f url.Values) {
			ls := lines218(f.Get("done_" + rk))
			if len(ls) > 0 {
				ls[0] = "1. " + ls[0]
			}
			f.Set("done_"+rk, strings.Join(ls, "\n"))
			f.Set("touch_"+rk, "1")
		}, func(b, a []string) bool { return len(a) == len(b) }},
		{"한 줄 수정", func(rk string, f url.Values) {
			ls := lines218(f.Get("plan_" + rk))
			if len(ls) > 0 {
				ls[0] = ls[0] + " 수정"
			}
			f.Set("plan_"+rk, strings.Join(ls, "\n"))
			f.Set("touch_"+rk, "1")
		}, func(b, a []string) bool { return len(a) == len(b) }},
		{"줄 추가", func(rk string, f url.Values) {
			f.Set("plan_"+rk, f.Get("plan_"+rk)+"\n새로 추가한 줄")
			f.Set("touch_"+rk, "1")
		}, func(b, a []string) bool { return len(a) == len(b)+1 }},
		{"줄 삭제", func(rk string, f url.Values) {
			ls := lines218(f.Get("done_" + rk))
			if len(ls) > 1 {
				ls = ls[1:]
			}
			f.Set("done_"+rk, strings.Join(ls, "\n"))
			f.Set("touch_"+rk, "1")
		}, func(b, a []string) bool { return len(a) <= len(b) }},
	}
	for _, rowKind := range []string{"project", "activity_tagged", "activity_tech"} {
		for _, e := range edits {
			v := setup218(t)
			oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
			rk := ""
			get := func() url.Values {
				return form218(v.do(t, "GET", w205+"&edit=1", nil, oa).Body.String())
			}
			f := get()
			for k := range f {
				if !strings.HasPrefix(k, "done_") {
					continue
				}
				id := strings.TrimPrefix(k, "done_")
				switch {
				case rowKind == "project" && id == v.key:
					rk = id
				case rowKind == "activity_tagged" && id == "ACT-biz_support":
					rk = id
				case rowKind == "activity_tech" && strings.Contains(f.Get(k), "제안 자료"):
					rk = id
				}
			}
			if rk == "" {
				t.Fatalf("%s: 행을 못 찾았다", rowKind)
			}
			// 첫 저장(자동 줄을 문서에 담는다) 뒤의 상태가 기준
			v.do(t, "POST", "/stats/company-weekly/save", f, oa)
			f = get()
			base := append(lines218(f.Get("done_"+rk)), lines218(f.Get("plan_"+rk))...)
			e.fn(rk, f)
			v.do(t, "POST", "/stats/company-weekly/save", f, oa)
			var first string
			for i := 1; i <= 3; i++ {
				f = get()
				cur := append(lines218(f.Get("done_"+rk)), lines218(f.Get("plan_"+rk))...)
				if i == 1 {
					if !e.want(base, cur) {
						t.Errorf("[%s·%s] 줄 수 base=%d cur=%d", rowKind, e.name, len(base), len(cur))
					}
					first = doc218(t, v)
				}
				v.do(t, "POST", "/stats/company-weekly/save", f, oa) // 수정 없이 반복 저장
				if got := doc218(t, v); got != first {
					t.Errorf("[%s·%s] 수정 없이 %d번째 저장에서 문서가 바뀜", rowKind, e.name, i)
				}
				again := append(lines218(get().Get("done_"+rk)), lines218(get().Get("plan_"+rk))...)
				if len(again) != len(cur) {
					t.Errorf("[%s·%s] 반복 저장 %d회 후 줄 수 %d → %d", rowKind, e.name, i, len(cur), len(again))
				}
			}
		}
	}
}

// 다시 집계를 반복해도 줄이 늘지 않는다.
func TestWeeklySaveIdempotentReaggregate(t *testing.T) {
	v := setup218(t)
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	f := form218(v.do(t, "GET", w205+"&edit=1", nil, oa).Body.String())
	v.do(t, "POST", "/stats/company-weekly/save", f, oa)
	count := func() int {
		n := 0
		f := form218(v.do(t, "GET", w205+"&edit=1", nil, oa).Body.String())
		for k, vv := range f {
			if strings.HasPrefix(k, "done_") || strings.HasPrefix(k, "plan_") {
				n += len(lines218(vv[0]))
			}
		}
		return n
	}
	base := count()
	g := form218(v.do(t, "GET", w205+"&edit=1", nil, oa).Body.String())
	g.Set("reaggregate", "1")
	var first string
	for i := 0; i < 3; i++ {
		v.do(t, "POST", "/stats/company-weekly/save", g, oa)
		if i == 0 {
			first = doc218(t, v)
		} else if doc218(t, v) != first {
			t.Errorf("다시 집계 %d번째에서 문서가 바뀜", i+1)
		}
		if n := count(); n != base {
			t.Errorf("다시 집계 %d회 후 줄 수 %d → %d", i+1, base, n)
		}
	}
}

// 이미 같은 줄이 두 번 저장된 문서(운영 캡처와 같은 형태)는 1번만 보이고, 저장하면 1번만 남는다.
func TestWeeklyDuplicateSavedDocShownOnce(t *testing.T) {
	v := setup218Plan(t, false)
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	rk := "ACT-biz_support"
	three := []string{"나라장터 물품(KLAS, RFID) 납품 목록 업데이트", "주간통계산출 및 업무보고자료 업데이트", "주간통계산출 및 주간업무보고자료 업데이트"}
	var dup []repository.CompanyWeeklyLine
	for i := 0; i < 2; i++ {
		for _, s := range three {
			dup = append(dup, repository.CompanyWeeklyLine{Text: s, Manual: true})
		}
	}
	doc := repository.CompanyWeeklyDoc{Rows: map[string]repository.CompanyWeeklySavedRow{rk: {PlanLines: dup, Plan: repository.JoinWeeklyLines(dup)}}}
	if err := v.h.Stats.repo.UpsertCompanyWeeklyReport("O03", "2026-10-26", "s", repository.EncodeCompanyWeeklyDoc(doc), "x", repository.CompanyWeeklyUpsertSave); err != nil {
		t.Fatal(err)
	}
	f := form218(v.do(t, "GET", w205+"&edit=1", nil, oa).Body.String())
	if n := len(lines218(f.Get("plan_" + rk))); n != 3 {
		t.Fatalf("중복 저장 문서가 %d줄로 보임(3줄이어야 함): %q", n, f.Get("plan_"+rk))
	}
	f.Set("touch_"+rk, "1")
	v.do(t, "POST", "/stats/company-weekly/save", f, oa)
	saved := repository.ParseCompanyWeeklyDoc(doc218(t, v))
	if n := len(saved.Rows[rk].PlanLines); n != 3 {
		t.Fatalf("저장 뒤 plan_lines %d줄(3줄이어야 함)", n)
	}
}

func TestNormalizeWeeklyLine(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"·1. 서산시립도서관 점검", "서산시립도서관 점검"},
		{"- 서산시립도서관  점검", "서산시립도서관 점검"},
		{"① 서산시립도서관　점검", "서산시립도서관 점검"},
		{"(2) 서산시립도서관 점검", "서산시립도서관 점검"},
		{"* + 서산시립도서관 점검 ", "서산시립도서관 점검"},
		{"(기관용)", "(기관용)"},
		{"2027년 충남도서관 제안서(1주 지연)", "2027년 충남도서관 제안서(1주 지연)"},
	} {
		if got := repository.NormalizeWeeklyLine(c.in); got != c.want {
			t.Errorf("정규화 %q → %q want %q", c.in, got, c.want)
		}
	}
	if repository.WeeklyLineHashKey("1. 가나 다") != repository.WeeklyLineHashKey("- 가나  다") {
		t.Error("정규화가 같으면 해시 키도 같아야 한다")
	}
}
