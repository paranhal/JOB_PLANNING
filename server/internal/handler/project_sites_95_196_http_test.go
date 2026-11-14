package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// 95-196: 사업 상세 「사이트·진행」 빈 상태·고객 하위 기관 카드·기본 유형 탭.

type env196 struct {
	e      *echo.Echo
	vision *http.Cookie
	oa     *http.Cookie
	oa2    *http.Cookie
	tech   *http.Cookie
}

func setup196(t *testing.T) env196 {
	t.Helper()
	e, _, _, _, db := newProjectServerWithSQL(t)
	exec := func(q string, a ...interface{}) {
		if _, err := db.Exec(q, a...); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	cust := func(id, name, industry, parent, kind string, active int, sido, sigungu string) {
		var par interface{}
		if parent != "" {
			par = parent
		}
		exec(`INSERT INTO customers(customer_id,org_name,official_name,industry,parent_customer_id,party_kind,is_active,addr_sido,addr_sigungu,has_parent)
			VALUES (?,?,?,?,?,?,?,?,?,?)`, id, name, name, industry, par, kind, active, sido, sigungu, map[bool]int{true: 1, false: 0}[parent != ""])
	}
	// 도서관 사업의 고객: 하위 도서관 3(+비활성 1, +거래처 1), 공공기관 1, 업종 없음 1, 손자 도서관 1(2단계), 4단계 도서관 1
	cust("C196-LIB", "충남교육청(시험)", "교육청", "", "customer", 1, "충청남도", "")
	cust("C196-L1", "가도서관", "도서관", "C196-LIB", "customer", 1, "충청남도", "천안시")
	cust("C196-L2", "나도서관", "도서관", "C196-LIB", "customer", 1, "충청남도", "홍성군")
	cust("C196-L3", "다도서관", "도서관", "C196-LIB", "customer", 1, "충청남도", "공주시")
	cust("C196-LX", "비활성도서관", "도서관", "C196-LIB", "customer", 0, "충청남도", "천안시")
	cust("C196-LV", "거래처도서관", "도서관", "C196-LIB", "vendor", 1, "충청남도", "천안시")
	cust("C196-P1", "라공공기관", "공공기관", "C196-LIB", "customer", 1, "충청남도", "천안시")
	cust("C196-N1", "마분류없음", "", "C196-LIB", "customer", 1, "충청남도", "천안시")
	cust("C196-G1", "손자도서관", "도서관", "C196-L1", "customer", 1, "충청남도", "천안시")
	cust("C196-G2", "증손도서관", "도서관", "C196-G1", "customer", 1, "충청남도", "천안시")
	cust("C196-G3", "고손도서관(4단계)", "도서관", "C196-G2", "customer", 1, "충청남도", "천안시")
	// 학교 사업의 고객: 하위 학교 2
	cust("C196-SCH", "학교 발주처", "교육청", "", "customer", 1, "충청남도", "")
	cust("C196-S1", "가초등학교", "학교", "C196-SCH", "customer", 1, "충청남도", "천안시")
	cust("C196-S2", "나초등학교", "학교", "C196-SCH", "customer", 1, "충청남도", "천안시")
	// 하위 기관이 없는 고객
	cust("C196-NONE", "하위 없는 고객", "교육청", "", "customer", 1, "충청남도", "")
	for _, q := range []string{
		`INSERT INTO work_projects(project_id,name,status,org_id,customer_id) VALUES ('P196-LIB','도서관 사업','active','O01','C196-LIB')`,
		`INSERT INTO work_projects(project_id,name,status,org_id,customer_id) VALUES ('P196-SCH','학교 사업','active','O01','C196-SCH')`,
		`INSERT INTO work_projects(project_id,name,status,org_id,customer_id) VALUES ('P196-NONE','하위 없는 사업','active','O01','C196-NONE')`,
		`INSERT INTO work_projects(project_id,name,status,org_id) VALUES ('P196-NOCUST','고객 없는 사업','active','O01')`,
		`INSERT INTO work_projects(project_id,name,status,org_id,customer_id) VALUES ('P196-EXIST','기존 사이트 사업','active','O01','C196-LIB')`,
		`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,org_id,is_active) VALUES ('PS196-1','P196-EXIST','C196-P1','라공공기관','planned','O01',1)`,
	} {
		exec(q)
	}
	return env196{e: e,
		vision: cookie183(t, "va", "vision_admin", "O01", false),
		oa:     cookie183(t, "oa", "org_admin", "O01", false),
		oa2:    cookie183(t, "oa2", "org_admin", "O02", false),
		tech:   cookie183(t, "tk", "tech", "O01", false),
	}
}

func (v env196) do(t *testing.T, method, path string, form url.Values, ck *http.Cookie, json bool) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, "http://localhost"+path, nil)
	}
	if json {
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Requested-With", "fetch")
	}
	req.AddCookie(ck)
	rec := httptest.NewRecorder()
	v.e.ServeHTTP(rec, req)
	return rec
}

type childRes196 struct {
	Total        int `json:"total"`
	Unregistered int `json:"unregistered"`
	Chips        []struct {
		Industry string `json:"industry"`
		Count    int    `json:"count"`
	} `json:"chips"`
	Items []struct {
		ID         string `json:"id"`
		Registered bool   `json:"registered"`
		Depth      int    `json:"depth"`
	} `json:"items"`
}

func (v env196) children(t *testing.T, pid string, ck *http.Cookie) childRes196 {
	t.Helper()
	r := v.do(t, "GET", "/projects/"+pid+"/sites/children", nil, ck, true)
	if r.Code != 200 {
		t.Fatalf("children %d %s", r.Code, r.Body.String())
	}
	var out childRes196
	if err := json.Unmarshal(r.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestHTTP95_196_ProjectShowEmptyCardAndChildrenButton(t *testing.T) {
	v := setup196(t)
	body := v.do(t, "GET", "/projects/P196-LIB", nil, v.vision, false).Body.String()
	for _, want := range []string{"data-site-empty-card", "이 사업에 등록된 사이트가 없습니다", "사이트 등록하기", "data-site-progress-reg-btn"} {
		if !strings.Contains(body, want) {
			t.Fatalf("빈 상태에 %q 없음", want)
		}
	}
	// 하위 기관: 가·나·다 도서관 + 공공기관 + 분류 없음 + 손자 + 증손 = 7곳(비활성·거래처·4단계 제외)
	if !strings.Contains(body, "data-site-children-btn") || !strings.Contains(body, "고객 하위 기관 7곳 등록") {
		t.Fatalf("하위 기관 보조 버튼/수가 맞지 않다")
	}
	// 고객 없는 사업·하위 없는 사업: 빈 카드와 큰 버튼은 있고 하위 기관 버튼은 없다
	for _, id := range []string{"P196-NOCUST", "P196-NONE"} {
		b := v.do(t, "GET", "/projects/"+id, nil, v.vision, false).Body.String()
		if !strings.Contains(b, "data-site-empty-card") || strings.Contains(b, "data-site-children-btn") {
			t.Fatalf("%s: 빈 카드는 있고 하위 기관 버튼은 없어야 한다", id)
		}
	}
	// 권한 없는 사용자(일반·타 조직 조직관리자): 버튼 없이 안내만
	for _, ck := range []*http.Cookie{v.tech, v.oa2} {
		r := v.do(t, "GET", "/projects/P196-LIB", nil, ck, false)
		if r.Code != 200 {
			continue
		}
		b := r.Body.String()
		if strings.Contains(b, "data-site-progress-reg-btn") || strings.Contains(b, "data-site-children-btn") || !strings.Contains(b, "data-site-register-denied") {
			t.Fatal("권한 없는 사용자에게 등록 버튼이 보이거나 안내가 없다")
		}
	}
	// 사이트가 있는 사업: 빈 카드 없음, 등록 N곳 문구 있음, 하위 기관 보조 버튼은 남음
	b := v.do(t, "GET", "/projects/P196-EXIST", nil, v.vision, false).Body.String()
	if strings.Contains(b, "data-site-empty-card") || !strings.Contains(b, "등록 1곳") || !strings.Contains(b, "data-site-children-btn") {
		t.Fatal("사이트가 있는 사업의 표시가 맞지 않다")
	}
}

func TestHTTP95_196_DefaultSiteTypeTab(t *testing.T) {
	v := setup196(t)
	tab := func(path string) string {
		b := v.do(t, "GET", path, nil, v.vision, false).Body.String()
		i := strings.Index(b, `data-site-type="`)
		if i < 0 {
			t.Fatalf("%s 에 data-site-type 없음", path)
		}
		rest := b[i+len(`data-site-type="`):]
		return rest[:strings.Index(rest, `"`)]
	}
	// 하위 기관 업종 최다(도서관 4 · 공공 1)가 기본 → 도서관 탭
	if got := tab("/projects/P196-LIB/sites"); got != "library" {
		t.Fatalf("도서관 사업 기본 탭 %q want library", got)
	}
	// 학교 사업은 학교 탭(회귀)
	if got := tab("/projects/P196-SCH/sites"); got != "school" {
		t.Fatalf("학교 사업 기본 탭 %q want school", got)
	}
	// 하위도 사이트도 없으면 학교
	if got := tab("/projects/P196-NOCUST/sites"); got != "school" {
		t.Fatalf("기본 탭 %q want school", got)
	}
	// 기존 사이트가 먼저: 공공기관 사이트가 있는 사업은 하위 기관 최다(도서관)보다 공공기관
	if got := tab("/projects/P196-EXIST/sites"); got != "public" {
		t.Fatalf("기존 사이트 업종 우선 %q want public", got)
	}
	// 명시한 type 이 최우선, 잘못된 값은 school
	if got := tab("/projects/P196-LIB/sites?type=public"); got != "public" {
		t.Fatalf("명시 type %q", got)
	}
	if got := tab("/projects/P196-LIB/sites?type=xx"); got != "school" {
		t.Fatalf("잘못된 type 은 school 이어야 한다 %q", got)
	}
}

func TestHTTP95_196_ChildrenCandidatesAndRegister(t *testing.T) {
	v := setup196(t)
	// 하위 기관 카드: 하위가 있는 고객에게만
	if !strings.Contains(v.do(t, "GET", "/projects/P196-LIB/sites", nil, v.vision, false).Body.String(), "data-site-children-card") {
		t.Fatal("하위 기관 카드가 없다")
	}
	for _, id := range []string{"P196-NOCUST"} {
		if strings.Contains(v.do(t, "GET", "/projects/"+id+"/sites", nil, v.vision, false).Body.String(), "data-site-children-card") {
			t.Fatalf("%s 에 하위 기관 카드가 보인다", id)
		}
	}
	res := v.children(t, "P196-LIB", v.vision)
	if res.Total != 7 || res.Unregistered != 7 {
		t.Fatalf("전체/미등록 %d/%d want 7/7", res.Total, res.Unregistered)
	}
	ids := map[string]int{}
	for _, it := range res.Items {
		ids[it.ID] = it.Depth
	}
	for _, bad := range []string{"C196-LX", "C196-LV", "C196-G3"} {
		if _, ok := ids[bad]; ok {
			t.Fatalf("%s 는 후보에 없어야 한다(비활성·거래처·4단계)", bad)
		}
	}
	if ids["C196-G1"] != 2 || ids["C196-G2"] != 3 {
		t.Fatalf("손자·증손 단계 %v", ids)
	}
	chips := map[string]int{}
	for _, c := range res.Chips {
		chips[c.Industry] = c.Count
	}
	if chips["도서관"] != 5 || chips["공공기관"] != 1 || chips["(분류 없음)"] != 1 {
		t.Fatalf("업종 칩 %v", chips)
	}
	// 등록: 기존 add-schools 그대로 — 새 저장 로직 없음
	form := url.Values{"customer_id": {"C196-L1", "C196-L2", "C196-P1"}}
	r := v.do(t, "POST", "/projects/P196-LIB/sites/add-schools", form, v.vision, true)
	if r.Code != 200 {
		t.Fatalf("등록 %d %s", r.Code, r.Body.String())
	}
	var add struct {
		Added   int `json:"added"`
		Skipped int `json:"skipped"`
		Total   int `json:"siteTotal"`
	}
	_ = json.Unmarshal(r.Body.Bytes(), &add)
	if add.Added != 3 || add.Total != 3 {
		t.Fatalf("등록 결과 %+v", add)
	}
	after := v.children(t, "P196-LIB", v.vision)
	reg := 0
	for _, it := range after.Items {
		if it.Registered {
			reg++
		}
	}
	if after.Unregistered != 4 || reg != 3 {
		t.Fatalf("등록 뒤 미등록 %d 등록됨 %d want 4/3", after.Unregistered, reg)
	}
	// 재등록은 건너뜀(skippedItems 규칙 그대로)
	r = v.do(t, "POST", "/projects/P196-LIB/sites/add-schools", url.Values{"customer_id": {"C196-L1", "C196-L3"}}, v.vision, true)
	var again struct {
		Added   int `json:"added"`
		Skipped int `json:"skipped"`
	}
	_ = json.Unmarshal(r.Body.Bytes(), &again)
	if again.Added != 1 || again.Skipped != 1 {
		t.Fatalf("재등록 %+v want added 1 skipped 1", again)
	}
	// 사업 상세에 등록 N곳 반영, 빈 카드 사라짐
	b := v.do(t, "GET", "/projects/P196-LIB", nil, v.vision, false).Body.String()
	if strings.Contains(b, "data-site-empty-card") || !strings.Contains(b, "등록 4곳") {
		t.Fatal("등록 뒤 사업 상세 표시가 맞지 않다")
	}
	// 같은 조직 조직관리자도 등록 가능, 일반·타 조직은 403
	if r := v.do(t, "POST", "/projects/P196-SCH/sites/add-schools", url.Values{"customer_id": {"C196-S1"}}, v.oa, true); r.Code != 200 {
		t.Fatalf("같은 조직 조직관리자 %d", r.Code)
	}
	for _, ck := range []*http.Cookie{v.tech, v.oa2} {
		if r := v.do(t, "POST", "/projects/P196-SCH/sites/add-schools", url.Values{"customer_id": {"C196-S2"}}, ck, true); r.Code != http.StatusForbidden {
			t.Fatalf("권한 없는 사용자 등록 %d want 403", r.Code)
		}
	}
}

// 한 번 2,000곳 상한과 같은 이름 하위 기관·비활성 고객 처리는 기존 AddSitesFromCustomers 규칙 그대로.
func TestHTTP95_196_RegisterRulesUnchanged(t *testing.T) {
	v := setup196(t)
	var many url.Values = url.Values{}
	for i := 0; i < 2001; i++ {
		many.Add("customer_id", "X"+strings.Repeat("0", 4)+string(rune('a'+i%26))+string(rune('a'+i/26%26))+string(rune('a'+i/676%26)))
	}
	if r := v.do(t, "POST", "/projects/P196-LIB/sites/add-schools", many, v.vision, true); r.Code != http.StatusBadRequest || !strings.Contains(r.Body.String(), "2,000") {
		t.Fatalf("2001곳 %d %s", r.Code, r.Body.String())
	}
	// 비활성 고객·거래처(party_kind≠customer)는 건너뜀
	r := v.do(t, "POST", "/projects/P196-LIB/sites/add-schools", url.Values{"customer_id": {"C196-LX", "C196-LV", "C196-L3"}}, v.vision, true)
	var out struct {
		Added   int `json:"added"`
		Skipped int `json:"skipped"`
		Items   []struct {
			CustomerID string `json:"customer_id"`
			Reason     string `json:"reason"`
		} `json:"skippedItems"`
	}
	_ = json.Unmarshal(r.Body.Bytes(), &out)
	if out.Added != 1 || out.Skipped != 2 || len(out.Items) != 2 {
		t.Fatalf("비활성·거래처는 건너뛰어야 한다 %+v", out)
	}
	for _, it := range out.Items {
		if !strings.Contains(it.Reason, "활성 고객이 아닙니다") {
			t.Fatalf("건너뜀 사유 %q", it.Reason)
		}
	}
}
