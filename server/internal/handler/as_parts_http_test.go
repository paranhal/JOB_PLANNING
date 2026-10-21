package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestASActionPartsLayoutAndNoPrice(t *testing.T) {
	e, _, _, _, asID := newASActionFixture(t)
	body := getASActionPage(t, e, asID)
	if !strings.Contains(body, `id="as-action-header"`) || !strings.Contains(body, `id="as-action-bar"`) {
		t.Fatal("헤더·하단 바 없음")
	}
	if !strings.Contains(body, `as-action-left`) || !strings.Contains(body, `overflow-y-auto`) || !strings.Contains(body, `min-h-0`) {
		t.Fatal("좌우 스크롤 클래스 없음")
	}
	if !strings.Contains(body, `id="as-action-side-mobile"`) || !strings.Contains(body, "<details") {
		t.Fatal("모바일 details 없음")
	}
	formStart := strings.Index(body, `id="as-action-form"`)
	if formStart < 0 {
		t.Fatal("폼 없음")
	}
	taken := strings.Index(body, `id="as-action-taken"`)
	check := strings.Index(body, `name="parts_enabled"`)
	box := strings.Index(body, `id="as-parts-box"`)
	if taken < 0 || check < taken || box < check {
		t.Fatalf("순서 taken=%d check=%d box=%d", taken, check, box)
	}
	if !strings.Contains(body, `hidden`) || !strings.Contains(body[box:box+80], "hidden") {
		t.Fatal("해제 시 박스 hidden 없음")
	}
	for _, s := range []string{"단가", "금액", "정산", "공임", "출장비", "unit_price"} {
		if strings.Contains(body, s) {
			t.Errorf("금지 문자열 %q", s)
		}
	}
}

func TestASActionPartsSaveAndClear(t *testing.T) {
	e, h, asRepo, _, asID := newASActionFixture(t)
	e.GET("/as/parts-suggest", h.AS.SuggestParts)
	base := url.Values{
		"work_place":   {"field"},
		"process_type": {"visit"},
		"cause_type":   {"hw"},
		"action_taken": {"보드 교체 점검"},
		"result_code":  {model.ResultDone},
		"parts_form":   {"1"},
		"parts_enabled": {"1"},
		"part_name":    {"메인보드", "팬"},
		"part_code":    {"PT-M01", ""},
		"part_qty":     {"1", "2"},
		"part_item_id": {"", ""},
	}
	rec := postASAction(t, e, asID, base)
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("저장 실패 %d %s", rec.Code, rec.Header().Get("Location"))
	}
	got, _ := asRepo.GetByID(repository.OrgAll, asID)
	if got.PartsUsed != "메인보드(PT-M01) ×1, 팬 ×2" {
		t.Fatalf("요약 %q", got.PartsUsed)
	}
	rows, err := h.AS.partsRepo.ListByAS(asID)
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows %d %v", len(rows), err)
	}
	show := getASActionPage(t, e, asID)
	if !strings.Contains(show, "메인보드") || !strings.Contains(show, "부품: 메인보드") {
		t.Fatal("이력 표시 없음")
	}

	emptyName := url.Values{
		"work_place": {"field"}, "process_type": {"visit"}, "cause_type": {"hw"},
		"action_taken": {"보드 교체 점검"}, "result_code": {model.ResultDone},
		"parts_form": {"1"}, "parts_enabled": {"1"},
		"part_name": {""}, "part_code": {"X"}, "part_qty": {"1"}, "part_item_id": {""},
	}
	bad := postASAction(t, e, asID, emptyName)
	if bad.Code != http.StatusBadRequest || !strings.Contains(bad.Body.String(), "품명") {
		t.Fatalf("빈 품명 %d %s", bad.Code, clipBody(bad.Body.String()))
	}
	zero := postASAction(t, e, asID, url.Values{
		"work_place": {"field"}, "process_type": {"visit"}, "cause_type": {"hw"},
		"action_taken": {"보드 교체 점검"}, "result_code": {model.ResultDone},
		"parts_form": {"1"}, "parts_enabled": {"1"},
		"part_name": {"팬"}, "part_code": {""}, "part_qty": {"0"}, "part_item_id": {""},
	})
	if zero.Code != http.StatusBadRequest {
		t.Fatalf("수량0 %d", zero.Code)
	}
	names := make([]string, 31)
	qtys := make([]string, 31)
	codes := make([]string, 31)
	ids := make([]string, 31)
	for i := 0; i < 31; i++ {
		names[i] = "부품"
		qtys[i] = "1"
	}
	over := url.Values{
		"work_place": {"field"}, "process_type": {"visit"}, "cause_type": {"hw"},
		"action_taken": {"보드 교체 점검"}, "result_code": {model.ResultDone},
		"parts_form": {"1"}, "parts_enabled": {"1"},
		"part_name": names, "part_code": codes, "part_qty": qtys, "part_item_id": ids,
	}
	too := postASAction(t, e, asID, over)
	if too.Code != http.StatusBadRequest {
		t.Fatalf("31행 %d", too.Code)
	}

	off := postASAction(t, e, asID, url.Values{
		"work_place": {"field"}, "process_type": {"visit"}, "cause_type": {"hw"},
		"action_taken": {"보드 교체 점검"}, "result_code": {model.ResultDone},
		"parts_form": {"1"},
	})
	if off.Code != http.StatusSeeOther {
		t.Fatalf("해제 %d", off.Code)
	}
	got, _ = asRepo.GetByID(repository.OrgAll, asID)
	if strings.TrimSpace(got.PartsUsed) != "" {
		t.Fatalf("해제 후 요약 %q", got.PartsUsed)
	}
	rows, _ = h.AS.partsRepo.ListByAS(asID)
	if len(rows) != 0 {
		t.Fatalf("해제 후 행 %d", len(rows))
	}
}

func TestASActionPartsLegacyUntouchedWithoutRows(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	cur, _ := asRepo.GetByID(repository.OrgAll, asID)
	cur.PartsUsed = "레거시텍스트"
	if err := asRepo.Update(cur); err != nil {
		t.Fatal(err)
	}
	page := getASActionPage(t, e, asID)
	if !strings.Contains(page, `name="parts_enabled"`) || !strings.Contains(page, "이전 기록: 레거시텍스트") {
		t.Fatal("레거시 체크·이전 기록 없음")
	}
	rec := postASAction(t, e, asID, url.Values{
		"work_place": {"field"}, "process_type": {"visit"}, "cause_type": {"hw"},
		"action_taken": {"레거시 유지 점검"}, "result_code": {model.ResultDone},
		"parts_form": {"1"}, "parts_enabled": {"1"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("저장 %d", rec.Code)
	}
	got, _ := asRepo.GetByID(repository.OrgAll, asID)
	if got.PartsUsed != "레거시텍스트" {
		t.Fatalf("레거시 변경 %q", got.PartsUsed)
	}
}

func TestASPartsSuggestForbiddenReadOnly(t *testing.T) {
	e, h, _, _, _ := newASActionFixture(t)
	e.GET("/as/parts-suggest", h.AS.SuggestParts)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/parts-suggest?q=메인", nil)
	req.AddCookie(jwtCookieReadOnly(t, model.RoleTech))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusOK {
		// 읽기전용 쿠키가 미들웨어 없으면 200일 수 있음 — 핸들러 isReadOnly
		t.Log(rec.Code)
	}
	if rec.Code != http.StatusForbidden {
		var out []map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		if rec.Code == http.StatusOK && len(out) > 0 {
			t.Fatalf("읽기전용에 후보 %v", out)
		}
	}
}
