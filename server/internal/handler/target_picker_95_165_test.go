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
	"customer-support/internal/repository"
)

func Test95_165_AssetSearchQueryAndScope(t *testing.T) {
	e, db, _, _ := new45DServer(t)
	if _, err := db.Exec(`UPDATE assets SET install_location='1층 창구' WHERE asset_id='A1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C2','다른곳','다른곳',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name, model_name, operation_status) VALUES ('A9','C2','다른자산','ZX','operating')`); err != nil {
		t.Fatal(err)
	}

	short := doGet(t, e, "/api/assets/C1?q="+url.QueryEscape("발"))
	if strings.TrimSpace(short.Body.String()) != "[]" {
		t.Fatalf("2글자 미만 %s", short.Body.String())
	}
	part := doGet(t, e, "/api/assets/C1?q="+url.QueryEscape("창구"))
	body := part.Body.String()
	if !strings.Contains(body, `"id":"A1"`) || !strings.Contains(body, `"name"`) || !strings.Contains(body, `"sub"`) || strings.Contains(body, "A9") || strings.Contains(body, "A2") {
		t.Fatalf("부분 일치 %s", body)
	}
	modelHit := doGet(t, e, "/api/assets/C1?q=M1")
	if !strings.Contains(modelHit.Body.String(), "A1") || strings.Contains(modelHit.Body.String(), "A2") {
		t.Fatalf("모델 %s", modelHit.Body.String())
	}
	other := doGet(t, e, "/api/assets/C1?q="+url.QueryEscape("다른"))
	if strings.Contains(other.Body.String(), "A9") || strings.TrimSpace(other.Body.String()) != "[]" {
		t.Fatalf("다른 거래처 %s", other.Body.String())
	}
	page := doGet(t, e, "/as/new")
	html := page.Body.String()
	if !strings.Contains(html, "js-target-picker") || !strings.Contains(html, "tp-q") || !strings.Contains(html, "자산 미연결") || !strings.Contains(html, `name="eq_asset_id"`) || !strings.Contains(html, "extra_site_id") || !strings.Contains(html, "선택 적용") {
		t.Fatal("접수 화면에 빈 선택기가 없다")
	}
	admin := doGet(t, e, "/admin-work/new")
	if !strings.Contains(admin.Body.String(), "js-target-picker") || !strings.Contains(admin.Body.String(), "선택 적용") {
		t.Fatal("행정 등록 화면에 선택기가 없다")
	}
}

func Test95_165_ASFormRedisplaysChosenAsset(t *testing.T) {
	e, h, _, _, _ := newReceiptPhotoFixture(t)
	a1 := &model.Asset{CustomerID: "cust_a", ProductName: "자가대출기", ModelName: "K-100", InstallLocation: "1층 창구"}
	if err := h.assetRepo.Create(a1); err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"customer_id":      {"cust_a"},
		"received_by":      {"테스터"},
		"receipt_datetime": {time.Now().Format("2006-01-02T15:04")},
		"eq_asset_id":      {a1.AssetID},
		"eq_symptom":       {"카드 인식 안 됨"},
		"eq_visit_date":    {"1800-01-01"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, a1.AssetID) || !strings.Contains(body, "자가대출기") || !strings.Contains(body, "js-target-picker") {
		t.Fatal("오류 화면에 고른 자산이 없다")
	}
}

func Test95_165_WorkFormsSaveAssetCounts(t *testing.T) {
	e, repo, _, db := newProjectWorkHarness(t)
	if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name, model_name, install_location, operation_status) VALUES
		('A-165','C-high','스위치','SW-100','본관','operating'),
		('A-166','C-high','발급기','M1','창구','operating'),
		('A-oth','C-oth','타자산','X','외부','operating')`); err != nil {
		t.Fatal(err)
	}
	page := doGet(t, e, "/workboard/register?view=day")
	if page.Code != http.StatusOK {
		t.Fatalf("register %d", page.Code)
	}
	html := page.Body.String()
	if !strings.Contains(html, "js-target-picker") || !strings.Contains(html, "선택 적용") || !strings.Contains(html, "tp-q") {
		t.Fatal("등록 화면에 자산 선택기가 없다")
	}
	none := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"자산없는작업"}, "work_subtype": {"other"},
		"work_date": {"2026-11-14"}, "assignee": {"관리자"},
	})
	if none.Code != http.StatusSeeOther {
		t.Fatalf("자산 0건 저장 %d %s", none.Code, none.Body.String())
	}
	one := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"sales"}, "title": {"영업자산"}, "work_date": {"2026-11-12"},
		"start_time": {"11:00"}, "end_time": {"12:00"}, "assignee": {"관리자"},
		"asset_id": {"A-165"},
	})
	if one.Code != http.StatusSeeOther {
		t.Fatalf("영업 1건 저장 %d %s", one.Code, one.Body.String())
	}
	many := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"작업자산"}, "work_subtype": {"other"},
		"work_date": {"2026-11-13"}, "assignee": {"관리자"},
		"asset_id": {"A-165", "A-166"},
	})
	if many.Code != http.StatusSeeOther {
		t.Fatalf("작업 N건 저장 %d %s", many.Code, many.Body.String())
	}
	admin := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"admin"}, "title": {"행정자산"}, "work_date": {"2026-11-11"},
		"start_time": {"09:00"}, "end_time": {"10:00"}, "assignee": {"관리자"},
		"asset_id": {"A-165", "A-166", "A-165"},
	})
	if admin.Code != http.StatusSeeOther {
		t.Fatalf("행정 저장 %d %s", admin.Code, admin.Body.String())
	}
	tasks, err := repo.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	byTitle := map[string]model.WorkTask{}
	for _, tk := range tasks {
		byTitle[tk.Title] = tk
	}
	assertAssets := func(title string, want ...string) {
		t.Helper()
		tk, ok := byTitle[title]
		if !ok {
			t.Fatalf("%s 없음", title)
		}
		got, err := repo.ListTaskAssets(tk.TaskID)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(want) {
			t.Fatalf("%s 연결 %d", title, len(got))
		}
		have := map[string]bool{}
		for _, a := range got {
			have[a.AssetID] = true
		}
		for _, id := range want {
			if !have[id] {
				t.Fatalf("%s 에 %s 없음", title, id)
			}
		}
	}
	assertAssets("자산없는작업")
	assertAssets("영업자산", "A-165")
	assertAssets("작업자산", "A-165", "A-166")
	assertAssets("행정자산", "A-165", "A-166")

	edit := doGet(t, e, "/workboard/tasks/"+byTitle["행정자산"].TaskID+"/edit")
	if !strings.Contains(edit.Body.String(), "data-standalone=\"1\"") || !strings.Contains(edit.Body.String(), "/assets") || !strings.Contains(edit.Body.String(), "tp-save") {
		t.Fatal("업무 상세 단독 저장이 없다")
	}
	saved := doForm(t, e, "/workboard/tasks/"+byTitle["자산없는작업"].TaskID+"/assets", url.Values{
		"asset_id": {"A-166"},
	})
	if saved.Code != http.StatusSeeOther && saved.Code != http.StatusOK {
		t.Fatalf("단독 저장 %d", saved.Code)
	}
	assertAssets("자산없는작업", "A-166")

	q := doGet(t, e, "/workboard/work-sites/search?q="+url.QueryEscape("계룡"))
	raw := q.Body.String()
	if !strings.Contains(raw, `"site_id"`) || !strings.Contains(raw, `"id"`) || !strings.Contains(raw, `"name"`) || !strings.Contains(raw, `"sub"`) {
		t.Fatalf("사이트 응답 규격 %s", raw)
	}
	var hits []repository.WorkTaskSite
	if err := json.Unmarshal(q.Body.Bytes(), &hits); err != nil || len(hits) == 0 || hits[0].SiteID == "" {
		t.Fatalf("기존 site_id 파싱 %v %d", err, len(hits))
	}
}
