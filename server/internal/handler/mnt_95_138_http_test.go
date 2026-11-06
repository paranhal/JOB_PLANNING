package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func Test95_138PlanShowEditKeepsModalAndNoItems0(t *testing.T) {
	root := findTemplateRoot(t)
	plan := string(mustRead(t, filepath.Join(root, "maintenance", "plan_show.html")))
	base := string(mustRead(t, filepath.Join(root, "layout", "base.html")))
	combo := string(mustRead(t, filepath.Join(root, "_customer_combo.html")))
	if strings.Contains(base, "|| items[0]") {
		t.Fatal("restore 가 items[0] 으로 대체한다")
	}
	if !strings.Contains(base, "목록에서 고객을 다시 선택하세요") {
		t.Fatal("고객 복원 실패 안내 없음")
	}
	if !strings.Contains(combo, "missHint") {
		t.Fatal("고객 칸 안내 없음")
	}
	if strings.Contains(combo, "required") {
		t.Fatal("hidden customer required")
	}
	if !strings.Contains(plan, "js-keep-modal") || !strings.Contains(plan, "data-form-error") {
		t.Fatal("수정 저장 오류를 창 안에 둘 훅이 없다")
	}
	if !strings.Contains(plan, "담당자를 지정하세요") || !strings.Contains(plan, ">닫기<") {
		t.Fatal("담당자 팝업 닫기 없음")
	}
	if !strings.Contains(plan, "dateInp.value = d.date") && !strings.Contains(plan, "dateInp.value = d.date || ''") {
		t.Fatal("열자마자 날짜를 넣지 않는다")
	}
	if !strings.Contains(plan, "st.selectedId = ''") {
		t.Fatal("수정 열 때 고객 초기화 없음")
	}
}

func Test95_138UpdateVisitKeepsAssigneeAndJSONCustomerError(t *testing.T) {
	e, db := newMntSyncServer(t, "mnt138.db")
	AttachHTTPErrorHandler(e)
	seedVisit(t, db, "mvs_138", "2026-08-12", "최혜영", "KLAS", "종촌동도서관")

	keep := doForm(t, e, "/maintenance/visits/mvs_138/update", url.Values{
		"plan_id":      {"MP1"},
		"visit_date":   {"2026-08-12"},
		"customer_id":  {"C_mvs_138"},
		"product_type": {"KLAS"},
		"assignee_multi": {"1"},
	})
	if keep.Code != http.StatusSeeOther {
		t.Fatalf("담당 유지 status=%d %s", keep.Code, keep.Body.String())
	}
	var asg string
	if err := db.QueryRow(`SELECT COALESCE(assignee,'') FROM maintenance_visits WHERE visit_id='mvs_138'`).Scan(&asg); err != nil {
		t.Fatal(err)
	}
	if asg != "최혜영" {
		t.Fatalf("assignee=%q want 최혜영", asg)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/maintenance/visits/mvs_138/update", strings.NewReader(url.Values{
		"plan_id":     {"MP1"},
		"visit_date":  {"2026-08-12"},
		"customer_id": {""},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Requested-With", "fetch")
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("고객 없음 status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Code == http.StatusSeeOther {
		t.Fatal("JSON 오류가 리다이렉트되어 창이 닫힌다")
	}
	if !strings.Contains(rec.Body.String(), "날짜와 고객") && !strings.Contains(rec.Body.String(), "고객") {
		t.Fatalf("오류 본문=%s", rec.Body.String())
	}
}
