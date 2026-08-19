package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestMeetingHTTP_YesterdayDoneTodayScheduled(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "meeting_http.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	users := repository.NewUserRepo(db)
	_ = users.Create(&model.User{Username: "yang", PasswordHash: "x", FullName: "양기헌", Role: model.RoleTech, IsActive: true})
	_ = users.Create(&model.User{Username: "choi", PasswordHash: "x", FullName: "최혜영", Role: model.RoleTech, IsActive: true})

	anchor := time.Date(2026, 8, 11, 0, 0, 0, 0, time.Local)
	today := anchor.Format("2006-01-02")
	yesterday := anchor.AddDate(0, 0, -1).Format("2006-01-02")

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','회의도서관','회의도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
		schedule_confirmed, status, assigned_to, complete_datetime, symptom, updated_at
	) VALUES
		('as-y','R2608-Y01','c1','2026-08-01',?,1,'completed','양기헌',?,'어제완료',?),
		('as-t','R2608-T01','c1','2026-08-05',?,1,'in_progress','양기헌',NULL,'오늘예정',?),
		('as-c','R2608-C01','c1','2026-08-05',?,1,'in_progress','최혜영',NULL,'다른담당',?)`,
		yesterday, yesterday+" 16:00:00", yesterday+" 16:00:00",
		today, today+" 09:00:00",
		today, today+" 09:00:00"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status)
		VALUES ('mp1', 2026, '계획', 'approved')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits (
		visit_id, plan_id, visit_date, customer_id, sort_order, completed, product_type
	) VALUES ('mv1','mp1',?, 'c1',1,0,'앤로보틱스')`, today); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/meeting", h.Meeting.Show)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/meeting?date="+today, nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		snippet := rec.Body.String()
		if len(snippet) > 400 {
			snippet = snippet[:400]
		}
		t.Fatalf("status=%d body=%s", rec.Code, snippet)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"일일 업무 회의", "전일 실적", "오늘 예정",
		"R2608-Y01", "R2608-T01", "회의도서관",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in body", want)
		}
	}
	if !strings.Contains(body, "/as/as-y") || !strings.Contains(body, "/as/as-t") {
		t.Fatal("AS 상세 링크 없음")
	}
	if !strings.Contains(body, "/maintenance/mp1") {
		t.Fatal("정기점검 링크 없음")
	}
	if !strings.Contains(body, "팀전체") || !strings.Contains(body, "양기헌") {
		t.Fatal("담당자 드롭다운이 없다")
	}
	if strings.Contains(body, "팀전체 보기") || strings.Contains(body, "내 업무만") {
		t.Fatal("내 것/전체 토글이 남아 있다")
	}
	if !strings.Contains(body, "양기헌 · ") || !strings.Contains(body, "최혜영 · ") {
		t.Fatal("담당자 소제목 줄이 없다")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "http://localhost/meeting?date="+today+"&assignee="+url.QueryEscape("양기헌"), nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("filter status=%d", rec.Code)
	}
	filtered := rec.Body.String()
	if !strings.Contains(filtered, "R2608-T01") {
		t.Fatal("양기헌 예정이 없다")
	}
	if strings.Contains(filtered, "R2608-C01") {
		t.Fatal("다른 담당자 건이 필터에 남았다")
	}
	if !strings.Contains(filtered, "assignee=") {
		t.Fatal("날짜 이동 링크에 담당자 선택이 없다")
	}
	if !strings.Contains(filtered, "양기헌 기준") {
		t.Fatal("상단 범위 안내가 담당자 기준이 아니다")
	}

	techTok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": "yang-id", "username": "yang", "role": "tech",
		"name": "양기헌", "exp": time.Now().Add(time.Hour).Unix(),
	})
	techStr, err := techTok.SignedString([]byte("cs-system-jwt-secret-2026"))
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "http://localhost/meeting?date="+today, nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: techStr, Path: "/"})
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("tech status=%d", rec.Code)
	}
	techBody := rec.Body.String()
	if !strings.Contains(techBody, "양기헌 기준") {
		t.Fatal("기술담당 기본이 본인 기준이 아니다")
	}
	if strings.Contains(techBody, "R2608-C01") {
		t.Fatal("기술담당 기본 보기에 다른 사람 건이 있다")
	}
	if !strings.Contains(techBody, `option value="all"`) {
		t.Fatal("기술담당도 팀전체를 고를 수 있어야 한다")
	}
}
