package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

func TestRegisterWorkBatchASSeparateAndJoint(t *testing.T) {
	e, wb := newWorkboardServer(t, "batch-as.db")
	db := wb.DB()
	if _, err := db.Exec(`INSERT INTO customers(customer_id, org_name, official_name, is_active, org_id)
		VALUES ('C-b1','학교A','학교A',1,?),('C-b2','학교B','학교B',1,?),('C-b3','학교C','학교C',1,?)`,
		model.OrgIDLibrary, model.OrgIDLibrary, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Format("2006-01-02")
	for i, id := range []string{"AS-B1", "AS-B2", "AS-B3"} {
		cust := []string{"C-b1", "C-b2", "C-b3"}[i]
		if _, err := db.Exec(`INSERT INTO as_receipts(as_id, as_number, receipt_datetime, customer_id, symptom, status, assigned_to, org_id, created_at, updated_at)
			VALUES (?,?,datetime('now'),?, '증상','received','양기헌',?, datetime('now'), datetime('now'))`, id, "N-"+id, cust, model.OrgIDLibrary); err != nil {
			t.Fatal(err)
		}
	}
	form := url.Values{}
	form.Set("work_type", model.WBWorkAS)
	form.Set("source_id", "AS-B1")
	form.Set("multi_site_mode", "separate")
	form.Set("work_date", now)
	form.Set("start_time", "16:10")
	form.Set("end_time", "16:40")
	form.Set("assignee", "양기헌")
	form.Set("redirect", "/workboard/register")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/workboard/tasks", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set("Accept", "application/json")
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusSeeOther {
		t.Fatalf("separate code %d body %s loc=%s", rec.Code, rec.Body.String(), rec.Header().Get("Location"))
	}
	if rec.Code == http.StatusOK {
		var res WorkBatchResult
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
		if len(res.Created) != 1 {
			t.Fatalf("created %d skipped %v failed %v body %s", len(res.Created), res.Skipped, res.Failed, rec.Body.String())
		}
	}

	form2 := url.Values{}
	form2.Set("work_type", model.WBWorkAS)
	form2.Set("source_id", "AS-B1")
	form2.Add("extra_as_id", "AS-B2")
	form2.Set("multi_site_mode", "joint")
	form2.Set("work_date", now)
	form2.Set("start_time", "11:00")
	form2.Set("end_time", "12:00")
	form2.Set("assignee", "양기헌")
	form2.Set("redirect", "/workboard/register")
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "http://localhost/workboard/tasks", strings.NewReader(form2.Encode()))
	req2.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req2.Header.Set("Accept", "application/json")
	req2.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec2, req2)
	var res2 WorkBatchResult
	_ = json.Unmarshal(rec2.Body.Bytes(), &res2)
	if len(res2.Created) != 0 && len(res2.Skipped) == 0 {
		t.Fatalf("이미 등록된 접수는 건너뛰어야 함: %+v", res2)
	}

	if _, err := db.Exec(`INSERT INTO as_receipts(as_id, as_number, receipt_datetime, customer_id, symptom, status, assigned_to, org_id, created_at, updated_at)
		VALUES ('AS-J1','NJ1',datetime('now'),'C-b1','증','received','양기헌',?, datetime('now'), datetime('now')),
		       ('AS-J2','NJ2',datetime('now'),'C-b2','증','received','양기헌',?, datetime('now'), datetime('now'))`, model.OrgIDLibrary, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	form3 := url.Values{}
	form3.Set("work_type", model.WBWorkAS)
	form3.Set("source_id", "AS-J1")
	form3.Add("extra_as_id", "AS-J2")
	form3.Set("multi_site_mode", "joint")
	form3.Set("work_date", now)
	form3.Set("start_time", "13:00")
	form3.Set("end_time", "14:00")
	form3.Set("assignee", "양기헌")
	form3.Set("redirect", "/workboard/register")
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodPost, "http://localhost/workboard/tasks", strings.NewReader(form3.Encode()))
	req3.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req3.Header.Set("Accept", "application/json")
	req3.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec3, req3)
	var res3 WorkBatchResult
	if err := json.Unmarshal(rec3.Body.Bytes(), &res3); err != nil {
		t.Fatal(rec3.Body.String())
	}
	if len(res3.Created) != 1 {
		t.Fatalf("joint created %d %+v %s", len(res3.Created), res3, rec3.Body.String())
	}
	if existing, _ := wb.GetTaskBySource(model.WBWorkAS, "AS-J2"); existing != nil {
		t.Fatal("joint 는 나머지 접수 업무를 만들면 안 됨")
	}
}
