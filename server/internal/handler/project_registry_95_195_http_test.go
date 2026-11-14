package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

// 95-195: 사업 등록 관리에 계약 정보가 없는 직접 등록 사업도 보인다(「계약 미등록」 배지·계약 구분 필터·합치기 링크).

func registryGet195(t *testing.T, e *echo.Echo, path string, ck *http.Cookie) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
	req.AddCookie(ck)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("%s %d", path, rec.Code)
	}
	return rec.Body.String()
}

func seed195(t *testing.T) (e *echo.Echo, vision, org *http.Cookie, ids map[string]string) {
	t.Helper()
	ee, repo, _, _, db := newProjectServerWithSQL(t)
	for _, f := range []url.Values{
		{"name": {"계약없는 직접등록 사업"}, "plan_year": {"2027"}, "status": {"active"}, "project_kind": {"build"}},
		{"name": {"계약된 사업"}, "plan_year": {"2027"}, "status": {"active"}, "project_kind": {"build"}, "contract_amount": {"1000000"}},
		{"name": {"같은 이름 사업"}, "plan_year": {"2027"}, "status": {"active"}, "project_kind": {"build"}},
		{"name": {"같은 이름 사업"}, "plan_year": {"2027"}, "status": {"active"}, "project_kind": {"build"}, "contract_amount": {"5000000"}},
	} {
		if r := doForm(t, ee, "/projects", f); r.Code != http.StatusSeeOther {
			t.Fatalf("등록 %d %s", r.Code, r.Body.String())
		}
	}
	ids = map[string]string{}
	rows, err := db.Query(`SELECT project_id, name, COALESCE(contract_amount,0) FROM work_projects WHERE name IN ('계약없는 직접등록 사업','계약된 사업','같은 이름 사업') ORDER BY created_at, project_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, n string
		var amt int
		_ = rows.Scan(&id, &n, &amt)
		switch {
		case n == "같은 이름 사업" && amt == 0:
			ids["dup-none"] = id
		case n == "같은 이름 사업":
			ids["dup-contract"] = id
		case n == "계약된 사업":
			ids["contract"] = id
		default:
			ids["none"] = id
		}
	}
	_ = repo
	return ee, cookie183(t, "va", "vision_admin", "O01", false), cookie183(t, "oa", "org_admin", "O01", false), ids
}

func TestHTTP95_195_RegistryShowsUncontractedWithBadgeAndFilter(t *testing.T) {
	e, vision, org, ids := seed195(t)
	body := registryGet195(t, e, "/projects/registry", vision)
	if !strings.Contains(body, "계약없는 직접등록 사업") {
		t.Fatal("계약 정보가 없는 직접 등록 사업이 등록 관리에 없다(재현)")
	}
	if !strings.Contains(body, "계약 미등록") || !strings.Contains(body, "data-uncontracted") {
		t.Fatal("「계약 미등록」 배지가 없다")
	}
	// 계약된 사업에는 배지가 없다: 사업 행을 잘라 확인
	rowOf := func(b, id string) string {
		i := strings.Index(b, `href="/projects/`+id+`"`)
		if i < 0 {
			return ""
		}
		j := strings.Index(b[i:], "</tr>")
		return b[i : i+j]
	}
	if strings.Contains(rowOf(body, ids["contract"]), "data-uncontracted") {
		t.Fatal("계약된 사업에 계약 미등록 배지")
	}
	if !strings.Contains(rowOf(body, ids["none"]), "data-uncontracted") {
		t.Fatal("계약 없는 사업 행에 배지가 없다")
	}
	// 계약 구분 필터
	only := registryGet195(t, e, "/projects/registry?contract=none", vision)
	if !strings.Contains(only, ids["none"]) || strings.Contains(rowOf(only, ids["contract"]), "계약된 사업") {
		t.Fatal("계약 미등록 필터가 계약된 사업을 걸러 내지 못한다")
	}
	done := registryGet195(t, e, "/projects/registry?contract=contracted", vision)
	if strings.Contains(done, `href="/projects/`+ids["none"]+`"`) || !strings.Contains(done, `href="/projects/`+ids["contract"]+`"`) {
		t.Fatal("계약됨 필터 결과가 맞지 않다")
	}
	// 합치기 링크: 비전관리자에게만, 같은 이름 안내에 달린다
	if !strings.Contains(body, "/projects/"+ids["dup-none"]+"/merge") && !strings.Contains(body, "/projects/"+ids["dup-contract"]+"/merge") {
		t.Fatal("같은 이름 안내에 합치기 링크가 없다")
	}
	if strings.Contains(registryGet195(t, e, "/projects/registry", org), "/merge") {
		t.Fatal("비전관리자가 아닌데 합치기 링크가 보인다")
	}
	_ = model.RoleVisionAdmin
}

// 다른 화면(계약 판정 ContractedSQL)은 그대로: includeAll=false 목록에는 계약 없는 사업이 여전히 없다.
func TestHTTP95_195_ContractedListsUnchanged(t *testing.T) {
	_, repo, _, _, db := newProjectServerWithSQL(t)
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id) VALUES ('WPT-195N','계약 없음','active','O01')`); err != nil {
		t.Fatal(err)
	}
	has := func(all bool) bool {
		list, err := repo.ListContracts("", "", 0, "", all)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range list {
			if p.ProjectID == "WPT-195N" {
				return true
			}
		}
		return false
	}
	if has(false) {
		t.Fatal("includeAll=false 목록에 계약 없는 사업이 생겼다(ContractedSQL 변경 금지)")
	}
	if !has(true) {
		t.Fatal("includeAll=true 목록에는 있어야 한다")
	}
}
