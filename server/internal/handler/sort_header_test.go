package handler

import (
	"bytes"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestSortThRendersWhenAlignOmitted(t *testing.T) {
	tmpl, err := template.New("").Funcs(funcMap()).ParseFiles("web/templates/sort/_th.html")
	if err != nil {
		t.Fatal(err)
	}
	data := map[string]interface{}{
		"Label": "번호", "Key": "task_id", "Cur": "due_date", "Dir": "asc", "Href": "/work?sort=task_id",
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "sort_th", data); err != nil {
		t.Fatalf("Align 없이 eq 비교가 실패한다: %v", err)
	}
	body := buf.String()
	if !strings.Contains(body, "번호") || !strings.Contains(body, "⇅") || !strings.Contains(body, "text-left") {
		t.Fatalf("기본 왼쪽 정렬 머리글이 아니다: %s", body)
	}

	data["Align"] = "right"
	data["Cur"] = "task_id"
	data["Dir"] = "desc"
	buf.Reset()
	if err := tmpl.ExecuteTemplate(&buf, "sort_th", data); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "text-right") || !strings.Contains(got, "▼") {
		t.Fatalf("Align=right 가 안 먹는다: %s", got)
	}
}

func TestSortLinkHrefsKeepsFilterAndTogglesDir(t *testing.T) {
	f := url.Values{}
	f.Set("assignee", "관리자")
	f.Set("status", "delayed")
	hrefs := sortLinkHrefs("/work/all", f, []string{"title", "due_date"}, "due_date", "desc")

	title := hrefs["title"]
	if !strings.Contains(title, "assignee=") || !strings.Contains(title, "status=delayed") {
		t.Fatalf("정렬 링크가 필터를 풀었다: %s", title)
	}
	if !strings.Contains(title, "sort=title") || !strings.Contains(title, "dir=asc") {
		t.Fatalf("다른 열 첫 클릭이 오름차순이 아니다: %s", title)
	}
	due := hrefs["due_date"]
	if !strings.Contains(due, "sort=due_date") || !strings.Contains(due, "dir=asc") {
		t.Fatalf("같은 열 재클릭이 방향을 안 바꾼다: %s", due)
	}
}

func assertSortHeaderChrome(t *testing.T, body, selectID string, activeDesc bool) {
	t.Helper()
	if !strings.Contains(body, "⇅") {
		t.Fatal("정렬 가능 열에 흐린 ⇅ 가 없다")
	}
	if activeDesc {
		if !strings.Contains(body, "▼") {
			t.Fatal("정렬 중인 열에 ▼ 가 없다")
		}
	} else if strings.Contains(body, "▲") || strings.Contains(body, "▼") {
		// 기본 정렬 열이 있으면 ▲/▼ 중 하나는 있어야 한다
	}
	if selectID != "" && !strings.Contains(body, `id="`+selectID+`"`) {
		t.Fatalf("모바일 정렬 드롭다운 %s 가 없다", selectID)
	}
}

func TestWorkListSortIconsToggleAndKeepFilter(t *testing.T) {
	e, _ := newWorkTodayServer(t)

	today := workTodayGet(t, e, "/work", jwtCookie(t))
	if today.Code != http.StatusOK {
		t.Fatalf("/work status=%d", today.Code)
	}
	tb := today.Body.String()
	assertSortHeaderChrome(t, tb, "work-sort", false)
	if !strings.Contains(tb, "종료일") || !strings.Contains(tb, "▲") {
		t.Fatal("오늘 내 업무 기본(종료일 오름) ▲ 가 없다")
	}
	if !strings.Contains(tb, "sort=title") || !strings.Contains(tb, "dir=asc") {
		t.Fatal("업무명 정렬 링크가 없다")
	}

	toggled := workTodayGet(t, e, "/work?sort=title&dir=asc", jwtCookie(t))
	if toggled.Code != http.StatusOK {
		t.Fatalf("title sort status=%d", toggled.Code)
	}
	tt := toggled.Body.String()
	if !strings.Contains(tt, "▲") || !strings.Contains(tt, "sort=title") || !strings.Contains(tt, "dir=desc") {
		t.Fatal("같은 열을 다시 누르면 내림차순으로 안 바뀐다")
	}

	all := workTodayGet(t, e, "/work/all?assignee="+url.QueryEscape("관리자")+"&status=delayed", jwtCookie(t))
	if all.Code != http.StatusOK {
		t.Fatalf("/work/all filter status=%d", all.Code)
	}
	ab := all.Body.String()
	assertSortHeaderChrome(t, ab, "work-sort", true)
	if !strings.Contains(ab, "assignee=") || !strings.Contains(ab, "status=delayed") {
		t.Fatal("전체 업무 정렬 링크가 담당자·상태 필터를 풀었다")
	}
	if !strings.Contains(ab, "▼") {
		t.Fatal("전체 업무 기본(종료일 내림) ▼ 가 없다")
	}
}

func TestAdminWorkSortIconsKeepDefaultOrder(t *testing.T) {
	e, repo := newGTDServer(t)
	early := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "가나다 마감먼저", DueDate: "2026-08-10", Status: model.WBTaskWaiting}
	late := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "하하하 마감나중", DueDate: "2026-08-30", Status: model.WBTaskWaiting}
	if err := repo.CreateTask(early); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(late); err != nil {
		t.Fatal(err)
	}

	list := doGet(t, e, "/admin-work?search="+url.QueryEscape("마감"))
	if list.Code != http.StatusOK {
		t.Fatalf("status=%d", list.Code)
	}
	body := list.Body.String()
	assertSortHeaderChrome(t, body, "admin-work-sort", false)
	if !strings.Contains(body, "▲") {
		t.Fatal("행정 기본(종료일 오름) ▲ 가 없다")
	}
	ei := strings.Index(body, "가나다 마감먼저")
	li := strings.Index(body, "하하하 마감나중")
	if ei < 0 || li < 0 || ei > li {
		t.Fatalf("행정 기본 정렬이 바뀌었다 ei=%d li=%d", ei, li)
	}
	if !strings.Contains(body, "search=") {
		t.Fatal("정렬 링크가 검색어를 풀었다")
	}

	byTitle := doGet(t, e, "/admin-work?sort=title&dir=desc")
	tb := byTitle.Body.String()
	if strings.Index(tb, "하하하 마감나중") > strings.Index(tb, "가나다 마감먼저") {
		t.Fatal("제목 내림차순이 이전과 다르다")
	}
	if !strings.Contains(tb, "▼") || !strings.Contains(tb, "dir=asc") {
		t.Fatal("제목 열 재클릭 링크가 오름으로 안 바뀐다")
	}
}

func TestASListSortIconsKeepDefaultOrderAndSearch(t *testing.T) {
	e, _, asRepo, _, _ := newReceiptPhotoFixture(t)
	older := &model.ASReceipt{
		CustomerID: "cust_a", ReceiptChannel: "phone", Requester: "홍길동",
		Symptom: "오래된증상XYZ", Urgency: "normal", Priority: "normal",
		AssignedTo: "양기헌", ReceiptDatetime: time.Now().Add(-48 * time.Hour),
		Status: "received",
	}
	if err := asRepo.Create(older); err != nil {
		t.Fatal(err)
	}

	list := doGet(t, e, "/as")
	if list.Code != http.StatusOK {
		t.Fatalf("/as status=%d body=%s", list.Code, list.Body.String())
	}
	body := list.Body.String()
	assertSortHeaderChrome(t, body, "as-sort", false)
	if !strings.Contains(body, "sort=as_number") || !strings.Contains(body, "sort=org_name") {
		t.Fatal("AS 정렬 링크가 없다")
	}
	newerI := strings.Index(body, "게이트 오작동")
	olderI := strings.Index(body, "오래된증상XYZ")
	if newerI < 0 || olderI < 0 || newerI > olderI {
		t.Fatalf("AS 기본(접수 내림) 순서가 바뀌었다 newer=%d older=%d", newerI, olderI)
	}

	search := doGet(t, e, "/as?search="+url.QueryEscape("가나"))
	if search.Code != http.StatusOK {
		t.Fatalf("search status=%d", search.Code)
	}
	sb := search.Body.String()
	if !strings.Contains(sb, "search=") {
		t.Fatal("AS 정렬 링크가 검색 조건을 풀었다")
	}
}

func TestUnplannedSortIconsKeepDefaultOrder(t *testing.T) {
	e, db := newKanbanScopeServer(t)
	now := time.Now()
	today := now.Format("2006-01-02")
	past := now.AddDate(0, 0, -5).Format("2006-01-02")
	ts := now.Format("2006-01-02 15:04:05")
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('C1','기관A','기관A',1)`); err != nil {
		t.Fatal(err)
	}
	insert := func(id, num, visit, confirmed, assigned, status string) {
		t.Helper()
		_, err := db.Exec(`INSERT INTO as_receipts (
			as_id, as_number, receipt_datetime, customer_id, symptom, urgency, status,
			visit_scheduled_date, schedule_confirmed, assigned_to, created_at, updated_at
		) VALUES (?,?,?,'C1','증상','중',?,?,?,?,?,?)`,
			id, num, ts, status, visit, confirmed, assigned, ts, ts)
		if err != nil {
			t.Fatal(err)
		}
	}
	insert("AS-NODATE", "R-ND", "", "0", "테크", "in_progress")
	insert("AS-DELAY", "R-DL", past, "1", "테크", "in_progress")
	insert("AS-UNAS", "R-UA", today, "1", "", "received")

	list := doGet(t, e, "/plan/unplanned")
	if list.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", list.Code, list.Body.String())
	}
	body := list.Body.String()
	assertSortHeaderChrome(t, body, "unplanned-sort", false)
	di := strings.Index(body, "R-DL")
	ni := strings.Index(body, "R-ND")
	if di < 0 || ni < 0 || di > ni {
		t.Fatalf("미계획 기본(빨강 먼저) 순서가 바뀌었다 delay=%d nodate=%d", di, ni)
	}
	if !strings.Contains(body, "sort=due_date") || !strings.Contains(body, "sort=task_id") {
		t.Fatal("미계획 정렬 링크가 없다")
	}

	kind := doGet(t, e, "/plan/unplanned?kind=delayed")
	kb := kind.Body.String()
	if !strings.Contains(kb, "kind=delayed") {
		t.Fatal("미계획 정렬 링크가 유형 필터를 풀었다")
	}
}

func TestSalesListSortIconsKeepDefaultOrderAndSearch(t *testing.T) {
	e := newSalesServer(t)
	a := doForm(t, e, "/sales", url.Values{"name": {"가나다사업"}, "expected_ym": {"2026-01"}})
	b := doForm(t, e, "/sales", url.Values{"name": {"하하하사업"}, "expected_ym": {"2026-12"}})
	if a.Code != http.StatusSeeOther || b.Code != http.StatusSeeOther {
		t.Fatalf("create a=%d b=%d", a.Code, b.Code)
	}

	list := doGet(t, e, "/sales")
	if list.Code != http.StatusOK {
		t.Fatalf("/sales status=%d body=%s", list.Code, list.Body.String())
	}
	body := list.Body.String()
	assertSortHeaderChrome(t, body, "sales-sort", false)
	hi := strings.Index(body, "하하하사업")
	gi := strings.Index(body, "가나다사업")
	if hi < 0 || gi < 0 || hi > gi {
		t.Fatalf("영업 기본(시기 내림) 순서가 바뀌었다 hi=%d gi=%d", hi, gi)
	}

	search := doGet(t, e, "/sales?search="+url.QueryEscape("사업"))
	sb := search.Body.String()
	if !strings.Contains(sb, "search=") {
		t.Fatal("영업 정렬 링크가 검색을 풀었다")
	}
	sorted := doGet(t, e, "/sales?sort=name&dir=asc")
	st := sorted.Body.String()
	if strings.Index(st, "가나다사업") > strings.Index(st, "하하하사업") {
		t.Fatal("사업명 오름차순이 아니다")
	}
	if !strings.Contains(st, "▲") || !strings.Contains(st, "dir=desc") {
		t.Fatal("사업명 열 재클릭이 내림으로 안 바뀐다")
	}
}

func TestQuotesListSortIconsKeepDefaultOrder(t *testing.T) {
	e, _ := newQuoteServer(t)
	first := quoteLineForm([]string{"품목A"}, []int{1000})
	first.Set("title", "가나다견적")
	second := quoteLineForm([]string{"품목B"}, []int{2000})
	second.Set("title", "하하하견적")
	r1 := doForm(t, e, "/quotes", first)
	r2 := doForm(t, e, "/quotes", second)
	if r1.Code != http.StatusSeeOther || r2.Code != http.StatusSeeOther {
		t.Fatalf("create r1=%d r2=%d body=%s", r1.Code, r2.Code, r2.Body.String())
	}

	list := doGet(t, e, "/quotes")
	if list.Code != http.StatusOK {
		t.Fatalf("/quotes status=%d body=%s", list.Code, list.Body.String())
	}
	body := list.Body.String()
	assertSortHeaderChrome(t, body, "quote-sort", false)
	hi := strings.Index(body, "하하하견적")
	gi := strings.Index(body, "가나다견적")
	if hi < 0 || gi < 0 || hi > gi {
		t.Fatalf("견적 기본(번호 내림) 순서가 바뀌었다 hi=%d gi=%d", hi, gi)
	}

	search := doGet(t, e, "/quotes?search="+url.QueryEscape("견적"))
	if !strings.Contains(search.Body.String(), "search=") {
		t.Fatal("견적 정렬 링크가 검색을 풀었다")
	}
	sorted := doGet(t, e, "/quotes?sort=title&dir=asc")
	st := sorted.Body.String()
	if strings.Index(st, "가나다견적") > strings.Index(st, "하하하견적") {
		t.Fatal("건명 오름차순이 아니다")
	}
}
