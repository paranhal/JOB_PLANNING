package handler

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

func doMultipartJSONWith(t *testing.T, e *echo.Echo, path string, form url.Values, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for k, vs := range form {
		for _, v := range vs {
			if err := w.WriteField(k, v); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, &body)
	req.Header.Set(echo.HeaderContentType, w.FormDataContentType())
	req.Header.Set(echo.HeaderAccept, echo.MIMEApplicationJSON)
	req.Header.Set("X-Requested-With", "fetch")
	if ck != nil {
		req.AddCookie(ck)
	}
	e.ServeHTTP(rec, req)
	return rec
}

func Test95_141B_AddSchoolsMultipartAndEmpty(t *testing.T) {
	e, _, _, db := newProjectWorkHarness(t)
	h := New(db)
	mount119(e, h)
	if _, err := db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id,party_kind)
		VALUES('C-mp-1','멀티1','멀티1',1,?,'customer'),
		      ('C-mp-2','멀티2','멀티2',1,?,'customer'),
		      ('C-mp-3','멀티3','멀티3',1,?,'customer')`,
		model.OrgIDLibrary, model.OrgIDLibrary, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE customers SET org_id='O03' WHERE customer_id='C-oth'`); err != nil {
		t.Fatal(err)
	}

	mp := doMultipartJSONWith(t, e, "/projects/WP-O01/sites/add-schools", url.Values{
		"customer_id": {"C-mp-1", "C-mp-2", "C-mp-3"},
	}, jwtCookie(t))
	if mp.Code != http.StatusOK {
		t.Fatalf("multipart %d %s", mp.Code, mp.Body.String())
	}
	var out map[string]interface{}
	if err := json.Unmarshal(mp.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["added"] != float64(3) || out["skipped"] != float64(0) {
		t.Fatalf("multipart 응답 %+v", out)
	}
	ids, _ := out["addedIDs"].([]interface{})
	if len(ids) != 3 {
		t.Fatalf("addedIDs %+v", out["addedIDs"])
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM project_sites WHERE project_id='WP-O01' AND customer_id IN ('C-mp-1','C-mp-2','C-mp-3') AND is_active=1`).Scan(&n)
	if n != 3 {
		t.Fatalf("DB 행 %d", n)
	}

	if _, err := db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id,party_kind)
		VALUES('C-ue-1','유알엘1','유알엘1',1,?,'customer')`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	ue := doFormJSONWith(t, e, "/projects/WP-O01/sites/add-schools", url.Values{"customer_id": {"C-ue-1"}}, jwtCookie(t))
	if ue.Code != http.StatusOK {
		t.Fatalf("urlencoded %d %s", ue.Code, ue.Body.String())
	}

	empty := doMultipartJSONWith(t, e, "/projects/WP-O01/sites/add-schools", url.Values{}, jwtCookie(t))
	if empty.Code != http.StatusBadRequest {
		t.Fatalf("0건 %d %s", empty.Code, empty.Body.String())
	}
	if !bytes.Contains(empty.Body.Bytes(), []byte("선택된 항목이 없습니다")) {
		t.Fatalf("0건 문구 %s", empty.Body.String())
	}

	before := 0
	_ = db.QueryRow(`SELECT COUNT(*) FROM project_sites WHERE project_id='WP-O01' AND is_active=1`).Scan(&before)
	mix := doMultipartJSONWith(t, e, "/projects/WP-O01/sites/add-schools", url.Values{
		"customer_id": {"C-mp-1", "C-oth"},
	}, jwtCookie(t))
	if mix.Code != http.StatusOK {
		t.Fatalf("타조직 %d %s", mix.Code, mix.Body.String())
	}
	after := 0
	_ = db.QueryRow(`SELECT COUNT(*) FROM project_sites WHERE project_id='WP-O01' AND is_active=1`).Scan(&after)
	if after != before+1 {
		t.Fatalf("타조직 포함 추가 %d→%d", before, after)
	}
	var siteOrg, custOrg string
	if err := db.QueryRow(`SELECT org_id FROM project_sites WHERE project_id='WP-O01' AND customer_id='C-oth'`).Scan(&siteOrg); err != nil || siteOrg != model.OrgIDLibrary {
		t.Fatalf("사이트 조직 %s %v", siteOrg, err)
	}
	if err := db.QueryRow(`SELECT org_id FROM customers WHERE customer_id='C-oth'`).Scan(&custOrg); err != nil || custOrg != "O03" {
		t.Fatalf("고객 조직 %s %v", custOrg, err)
	}
}

func Test95_141B_MonthTargetsMultipart(t *testing.T) {
	e, repo, p := newMntManageApp(t)
	db := repo.DB()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, addr_sido, addr_sigungu)
		VALUES ('mp-cn','멀티도서관','멀티도서관',1,?,'충청남도','천안시')`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "mp-cn", ShortName: "멀티", Region: "충남", InspectionCycle: "semi", OrgID: model.OrgIDLibrary, HasKlas: true,
	}); err != nil {
		t.Fatal(err)
	}
	ok := doMultipartJSONWith(t, e, "/maintenance/"+p.PlanID+"/month-targets", url.Values{
		"month": {"2"}, "source": {"site"}, "customer_id": {"mp-cn"},
	}, jwtCookie(t))
	if ok.Code != http.StatusOK {
		t.Fatalf("month-targets multipart %d %s", ok.Code, ok.Body.String())
	}
	var out map[string]interface{}
	if err := json.Unmarshal(ok.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["ok"] != true {
		t.Fatalf("month-targets 응답 %+v", out)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM maintenance_month_targets WHERE customer_id='mp-cn'`).Scan(&n)
	if n != 1 {
		t.Fatalf("month_targets 행 %d", n)
	}
}
