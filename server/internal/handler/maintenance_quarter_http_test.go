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

func mntPostWithCookie(t *testing.T, e *echo.Echo, path string, form url.Values, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	return rec
}

func TestShowPlanQuarterConfirmBannerAndSkip(t *testing.T) {
	e, db, h, _ := newMntProtectApp(t)
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c_q','법제처','법제처',1)`); err != nil {
		t.Fatal(err)
	}
	repo := h.Maintenance.repo
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c_q", ShortName: "법제처", HasKlas: true, InspectionCycle: "quarterly",
	}); err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}

	rec := mntGet(t, e, "/maintenance/"+p.PlanID+"?view=list&month=10")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "분기 점검 대기") {
		t.Fatalf("10월 안내 없음: %s", body)
	}
	if strings.Contains(body, "분기 점검 확인") {
		t.Fatal("10월에 확인 3입력 박스가 나오면 안 됨")
	}
	if strings.Contains(body, ">분기 점검 미확정<") {
		t.Fatal("10월 미배정 표에 분기 미확정이 올라오면 안 됨")
	}

	skipEarly := mntPostWithCookie(t, e, "/maintenance/"+p.PlanID+"/quarter-confirm", url.Values{
		"action": {"skip"}, "customer_id": {"c_q"}, "product_type": {"KLAS"},
		"quarter": {"4"}, "reason": {"이번 분기는 방문 안 함"}, "month": {"10"}, "view": {"list"},
	}, jwtCookieRole(t, model.RoleTech))
	if skipEarly.Code != http.StatusSeeOther {
		t.Fatalf("skip-early status=%d loc=%s", skipEarly.Code, skipEarly.Header().Get("Location"))
	}
	if !strings.Contains(locText(skipEarly), "err=") {
		t.Fatalf("10월 건너뜀이 막혀야 함 loc=%s", locText(skipEarly))
	}

	skip := mntPostWithCookie(t, e, "/maintenance/"+p.PlanID+"/quarter-confirm", url.Values{
		"action": {"skip"}, "customer_id": {"c_q"}, "product_type": {"KLAS"},
		"quarter": {"4"}, "reason": {"이번 분기는 방문 안 함"}, "month": {"12"}, "view": {"list"},
	}, jwtCookieRole(t, model.RoleTech))
	if skip.Code != http.StatusSeeOther {
		t.Fatalf("skip status=%d loc=%s", skip.Code, skip.Header().Get("Location"))
	}
	if !strings.Contains(locText(skip), "ok=quarter-skip") {
		t.Fatalf("skip loc=%s", locText(skip))
	}

	after := mntGet(t, e, "/maintenance/"+p.PlanID+"?view=list&month=12")
	if strings.Contains(after.Body.String(), "분기 점검 확인") && strings.Contains(after.Body.String(), "법제처") {
		pending, _ := repo.PendingQuarterSlots(p.PlanID, 2026, 4)
		if len(pending) != 0 {
			t.Fatalf("건너뛴 뒤에도 대기=%v body=%s", pending, after.Body.String())
		}
	}
}
