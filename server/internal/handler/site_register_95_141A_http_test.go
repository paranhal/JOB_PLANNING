package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func Test95_141A_AddSitesResponseAndSameName(t *testing.T) {
	e, _, _, db := newProjectWorkHarness(t)
	h := New(db)
	mount119(e, h)
	if _, err := db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id,party_kind,industry,addr_sido,addr_sigungu)
		VALUES('C-lib-a','중앙도서관','중앙도서관',1,?,'customer','도서관','충남','아산시'),
		      ('C-lib-b','중앙도서관','중앙도서관',1,?,'customer','도서관','세종','세종시'),
		      ('C-lib-c','별빛도서관','별빛도서관',1,?,'customer','도서관','충남','천안시')`,
		model.OrgIDLibrary, model.OrgIDLibrary, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE customers SET org_id='O03' WHERE customer_id='C-oth'`); err != nil {
		t.Fatal(err)
	}

	ok := doFormJSONWith(t, e, "/projects/WP-O01/sites/add-schools", url.Values{"customer_id": {"C-lib-a", "C-lib-b", "C-lib-c"}}, jwtCookie(t))
	if ok.Code != http.StatusOK {
		t.Fatalf("정상 %d %s", ok.Code, ok.Body.String())
	}
	var out map[string]interface{}
	_ = json.Unmarshal(ok.Body.Bytes(), &out)
	if out["added"] != float64(3) || out["skipped"] != float64(0) {
		t.Fatalf("정상 응답 %+v", out)
	}
	ids, _ := out["addedIDs"].([]interface{})
	if len(ids) != 3 {
		t.Fatalf("addedIDs %+v", out["addedIDs"])
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM project_sites WHERE project_id='WP-O01' AND customer_id IN ('C-lib-a','C-lib-b','C-lib-c') AND is_active=1`).Scan(&n)
	if n != 3 {
		t.Fatalf("DB 행 %d", n)
	}
	var nm string
	_ = db.QueryRow(`SELECT site_name FROM project_sites WHERE project_id='WP-O01' AND customer_id='C-lib-b'`).Scan(&nm)
	if !strings.Contains(nm, "세종") {
		t.Fatalf("동명 구분 이름 %s", nm)
	}

	dup := doFormJSONWith(t, e, "/projects/WP-O01/sites/add-schools", url.Values{"customer_id": {"C-lib-a"}}, jwtCookie(t))
	_ = json.Unmarshal(dup.Body.Bytes(), &out)
	if dup.Code != http.StatusOK || out["added"] != float64(0) || out["skipped"] != float64(1) {
		t.Fatalf("이미 등록 %+v %d", out, dup.Code)
	}
	sk, _ := out["skippedItems"].([]interface{})
	if len(sk) != 1 {
		t.Fatalf("skippedItems %+v", out["skippedItems"])
	}
	row, _ := sk[0].(map[string]interface{})
	if row["reason"] != "이미 이 사업에 등록됨" {
		t.Fatalf("이미 등록 사유 %+v", row)
	}

	before := 0
	_ = db.QueryRow(`SELECT COUNT(*) FROM project_sites WHERE project_id='WP-O01' AND is_active=1`).Scan(&before)
	mix := doFormJSONWith(t, e, "/projects/WP-O01/sites/add-schools", url.Values{"customer_id": {"C-lib-c", "C-oth"}}, jwtCookie(t))
	if mix.Code != http.StatusOK {
		t.Fatalf("타조직 %d %s", mix.Code, mix.Body.String())
	}
	_ = json.Unmarshal(mix.Body.Bytes(), &out)
	if out["added"] != float64(1) || out["skipped"] != float64(1) {
		t.Fatalf("이미 등록+타조직 %+v", out)
	}
	after := 0
	_ = db.QueryRow(`SELECT COUNT(*) FROM project_sites WHERE project_id='WP-O01' AND is_active=1`).Scan(&after)
	if after != before+1 {
		t.Fatalf("타조직 추가 %d→%d", before, after)
	}
	var siteOrg string
	if err := db.QueryRow(`SELECT org_id FROM project_sites WHERE project_id='WP-O01' AND customer_id='C-oth'`).Scan(&siteOrg); err != nil || siteOrg != model.OrgIDLibrary {
		t.Fatalf("사이트 조직 %s %v", siteOrg, err)
	}
}
