package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-211(긴급): 주간 업무 보고서 「팀 전체」(금주 추진내역·차주 추진계획) 란은 어떤 경우에도 항상 그려진다(규칙 R).
//
// ⚠ 이 시험은 게이트에 들어 있다. company_weekly.html 의 「팀 전체」 영역을 조건(`{{if …}}`)으로 감싸거나 분기 안으로 옮기면
//   아래 상태 조합 중 하나가 실패한다. 템플릿을 고칠 때 이 시험이 걸리면 규칙 R(95-211)을 먼저 읽을 것.

const w211 = "/stats/company-weekly?week=2026-10-19&org=O03"

// TestWeeklyTeamBoxAlwaysRendered {보기/편집} × {확정/작성 중} × {카드 있음/0개} × {팀 전체 내용 있음/없음} × {쓰기 가능/읽기 전용}
func TestWeeklyTeamBoxAlwaysRendered(t *testing.T) {
	type combo struct {
		edit, confirmed, cards, content, writable bool
	}
	var combos []combo
	for _, edit := range []bool{false, true} {
		for _, confirmed := range []bool{false, true} {
			for _, cards := range []bool{true, false} {
				for _, content := range []bool{true, false} {
					for _, writable := range []bool{true, false} {
						combos = append(combos, combo{edit, confirmed, cards, content, writable})
					}
				}
			}
		}
	}
	if len(combos) != 32 {
		t.Fatalf("조합 %d", len(combos))
	}
	for _, c := range combos {
		name := fmt.Sprintf("edit=%v/confirmed=%v/cards=%v/content=%v/writable=%v", c.edit, c.confirmed, c.cards, c.content, c.writable)
		t.Run(name, func(t *testing.T) {
			v := setup190(t, c.cards)
			oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
			done, plan := "", ""
			if c.content {
				done, plan = "팀 금주 가", "팀 차주 나"
			}
			// 보고서를 저장해 둔다(touch_team=1 로 팀 전체를 값 또는 빈 칸으로)
			form := url.Values{"week": {"2026-10-19"}, "org": {"O03"}, "edit": {"1"}, "touch_team": {"1"}, "team_done": {done}, "team_plan": {plan}}
			if r := v.do(t, "POST", "/stats/company-weekly/save", form, oa); r.Code != http.StatusSeeOther {
				t.Fatalf("저장 %d", r.Code)
			}
			if c.confirmed {
				if r := v.do(t, "POST", "/stats/company-weekly/ui-confirm", url.Values{"week": {"2026-10-19"}, "org": {"O03"}}, oa); r.Code != http.StatusSeeOther {
					t.Fatalf("확정 %d", r.Code)
				}
			}
			ck := oa
			if !c.writable {
				ck = jwtCookieRole(t, model.RoleCLevel)
			}
			path := w211 + "&edit=0"
			if c.edit {
				path = w211 + "&edit=1"
			}
			body := v.do(t, "GET", path, nil, ck).Body.String()
			if !strings.Contains(body, "전주 추진내역") || !strings.Contains(body, "차주 추진계획") {
				t.Fatal("전주 추진내역·차주 추진계획 제목이 없다")
			}
			// 쓰기 가능 + 편집이면 입력칸 박스, 아니면 읽기 박스 — 어느 쪽이든 「팀 전체」 박스가 반드시 있다
			wantEditBox := c.edit && c.writable
			hasEdit := strings.Contains(body, "data-weekly-team>") || strings.Contains(body, "data-weekly-team ")
			hasRead := strings.Contains(body, "data-weekly-team-read")
			if wantEditBox {
				if !strings.Contains(body, `data-weekly-team>`) || !strings.Contains(body, `name="team_done"`) || !strings.Contains(body, `name="team_plan"`) || !strings.Contains(body, "data-team-done") || !strings.Contains(body, "data-team-plan") {
					t.Fatalf("편집 모드인데 팀 전체 입력칸이 없다(edit박스=%v)", hasEdit)
				}
				if !strings.Contains(body, "data-weekly-form") || !strings.Contains(body, "data-weekly-savebar") {
					t.Fatal("편집 모드인데 저장 폼·저장 막대가 없다")
				}
				if c.content && (!strings.Contains(body, "팀 금주 가") || !strings.Contains(body, "팀 차주 나")) {
					t.Fatal("저장한 팀 전체 내용이 입력칸에 없다")
				}
			} else {
				if !hasRead {
					t.Fatal("보기 모드(또는 읽기 전용)인데 팀 전체 박스(data-weekly-team-read)가 없다")
				}
				if c.content && (!strings.Contains(body, "팀 금주 가") || !strings.Contains(body, "팀 차주 나")) {
					t.Fatal("팀 전체 내용이 보기에 없다")
				}
				if !c.content && !strings.Contains(body, "입력된 내용이 없습니다") {
					t.Fatal("내용이 없을 때 「입력된 내용이 없습니다」 가 없다")
				}
			}
		})
	}
}

// 보고서가 아직 없고 카드도 없는 주차(자동 생성 화면)에서도 팀 전체 박스는 보인다.
func TestWeeklyTeamBoxShownWithoutReport95_211(t *testing.T) {
	v := setup190(t, false)
	for _, ck := range []*http.Cookie{jwtCookieOrg(t, model.RoleOrgAdmin, "O03"), jwtCookieRole(t, model.RoleCLevel)} {
		body := v.do(t, "GET", w211, nil, ck).Body.String()
		if !strings.Contains(body, "data-weekly-team-read") || !strings.Contains(body, "입력된 내용이 없습니다") {
			t.Fatal("보고서가 없는 주차에 팀 전체 박스가 없다")
		}
	}
}

// 편집 모드 카드 0개: 입력칸에 적고 저장하면 JSON 에 남고 다시 열면 보인다. 다시 집계해도 팀 전체는 그대로.
func TestWeeklyTeamEditWithZeroCardsSaves95_211(t *testing.T) {
	v := setup190(t, false)
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	// 보고서를 한 번 만든다(내용 없음)
	if r := v.do(t, "POST", "/stats/company-weekly/generate", url.Values{"week": {"2026-10-19"}, "org": {"O03"}}, oa); r.Code != http.StatusSeeOther {
		t.Fatalf("생성 %d", r.Code)
	}
	body := v.do(t, "GET", w211+"&edit=1", nil, oa).Body.String()
	if !strings.Contains(body, "data-weekly-no-lines") || !strings.Contains(body, "data-weekly-team>") {
		t.Fatal("카드 0개 편집 화면에 안내와 팀 전체 입력칸이 함께 있어야 한다")
	}
	if !strings.Contains(body, `data-weekly-save`) || !strings.Contains(body, `name="reaggregate"`) {
		t.Fatal("카드 0개에서도 저장 버튼과 다시 집계가 있어야 한다")
	}
	form := url.Values{"week": {"2026-10-19"}, "org": {"O03"}, "edit": {"1"}, "touch_team": {"1"}, "team_done": {"0카드 금주\n둘째 줄"}, "team_plan": {"0카드 차주"}}
	if r := v.do(t, "POST", "/stats/company-weekly/save", form, oa); r.Code != http.StatusSeeOther {
		t.Fatalf("저장 %d", r.Code)
	}
	saved, _ := v.h.Stats.repo.GetCompanyWeeklyReport("O03", "2026-10-26")
	doc := repository.ParseCompanyWeeklyDoc(saved.ContentJSON)
	if len(doc.TeamDone) != 2 || len(doc.TeamPlan) != 1 || doc.TeamDone[0].Text != "0카드 금주" {
		t.Fatalf("JSON 에 팀 전체가 없다 %+v", doc)
	}
	// 다시 집계(저장 막대의 버튼과 같은 폼 전송) — 팀 전체는 그대로
	form.Set("reaggregate", "1")
	if r := v.do(t, "POST", "/stats/company-weekly/save", form, oa); r.Code != http.StatusSeeOther {
		t.Fatalf("다시 집계 %d", r.Code)
	}
	again := v.do(t, "GET", w211+"&edit=1", nil, oa).Body.String()
	if !strings.Contains(again, "0카드 금주") || !strings.Contains(again, "0카드 차주") {
		t.Fatal("다시 집계 뒤 팀 전체가 사라졌다")
	}
	// 보기 모드에서도 같은 줄
	ro := v.do(t, "GET", w211+"&edit=0", nil, oa).Body.String()
	if !strings.Contains(ro, "· 0카드 금주") || !strings.Contains(ro, "· 둘째 줄") || !strings.Contains(ro, "· 0카드 차주") {
		t.Fatal("보기 모드 줄 표시(현행 골든)")
	}
}

// 팀 전체를 비우는 경로는 편집 폼의 touch_team=1 뿐이다: 생성·확정·확정 해제·touch_team 없는 저장은 팀 전체를 그대로 둔다.
func TestWeeklyTeamKeptByOtherPaths95_211(t *testing.T) {
	v := setup190(t, true)
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	if r := v.do(t, "POST", "/stats/company-weekly/save", url.Values{"week": {"2026-10-19"}, "org": {"O03"}, "touch_team": {"1"}, "team_done": {"유지 금주"}, "team_plan": {"유지 차주"}}, oa); r.Code != http.StatusSeeOther {
		t.Fatalf("저장 %d", r.Code)
	}
	check := func(step string) {
		t.Helper()
		saved, _ := v.h.Stats.repo.GetCompanyWeeklyReport("O03", "2026-10-26")
		doc := repository.ParseCompanyWeeklyDoc(saved.ContentJSON)
		if len(doc.TeamDone) != 1 || len(doc.TeamPlan) != 1 {
			t.Fatalf("%s 뒤 팀 전체가 사라졌다 %+v", step, doc)
		}
	}
	for _, step := range []struct {
		name, path string
		form       url.Values
	}{
		{"생성", "/stats/company-weekly/generate", url.Values{"week": {"2026-10-19"}, "org": {"O03"}}},
		{"행 저장(touch_team 없음)", "/stats/company-weekly/save", url.Values{"week": {"2026-10-19"}, "org": {"O03"}, "touch_" + v.key: {"1"}, "done_" + v.key: {"행 금주"}}},
		{"다시 집계(touch_team 없음)", "/stats/company-weekly/save", url.Values{"week": {"2026-10-19"}, "org": {"O03"}, "reaggregate": {"1"}}},
		{"확정", "/stats/company-weekly/ui-confirm", url.Values{"week": {"2026-10-19"}, "org": {"O03"}}},
	} {
		if r := v.do(t, "POST", step.path, step.form, oa); r.Code != http.StatusSeeOther {
			t.Fatalf("%s %d", step.name, r.Code)
		}
		check(step.name)
	}
}
