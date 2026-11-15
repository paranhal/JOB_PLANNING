package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-202: 「AS 접수·조치」 하단 「자산 연결이 없는 작업」.
type env202 struct {
	e  *echo.Echo
	db *sql.DB
	h  *Handler
	oa *http.Cookie
}

func setup202(t *testing.T) env202 {
	t.Helper()
	e, db, h := newAS151Server(t) // 도서관(O01) 조직관리자 oa151, 고객 C151·C151b
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/as", h.AS.List)
	g.GET("/as/kanban", h.AS.Kanban)
	g.GET("/as/unlinked/counts", h.AS.UnlinkedCounts)
	g.GET("/as/unlinked/as", h.AS.UnlinkedAS)
	g.GET("/as/unlinked/mnt", h.AS.UnlinkedMnt)
	g.POST("/as/unlinked/link-single", h.AS.UnlinkedLinkSingle)
	g.GET("/maintenance/visits/:visit_id/action", h.Maintenance.VisitAction)
	for _, q := range []string{
		`INSERT OR IGNORE INTO product_groups(group_id,class_id,type_name,is_active) VALUES ('PG-RFID','CL','RFID자동화',1),('PG-MAT','CL','자료관리',1)`,
		// C151: 자산 3대(자료관리 1·RFID자동화 2) / C151b: 자산 없음
		`INSERT INTO assets(asset_id,customer_id,product_name,product_type,product_category,model_name,product_group_id,org_id) VALUES
			('A202-1','C151','KLAS 서버','SW','klas','','PG-MAT','O01'),
			('A202-2','C151','무인반납기','HW','rfid','','PG-RFID','O01'),
			('A202-3','C151','자가대출기','HW','rfid','','PG-RFID','O01')`,
		`INSERT INTO maintenance_plans(plan_id,plan_year,title,status,org_id) VALUES ('MP202',2026,'정기점검','approved','O01'),('MP202X',2026,'타조직','approved','O02')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return env202{e: e, db: db, h: h, oa: as151OA(t, db)}
}

func (v env202) hx(t *testing.T, path string, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	if ck == nil {
		ck = v.oa
	}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Current-URL", "http://localhost/as?status=in_progress")
	req.AddCookie(ck)
	rec := httptest.NewRecorder()
	v.e.ServeHTTP(rec, req)
	return rec
}

func (v env202) as(t *testing.T, id, cust, status, recv string) {
	t.Helper()
	// 완료 계열은 완료 일시를 채운다(/as 기존 목록이 days_elapsed 를 NULL 없이 읽기 위한 실제 데이터 모양)
	var comp interface{}
	if status == "completed" || status == "partial_complete" {
		comp = recv + " 17:00:00"
	}
	if _, err := v.db.Exec(`INSERT INTO as_receipts(as_id,as_number,customer_id,receipt_datetime,complete_datetime,status,symptom,assigned_to,org_id,cause_cat1)
		VALUES (?,?,?,?,?,?,?,?,'O01',?)`, id, id, cust, recv+" 09:00:00", comp, status, "증상 "+id, "도서관장", "klas"); err != nil {
		t.Fatal(err)
	}
}

func (v env202) visit(t *testing.T, id, plan, cust, date, prod string, round int, done int) {
	t.Helper()
	if _, err := v.db.Exec(`INSERT INTO maintenance_visits(visit_id,plan_id,visit_date,customer_id,product_type,round_no,completed,assignee,notes,duration_min)
		VALUES (?,?,?,?,?,?,?,'점검담당','비고 '||?,90)`, id, plan, date, cust, prod, round, done, id); err != nil {
		t.Fatal(err)
	}
}

func countIn(body, tab string) int {
	m := regexp.MustCompile(`data-unlinked-count="` + tab + `">(\d+)<`).FindStringSubmatch(body)
	if m == nil {
		return -1
	}
	n := 0
	fmt.Sscanf(m[1], "%d", &n)
	return n
}

func seed202Basic(t *testing.T, v env202) {
	t.Helper()
	// AS: 연결 없음 3(진행·완료·부분완료), 연결 있음 2(주 자산·업무 자산), 제외 2(취소·테스트)
	v.as(t, "U-AS1", "C151", "in_progress", "2026-11-10")
	v.as(t, "U-AS2", "C151", "completed", "2026-11-05")
	v.as(t, "U-AS3", "C151b", "partial_complete", "2026-11-01")
	v.as(t, "L-AS1", "C151", "completed", "2026-11-02")
	v.as(t, "L-AS2", "C151", "completed", "2026-11-03")
	v.as(t, "X-CANCEL", "C151", "cancelled", "2026-11-04")
	v.as(t, "X-TEST", "C151", "completed", "2026-11-04")
	for _, q := range []string{
		`UPDATE as_receipts SET asset_id='A202-1' WHERE as_id='L-AS1'`,
		`UPDATE as_receipts SET is_test=1 WHERE as_id='X-TEST'`,
		`INSERT INTO work_tasks(task_id,work_type,title,status,org_id,source_type,source_id) VALUES ('WT-LAS2','admin','AS 업무','complete','O01','as','L-AS2')`,
		`INSERT INTO work_task_assets(task_id,asset_id) VALUES ('WT-LAS2','A202-2')`,
	} {
		if _, err := v.db.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	// 정기점검: 연결 없음 3(방문 targets 없음·targets 에 빈 asset_id·완료), 연결 있음 2(targets 자산·업무 자산), 제외 2(테스트·타 조직 계획)
	v.visit(t, "U-V1", "MP202", "C151", "2026-11-20", "KLAS", 2, 0)
	v.visit(t, "U-V2", "MP202", "C151b", "2026-10-20", "앤로보틱스", 1, 1)
	v.visit(t, "U-V3", "MP202", "C151", "2026-10-01", "KLAS", 1, 1)
	v.visit(t, "L-V1", "MP202", "C151", "2026-09-20", "KLAS", 1, 1)
	v.visit(t, "L-V2", "MP202", "C151", "2026-09-10", "KLAS", 1, 1)
	v.visit(t, "X-VT", "MP202", "C151", "2026-09-05", "KLAS", 1, 0)
	v.visit(t, "X-VO", "MP202X", "C151", "2026-09-04", "KLAS", 1, 0)
	for _, q := range []string{
		`UPDATE maintenance_visits SET is_test=1 WHERE visit_id='X-VT'`,
		`INSERT INTO maintenance_visit_targets(target_id,visit_id,asset_id,name) VALUES ('T1','U-V3','','빈 자산'),('T2','L-V1','A202-3','연결')`,
		`INSERT INTO work_tasks(task_id,work_type,title,status,org_id,source_type,source_id) VALUES ('WT-LV2','admin','점검 업무','complete','O01','maintenance','L-V2')`,
		`INSERT INTO work_task_assets(task_id,asset_id) VALUES ('WT-LV2','A202-1')`,
	} {
		if _, err := v.db.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
}

// 재현: 수정 전에는 /as 하단에 이 영역이 없었다. 첫 응답에는 틀만 있고 목록·건수 쿼리는 실행되지 않는다.
func TestHTTP95_202_FirstPaintHasShellOnly(t *testing.T) {
	v := setup202(t)
	seed202Basic(t, v)
	for _, path := range []string{"/as", "/as?display=kanban"} {
		body := doAS151Get(t, v.e, path, v.oa).Body.String()
		for _, want := range []string{`data-unlinked-root`, `hx-get="/as/unlinked/counts"`, `hx-trigger="intersect once"`, "자산 연결이 없는 작업", "/static/js/as_unlinked.js?v=" + AssetQuery()} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s: 첫 응답에 %q 가 없다", path, want)
			}
		}
		if strings.Contains(body, "data-unlinked-row") {
			t.Fatalf("%s: 첫 응답에 목록이 들어 있다(지연 로딩이어야 한다)", path)
		}
	}
	// 첫 응답이 이 목록의 쿼리를 실행하지 않는다: 이 목록만 읽는 테이블을 없애도 /as 는 그대로 200 이다
	if _, err := v.db.Exec(`ALTER TABLE maintenance_visit_targets RENAME TO mvt_gone`); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/as", "/as?display=kanban"} {
		if r := doAS151Get(t, v.e, path, v.oa); r.Code != http.StatusOK {
			t.Fatalf("%s: %d — 첫 화면이 지연 로딩 쿼리에 의존한다", path, r.Code)
		}
	}
	// 부분 응답을 부르면 그때서야 실행되어(여기서는 실패해서) 영역 안에 오류가 보인다
	body := v.hx(t, "/as/unlinked/counts", nil).Body.String()
	if !strings.Contains(body, "data-unlinked-error") || !strings.Contains(body, "다시 시도") {
		t.Fatalf("조회 실패가 영역 안 오류로 보이지 않는다: %s", body[:min(300, len(body))])
	}
}

func TestHTTP95_202_CountsAndJudgment(t *testing.T) {
	v := setup202(t)
	seed202Basic(t, v)
	body := v.hx(t, "/as/unlinked/counts", nil).Body.String()
	if got := countIn(body, "as"); got != 3 {
		t.Fatalf("AS 건수 %d (기대 3: 주 자산·업무 자산 있는 건, 취소·테스트 제외)", got)
	}
	if got := countIn(body, "mnt"); got != 3 {
		t.Fatalf("정기점검 건수 %d (기대 3: targets·업무 자산 있는 방문, 테스트·타 조직 제외)", got)
	}
	if !strings.Contains(body, `role="tablist"`) || !strings.Contains(body, `hx-trigger="load"`) {
		t.Fatal("탭·본문 틀이 없다")
	}
	// 건수와 목록이 같은 판정을 쓴다
	for tab, want := range map[string]int{"as": 3, "mnt": 3} {
		list := v.hx(t, "/as/unlinked/"+tab, nil).Body.String()
		if n := strings.Count(list, `data-unlinked-row=`) / 2; n != want { // 표 + 카드형 두 벌
			t.Fatalf("%s 목록 행 %d (기대 %d)", tab, n, want)
		}
		if !strings.Contains(list, fmt.Sprintf(`data-unlinked-total-n>%d<`, want)) {
			t.Fatalf("%s 목록 총건수 표시가 건수와 다르다", tab)
		}
	}
	// 제외 대상은 어디에도 나오지 않는다
	asList := v.hx(t, "/as/unlinked/as", nil).Body.String()
	for _, id := range []string{"L-AS1", "L-AS2", "X-CANCEL", "X-TEST"} {
		if strings.Contains(asList, id) {
			t.Fatalf("제외 대상 %s 가 목록에 있다", id)
		}
	}
	mntList := v.hx(t, "/as/unlinked/mnt", nil).Body.String()
	for _, id := range []string{"L-V1", "L-V2", "X-VT", "X-VO"} {
		if strings.Contains(mntList, id) {
			t.Fatalf("제외 대상 %s 가 목록에 있다", id)
		}
	}
}

func rowOf202(t *testing.T, body, id string) string {
	t.Helper()
	i := strings.Index(body, `data-unlinked-row="`+id+`"`)
	if i < 0 {
		t.Fatalf("행 %s 가 없다", id)
	}
	end := strings.Index(body[i:], "</tr>")
	if end < 0 {
		t.Fatalf("행 %s 끝이 없다", id)
	}
	return body[i : i+end]
}

func TestHTTP95_202_RowContentAndLinks(t *testing.T) {
	v := setup202(t)
	seed202Basic(t, v)
	// 행 단위 값 일치: U-AS1(진행중) — 고객 C151(자산 3대: 자료관리 1·RFID자동화 2)
	if _, err := v.db.Exec(`UPDATE as_receipts SET symptom='KLAS 접속이 안 되고 화면이 멈춥니다. 재부팅해도 같은 증상이 반복되어 자료 검색과 대출 반납이 모두 불가능합니다 가나다라마바사아자차카타파하', project_id='WP-A', action_taken='재부팅 후 정상' WHERE as_id='U-AS1'`); err != nil {
		t.Fatal(err)
	}
	seed151Projects(t, v.db)
	_, _ = v.db.Exec(`UPDATE as_receipts SET project_id='WP-A' WHERE as_id='U-AS1'`)
	asList := v.hx(t, "/as/unlinked/as", nil).Body.String()
	row := rowOf202(t, asList, "U-AS1")
	for _, want := range []string{
		`href="` + asOpenHref("U-AS1") + `"`, "성환도서관", "2026-11-10", "KLAS 접속이 안 되고", "…", "도서관장", "진행중", "사업A",
		"3대 · RFID자동화 2 · 자료관리 1", `badge-neutral`, ">추정<",
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("AS 행에 %q 가 없다:\n%s", want, row)
		}
	}
	if !regexp.MustCompile(`KLAS\s*<span[^>]*>추정</span>`).MatchString(row) {
		t.Fatalf("제품 추정(KLAS)과 「추정」 배지가 한 칸에 없다:\n%s", row)
	}
	// 증상 전체는 title 툴팁에, 조치 요약은 값 그대로
	if !strings.Contains(row, `title="KLAS 접속이 안 되고 화면이 멈춥니다. 재부팅해도 같은 증상이 반복되어 자료 검색과 대출 반납이 모두 불가능합니다 가나다라마바사아자차카타파하"`) || !strings.Contains(row, "재부팅 후 정상") {
		t.Fatalf("증상 툴팁·조치 요약:\n%s", row)
	}
	// 자산이 없는 고객 + 원인 분류 라벨 + 사업 없음
	row3 := rowOf202(t, asList, "U-AS3")
	t.Logf("U-AS3 행: %s", row3)
	for _, want := range []string{"타기관", "부분완료", "자산 없음", "사업 없음", "KLAS"} {
		if !strings.Contains(row3, want) {
			t.Fatalf("U-AS3 행에 %q 가 없다:\n%s", want, row3)
		}
	}
	// 정기점검 행
	mnt := v.hx(t, "/as/unlinked/mnt", nil).Body.String()
	r1 := rowOf202(t, mnt, "U-V1")
	for _, want := range []string{`href="/maintenance/visits/U-V1/action"`, "성환도서관 KLAS 정기점검 2회", "2026-11-20", "예정", "KLAS · 2회", "점검담당", "사업 없음", "90분", "3대 · RFID자동화 2 · 자료관리 1", "비고 U-V1"} {
		if !strings.Contains(r1, want) {
			t.Fatalf("정기점검 행에 %q 가 없다:\n%s", want, r1)
		}
	}
	r2 := rowOf202(t, mnt, "U-V2")
	for _, want := range []string{"타기관", "RFID(앤로보틱스) · 1회", "완료", "자산 없음"} {
		if !strings.Contains(r2, want) {
			t.Fatalf("U-V2 행에 %q 가 없다:\n%s", want, r2)
		}
	}
	// 최근 순: 방문일 내림차순(11-20 → 10-20 → 10-01), 접수일 내림차순(11-10 → 11-05 → 11-01)
	if !(strings.Index(mnt, "U-V1") < strings.Index(mnt, "U-V2") && strings.Index(mnt, "U-V2") < strings.Index(mnt, "U-V3")) {
		t.Fatal("정기점검이 방문일 최근 순이 아니다")
	}
	if !(strings.Index(asList, "U-AS1") < strings.Index(asList, "U-AS2") && strings.Index(asList, "U-AS2") < strings.Index(asList, "U-AS3")) {
		t.Fatal("AS 가 접수일 최근 순이 아니다")
	}
	// 링크 요청이 열린다(AS 조치 화면 200). 정기점검 방문 조치 화면은 이 환경에서 렌더 가능한 만큼만 본다.
	if r := doAS151Get(t, v.e, asOpenHref("U-AS1"), v.oa); r.Code != http.StatusOK {
		t.Fatalf("AS 링크 %d", r.Code)
	}
}

func TestHTTP95_202_DisappearsWhenLinked(t *testing.T) {
	v := setup202(t)
	seed202Basic(t, v)
	n := func(tab string) int { return countIn(v.hx(t, "/as/unlinked/counts", nil).Body.String(), tab) }
	if n("as") != 3 || n("mnt") != 3 {
		t.Fatal("시작 건수")
	}
	// AS: 주 자산 경로
	_, _ = v.db.Exec(`UPDATE as_receipts SET asset_id='A202-1' WHERE as_id='U-AS1'`)
	if n("as") != 2 {
		t.Fatalf("주 자산 연결 뒤 %d", n("as"))
	}
	// AS: 업무 자산 경로
	_, _ = v.db.Exec(`INSERT INTO work_tasks(task_id,work_type,title,status,org_id,source_type,source_id) VALUES ('WT-U2','admin','t','complete','O01','as','U-AS2')`)
	_, _ = v.db.Exec(`INSERT INTO work_task_assets(task_id,asset_id) VALUES ('WT-U2','A202-2')`)
	if n("as") != 1 {
		t.Fatalf("업무 자산 연결 뒤 %d", n("as"))
	}
	// 정기점검: 방문 조치 자산(targets) 경로 / 방문 업무 자산 경로
	_, _ = v.db.Exec(`INSERT INTO maintenance_visit_targets(target_id,visit_id,asset_id,name) VALUES ('T9','U-V1','A202-1','연결')`)
	if n("mnt") != 2 {
		t.Fatalf("방문 자산 연결 뒤 %d", n("mnt"))
	}
	_, _ = v.db.Exec(`INSERT INTO work_tasks(task_id,work_type,title,status,org_id,source_type,source_id) VALUES ('WT-UV2','admin','t','complete','O01','maintenance','U-V2')`)
	_, _ = v.db.Exec(`INSERT INTO work_task_assets(task_id,asset_id) VALUES ('WT-UV2','A202-3')`)
	if n("mnt") != 1 {
		t.Fatalf("방문 업무 자산 연결 뒤 %d", n("mnt"))
	}
	if strings.Contains(v.hx(t, "/as/unlinked/as", nil).Body.String(), "U-AS1") {
		t.Fatal("연결한 AS 가 목록에 남아 있다")
	}
}

func TestHTTP95_202_FiltersAndPaging(t *testing.T) {
	v := setup202(t)
	// 연결 없는 AS 45건: 2026-10-01.. 하루씩(최근 순), 담당자는 3의 배수만 「김점검」, 완료/진행 번갈아
	for i := 1; i <= 45; i++ {
		id := fmt.Sprintf("PG-%02d", i)
		st := "completed"
		if i%2 == 0 {
			st = "in_progress"
		}
		v.as(t, id, "C151", st, fmt.Sprintf("2026-10-%02d", min(i, 28)))
		if i > 28 {
			_, _ = v.db.Exec(`UPDATE as_receipts SET receipt_datetime=? WHERE as_id=?`, fmt.Sprintf("2026-11-%02d 09:00:00", i-28), id)
		}
		if i%3 == 0 {
			_, _ = v.db.Exec(`UPDATE as_receipts SET assigned_to='김점검' WHERE as_id=?`, id)
		}
	}
	q := func(raw string) string { return v.hx(t, "/as/unlinked/as?"+raw, nil).Body.String() }
	rows := func(b string) int { return strings.Count(b, `data-unlinked-row=`) / 2 }
	p1, p2, p3 := q(""), q("page=2"), q("page=3")
	if rows(p1) != 20 || rows(p2) != 20 || rows(p3) != 5 {
		t.Fatalf("20건 페이지 %d/%d/%d", rows(p1), rows(p2), rows(p3))
	}
	if !strings.Contains(p1, "data-unlinked-pager") || !strings.Contains(p1, `aria-current="page"`) {
		t.Fatal("페이지 이동이 없다")
	}
	// 최근 순: 1쪽 첫 행이 가장 최근(PG-45), 마지막 쪽 마지막 행이 가장 오래된 것(PG-01)
	if strings.Index(p1, "PG-45") > strings.Index(p1, "PG-44") || !strings.Contains(p3, "PG-01") {
		t.Fatal("최근 순이 아니다")
	}
	// 페이지 링크는 /as 주소에 ul_* 를 남긴다(HX-Current-URL 의 다른 파라미터는 보존, 이전 ul_* 는 교체)
	if !strings.Contains(p1, `hx-push-url="/as?status=in_progress&amp;ul_page=2&amp;ul_tab=as"`) && !strings.Contains(p1, `hx-push-url="/as?status=in_progress&ul_page=2&ul_tab=as"`) {
		t.Fatalf("페이지 링크의 주소 유지 속성이 없다: %s", regexp.MustCompile(`hx-push-url="[^"]*"`).FindString(p1))
	}
	// 필터: 상태·기간·담당자·검색 조합
	done := q("status=done")
	if !strings.Contains(done, `data-unlinked-total-n>23<`) { // 홀수 23건
		t.Fatalf("완료 필터 %s", regexp.MustCompile(`data-unlinked-total-n>\d+<`).FindString(done))
	}
	open := q("status=open")
	if !strings.Contains(open, `data-unlinked-total-n>22<`) {
		t.Fatal("진행·예정 필터")
	}
	if b := q("from=2026-10-10&to=2026-10-12"); !strings.Contains(b, `data-unlinked-total-n>3<`) {
		t.Fatalf("기간 필터 %s", regexp.MustCompile(`data-unlinked-total-n>\d+<`).FindString(b))
	}
	if b := q("who=김점검"); !strings.Contains(b, `data-unlinked-total-n>15<`) {
		t.Fatalf("담당자 필터 %s", regexp.MustCompile(`data-unlinked-total-n>\d+<`).FindString(b))
	}
	if b := q("q=PG-07"); !strings.Contains(b, `data-unlinked-total-n>1<`) || !strings.Contains(b, "PG-07") {
		t.Fatal("검색 필터")
	}
	if b := q("status=done&who=김점검&from=2026-10-01&to=2026-10-30"); !strings.Contains(b, "조건에 맞는") {
		t.Fatal("조합 필터 표시")
	}
	if b := q("q=없는검색어"); !strings.Contains(b, "data-unlinked-empty") || !strings.Contains(b, "조건에 맞는 자산 연결이 없는 AS가 없습니다") {
		t.Fatal("빈 상태 문구")
	}
	// 잘못된 값은 무시된다(500 없음)
	if r := v.hx(t, "/as/unlinked/as?status=zzz&page=-3&from=nope", nil); r.Code != http.StatusOK {
		t.Fatalf("잘못된 파라미터 %d", r.Code)
	}
	// 건수 탭은 필터와 무관하게 범위 전체
	if countIn(v.hx(t, "/as/unlinked/counts?ul_tab=as&ul_status=done", nil).Body.String(), "as") != 45 {
		t.Fatal("탭 건수는 범위 전체여야 한다")
	}
	// 카운트 응답은 ul_* 로 받은 탭·필터를 본문 주소에 옮긴다(새로고침 유지)
	cb := v.hx(t, "/as/unlinked/counts?ul_tab=mnt&ul_status=done&ul_page=2", nil).Body.String()
	if !strings.Contains(cb, `hx-get="/as/unlinked/mnt?`) || !strings.Contains(cb, "status=done") || !strings.Contains(cb, "page=2") {
		t.Fatalf("주소의 탭·필터가 본문 요청에 옮겨지지 않았다: %s", regexp.MustCompile(`id="as-unlinked-body"[^>]*`).FindString(cb))
	}
}

func TestHTTP95_202_ScopeAndAuth(t *testing.T) {
	v := setup202(t)
	seed202Basic(t, v)
	users := repository.NewUserRepo(v.db)
	if _, err := v.db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES ('O02','O02','다른팀','business',1,20)`); err != nil {
		t.Fatal(err)
	}
	other := &model.User{Username: "oa202x", PasswordHash: HashPassword("x"), FullName: "타조직관리자", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O02", ProfileDone: true}
	tech := &model.User{Username: "tech202", PasswordHash: HashPassword("x"), FullName: "도서관장", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	techOther := &model.User{Username: "tech202b", PasswordHash: HashPassword("x"), FullName: "다른기술", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	for _, u := range []*model.User{other, tech, techOther} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	// 타 조직 사용자에게는 이 조직 건이 나오지 않는다(AS·정기점검 모두)
	ob := v.hx(t, "/as/unlinked/counts", jwtCookieVerified(t, other)).Body.String()
	if countIn(ob, "as") != 0 || countIn(ob, "mnt") != 1 { // 정기점검은 자기 조직 계획의 방문(X-VO) 1건만
		t.Fatalf("타 조직 범위가 다르다: as=%d mnt=%d", countIn(ob, "as"), countIn(ob, "mnt"))
	}
	om := v.hx(t, "/as/unlinked/mnt", jwtCookieVerified(t, other)).Body.String()
	if !strings.Contains(om, "X-VO") || strings.Contains(om, "U-V1") || strings.Contains(om, "U-V2") {
		t.Fatal("타 조직의 정기점검 목록 범위가 계획 조직과 다르다")
	}
	if strings.Contains(v.hx(t, "/as/unlinked/as", jwtCookieVerified(t, other)).Body.String(), "U-AS1") {
		t.Fatal("타 조직에 AS 목록이 보인다")
	}
	// 기술담당은 /as 처럼 「내 배정」이 기본(담당자가 「도서관장」인 건만). 해제(mine=0)하면 전체
	tb := v.hx(t, "/as/unlinked/counts", jwtCookieVerified(t, tech)).Body.String()
	if countIn(tb, "as") != 3 { // 모든 AS 의 담당자가 「도서관장」
		t.Fatalf("내 배정 AS %d", countIn(tb, "as"))
	}
	if countIn(tb, "mnt") != 0 { // 방문 담당자는 「점검담당」
		t.Fatalf("내 배정 정기점검 %d", countIn(tb, "mnt"))
	}
	if n := countIn(v.hx(t, "/as/unlinked/counts", jwtCookieVerified(t, techOther)).Body.String(), "as"); n != 0 {
		t.Fatalf("남의 배정이 보인다 %d", n)
	}
	if n := countIn(v.hx(t, "/as/unlinked/counts?mine=0", jwtCookieVerified(t, techOther)).Body.String(), "as"); n != 3 {
		t.Fatalf("mine=0 전체 보기 %d", n)
	}
	// 정기점검 담당자 지정: assignee_name 으로 범위가 /as 와 같이 좁혀진다
	if n := countIn(v.hx(t, "/as/unlinked/counts?assignee_name=점검담당", nil).Body.String(), "mnt"); n != 3 {
		t.Fatalf("담당자 범위 %d", n)
	}
	// 로그인 없이는 거절
	req := httptest.NewRequest(http.MethodGet, "/as/unlinked/counts", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	v.e.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "data-unlinked-count") {
		t.Fatalf("로그인 없이 목록이 보인다 %d", rec.Code)
	}
}

func TestHTTP95_202_NoN1AndLargeData(t *testing.T) {
	v := setup202(t)
	// AS 850건 + 방문 870건(연결 없음)
	tx, _ := v.db.Begin()
	for i := 0; i < 850; i++ {
		_, _ = tx.Exec(`INSERT INTO as_receipts(as_id,as_number,customer_id,receipt_datetime,complete_datetime,status,symptom,assigned_to,org_id) VALUES (?,?,?,?,?,?,?,?, 'O01')`,
			fmt.Sprintf("BIG-%04d", i), fmt.Sprintf("BIG-%04d", i), []string{"C151", "C151b"}[i%2], fmt.Sprintf("2026-%02d-%02d 09:00:00", 1+i%10, 1+i%27), fmt.Sprintf("2026-%02d-%02d 17:00:00", 1+i%10, 1+i%27), "completed", "대량 증상", "도서관장")
	}
	for i := 0; i < 870; i++ {
		_, _ = tx.Exec(`INSERT INTO maintenance_visits(visit_id,plan_id,visit_date,customer_id,product_type,round_no,completed) VALUES (?, 'MP202', ?, ?, 'KLAS', 1, 1)`,
			fmt.Sprintf("BV-%04d", i), time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i/2).Format("2006-01-02"), []string{"C151", "C151b"}[i%2])
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	before := repository.UnlinkedCandidateQueries()
	t0 := time.Now()
	body := v.hx(t, "/as/unlinked/as", nil).Body.String()
	dAS := time.Since(t0)
	if got := repository.UnlinkedCandidateQueries() - before; got != 1 {
		t.Fatalf("후보 자산 집계 쿼리 %d회(목록당 1회여야 한다, N+1 금지)", got)
	}
	if !strings.Contains(body, `data-unlinked-total-n>850<`) {
		t.Fatalf("대량 총건수 %s", regexp.MustCompile(`data-unlinked-total-n>\d+<`).FindString(body))
	}
	before = repository.UnlinkedCandidateQueries()
	t1 := time.Now()
	mnt := v.hx(t, "/as/unlinked/mnt", nil).Body.String()
	dM := time.Since(t1)
	if got := repository.UnlinkedCandidateQueries() - before; got != 1 {
		t.Fatalf("정기점검 후보 자산 집계 쿼리 %d회", got)
	}
	if !strings.Contains(mnt, `data-unlinked-total-n>870<`) {
		t.Fatalf("정기점검 대량 총건수 %s", regexp.MustCompile(`data-unlinked-total-n>\d+<`).FindString(mnt))
	}
	t2 := time.Now()
	counts := v.hx(t, "/as/unlinked/counts", nil).Body.String()
	dC := time.Since(t2)
	if countIn(counts, "as") != 850 || countIn(counts, "mnt") != 870 {
		t.Fatalf("대량 건수 %d/%d", countIn(counts, "as"), countIn(counts, "mnt"))
	}
	t.Logf("대량(AS 850·방문 870) 응답 시간: AS 목록 %v · 정기점검 목록 %v · 건수 %v", dAS, dM, dC)
	for name, d := range map[string]time.Duration{"AS 목록": dAS, "정기점검 목록": dM, "건수": dC} {
		if d > 2*time.Second {
			t.Fatalf("%s 응답이 너무 느리다 %v", name, d)
		}
	}
}

// /as 첫 화면 성능 가드(기존 시험)가 이 변경 뒤에도 통과해야 한다: List 본문에 이 목록이 끼어들지 않는다.
func TestHTTP95_202_ListFunctionUntouched(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("internal", "handler", "as.go"))
	if err != nil {
		t.Fatal(err)
	}
	listFn := handlerMethod(string(src), "func (h *ASHandler) List")
	if strings.Contains(listFn, "Unlinked") || strings.Contains(listFn, "unlinked") {
		t.Fatal("List 본문이 자산 미연결 목록을 읽는다")
	}
}

var _ = url.Values{}

// Test95_202_BrowserUnlinkedWorks 실제 브라우저(390/820/1440): 탭 전환 → 필터 → 다음 페이지 → 작업명 클릭 → 뒤로 가기에서 필터 유지.
func TestBrowser95_202_UnlinkedWorks(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	v := setup202(t)
	// AS 27건(완료 22·진행 5), 정기점검 5건
	for i := 1; i <= 27; i++ {
		st := "completed"
		if i <= 5 {
			st = "in_progress"
		}
		v.as(t, fmt.Sprintf("BR-%02d", i), "C151", st, fmt.Sprintf("2026-10-%02d", i))
	}
	for i := 1; i <= 5; i++ {
		done := 1
		if i <= 2 {
			done = 0
		}
		v.visit(t, fmt.Sprintf("BV-%d", i), "MP202", "C151", fmt.Sprintf("2026-11-%02d", i), []string{"KLAS", "앤로보틱스"}[i%2], i, done)
	}
	g := v.e.Group("")
	g.Use(v.h.Auth.AuthMiddleware)
	g.POST("/as/:id/action-sets", v.h.AS.SaveActionSet)
	staticDir, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	v.e.Static("/static", staticDir)
	srv := httptest.NewServer(v.e)
	t.Cleanup(srv.Close)
	shot, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-202"))
	cmd := exec.Command("node", "95_202_unlinked_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+srv.URL,
		"AUTH_COOKIE="+jwtCookieVerified(t, mustUser203(t, env203{env201: env201{db: v.db}})).Value, "SCREENSHOT_DIR="+shot)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Executable doesn't exist") || strings.Contains(s, "browserType.launch") {
			t.Skip(s)
		}
		t.Fatalf("browser: %v", err)
	}
}
