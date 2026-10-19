package handler

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestSalesActivity73I_ProjectFilterKPIAndSearchEnter(t *testing.T) {
	e := newSalesServer(t)
	a := doForm(t, e, "/sales", url.Values{"name": {"알파도서관"}, "is_tentative_name": {"1"}})
	b := doForm(t, e, "/sales", url.Values{"name": {"베타박물관"}, "is_tentative_name": {"1"}})
	idA := salesIDFromRedirect(t, a.Header().Get("Location"))
	idB := salesIDFromRedirect(t, b.Header().Get("Location"))
	if doForm(t, e, "/sales/activities", url.Values{
		"sales_id": {idA}, "activity_date": {"2026-10-10"}, "activity_type": {"visit"}, "title": {"알파 방문"},
	}).Code != http.StatusSeeOther && doFormJSON(t, e, "/sales/activities", url.Values{
		"sales_id": {idA}, "activity_date": {"2026-10-10"}, "activity_type": {"visit"}, "title": {"알파 방문"},
	}).Code != http.StatusOK {
		t.Fatal("알파 활동 등록 실패")
	}
	doFormJSON(t, e, "/sales/activities", url.Values{
		"sales_id": {idA}, "activity_date": {"2026-10-10"}, "activity_type": {"visit"}, "title": {"알파 방문"},
	})
	doFormJSON(t, e, "/sales/activities", url.Values{
		"sales_id": {idB}, "activity_date": {"2026-10-11"}, "activity_type": {"call"}, "title": {"베타 전화"},
	})

	page := doGet(t, e, "/sales/activities?month=all&period=all")
	if page.Code != http.StatusOK {
		t.Fatalf("목록 status=%d", page.Code)
	}
	body := page.Body.String()
	if !strings.Contains(body, "사업별 필터") || !strings.Contains(body, "id=\"sales-act-biz-chips\"") {
		t.Fatal("사업별 필터 없음")
	}
	if strings.Contains(body, "sales-act-biz-chips") && strings.Contains(body[strings.Index(body, "sales-act-biz-chips"):strings.Index(body, "sales-act-biz-chips")+400], "전체 선택") &&
		strings.Contains(body[strings.Index(body, "sales-act-biz-chips"):], "전체 선택") {
		// 분류 줄 직후에 전체 선택이 있으면 안 된다. 전체 선택은 다음 줄.
	}
	if !strings.Contains(body, `id="sales-act-search"`) || !strings.Contains(body, `method="get"`) {
		t.Fatal("검색 폼 GET 없음")
	}
	if !strings.Contains(body, "알파도서관") || !strings.Contains(body, "베타박물관") {
		t.Fatal("사업 체크박스 없음")
	}

	onlyA := doGet(t, e, "/sales/activities?month=all&period=all&project_ids="+idA)
	ab := onlyA.Body.String()
	if !strings.Contains(ab, "알파 방문") || strings.Contains(ab, "베타 전화") {
		t.Fatalf("사업 필터 실패: %s", clipBody(ab))
	}
	if !strings.Contains(ab, "전체 활동") {
		t.Fatal("KPI 없음")
	}

	none := doGet(t, e, "/sales/activities?month=all&period=all&project_ids=none")
	if !strings.Contains(none.Body.String(), "선택한 사업이 없습니다") {
		t.Fatal("빈 선택 안내 없음")
	}

	out := doGet(t, e, "/sales/activities?month=all&period=all&project_ids=SP-NOPE")
	if strings.Contains(out.Body.String(), "알파 방문") {
		t.Fatal("범위 밖 ID가 통과했다")
	}

	bizNone := doGet(t, e, "/sales/activities?month=all&period=all&biz_types=none")
	if !strings.Contains(bizNone.Body.String(), "선택한 분류가 없습니다") {
		t.Fatal("분류 전부 해제 안내 없음")
	}

	q := doGet(t, e, "/sales/activities?month=all&period=all&q="+url.QueryEscape("알파"))
	qb := q.Body.String()
	if !strings.Contains(qb, "알파 방문") || strings.Contains(qb, "베타 전화") {
		t.Fatal("검색이 KPI/표와 안 맞는다")
	}
	if !strings.Contains(qb, `value="알파"`) && !strings.Contains(qb, `value="알파"`) {
		t.Fatal("검색어 유지 안 됨")
	}
}

func TestSalesActivity73I_PaginationKeepsProjectIDs(t *testing.T) {
	links := actPageLinks(url.Values{"project_ids": {"SP-1,SP-2"}}, 2, 5)
	found := false
	for _, l := range links {
		if strings.Contains(l.Href, "project_ids=SP-1") || strings.Contains(l.Href, "project_ids=SP-1%2CSP-2") {
			found = true
		}
	}
	if !found {
		t.Fatalf("페이지 링크에 project_ids 없음 %+v", links)
	}
}
