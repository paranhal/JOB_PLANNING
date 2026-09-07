package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestASKeywordSuggestOmitsGreetings(t *testing.T) {
	e, _, _, _ := newASSearchHTTP(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/keywords/suggest?q="+url.QueryEscape("안녕하세요 감사합니다 팝업 오류"), nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Items []model.ASKeyword `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, it := range out.Items {
		if it.Keyword == "안녕하세요" || it.Keyword == "감사합니다" {
			t.Fatalf("후보에 인사말: %+v", it)
		}
		got[it.Keyword] = true
	}
	if !got["팝업"] || !got["오류"] {
		t.Fatalf("사전 대조 후보: %+v", out.Items)
	}
}

func TestASKeywordBrowseAndDictPage(t *testing.T) {
	e, asRepo, _, kw := newASSearchHTTP(t)
	items, _, _ := asRepo.SearchAS(model.ASSearchFilter{Query: "무인예약", PageSize: 5})
	if len(items) == 0 {
		t.Fatal("seed")
	}
	if err := kw.ReplaceLinks(items[0].ASID, model.KWFieldSymptom, []string{"KW005"}, []string{"KW005"}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/search?keyword_id=KW005", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, items[0].ASNumber) {
		t.Fatal("키워드 모아보기에 연결 건이 없다")
	}
	if !strings.Contains(body, "무인예약") {
		t.Fatal("모아보기 화면에 키워드가 없다")
	}

	pg := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "http://localhost/as/keywords", nil)
	req2.AddCookie(jwtCookie(t))
	e.ServeHTTP(pg, req2)
	if pg.Code != http.StatusOK {
		t.Fatalf("dict status=%d", pg.Code)
	}
	if strings.Contains(pg.Body.String(), `name="keyword" value="안녕하세요"`) {
		t.Fatal("빈도 후보에 안녕하세요가 있다")
	}
}
