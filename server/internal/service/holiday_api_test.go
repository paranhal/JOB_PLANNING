package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildRestDeURLMethodA(t *testing.T) {
	// Decoding 키(+ ==). Encode() 한 번만.
	u, err := buildRestDeURL(holidayAPIBase, "abc+def==", 2026, 9)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(u, "/getRestDeInfo?") {
		t.Fatalf("getRestDeInfo 만 써야 함: %s", u)
	}
	if strings.Contains(u, "getHoliDeInfo") {
		t.Fatal("getHoliDeInfo 를 쓰면 안 된다")
	}
	if strings.Contains(u, "%252B") {
		t.Fatalf("이중 인코딩: %s", u)
	}
	if !strings.Contains(u, "serviceKey=abc%2Bdef%3D%3D") {
		t.Fatalf("Decoding 키가 한 번만 인코딩돼야 함: %s", u)
	}
	if !strings.Contains(u, "solYear=2026") || !strings.Contains(u, "solMonth=09") {
		t.Fatalf("연월: %s", u)
	}
	if !strings.Contains(u, "numOfRows=100") || !strings.Contains(u, "_type=json") {
		t.Fatalf("고정 파라미터: %s", u)
	}

	// Encoding 키는 한 번 디코드한 뒤 Encode. %252B 가 되면 안 된다.
	u2, err := buildRestDeURL(holidayAPIBase, "abc%2Bdef%3D%3D", 2026, 9)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(u2, "%252B") {
		t.Fatalf("Encoding 키 이중 인코딩: %s", u2)
	}
	if !strings.Contains(u2, "serviceKey=abc%2Bdef%3D%3D") {
		t.Fatalf("Encoding 키 정규화 실패: %s", u2)
	}
}

func TestParseRestDeJSON(t *testing.T) {
	raw := []byte(`{
		"response":{"header":{"resultCode":"00","resultMsg":"NORMAL SERVICE."},
		"body":{"items":{"item":[
			{"dateName":"추석","isHoliday":"Y","locdate":20260925},
			{"dateName":"추석 연휴","isHoliday":"Y","locdate":"20260924"},
			{"dateName":"제헌절","isHoliday":"N","locdate":20260717}
		]}}}
	}`)
	items, err := parseRestDeJSON(raw, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("isHoliday=Y 만 %d", len(items))
	}
	seen := map[string]string{}
	for _, h := range items {
		seen[h.Date] = h.Name
	}
	if seen["2026-09-25"] != "추석" || seen["2026-09-24"] != "추석 연휴" {
		t.Fatalf("%v", seen)
	}
	if _, ok := seen["2026-07-17"]; ok {
		t.Fatal("isHoliday=N 은 넣으면 안 됨")
	}
}

func TestParseRestDeJSONAuthError(t *testing.T) {
	raw := []byte(`{"response":{"header":{"resultCode":"30","resultMsg":"SERVICE_KEY_IS_NOT_REGISTERED_ERROR"}}}`)
	_, err := parseRestDeJSON(raw, 2026)
	if err == nil || !strings.Contains(err.Error(), "SERVICE_KEY_IS_NOT_REGISTERED_ERROR") {
		t.Fatalf("err=%v", err)
	}
}

func TestFetchRestDeUsesGetRestDeInfo(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"response":{"header":{"resultCode":"00"},"body":{"items":""}}}`))
	}))
	defer srv.Close()
	c := &HolidayAPIClient{HTTP: srv.Client(), BaseURL: srv.URL, Key: "abc+def=="}
	items, err := c.FetchRestDe(2026, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("%d", len(items))
	}
	if !strings.HasSuffix(gotPath, "/getRestDeInfo") {
		t.Fatalf("path=%s", gotPath)
	}
}
