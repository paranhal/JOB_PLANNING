package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_44_CauseCatCustomSaveAndClear(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	rec := postASAction(t, e, asID, url.Values{
		"work_place":        {"field"},
		"process_type":      {"visit"},
		"cause_cat1":        {model.CauseCatCustom},
		"cause_cat1_custom": {"CCTV"},
		"action_taken":      {"카메라 점검"},
		"result_code":       {model.ResultDone},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("save %s", rec.Header().Get("Location"))
	}
	got, _ := asRepo.GetByID(repository.OrgAll, asID)
	if got.CauseCat1 != model.CauseCatCustom || got.CauseCat1Custom != "CCTV" || got.CauseCat2 != "" {
		t.Fatalf("%+v", got)
	}
	page := getASActionPage(t, e, asID)
	if !strings.Contains(page, "직접 입력") || !strings.Contains(page, "목록에 없으면 직접 입력") {
		t.Fatal("직접 입력 옵션 없음")
	}
	if !strings.Contains(page, "CCTV") {
		t.Fatal("라벨 CCTV 없음")
	}

	empty := postASAction(t, e, asID, url.Values{
		"work_place":        {"field"},
		"process_type":      {"visit"},
		"cause_cat1":        {model.CauseCatCustom},
		"cause_cat1_custom": {"   "},
		"action_taken":      {"카메라 점검2"},
		"result_code":       {model.ResultDone},
	})
	if !strings.Contains(empty.Header().Get("Location"), "err=cause_cat1_custom") {
		t.Fatalf("empty %s", empty.Header().Get("Location"))
	}

	clear := postASAction(t, e, asID, url.Values{
		"work_place":        {"field"},
		"process_type":      {"visit"},
		"cause_cat1":        {"rfid"},
		"cause_cat2":        {"rfid.hw"},
		"cause_cat3":        {"rfid.hw.part_check"},
		"cause_cat1_custom": {"CCTV"},
		"action_taken":      {"RFID 점검"},
		"result_code":       {model.ResultDone},
	})
	if strings.Contains(clear.Header().Get("Location"), "err=") {
		t.Fatalf("rfid %s", clear.Header().Get("Location"))
	}
	got, _ = asRepo.GetByID(repository.OrgAll, asID)
	if got.CauseCat1 != "rfid" || got.CauseCat1Custom != "" {
		t.Fatalf("cleared %+v", got)
	}
}

func Test95_44_ASNewExtraSitesGroupAndCap(t *testing.T) {
	e, h, asRepo, _, _ := newReceiptPhotoFixture(t)
	for _, id := range []string{"cust_b", "cust_c"} {
		if _, err := h.AS.db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
			id, id+"학교", id+"학교"); err != nil {
			t.Fatal(err)
		}
	}
	tok := "tok-44-a"
	form := url.Values{
		"customer_id":         {"cust_a"},
		"symptom":             {"공통증상44"},
		"received_by":         {"테스터"},
		"assigned_to":         {"최혜영"},
		"receipt_datetime":    {time.Now().Format("2006-01-02T15:04")},
		"extra_site_id":       {"cust_b", "cust_c"},
		"extra_site_symptom":  {"B증상", ""},
		"submit_token":        {tok},
	}
	post := func(f url.Values) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost/as", strings.NewReader(f.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		req.AddCookie(jwtCookie(t))
		e.ServeHTTP(rec, req)
		return rec
	}
	rec := post(form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Location") != "/as" {
		t.Fatalf("loc %s", rec.Header().Get("Location"))
	}
	listed, _, _ := asRepo.ListFiltered(repository.OrgAll, "", "공통증상44", "", nil, "", "", 1, 50)
	if len(listed) < 2 {
		listed, _, _ = asRepo.ListFiltered(repository.OrgAll, "", "B증상", "", nil, "", "", 1, 50)
	}
	var gid string
	var n int
	rows, err := h.AS.db.Query(`SELECT as_id, customer_id, symptom, receipt_group_id, org_id FROM as_receipts WHERE receipt_group_id != '' ORDER BY as_number`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	byCust := map[string]string{}
	for rows.Next() {
		var id, cid, sym, g, org string
		if err := rows.Scan(&id, &cid, &sym, &g, &org); err != nil {
			t.Fatal(err)
		}
		if gid == "" {
			gid = g
		}
		if g == gid {
			n++
			byCust[cid] = sym
			if org != "" && org != model.OrgIDLibrary {
				t.Fatalf("org %s", org)
			}
		}
	}
	if n != 3 {
		t.Fatalf("group n=%d %+v listed=%d", n, byCust, len(listed))
	}
	if byCust["cust_a"] != "공통증상44" || byCust["cust_b"] != "B증상" || byCust["cust_c"] != "공통증상44" {
		t.Fatalf("symptoms %+v", byCust)
	}
	second := post(form)
	if second.Code != http.StatusSeeOther {
		t.Fatalf("token 2nd %d", second.Code)
	}
	var total int
	_ = h.AS.db.QueryRow(`SELECT COUNT(*) FROM as_receipts WHERE receipt_group_id=?`, gid).Scan(&total)
	if total != 3 {
		t.Fatalf("token replay n=%d", total)
	}

	too := url.Values{
		"customer_id":      {"cust_a"},
		"symptom":          {"21곳"},
		"received_by":      {"테스터"},
		"assigned_to":      {"최혜영"},
		"receipt_datetime": {time.Now().Format("2006-01-02T15:04")},
		"submit_token":     {"tok-44-21"},
	}
	for i := 0; i < 21; i++ {
		too.Add("extra_site_id", fmt.Sprintf("sx%d", i))
		too.Add("extra_site_symptom", "")
	}
	capRec := post(too)
	if capRec.Code != http.StatusUnprocessableEntity && !strings.Contains(capRec.Body.String(), "사이트는 최대") {
		if capRec.Code == http.StatusSeeOther {
			t.Fatal("21곳이 저장됨")
		}
		if !strings.Contains(capRec.Body.String(), "사이트는 최대") {
			t.Fatalf("cap status=%d body=%s", capRec.Code, capRec.Body.String())
		}
	}
}

func Test95_44_ApplyActionToOtherAS(t *testing.T) {
	e, h, asRepo, attachRepo, asID := newASActionFixture(t)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/as/open-for-apply", h.AS.SearchOpenForApply)
	_ = attachRepo
	if _, err := h.AS.db.Exec(`UPDATE as_receipts SET org_id=?, assigned_user_id=? WHERE as_id=?`, model.OrgIDLibrary, "admin-id", asID); err != nil {
		t.Fatal(err)
	}
	mk := func(status, org string) string {
		t.Helper()
		r := &model.ASReceipt{
			CustomerID: "cust_a", Symptom: "다른건 " + status + org, ReceiptDatetime: time.Now(),
			Status: status, OrgID: org, AssignedTo: "관리자", AssignedUserID: "admin-id",
		}
		if err := asRepo.Create(r); err != nil {
			t.Fatal(err)
		}
		got, _ := asRepo.GetByID(repository.OrgAll, r.ASID)
		got.Status = status
		got.OrgID = org
		got.AssignedUserID = "admin-id"
		_ = asRepo.Update(got)
		return r.ASID
	}
	idB := mk("in_progress", model.OrgIDLibrary)
	idC := mk("in_progress", model.OrgIDLibrary)
	idDone := mk("completed", model.OrgIDLibrary)
	idOth := mk("in_progress", "O02")

	rec := postASAction(t, e, asID, url.Values{
		"work_place":          {"field"},
		"process_type":        {"visit"},
		"cause_cat1":          {model.CauseCatCustom},
		"cause_cat1_custom":   {"CCTV"},
		"action_taken":        {"원본조치내용충분"},
		"result_code":         {model.ResultDone},
		"apply_as_confirm":    {"1"},
		"extra_as_id":         {idB, idC, idDone, idOth},
		"extra_action_taken":  {"B개별조치내용충분", ""},
		"parts_form":          {"1"},
		"parts_enabled":       {"1"},
		"part_name":           {"메인보드"},
		"part_code":           {""},
		"part_qty":            {"1"},
		"part_item_id":        {""},
	})
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || strings.Contains(loc, "err=") {
		t.Fatalf("apply %s", loc)
	}
	if !strings.Contains(loc, "apply_n=2") {
		t.Fatalf("apply_n loc=%s", loc)
	}
	unesc, _ := url.QueryUnescape(loc)
	if !strings.Contains(unesc, "완료") && !strings.Contains(unesc, "타 조직") {
		t.Fatalf("skip loc=%s", loc)
	}
	b, _ := asRepo.GetByID(repository.OrgAll, idB)
	cgot, _ := asRepo.GetByID(repository.OrgAll, idC)
	if b == nil || b.ActionTaken != "B개별조치내용충분" || b.CauseCat1Custom != "CCTV" {
		t.Fatalf("b %+v", b)
	}
	if cgot == nil || cgot.ActionTaken != "원본조치내용충분" {
		t.Fatalf("c %+v", cgot)
	}
	pb, _ := h.AS.processRepo.ListByAS(idB)
	pc, _ := h.AS.processRepo.ListByAS(idC)
	ps, _ := h.AS.processRepo.ListByAS(asID)
	if len(pb) < 1 || len(pc) < 1 || len(ps) < 1 {
		t.Fatalf("procs src=%d b=%d c=%d", len(ps), len(pb), len(pc))
	}
	if rows, _ := h.AS.partsRepo.ListByAS(idB); len(rows) != 0 {
		t.Fatal("parts copied")
	}
	srcParts, _ := h.AS.partsRepo.ListByAS(asID)
	if len(srcParts) == 0 {
		t.Fatal("원본 자재 없음")
	}
	done, _ := asRepo.GetByID(repository.OrgAll, idDone)
	if done != nil && done.CauseCat1 == model.CauseCatCustom {
		t.Fatal("완료 건에 적용됨")
	}
}

func Test95_44_OpenForApplyOrgAndRole(t *testing.T) {
	e, h, asRepo, _, asID := newASActionFixture(t)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/as/open-for-apply", h.AS.SearchOpenForApply)
	if _, err := h.AS.db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES('O02','O02','미디어','business',1,2)`); err != nil {
		t.Fatal(err)
	}
	if _, err := h.AS.db.Exec(`UPDATE as_receipts SET org_id=?, assigned_user_id=? WHERE as_id=?`, model.OrgIDLibrary, "admin-id", asID); err != nil {
		t.Fatal(err)
	}
	other := &model.ASReceipt{CustomerID: "cust_a", Symptom: "미디어건", ReceiptDatetime: time.Now(), Status: "in_progress", OrgID: "O02", AssignedTo: "관리자"}
	if err := asRepo.Create(other); err != nil {
		t.Fatal(err)
	}
	if _, err := h.AS.db.Exec(`UPDATE as_receipts SET org_id='O02' WHERE as_id=?`, other.ASID); err != nil {
		t.Fatal(err)
	}

	get := func(ck *http.Cookie, q string) []repository.WorkTaskSite {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://localhost/as/open-for-apply?q="+url.QueryEscape(q), nil)
		req.AddCookie(ck)
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d %s", rec.Code, rec.Body.String())
		}
		var list []repository.WorkTaskSite
		if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
			t.Fatal(err)
		}
		return list
	}
	lib := get(jwtCookieOrg(t, "org_admin", model.OrgIDLibrary), "가나")
	for _, s := range lib {
		if s.SiteID == other.ASID {
			t.Fatal("도서관 검색에 미디어 접수")
		}
	}
	med := get(jwtCookieOrg(t, "org_admin", "O02"), "가나")
	found := false
	for _, s := range med {
		if s.SiteID == other.ASID {
			found = true
		}
		if s.SiteID == asID {
			t.Fatal("미디어 검색에 도서관 접수")
		}
	}
	if !found {
		t.Fatalf("미디어 검색 없음 %+v", med)
	}
}

func Test95_44_AdminTaskThreeSites(t *testing.T) {
	e, db, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "a44.db"))
	ck := jwtCookieUser(t, me)
	for _, id := range []string{"c-a", "c-b", "c-c"} {
		if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`, id, id+"교", id+"교"); err != nil {
			t.Fatal(err)
		}
	}
	day := time.Now().Format("2006-01-02")
	form := url.Values{
		"work_type": {"admin"}, "title": {"행정3사이트"}, "work_date": {day}, "due_date": {day},
		"start_time": {"09:00"}, "end_time": {"09:30"},
		"assignee": {me.FullName}, "site_id": {"c-a", "c-b", "c-c"},
		"redirect": {"/workboard/board?view=day"},
	}
	rec := doFormUser(t, e, "/workboard/tasks", form, ck)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("%d loc=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	var tid string
	if err := db.QueryRow(`SELECT task_id FROM work_tasks WHERE title='행정3사이트'`).Scan(&tid); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_task_sites WHERE task_id=?`, tid).Scan(&n)
	if n != 3 {
		t.Fatalf("sites %d", n)
	}
	line, err := repo.TaskSiteLine(tid)
	if err != nil || !strings.Contains(line, "외 2곳") {
		t.Fatalf("line %q %v", line, err)
	}
	zero := url.Values{
		"work_type": {"admin"}, "title": {"행정0사이트"}, "work_date": {day}, "due_date": {day},
		"start_time": {"10:00"}, "end_time": {"10:30"},
		"assignee": {me.FullName}, "customer_id": {"c-a"},
		"redirect": {"/workboard/board?view=day"},
	}
	z := doFormUser(t, e, "/workboard/tasks", zero, ck)
	if z.Code != http.StatusSeeOther {
		t.Fatalf("zero %d", z.Code)
	}
	var tid0 string
	_ = db.QueryRow(`SELECT task_id FROM work_tasks WHERE title='행정0사이트'`).Scan(&tid0)
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_task_sites WHERE task_id=?`, tid0).Scan(&n)
	if n != 0 {
		t.Fatalf("zero sites %d", n)
	}
}

func Test95_44_SiteMultiPickerMarkup(t *testing.T) {
	files := []string{
		filepath.Join("web", "templates", "workboard", "_modals.html"),
		filepath.Join("web", "templates", "as", "form.html"),
		filepath.Join("web", "templates", "as", "action.html"),
		filepath.Join("web", "templates", "workboard", "task_show.html"),
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), `{{template "site_multi_picker"`) {
			t.Fatalf("%s 공용 칩 없음", f)
		}
	}
	newPage := getASNewHasPicker(t)
	if !strings.Contains(newPage, "다른 사이트도 함께 접수") || !strings.Contains(newPage, "extra_site_id") {
		t.Fatal("접수 칩 없음")
	}
}

func getASNewHasPicker(t *testing.T) string {
	t.Helper()
	e, _, _, _, _ := newReceiptPhotoFixture(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/new", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("new %d", rec.Code)
	}
	return rec.Body.String()
}
