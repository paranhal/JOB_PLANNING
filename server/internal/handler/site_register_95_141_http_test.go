package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_141_SiteCandidatesFourTypesPageAndRegistered(t *testing.T) {
	e, _, _, db := newProjectWorkHarness(t)
	h := New(db)
	mount119(e, h)
	if _, err := db.Exec(`UPDATE customers SET industry='학교' WHERE customer_id IN ('C-high','C-mid','C-pct')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id,party_kind,industry,addr_sido,addr_sigungu)
		VALUES('C-pub1','공공A','공공A',1,?,'customer','공공기관','충남','천안시'),
		      ('C-lib1','도서관A','도서관A',1,?,'customer','도서관','충남','아산시'),
		      ('C-prv1','민간A','민간A',1,?,'customer','민간','대전','서구'),
		      ('C-emp1','빈업종','빈업종',1,?,'customer','','충남','계룡시')`,
		model.OrgIDLibrary, model.OrgIDLibrary, model.OrgIDLibrary, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,customer_id,site_name,region,status,org_id,is_active)
		VALUES('S-pub','WP-O01','C-pub1','공공A','충남','in_progress',?,1)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}

	mustID := func(typ, id string, registered *bool) repository.SchoolSiteCandidate {
		t.Helper()
		cand := doGet(t, e, "/projects/WP-O01/sites/candidates?type="+typ)
		if cand.Code != http.StatusOK {
			t.Fatalf("%s %d %s", typ, cand.Code, cand.Body.String())
		}
		var res repository.SchoolSiteCandidateResult
		if err := json.Unmarshal(cand.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
		for _, it := range res.Items {
			if it.ID == id {
				if registered != nil && it.Registered != *registered {
					t.Fatalf("%s %s registered=%v want %v", typ, id, it.Registered, *registered)
				}
				return it
			}
		}
		t.Fatalf("%s 후보에 %s 없음 total=%d", typ, id, res.Total)
		return repository.SchoolSiteCandidate{}
	}
	absent := func(typ, id string) {
		t.Helper()
		cand := doGet(t, e, "/projects/WP-O01/sites/candidates?type="+typ)
		var res repository.SchoolSiteCandidateResult
		_ = json.Unmarshal(cand.Body.Bytes(), &res)
		for _, it := range res.Items {
			if it.ID == id {
				t.Fatalf("%s 에 %s 있으면 안 됨", typ, id)
			}
		}
	}

	mustID("school", "C-high", nil)
	mustID("public", "C-pub1", boolPtr(true))
	mustID("library", "C-lib1", nil)
	mustID("private", "C-prv1", nil)
	mustID("private", "C-emp1", nil)
	absent("school", "C-pub1")
	absent("public", "C-lib1")
	absent("library", "C-prv1")
	absent("private", "C-high")

	school := doGet(t, e, "/projects/WP-O01/sites")
	if school.Code != http.StatusOK {
		t.Fatalf("page %d", school.Code)
	}
	sb := school.Body.String()
	if strings.Contains(sb, "화면 준비중") {
		t.Fatal("준비중")
	}
	for _, need := range []string{"선택 학교", "선택 기관", "지역 검색", "지역 및 이름 검색", "조건에 맞는 학교가 없습니다", "조건에 맞는 기관이 없습니다"} {
		if !strings.Contains(sb, need) {
			t.Fatalf("없음 %q", need)
		}
	}
	lib := doGet(t, e, "/projects/WP-O01/sites?type=library")
	if lib.Code != http.StatusOK || !strings.Contains(lib.Body.String(), `data-site-type="library"`) {
		t.Fatal("도서관 유형")
	}
	if strings.Contains(lib.Body.String(), "화면 준비중") {
		t.Fatal("도서관 준비중")
	}
	weird := doGet(t, e, "/projects/WP-O01/sites?type=other")
	if !strings.Contains(weird.Body.String(), `data-site-type="school"`) {
		t.Fatal("그 외 school")
	}
}

func boolPtr(v bool) *bool { return &v }
