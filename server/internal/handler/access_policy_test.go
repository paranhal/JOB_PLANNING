package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

func policyEchoCtx(role, org, simRole, simOrg, viewOrg string) echo.Context {
	e := echo.New()
	path := "/"
	if viewOrg != "" {
		path = "/?view_org_id=" + url.QueryEscape(viewOrg)
	}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	c := e.NewContext(req, httptest.NewRecorder())
	c.Set("role", role)
	c.Set("org_id", org)
	if simRole != "" {
		c.Set("sim_role", simRole)
	}
	if simOrg != "" {
		c.Set("sim_org_id", simOrg)
	}
	if viewOrg != "" {
		c.Set("view_org_id", viewOrg)
	}
	if role == model.RoleCLevel || simRole == model.RoleCLevel || role == model.RoleObserver {
		c.Set("is_readonly", true)
	}
	return c
}

func Test95_99_T1_DataScopeFor(t *testing.T) {
	rows := []struct {
		name, role, org, simRole, simOrg, viewOrg string
		all, ro                                   bool
		ids                                       []string
		viewO02                                   bool
	}{
		{"비전", model.RoleVisionAdmin, model.OrgIDLibrary, "", "", "", true, false, nil, true},
		{"C-Level", model.RoleCLevel, "O08", "", "", "", true, true, nil, true},
		{"C-Level 보기전환", model.RoleCLevel, "O08", "", "", "O02", true, true, nil, true},
		{"조직관리자", model.RoleOrgAdmin, model.OrgIDLibrary, "", "", "", false, false, []string{model.OrgIDLibrary}, false},
		{"조직관리자 부문", model.RoleOrgAdmin, "O07", "", "", "", false, false, []string{"O07", "O01", "O05"}, false},
		{"영업", model.RoleSales, model.OrgIDLibrary, "", "", "", false, false, []string{model.OrgIDLibrary}, false},
		{"기술", model.RoleTech, "O02", "", "", "", false, false, []string{"O02"}, true},
		{"지원", model.RoleSupport, model.OrgIDLibrary, "", "", "", false, false, []string{model.OrgIDLibrary}, false},
		{"옵저버", model.RoleObserver, model.OrgIDLibrary, "", "", "", false, true, []string{model.OrgIDLibrary}, false},
		{"비전→기술 시뮬", model.RoleVisionAdmin, model.OrgIDLibrary, model.RoleTech, "O02", "", false, false, []string{"O02"}, true},
		{"부문 없음 일반", model.RoleTech, model.OrgIDLibrary, "", "", "", false, false, []string{model.OrgIDLibrary}, false},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			c := policyEchoCtx(row.role, row.org, row.simRole, row.simOrg, row.viewOrg)
			if row.name == "조직관리자 부문" {
				c.Set("org_child_ids", map[string][]string{"O07": {"O01", "O05"}})
			}
			s := DataScopeFor(c)
			if s.All != row.all || s.ReadOnly != row.ro {
				t.Fatalf("All=%v ro=%v want All=%v ro=%v", s.All, s.ReadOnly, row.all, row.ro)
			}
			if !row.all {
				if len(s.OrgIDs) != len(row.ids) {
					t.Fatalf("OrgIDs %v want %v", s.OrgIDs, row.ids)
				}
				for i := range row.ids {
					if s.OrgIDs[i] != row.ids[i] {
						t.Fatalf("OrgIDs %v want %v", s.OrgIDs, row.ids)
					}
				}
			}
			if OrgScopeIDs(c) == nil != row.all {
				t.Fatalf("OrgScopeIDs All 감싸기")
			}
			if CanViewOrg(c, "O02") != row.viewO02 {
				t.Fatalf("CanViewOrg O02=%v", CanViewOrg(c, "O02"))
			}
			if CanWriteOrg(c, model.OrgIDLibrary) && s.ReadOnly {
				t.Fatal("읽기 전용인데 쓰기")
			}
		})
	}
}

func Test95_99_T1_CLevelWriteBlocked(t *testing.T) {
	if cLevelWriteBlocked(http.MethodGet, "/meeting") {
		t.Fatal("GET /meeting")
	}
	if cLevelWriteBlocked(http.MethodGet, "/workboard/board") {
		t.Fatal("GET /workboard/board")
	}
	if cLevelWriteBlocked(http.MethodGet, "/as/knowledge") {
		t.Fatal("GET /as/knowledge")
	}
	if !cLevelWriteBlocked(http.MethodGet, "/workboard/register") {
		t.Fatal("GET register")
	}
	if !cLevelWriteBlocked(http.MethodGet, "/workboard/new-work/form") {
		t.Fatal("GET form")
	}
	if !cLevelWriteBlocked(http.MethodGet, "/as/new") {
		t.Fatal("GET /as/new")
	}
	if !cLevelWriteBlocked(http.MethodPost, "/meeting") {
		t.Fatal("POST /meeting")
	}
	if cLevelWriteBlocked(http.MethodPost, "/logout") {
		t.Fatal("POST logout exempt")
	}
	if !cLevelHiddenPath("/meeting") {
		t.Fatal("cLevelHiddenPath 유지")
	}
}

func Test95_99_T1_CanViewRecord(t *testing.T) {
	tech := policyEchoCtx(model.RoleTech, model.OrgIDLibrary, "", "", "")
	if !CanViewRecord(tech, &model.ASReceipt{OrgID: model.OrgIDLibrary}) {
		t.Fatal("자기 조직")
	}
	if CanViewRecord(tech, &model.ASReceipt{OrgID: "O02"}) {
		t.Fatal("타 조직")
	}
	cl := policyEchoCtx(model.RoleCLevel, "O08", "", "", "")
	if !CanViewRecord(cl, &model.ASReceipt{OrgID: "O02"}) {
		t.Fatal("C-Level 전체")
	}
}
