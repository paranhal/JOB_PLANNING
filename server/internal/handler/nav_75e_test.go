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

func TestNav75E_SalesListMenuRemovedPipelineDefault(t *testing.T) {
	ctx := visionMenuCtx()
	res := FilterNav(ctx, NavSalesPipeline, NavBadges{})
	hrefs := collectNavHrefs(res.Groups)
	salesExact, pipe := 0, 0
	for _, h := range hrefs {
		if h == "/sales" {
			salesExact++
		}
		if h == "/sales?view=pipeline" {
			pipe++
		}
	}
	if salesExact != 0 {
		t.Fatalf("사이드바에 /sales 잎 %d건", salesExact)
	}
	if pipe != 1 {
		t.Fatalf("파이프라인 링크 %d건", pipe)
	}
	if n := countNavActive(res.Groups); n != 1 {
		t.Fatalf("파이프라인 활성 개수=%d", n)
	}
	if missing := oldSidebarUnreachable(ctx); len(missing) > 0 {
		t.Fatalf("도달 누락: %v", missing)
	}

	e := newSalesServer(t)
	pipePage := doGet(t, e, "/sales")
	if pipePage.Code != http.StatusOK {
		t.Fatalf("/sales status=%d", pipePage.Code)
	}
	pb := pipePage.Body.String()
	if !strings.Contains(pb, "영업 파이프라인") || !strings.Contains(pb, `id="sales-view-switch"`) {
		t.Fatal("기본 보기가 파이프라인이 아니다")
	}
	if !strings.Contains(pb, "영업 사업 등록") {
		t.Fatal("파이프라인에 등록 단추가 없다")
	}

	for _, q := range []string{"display=list", "display=kanban", "display=timeline"} {
		rec := doGet(t, e, "/sales?"+q)
		if rec.Code != http.StatusOK {
			t.Fatalf("/sales?%s status=%d", q, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), `id="sales-view-switch"`) {
			t.Fatalf("%s 보기 전환 없음", q)
		}
	}

	rec := doForm(t, e, "/sales", url.Values{"name": {"75E 상세"}, "is_tentative_name": {"1"}})
	id := salesIDFromRedirect(t, rec.Header().Get("Location"))
	show := doGet(t, e, "/sales/"+id)
	if show.Code != http.StatusOK {
		t.Fatalf("상세 status=%d", show.Code)
	}
	sb := show.Body.String()
	if !strings.Contains(sb, `href="/sales?view=pipeline" class="bg-blue-600 text-white nav-active`) {
		t.Fatal("상세에서 파이프라인이 활성이 아니다")
	}

	refBody := doGetReferer(t, e, "/sales/"+id, "/sales?display=kanban&search=foo")
	if refBody.Code != http.StatusOK {
		t.Fatalf("referer 상세 status=%d", refBody.Code)
	}
	rb := refBody.Body.String()
	if !strings.Contains(rb, `href="/sales?display=kanban&amp;search=foo"`) && !strings.Contains(rb, `href="/sales?display=kanban&search=foo"`) {
		t.Fatalf("ListBack 이 이전 보기를 유지하지 않음: %s", clipBody(rb))
	}

	filtered := doGet(t, e, "/sales?display=list&search=75E")
	fb := filtered.Body.String()
	if !strings.Contains(fb, "display=kanban") || !strings.Contains(fb, "search=75E") {
		t.Fatal("보기 전환이 필터를 유지하지 않는다")
	}

	ro := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/sales", nil)
	req.AddCookie(jwtCookieReadOnly(t, "sales"))
	e.ServeHTTP(ro, req)
	if ro.Code == http.StatusOK && strings.Contains(ro.Body.String(), "영업 사업 등록") {
		t.Fatal("읽기 전용에 등록 단추가 보인다")
	}
}

func doGetReferer(t *testing.T, e *echo.Echo, path, referer string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
	req.Header.Set("Referer", "http://localhost"+referer)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	return rec
}

func TestSalesListViewDefaultPipeline(t *testing.T) {
	if got := salesListView("", ""); got != "pipeline" {
		t.Fatalf("empty=%s", got)
	}
	if got := salesListView("list", ""); got != "list" {
		t.Fatalf("list=%s", got)
	}
	if got := salesListView("", "pipeline"); got != "pipeline" {
		t.Fatalf("view pipeline=%s", got)
	}
}

func TestSalesListBackReferer(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/sales/x", nil)
	req.Header.Set("Referer", "http://localhost/sales?display=timeline")
	c := e.NewContext(req, nil)
	p := &model.SalesProject{}
	if got := salesListBack(c, p); got != "/sales?display=timeline" {
		t.Fatalf("got %s", got)
	}
}
