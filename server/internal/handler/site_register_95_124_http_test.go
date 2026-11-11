package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_124_SiteRegisterPageAndCandidates(t *testing.T) {
	e, _, users, db := newProjectWorkHarness(t)
	h := New(db)
	mount119(e, h)
	if _, err := db.Exec(`UPDATE customers SET industry='학교', addr_sido='충청남도' WHERE customer_id IN ('C-high','C-mid','C-pct','C-und')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE customers SET addr_sigungu='천안시' WHERE customer_id='C-pct'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE customers SET addr_sigungu='계룡시' WHERE customer_id IN ('C-high','C-mid')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE customers SET industry='학교', org_id='O03' WHERE customer_id='C-oth'`); err != nil {
		t.Fatal(err)
	}

	page := doGet(t, e, "/projects/WP-O01/sites")
	if page.Code != http.StatusOK {
		t.Fatalf("page %d", page.Code)
	}
	body := page.Body.String()
	for _, need := range []string{"사이트 등록", "등록된 사이트", "등록 장비", "유형", "시도지역", "시군 지역", "지역 검색", "선택 학교", `id="btn-site-reg"`, "overflow-x-hidden"} {
		if !strings.Contains(body, need) {
			t.Fatalf("없음 %q", need)
		}
	}
	if !strings.Contains(body, "학교") || !strings.Contains(body, "공공기관") || !strings.Contains(body, "도서관") || !strings.Contains(body, "민간업체") {
		t.Fatal("유형 버튼")
	}
	if strings.Contains(body, "엑셀 일괄 등록") || strings.Contains(body, "장비 등록") || strings.Contains(body, "사이트 저장") {
		t.Fatal("예전 UI")
	}
	if !strings.Contains(body, `data-site-type="school"`) {
		t.Fatal("기본 유형 학교")
	}

	cand := doGet(t, e, "/projects/WP-O01/sites/candidates?type=school")
	if cand.Code != http.StatusOK {
		t.Fatalf("candidates %d %s", cand.Code, cand.Body.String())
	}
	var res repository.SchoolSiteCandidateResult
	if err := json.Unmarshal(cand.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Total < 3 {
		t.Fatalf("후보 %d %+v", res.Total, res.Items)
	}
	byID := map[string]repository.SchoolSiteCandidate{}
	for _, it := range res.Items {
		byID[it.ID] = it
	}
	if it, ok := byID["C-oth"]; !ok || it.Registered {
		t.Fatalf("타조직 고객도 후보 %+v", it)
	}
	if !byID["C-high"].Registered {
		t.Fatal("등록됨 배지 원본")
	}
	if byID["C-pct"].Registered || byID["C-pct"].Sido != "충남" || byID["C-pct"].Sigungu != "천안시" {
		t.Fatalf("천안 %+v", byID["C-pct"])
	}
	if _, ok := byID["C-pct"]; !ok {
		t.Fatal("상위 없는 학교")
	}

	q := doGet(t, e, "/projects/WP-O01/sites/candidates?type=school&q="+url.QueryEscape("천안"))
	var qres repository.SchoolSiteCandidateResult
	_ = json.Unmarshal(q.Body.Bytes(), &qres)
	if qres.Total != 1 || qres.Items[0].ID != "C-pct" {
		t.Fatalf("천안 검색 %+v", qres)
	}

	pub := doGet(t, e, "/projects/WP-O01/sites?type=public")
	if pub.Code != http.StatusOK || !strings.Contains(pub.Body.String(), `data-site-type="public"`) {
		t.Fatal("유형 쿼리")
	}

	add := doFormJSONWith(t, e, "/projects/WP-O01/sites/add-schools", url.Values{"customer_id": {"C-pct"}}, jwtCookie(t))
	if add.Code != http.StatusOK {
		t.Fatalf("등록 %d %s", add.Code, add.Body.String())
	}
	var out map[string]interface{}
	_ = json.Unmarshal(add.Body.Bytes(), &out)
	if out["added"] != float64(1) {
		t.Fatalf("added %+v", out)
	}
	dup := doFormJSONWith(t, e, "/projects/WP-O01/sites/add-schools", url.Values{"customer_id": {"C-pct"}}, jwtCookie(t))
	_ = json.Unmarshal(dup.Body.Bytes(), &out)
	if dup.Code != http.StatusOK || out["added"] != float64(0) || out["skipped"] != float64(1) {
		t.Fatalf("중복 %+v %d", out, dup.Code)
	}
	foreign := doFormJSONWith(t, e, "/projects/WP-O01/sites/add-schools", url.Values{"customer_id": {"C-oth"}}, jwtCookie(t))
	if foreign.Code != http.StatusOK {
		t.Fatalf("타조직 POST %d %s", foreign.Code, foreign.Body.String())
	}
	var siteOrg, custOrg string
	if err := db.QueryRow(`SELECT org_id FROM project_sites WHERE project_id='WP-O01' AND customer_id='C-oth'`).Scan(&siteOrg); err != nil || siteOrg != model.OrgIDLibrary {
		t.Fatalf("사이트 조직 %s %v", siteOrg, err)
	}
	if err := db.QueryRow(`SELECT org_id FROM customers WHERE customer_id='C-oth'`).Scan(&custOrg); err != nil || custOrg != "O03" {
		t.Fatalf("고객 조직 %s %v", custOrg, err)
	}

	execU := model.User{Username: "exec124", PasswordHash: HashPassword("password10"), FullName: "수행",
		Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	if err := users.Create(&execU); err != nil {
		t.Fatal(err)
	}
	ex, _ := users.GetByUsername("exec124")
	deny := doFormJSONWith(t, e, "/projects/WP-O01/sites/add-schools", url.Values{"customer_id": {"C-und"}}, jwtCookieUser(t, ex))
	if deny.Code != http.StatusForbidden {
		t.Fatalf("수행 JSON %d %s", deny.Code, deny.Body.String())
	}
	if !strings.Contains(deny.Body.String(), "권한") {
		t.Fatal("실패 메시지")
	}

	libOA := model.User{Username: "oa01lib", PasswordHash: HashPassword("password10"), FullName: "도서관리자",
		Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	if err := users.Create(&libOA); err != nil {
		t.Fatal(err)
	}
	oa1, _ := users.GetByUsername("oa01lib")
	okOA := doFormJSONWith(t, e, "/projects/WP-O01/sites/add-schools", url.Values{"customer_id": {"C-und"}}, jwtCookieUser(t, oa1))
	if okOA.Code != http.StatusOK {
		t.Fatalf("조직관리자 %d %s", okOA.Code, okOA.Body.String())
	}

	too := url.Values{}
	for i := 0; i < 2001; i++ {
		too.Add("customer_id", fmt.Sprintf("C-x%04d", i))
	}
	lim := doFormJSONWith(t, e, "/projects/WP-O01/sites/add-schools", too, jwtCookie(t))
	if lim.Code != http.StatusBadRequest || !strings.Contains(lim.Body.String(), "2,000") {
		t.Fatalf("한도 %d %s", lim.Code, lim.Body.String())
	}

	panel := doGet(t, e, "/projects/WP-O01/sites-panel")
	if panel.Code != http.StatusOK || strings.Contains(panel.Body.String(), "/sites/pick") {
		t.Fatal("패널 링크")
	}
	if !strings.Contains(panel.Body.String(), `/projects/WP-O01/sites"`) && !strings.Contains(panel.Body.String(), `/projects/WP-O01/sites'`) {
		if !strings.Contains(panel.Body.String(), "/projects/WP-O01/sites") {
			t.Fatal("패널 href")
		}
	}

	b, err := os.ReadFile(filepath.Join(findTemplateRoot(t), "project", "show.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "사이트·장비") {
		t.Fatal("상세 구 문구")
	}
}

func Test95_124_ManyCandidates(t *testing.T) {
	e, _, _, db := newProjectWorkHarness(t)
	h := New(db)
	mount119(e, h)
	if _, err := db.Exec(`UPDATE customers SET industry='학교' WHERE org_id=? AND customer_id != 'C-edu'`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 800; i++ {
		id := fmt.Sprintf("C-m%03d", i)
		name := fmt.Sprintf("대량교%03d", i)
		if _, err := db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id,party_kind,industry,addr_sido,addr_sigungu)
			VALUES(?,?,?,1,?,'customer','학교','충남','천안시')`, id, name, name, model.OrgIDLibrary); err != nil {
			t.Fatal(err)
		}
	}
	cand := doGet(t, e, "/projects/WP-O01/sites/candidates?type=school")
	if cand.Code != http.StatusOK {
		t.Fatalf("candidates %d", cand.Code)
	}
	var res repository.SchoolSiteCandidateResult
	if err := json.Unmarshal(cand.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Total < 800 {
		t.Fatalf("total %d", res.Total)
	}
}

func Test95_124_JSAndTemplateContract(t *testing.T) {
	root := findTemplateRoot(t)
	js := string(mustRead(t, filepath.Join(root, "..", "static", "js", "school_site_register.js")))
	if !strings.Contains(js, "ssrSyncRegionsToSearch") || !strings.Contains(js, "등록 실패") || !strings.Contains(js, "등록 결과") {
		t.Fatal("js 모달·검색")
	}
	tpl := string(mustRead(t, filepath.Join(root, "project", "sites.html")))
	if !strings.Contains(tpl, `role="dialog"`) || !strings.Contains(tpl, "닫기") || !strings.Contains(tpl, "max-w-md") {
		t.Fatal("모달 마크업")
	}
	if !strings.Contains(tpl, "overflow-x-hidden") {
		t.Fatal("가로 스크롤 방지")
	}
}
