package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type pkgChoicesResp struct {
	Has      bool                       `json:"has"`
	Packages []repository.PackageChoice `json:"packages"`
}

type pkgSitesResp struct {
	Items []repository.PackageSiteItem `json:"items"`
	Total int                          `json:"total"`
}

// 작업 등록: 수행 구조가 있는 사업은 하부 업무가 필수이고, 사이트 후보는 그 하부 업무에 등록된 곳뿐이다. 95-182.
func Test95_182_PackagePickForWorkRegister(t *testing.T) {
	e, repo, _ := newProjectWorkServer(t)
	cids := seedBadgeData(t, repo.DB(), "WP-O01", model.OrgIDLibrary) // 홍성고·천안고, 하부 업무 GB·FW 둘 다 두 곳 등록
	pr := repository.NewProjectRepo(repo.DB())
	phases, _ := pr.ListPhases("WP-O01")
	// 세 번째 사이트(계룡고)는 사업에는 있지만 하부 업무에는 등록하지 않는다. 고객 업종은 학교로 둔다.
	if _, err := repo.DB().Exec(`UPDATE customers SET industry='학교', addr_sido='충청남도', addr_sigungu='홍성군' WHERE customer_id=?`, cids[0]); err != nil {
		t.Fatal(err)
	}
	_ = phases

	// 수행 구조가 없는 사업
	var none pkgChoicesResp
	rec := doGet(t, e, "/workboard/project-packages?project_id=WP-O03")
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &none) != nil || none.Has {
		t.Fatalf("구조 없는 사업: %d %s", rec.Code, rec.Body.String())
	}
	// 구조가 있는 사업: 사이트형 하부 업무 두 개, 등록 2곳씩
	var has pkgChoicesResp
	rec = doGet(t, e, "/workboard/project-packages?project_id=WP-O01")
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &has) != nil || !has.Has || len(has.Packages) != 2 || has.Packages[0].Sites != 2 {
		t.Fatalf("구조 있는 사업: %d %s", rec.Code, rec.Body.String())
	}
	gb := has.Packages[0].ID
	var sites pkgSitesResp
	rec = doGet(t, e, "/workboard/package-sites?project_id=WP-O01&package_id="+gb)
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &sites) != nil || sites.Total != 2 {
		t.Fatalf("하부 업무 사이트: %d %s", rec.Code, rec.Body.String())
	}
	var hs repository.PackageSiteItem
	for _, it := range sites.Items {
		if it.ID == cids[0] {
			hs = it
		}
	}
	if hs.Name != "홍성고등학교" || hs.Type != "school" || hs.Sigungu != "홍성군" {
		t.Fatalf("항목 %+v", hs)
	}
	// 다른 사업의 하부 업무는 조회 불가
	if rec = doGet(t, e, "/workboard/package-sites?project_id=WP-O03&package_id="+gb); rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), cids[0]) {
		t.Fatal("다른 사업으로 하부 업무 사이트가 보인다")
	}

	create := func(extra url.Values) *httptest.ResponseRecorder {
		v := url.Values{
			"work_type": {"project_work"}, "title": {"하부업무등록"}, "work_subtype": {"build"}, "project_id": {"WP-O01"},
			"work_date": {"2026-10-21"}, "assignee": {"관리자"}, "start_time": {"09:00"}, "end_time": {"09:30"},
		}
		for k, a := range extra {
			v[k] = a
		}
		return doForm(t, e, "/workboard/tasks", v)
	}
	// 하부 업무 없이 → 거절
	if r := create(url.Values{"site_id": {cids[0]}}); r.Code != http.StatusBadRequest || !strings.Contains(r.Body.String(), "하부 업무") {
		t.Fatalf("하부 업무 없이 저장 %d %s", r.Code, clipBody(r.Body.String()))
	}
	// 하부 업무에 없는 사이트 → 거절
	if _, err := repo.DB().Exec(`INSERT INTO customers(customer_id, org_name, official_name, is_active, org_id) VALUES ('C-out','밖학교','밖학교',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB().Exec(`INSERT INTO project_sites(id, project_id, customer_id, site_name, region, status, org_id, is_active) VALUES ('S-out','WP-O01','C-out','밖학교','충남','planned',?,1)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if r := create(url.Values{"package_id": {gb}, "site_id": {cids[0], "C-out"}}); r.Code != http.StatusBadRequest || !strings.Contains(r.Body.String(), "등록되지 않은 사이트") {
		t.Fatalf("하부 업무 밖 사이트 %d %s", r.Code, clipBody(r.Body.String()))
	}
	if ts, _ := repo.ListTasks(); len(ts) != 0 {
		t.Fatalf("거절됐는데 업무 %d건이 만들어졌다", len(ts))
	}
	// 정상: 하부 업무 + 그 하부 업무의 사이트 둘
	if r := create(url.Values{"package_id": {gb}, "site_id": {cids[0], cids[1]}}); r.Code != http.StatusSeeOther || strings.Contains(r.Header().Get("Location"), "err=") {
		t.Fatalf("정상 저장 %d loc=%s body=%s", r.Code, r.Header().Get("Location"), clipBody(r.Body.String()))
	}
	ts, _ := repo.ListTasks()
	if len(ts) != 1 {
		t.Fatalf("업무 %d건", len(ts))
	}
	if pkg, _ := repo.TaskPlanIDs(ts[0].TaskID); pkg != gb {
		t.Fatalf("하부 업무 연결 %q", pkg)
	}
	if got, _ := repo.ListTaskSites(ts[0].TaskID); len(got) != 2 {
		t.Fatalf("대상 사이트 %d곳", len(got))
	}
	// 수행 구조가 없는 사업은 하부 업무 없이도 저장된다(기존 로직 그대로)
	v := url.Values{"work_type": {"project_work"}, "title": {"구조없음"}, "work_subtype": {"build"}, "project_id": {"WP-O03"},
		"work_date": {"2026-10-22"}, "assignee": {"관리자"}, "start_time": {"10:00"}, "end_time": {"10:30"}}
	if r := doForm(t, e, "/workboard/tasks", v); r.Code != http.StatusSeeOther || strings.Contains(r.Header().Get("Location"), "err=") {
		t.Fatalf("구조 없는 사업 저장 %d loc=%s", r.Code, r.Header().Get("Location"))
	}
}

// 등록 화면(작업·행정/지원 모달)에 공통 하부 업무·사이트 선택이 들어 있다. 95-182.
func Test95_182_RegisterPageHasPackagePicker(t *testing.T) {
	e, _, _ := newProjectWorkServer(t)
	body := doGet(t, e, "/workboard/register").Body.String()
	if strings.Count(body, "data-package-picker") < 2 {
		t.Fatalf("작업·행정/지원 두 폼에 공통 선택이 있어야 한다(%d)", strings.Count(body, "data-package-picker"))
	}
	for _, want := range []string{"data-package-select", "data-package-types", "data-package-sido", "data-package-sigungu", "data-package-grid", "package_site_picker.js"} {
		if !strings.Contains(body, want) {
			t.Fatalf("화면에 %q 없음", want)
		}
	}
}

func Test95_182_JSUnit(t *testing.T) {
	cmd := exec.Command("node", "package_site_picker_test.js")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("js: %v\n%s", err, out)
	}
}
