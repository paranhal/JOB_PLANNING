package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestCreateKnowledgeBulkPerReceiptAndLeavesProcesses(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "kb-bulk.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"c1", "가나도서관", "가나도서관"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name) VALUES ('A1','c1','KLAS')`); err != nil {
		t.Fatal(err)
	}
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	asRepo := repository.NewASRepo(db)
	proc := repository.NewASProcessRepo(db)
	symptoms := []string{
		"홈페이지 로그인이 안 됨. 아이디는 맞는데 비밀번호를 바꿔도 같다",
		"홈페이지 배너 이미지가 안 보임",
		"홈페이지 로그인 화면이 안 뜸",
	}
	var receipts []*model.ASReceipt
	for i, sym := range symptoms {
		as := &model.ASReceipt{
			CustomerID: "c1", AssetID: "A1", Symptom: sym,
			ReceiptDatetime: time.Date(2026, 7, 10+i, 9, 0, 0, 0, time.Local),
			Urgency:         "normal", Priority: "normal",
		}
		if err := asRepo.Create(as); err != nil {
			t.Fatal(err)
		}
		if err := proc.Create(&model.ASProcess{ASID: as.ASID, WorkContent: "짧음", TimeSpent: 10}); err != nil {
			t.Fatal(err)
		}
		receipts = append(receipts, as)
	}
	before := map[string]string{}
	for _, as := range receipts {
		before[as.ASID] = asRepo.ProcessWorkSnapshot(as.ASID)
		if before[as.ASID] == "" {
			t.Fatal("원본 스냅샷이 비었다")
		}
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.POST("/as/knowledge/bulk", h.AS.CreateKnowledgeBulk)

	post := func(form url.Values) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost/as/knowledge/bulk", strings.NewReader(form.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		req.AddCookie(jwtCookie(t))
		e.ServeHTTP(rec, req)
		return rec
	}

	ok := url.Values{"action_text": {"세션 만료 시간이 짧아 발생. WAS 설정에서 시간을 늘린다"}}
	ok["as_id"] = []string{receipts[0].ASID, receipts[1].ASID, receipts[2].ASID}
	if rec := post(ok); rec.Code != http.StatusSeeOther {
		t.Fatalf("3건 저장 %d %s", rec.Code, rec.Body.String())
	}

	rows, err := db.Query(`SELECT as_id, symptom_text, author_id, author_name, created_at FROM as_kb_entries WHERE is_current=1 ORDER BY as_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]struct{ symptom, authorID, authorName, at string }{}
	for rows.Next() {
		var asID, symptom, authorID, authorName, at string
		if err := rows.Scan(&asID, &symptom, &authorID, &authorName, &at); err != nil {
			t.Fatal(err)
		}
		got[asID] = struct{ symptom, authorID, authorName, at string }{symptom, authorID, authorName, at}
	}
	if len(got) != 3 {
		t.Fatalf("as_kb_entries n=%d want 3", len(got))
	}
	for _, as := range receipts {
		row, ok := got[as.ASID]
		if !ok {
			t.Fatalf("as_id %s 행이 없다", as.ASID)
		}
		if row.symptom != as.Symptom {
			t.Fatalf("symptom_text=%q want %q", row.symptom, as.Symptom)
		}
		if row.authorID != "admin-id" || row.authorName != "관리자" || strings.TrimSpace(row.at) == "" {
			t.Fatalf("작성자·시각 %+v", row)
		}
		if asRepo.ProcessWorkSnapshot(as.ASID) != before[as.ASID] {
			t.Fatal("저장 후 as_processes.work_content 가 바뀌었다")
		}
	}

	none := post(url.Values{"action_text": {"세션 만료 시간이 짧아 발생. WAS 설정에서 시간을 늘린다"}})
	if none.Code != http.StatusSeeOther || !strings.Contains(none.Header().Get("Location"), "bulk_none") {
		t.Fatalf("0건 거부 %d loc=%s", none.Code, none.Header().Get("Location"))
	}

	tooMany := url.Values{"action_text": {"세션 만료 시간이 짧아 발생. WAS 설정에서 시간을 늘린다"}}
	ids := make([]string, 51)
	for i := range ids {
		ids[i] = fmt.Sprintf("X%d", i)
	}
	tooMany["as_id"] = ids
	over := post(tooMany)
	if over.Code != http.StatusSeeOther || !strings.Contains(over.Header().Get("Location"), "bulk_limit") {
		t.Fatalf("51건 거부 %d loc=%s", over.Code, over.Header().Get("Location"))
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM as_kb_entries`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("거부 후에도 행이 늘었다 n=%d", n)
	}
}

func TestCreateKnowledgeHXLeavesProcess(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "kb-one.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"c1", "가나도서관", "가나도서관"); err != nil {
		t.Fatal(err)
	}
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	asRepo := repository.NewASRepo(db)
	proc := repository.NewASProcessRepo(db)
	as := &model.ASReceipt{
		CustomerID: "c1", Symptom: "홈페이지 로그인이 안 됨",
		ReceiptDatetime: time.Now(), Urgency: "normal", Priority: "normal",
	}
	if err := asRepo.Create(as); err != nil {
		t.Fatal(err)
	}
	if err := proc.Create(&model.ASProcess{ASID: as.ASID, WorkContent: "재시작", TimeSpent: 10}); err != nil {
		t.Fatal(err)
	}
	before := asRepo.ProcessWorkSnapshot(as.ASID)

	e := echo.New()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.POST("/as/knowledge", h.AS.CreateKnowledge)

	form := url.Values{
		"as_id": {as.ASID}, "symptom_text": {as.Symptom},
		"action_text": {"WAS 세션 시간을 늘린 뒤 다시 로그인하게 안내"}, "group_key": {"KW001"}, "from": {"gaps"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as/knowledge", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set("HX-Request", "true")
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "지식 있음") {
		t.Fatalf("hx %d %s", rec.Code, rec.Body.String())
	}
	if kb := asRepo.CurrentKBByAS(as.ASID); kb == nil || kb.ASID != as.ASID {
		t.Fatal("저장이 그 건에 되지 않았다")
	}
	if trig := rec.Header().Get("HX-Trigger"); !strings.Contains(trig, `"key":"KW001"`) || !strings.Contains(trig, `"n":1`) {
		t.Fatalf("묶음·총계 트리거 없음 %s", trig)
	}
	if asRepo.ProcessWorkSnapshot(as.ASID) != before {
		t.Fatal("한 건 저장 후 원본이 바뀌었다")
	}
	if asRepo.CurrentKBByAS(as.ASID) == nil {
		t.Fatal("지식이 없다")
	}
}
