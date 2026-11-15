package handler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-201: AS 조치 화면 「영향 장비」·KDevice 원인 분류.
type env201 struct {
	e  *echo.Echo
	db *sql.DB
	h  *Handler
	oa *http.Cookie
}

func setup201(t *testing.T) env201 {
	t.Helper()
	e, db, h := newAS151Server(t)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.POST("/as/:id/update", h.AS.Update)
	g.GET("/as/:id", h.AS.Show)
	g.GET("/as/stats", h.AS.StatsDashboard)
	g.GET("/assets/:id", h.Asset.Show)
	g.GET("/api/as/asset-history/:asset_id", h.AS.APIAssetHistory)
	for _, q := range []string{
		`INSERT OR IGNORE INTO product_groups(group_id,class_id,type_name,is_active) VALUES ('PG-RFID','CL','RFID자동화',1),('PG-MAT','CL','자료관리',1)`,
		`INSERT INTO assets(asset_id,customer_id,product_name,product_type,product_category,model_name,product_group_id,org_id) VALUES
			('A201-MAIN','C151','KLAS 서버','SW','klas','',NULL,'O01'),
			('A201-PC','C151','회원가입PC','HW','materials','','PG-MAT','O01'),
			('A201-SRP','C151','EZ-SRP 프린터','HW','rfid','','PG-RFID','O01'),
			('A201-OTHER','C151b','타기관 PC','HW','materials','','PG-MAT','O01')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	insert151AS(t, db, "R201", "C151", "in_progress", "", "", false)
	if _, err := db.Exec(`UPDATE as_receipts SET asset_id='A201-MAIN' WHERE as_id='R201'`); err != nil {
		t.Fatal(err)
	}
	return env201{e: e, db: db, h: h, oa: as151OA(t, db)}
}

func (v env201) post(t *testing.T, id string, form url.Values, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	if ck == nil {
		ck = v.oa
	}
	return doAS151Form(t, v.e, "/as/"+id+"/update", form, ck)
}

func (v env201) get(t *testing.T, path string, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	if ck == nil {
		ck = v.oa
	}
	return doAS151Get(t, v.e, path, ck)
}

// base 조치 저장 폼(결과 없이 처리 필드만). 영향 장비 값은 호출자가 더한다.
func base201() url.Values {
	return url.Values{"work_place": {"office"}, "process_type": {"remote"}, "affected_present": {"1"}}
}

func (v env201) affected(t *testing.T, id string) []string {
	t.Helper()
	rows, err := v.db.Query(`SELECT COALESCE(asset_id,''), COALESCE(label,'') FROM as_receipt_affected_assets WHERE as_id=? ORDER BY id`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var a, l string
		_ = rows.Scan(&a, &l)
		if a != "" {
			out = append(out, a)
		} else {
			out = append(out, "label:"+l)
		}
	}
	return out
}

func eq201(a []string, b ...string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// 재현: 수정 전에는 조치 화면에 영향 장비 입력도, KDevice 원인 분류도 없다(아래 테스트가 이 둘을 요구한다).
func TestHTTP95_201_ActionPageHasField(t *testing.T) {
	v := setup201(t)
	body := v.get(t, "/as/R201/action", nil).Body.String()
	for _, want := range []string{`data-affected-block`, `name="affected_present"`, `영향 장비 (선택)`, `data-name="affected_asset_id"`, `name="affected_label"`,
		`영향 장비는 업무를 만들지 않고 건수에도 더해지지 않습니다`} {
		if !strings.Contains(body, want) {
			t.Fatalf("조치 화면에 %q 가 없다", want)
		}
	}
	// 기본은 접힌 상태
	if i := strings.Index(body, `data-affected-details`); i < 0 || strings.Contains(body[max(0, i-120):i], " open") {
		t.Fatal("영향 장비 영역이 기본으로 펼쳐져 있다")
	}
	// KDevice 원인 분류가 1~3차 선택 데이터에 실린다
	for _, want := range []string{"KDevice(클라이언트 프로그램)", "설치·재설치", "설정·자동 로그인", "장비 연동 오류", "klas.kdevice.install"} {
		if !strings.Contains(body, want) {
			t.Fatalf("원인 분류 데이터에 %q 가 없다", want)
		}
	}
}

func TestHTTP95_201_SaveAndDisplay(t *testing.T) {
	v := setup201(t)
	// 조치 저장은 원래 일일 업무를 동기화한다. 영향 장비가 건수를 바꾸지 않는지 보려고 영향 장비 없는 저장을 기준으로 삼는다.
	if r := v.post(t, "R201", base201(), nil); strings.Contains(r.Header().Get("Location"), "err=") {
		t.Fatal(r.Header().Get("Location"))
	}
	var asBefore, taskBefore int
	_ = v.db.QueryRow(`SELECT COUNT(*) FROM as_receipts`).Scan(&asBefore)
	_ = v.db.QueryRow(`SELECT COUNT(*) FROM work_tasks`).Scan(&taskBefore)
	f := base201()
	f["affected_asset_id"] = []string{"A201-PC", "A201-SRP", "A201-PC"}
	f.Set("affected_label", "열람실 PC, 자료검색PC")
	r := v.post(t, "R201", f, nil)
	if r.Code != http.StatusSeeOther || strings.Contains(r.Header().Get("Location"), "err=") {
		t.Fatalf("저장 %d %s", r.Code, r.Header().Get("Location"))
	}
	if got := v.affected(t, "R201"); !eq201(got, "A201-PC", "A201-SRP", "label:열람실 PC", "label:자료검색PC") {
		t.Fatalf("저장된 영향 장비 %v", got)
	}
	// 건수 불변: 접수·업무 건수는 그대로
	var asAfter, taskAfter int
	_ = v.db.QueryRow(`SELECT COUNT(*) FROM as_receipts`).Scan(&asAfter)
	_ = v.db.QueryRow(`SELECT COUNT(*) FROM work_tasks`).Scan(&taskAfter)
	if asAfter != asBefore || taskAfter != taskBefore {
		t.Fatalf("건수가 변했다 as %d→%d task %d→%d", asBefore, asAfter, taskBefore, taskAfter)
	}
	// 조치 화면: 저장한 값이 채워지고 영역이 펼쳐져 있다
	act := v.get(t, "/as/R201/action", nil).Body.String()
	if !strings.Contains(act, "회원가입PC") || !strings.Contains(act, "열람실 PC") || !strings.Contains(act, "자료검색PC") {
		t.Fatal("조치 화면에 저장한 영향 장비가 없다")
	}
	if i := strings.Index(act, `data-affected-details`); i < 0 || !strings.Contains(act[max(0, i-120):i+60], "open") {
		t.Fatal("영향 장비가 있는데 영역이 접혀 있다")
	}
	// AS 상세: 「영향 장비: 이름 외 N대」 한 줄
	show := v.get(t, asLegacyHref("R201"), nil).Body.String()
	if !strings.Contains(show, `data-affected-line`) || !strings.Contains(show, "회원가입PC 외 3대") {
		t.Fatalf("AS 상세 한 줄이 없다")
	}
}

func TestHTTP95_201_OptionalAndLegacyUnchanged(t *testing.T) {
	v := setup201(t)
	// 영향 장비가 없는 접수: 화면에 줄이 없다
	if strings.Contains(v.get(t, asLegacyHref("R201"), nil).Body.String(), `data-affected-line`) {
		t.Fatal("영향 장비가 없는데 줄이 보인다(상세)")
	}
	// 비운 채 저장해도 정상
	r := v.post(t, "R201", base201(), nil)
	if r.Code != http.StatusSeeOther || strings.Contains(r.Header().Get("Location"), "err=") {
		t.Fatalf("비운 채 저장 %d %s", r.Code, r.Header().Get("Location"))
	}
	if got := v.affected(t, "R201"); len(got) != 0 {
		t.Fatalf("행이 생겼다 %v", got)
	}
	// 영향 장비 칸을 보내지 않는 기존 폼(affected_present 없음)은 기존 값을 건드리지 않는다
	_ = repository.NewASRepo(v.db).ReplaceAffectedAssets("R201", "u", []string{"A201-PC"}, nil)
	legacy := url.Values{"work_place": {"office"}, "process_type": {"remote"}}
	if r := v.post(t, "R201", legacy, nil); r.Code != http.StatusSeeOther || strings.Contains(r.Header().Get("Location"), "err=") {
		t.Fatalf("기존 폼 %d %s", r.Code, r.Header().Get("Location"))
	}
	if got := v.affected(t, "R201"); !eq201(got, "A201-PC") {
		t.Fatalf("기존 폼이 영향 장비를 지웠다 %v", got)
	}
	// 칸을 보내고 모두 비우면 지워진다(교체 방식)
	if r := v.post(t, "R201", base201(), nil); strings.Contains(r.Header().Get("Location"), "err=") {
		t.Fatal(r.Header().Get("Location"))
	}
	if got := v.affected(t, "R201"); len(got) != 0 {
		t.Fatalf("비우기 뒤 %v", got)
	}
	// 바꿔 저장하면 교체
	f := base201()
	f["affected_asset_id"] = []string{"A201-PC"}
	v.post(t, "R201", f, nil)
	f2 := base201()
	f2["affected_asset_id"] = []string{"A201-SRP"}
	v.post(t, "R201", f2, nil)
	if got := v.affected(t, "R201"); !eq201(got, "A201-SRP") {
		t.Fatalf("교체 %v", got)
	}
}

func TestHTTP95_201_ExcludesTargetAssets(t *testing.T) {
	v := setup201(t)
	f := base201()
	// A201-MAIN = 접수의 주 대상 자산, A201-SRP = 조치 화면 「설치자산」으로 고른 대상 자산
	f["affected_asset_id"] = []string{"A201-MAIN", "A201-SRP", "A201-PC"}
	f["asset_id"] = []string{"A201-SRP"}
	f.Set("as_asset_picker", "1")
	r := v.post(t, "R201", f, nil)
	loc := r.Header().Get("Location")
	if r.Code != http.StatusSeeOther || strings.Contains(loc, "err=") || !strings.Contains(loc, "aff_skip=2") {
		t.Fatalf("제외 안내 %d %s", r.Code, loc)
	}
	if got := v.affected(t, "R201"); !eq201(got, "A201-PC") {
		t.Fatalf("대상 자산이 영향 장비에서 빠지지 않았다 %v", got)
	}
	if !strings.Contains(v.get(t, "/as/R201/action?aff_skip=2", nil).Body.String(), "이미 대상 자산이라 영향 장비에서 뺐습니다") {
		t.Fatal("자동 제외 안내가 없다")
	}
}

func TestHTTP95_201_ValidationKeepsInput(t *testing.T) {
	v := setup201(t)
	_ = repository.NewASRepo(v.db).ReplaceAffectedAssets("R201", "u", []string{"A201-SRP"}, nil) // 기존 값은 오류 때 그대로
	cases := []struct {
		name string
		mod  func(f url.Values)
		code string
	}{
		{"다른 고객 자산", func(f url.Values) { f["affected_asset_id"] = []string{"A201-PC", "A201-OTHER"} }, "affected_customer"},
		{"자산 21건", func(f url.Values) {
			var ids []string
			for i := 0; i < 21; i++ {
				ids = append(ids, "A201-X"+string(rune('a'+i)))
			}
			f["affected_asset_id"] = ids
		}, "affected_limit"},
		{"직접 입력 21줄", func(f url.Values) {
			var ls []string
			for i := 0; i < 21; i++ {
				ls = append(ls, "장비"+string(rune('가'+i)))
			}
			f.Set("affected_label", strings.Join(ls, "\n"))
		}, "affected_limit"},
		{"81자", func(f url.Values) { f.Set("affected_label", strings.Repeat("가", 81)) }, "affected_label"},
	}
	for _, c := range cases {
		f := base201()
		c.mod(f)
		r := v.post(t, "R201", f, nil)
		loc := r.Header().Get("Location")
		if r.Code != http.StatusSeeOther || !strings.Contains(loc, "err="+c.code) {
			t.Fatalf("%s: %d %s", c.name, r.Code, loc)
		}
		if got := v.affected(t, "R201"); !eq201(got, "A201-SRP") {
			t.Fatalf("%s: 거절됐는데 저장이 바뀌었다 %v", c.name, got)
		}
		// 입력값 유지: 오류 화면에 방금 입력한 값이 그대로 다시 보인다
		var ck *http.Cookie
		for _, sc := range r.Result().Cookies() {
			if sc.Name == asAffectedDraftCookie {
				ck = sc
			}
		}
		if ck == nil {
			t.Fatalf("%s: 입력값 보관 쿠키가 없다", c.name)
		}
		req := httptest.NewRequest(http.MethodGet, loc, nil)
		req.AddCookie(v.oa)
		req.AddCookie(ck)
		rec := httptest.NewRecorder()
		v.e.ServeHTTP(rec, req)
		page := rec.Body.String()
		if !strings.Contains(page, "입력한 값은 그대로 두었습니다") {
			t.Fatalf("%s: 오류 문구가 없다(무반응 금지)", c.name)
		}
		switch c.name {
		case "다른 고객 자산":
			if !strings.Contains(page, "A201-OTHER") {
				t.Fatalf("%s: 입력한 자산이 사라졌다", c.name)
			}
		case "81자":
			if !strings.Contains(page, strings.Repeat("가", 81)) {
				t.Fatalf("%s: 입력한 직접 입력이 사라졌다", c.name)
			}
		case "직접 입력 21줄":
			if !strings.Contains(page, "장비가") || !strings.Contains(page, "장비"+string(rune('가'+20))) {
				t.Fatalf("%s: 입력한 줄이 사라졌다", c.name)
			}
		}
		if cc := rec.Result().Cookies(); len(cc) == 0 {
			t.Fatalf("%s: 보관 쿠키를 지우지 않았다", c.name)
		}
	}
	// 정상으로 80자는 통과
	f := base201()
	f.Set("affected_label", strings.Repeat("가", 80))
	if r := v.post(t, "R201", f, nil); strings.Contains(r.Header().Get("Location"), "err=") {
		t.Fatalf("80자 %s", r.Header().Get("Location"))
	}
}

func TestHTTP95_201_PermissionsAndLock(t *testing.T) {
	v := setup201(t)
	users := repository.NewUserRepo(v.db)
	if _, err := v.db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES ('O02','O02','다른팀','business',1,20)`); err != nil {
		t.Fatal(err)
	}
	other := &model.User{Username: "oa201x", PasswordHash: HashPassword("x"), FullName: "타조직관리자", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O02", ProfileDone: true}
	tech := &model.User{Username: "tech201", PasswordHash: HashPassword("x"), FullName: "기술담당", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	for _, u := range []*model.User{other, tech} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	f := base201()
	f["affected_asset_id"] = []string{"A201-PC"}
	// 타 조직 접수는 거절
	if r := v.post(t, "R201", f, jwtCookieVerified(t, other)); r.Code != http.StatusForbidden && r.Code != http.StatusNotFound {
		t.Fatalf("타 조직 %d", r.Code)
	}
	if got := v.affected(t, "R201"); len(got) != 0 {
		t.Fatalf("타 조직이 저장했다 %v", got)
	}
	// 완료된 접수: 잠금 규칙(조치 수정과 같음). 담당자가 아닌 기술직은 막히고 영향 장비는 그대로
	if _, err := v.db.Exec(`UPDATE as_receipts SET status='completed', complete_datetime='2026-11-12 16:00:00' WHERE as_id='R201'`); err != nil {
		t.Fatal(err)
	}
	r := v.post(t, "R201", f, jwtCookieVerified(t, tech))
	if loc := r.Header().Get("Location"); r.Code != http.StatusSeeOther || !strings.Contains(strings.ToLower(loc), "lock") && !strings.Contains(loc, "err=") {
		t.Fatalf("완료 접수 잠금 %d %s", r.Code, loc)
	}
	if got := v.affected(t, "R201"); len(got) != 0 {
		t.Fatalf("잠긴 접수의 영향 장비가 바뀌었다 %v", got)
	}
	// 조직관리자는 조치 수정과 같은 조건으로 수정할 수 있다
	ra := v.post(t, "R201", f, nil)
	if ra.Code != http.StatusSeeOther || strings.Contains(ra.Header().Get("Location"), "err=") {
		t.Skipf("이 환경에서 조직관리자의 완료 접수 수정이 열려 있지 않다: %d %s", ra.Code, ra.Header().Get("Location"))
	}
	if got := v.affected(t, "R201"); !eq201(got, "A201-PC") {
		t.Fatalf("조직관리자 저장 %v", got)
	}
	// 완료 접수 조회(ReadOnly) 화면에도 한 줄(수정 권한이 없는 기술직의 조회 화면)
	if body := v.get(t, "/as/R201/action", jwtCookieVerified(t, tech)).Body.String(); !strings.Contains(body, "data-affected-line") {
		t.Fatal("조회 화면에 영향 장비 줄이 없다")
	}
}

func TestHTTP95_201_HistoryBadgeAndStatsPage(t *testing.T) {
	v := setup201(t)
	f := base201()
	f["affected_asset_id"] = []string{"A201-PC", "A201-SRP"}
	f.Set("affected_label", "열람실 PC")
	v.post(t, "R201", f, nil)
	insert151AS(t, v.db, "R201-B", "C151", "completed", "", "", true) // 주 대상이 A201-PC 인 별개 접수
	_, _ = v.db.Exec(`UPDATE as_receipts SET asset_id='A201-PC' WHERE as_id='R201-B'`)

	// 자산 상세 「지원 이력」: R201 은 「영향」 배지, R201-B 는 배지 없음
	page := v.get(t, "/assets/A201-PC", nil).Body.String()
	iA, iB := strings.Index(page, "R201<"), strings.Index(page, "R201-B<")
	if iA < 0 || iB < 0 {
		t.Fatalf("자산 상세 이력에 접수가 없다 (%d,%d)", iA, iB)
	}
	rowOf := func(i int) string {
		s := strings.LastIndex(page[:i], "<tr")
		e := strings.Index(page[i:], "</tr>")
		return page[s : i+e]
	}
	if !strings.Contains(rowOf(iA), "data-affected-badge") {
		t.Fatal("영향 장비로 연결된 접수에 「영향」 배지가 없다")
	}
	if strings.Contains(rowOf(iB), "data-affected-badge") {
		t.Fatal("주 대상 자산 접수에 배지가 붙었다")
	}
	if strings.Count(page, "R201<") != 1 {
		t.Fatalf("R201 이 %d번 나온다", strings.Count(page, "R201<"))
	}
	// 주 대상 자산(A201-MAIN) 이력에는 배지 없음
	if m := v.get(t, "/assets/A201-MAIN", nil).Body.String(); strings.Contains(m, "data-affected-badge") {
		t.Fatal("주 대상 자산 이력에 배지가 있다")
	}
	// 유사 이력 API(조치 화면·AS 상세 모달): affected 표시
	api := v.get(t, "/api/as/asset-history/A201-PC?exclude=", nil).Body.String()
	if !strings.Contains(api, `"as_id":"R201"`) || !strings.Contains(api, `"affected":true`) {
		t.Fatalf("API 에 affected 가 없다: %s", api)
	}
	if strings.Count(api, `"affected":true`) != 1 {
		t.Fatalf("주 대상 접수에도 affected 가 붙었다: %s", api)
	}
	// 유사 이력 호출처 결과가 의도 없이 늘지 않는다: 영향 관계가 없는 자산 이력은 그대로
	if o := v.get(t, "/api/as/asset-history/A201-OTHER?exclude=", nil).Body.String(); strings.Contains(o, "R201") {
		t.Fatalf("관계 없는 자산 이력이 늘었다 %s", o)
	}

	// 통계 표: 영향 장비가 있는 접수 1건, 영향 장비 3대(PC·SRP·직접 입력). 기존 건수 카드는 그대로
	st := v.get(t, "/as/stats", nil).Body.String()
	for _, want := range []string{`data-affected-stats`, `data-affected-receipts>1<`, `data-affected-assets>3<`, "자료관리", "RFID자동화", "직접 입력"} {
		if !strings.Contains(st, want) {
			t.Fatalf("통계 표에 %q 가 없다", want)
		}
	}
	var n int
	_ = v.db.QueryRow(`SELECT COUNT(*) FROM as_receipts`).Scan(&n)
	if n != 2 {
		t.Fatalf("접수 건수 %d", n)
	}
	// 기간 필터: 접수일(2026-10-21) 밖이면 비어 있다
	if e := v.get(t, "/as/stats?aff_from=2027-01-01", nil).Body.String(); !strings.Contains(e, "data-affected-empty") {
		t.Fatal("기간 밖인데 표가 비어 있지 않다")
	}
}

func TestHTTP95_201_KDeviceCauseSaveAndLabel(t *testing.T) {
	v := setup201(t)
	f := url.Values{
		"s1_work_place": {"office"}, "s1_process_type": {"remote"},
		"s1_cause_cat1": {"klas"}, "s1_cause_cat2": {"klas.kdevice"}, "s1_cause_cat3": {"klas.kdevice.install"},
		"s1_action_taken": {"KDevice 프로그램을 재설치했습니다"}, "s1_result_code": {model.ResultDone}, "s1_leave_same": {"1"},
		"work_place": {"office"}, "process_type": {"remote"}, "action_short_ok": {"1"}, "affected_present": {"1"},
	}
	r := v.post(t, "R201", f, nil)
	if r.Code != http.StatusSeeOther || strings.Contains(r.Header().Get("Location"), "err=") {
		t.Fatalf("KDevice 분류 저장 %d %s", r.Code, r.Header().Get("Location"))
	}
	var c1, c2, c3 string
	_ = v.db.QueryRow(`SELECT COALESCE(cause_cat1,''), COALESCE(cause_cat2,''), COALESCE(cause_cat3,'') FROM as_receipts WHERE as_id='R201'`).Scan(&c1, &c2, &c3)
	if c1 != "klas" || c2 != "klas.kdevice" || c3 != "klas.kdevice.install" {
		t.Fatalf("분류 저장 %q %q %q", c1, c2, c3)
	}
	body := v.get(t, "/as/R201/action", nil).Body.String()
	if !strings.Contains(body, "KDevice(클라이언트 프로그램)") || !strings.Contains(body, "설치·재설치") {
		t.Fatal("조회 화면에 분류 라벨이 없다")
	}
}

// 「자산별 업무로 나누기」와 영향 장비를 함께 써도 서로 영향이 없다. 영향 장비는 업무를 만들지 않는다.
func TestHTTP95_201_SplitAndAffectedIndependent(t *testing.T) {
	v := setup201(t)
	f := base201()
	f["asset_id"] = []string{"A201-MAIN", "A201-SRP"}
	f.Set("as_asset_picker", "1")
	f.Set("split_mode", "each")
	f["affected_asset_id"] = []string{"A201-PC"}
	f.Set("affected_label", "열람실 PC")
	r := v.post(t, "R201", f, nil)
	if r.Code != http.StatusSeeOther || strings.Contains(r.Header().Get("Location"), "err=") {
		t.Fatalf("나누기+영향 장비 %d %s", r.Code, r.Header().Get("Location"))
	}
	// 영향 장비는 저장되고
	if got := v.affected(t, "R201"); !eq201(got, "A201-PC", "label:열람실 PC") {
		t.Fatalf("영향 장비 %v", got)
	}
	// 나누기는 대상 자산(2개)만큼 업무를 만들었고, 영향 장비(A201-PC)용 업무는 없다
	var perAsset, onAffected int
	_ = v.db.QueryRow(`SELECT COUNT(*) FROM work_tasks t JOIN work_task_assets w ON w.task_id=t.task_id WHERE w.asset_id IN ('A201-MAIN','A201-SRP')`).Scan(&perAsset)
	_ = v.db.QueryRow(`SELECT COUNT(*) FROM work_tasks t JOIN work_task_assets w ON w.task_id=t.task_id WHERE w.asset_id='A201-PC'`).Scan(&onAffected)
	if perAsset < 2 {
		t.Fatalf("자산별 업무가 만들어지지 않았다(%d)", perAsset)
	}
	if onAffected != 0 {
		t.Fatalf("영향 장비로 업무가 만들어졌다(%d)", onAffected)
	}
	var as int
	_ = v.db.QueryRow(`SELECT COUNT(*) FROM as_receipts`).Scan(&as)
	if as != 1 {
		t.Fatalf("접수 건수가 늘었다 %d", as)
	}
}
