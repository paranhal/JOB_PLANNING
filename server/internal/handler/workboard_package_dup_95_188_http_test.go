package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-188 A: package_id 가 여러 개 와도(빈 값이 먼저) 비어 있지 않은 첫 값을 쓴다.
func TestHTTP95_188_PostedPackageIDFirstNonEmpty(t *testing.T) {
	e, db, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "p188h.db"))
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id, is_test, is_paid) VALUES ('WP-O01','도서관사업','active','O01',0,1)`); err != nil {
		t.Fatal(err)
	}
	seedBadgeData(t, db, "WP-O01", "O01")
	pr := repository.NewProjectRepo(db)
	choices, _ := pr.PackageChoices("WP-O01")
	if len(choices) == 0 {
		t.Fatal("하부 업무 없음")
	}
	pkg := choices[0].ID
	if _, err := pr.CreatePlan("WP-O01", repository.PlanDraft{PackageID: pkg, Name: "1차 계획", PlanStart: "2027-04-01", PlanEnd: "2027-05-01", Basis: model.BasisSite, Actor: "t"}); err != nil {
		t.Fatal(err)
	}
	_ = e
	_ = me
	h := New(db)
	check := func(vals url.Values) error {
		req := httptest.NewRequest(http.MethodPost, "/workboard/tasks", strings.NewReader(vals.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		c := echo.New().NewContext(req, httptest.NewRecorder())
		return h.Workboard.checkPostedPackageSites(c, "WP-O01")
	}
	// 빈 값이 먼저, 고른 값이 뒤 → 오류 없음(수정 전에는 「하부 업무를 선택하세요」)
	v := url.Values{}
	v["package_id"] = []string{"", pkg}
	v["plan_id"] = []string{""}
	if err := check(v); err != nil {
		t.Fatalf("빈 package_id 가 먼저 읽혀 오류: %v", err)
	}
	// 둘 다 비면 기존대로 오류
	v = url.Values{}
	v["package_id"] = []string{"", ""}
	if err := check(v); err == nil || !strings.Contains(err.Error(), "하부 업무를 선택하세요") {
		t.Fatalf("둘 다 비면 오류여야 한다: %v", err)
	}
	// 값 하나만 와도 그대로 통과
	if err := check(url.Values{"package_id": {pkg}}); err != nil {
		t.Fatal(err)
	}
}
