package handler

import (
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

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-204 ③ 일괄 연결: 대상 판정(T1)·서버 재검증/권한/멱등(T2)·연결 결과(T3).

func (v env202) post204(t *testing.T, query string, form url.Values, ck *http.Cookie, withCSRF bool) *httptest.ResponseRecorder {
	t.Helper()
	if ck == nil {
		ck = v.oa
	}
	if form == nil {
		form = url.Values{}
	}
	const tok = "tok204tok204tok204"
	if withCSRF {
		form.Set("csrf", tok)
	}
	req := httptest.NewRequest(http.MethodPost, "/as/unlinked/link-single"+query, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Current-URL", "http://localhost/as?status=in_progress")
	req.AddCookie(ck)
	if withCSRF {
		req.AddCookie(&http.Cookie{Name: "jp_csrf", Value: tok})
	}
	rec := httptest.NewRecorder()
	v.e.ServeHTTP(rec, req)
	return rec
}

func linkForm(kind string, ids ...string) url.Values {
	f := url.Values{"kind": {kind}}
	for _, id := range ids {
		f.Add("ids", id)
	}
	return f
}

// seed204Link 자산이 1개인 고객 두 곳(자료관리 C204S·RFID C204T)과 연결 대상·비대상 건.
func seed204Link(t *testing.T, v env202) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id) VALUES ('C204S','솔도서관','솔도서관',1,'O01'),('C204T','티도서관','티도서관',1,'O01')`,
		`INSERT INTO assets(asset_id,customer_id,product_name,product_type,product_category,model_name,product_group_id,org_id) VALUES
			('A204S','C204S','자료관리서버','SW','materials','','PG-MAT','O01'),
			('A204T','C204T','무인반납기','HW','rfid','','PG-RFID','O01')`,
	} {
		if _, err := v.db.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
}

func hasCheck(body, id string) bool {
	return strings.Contains(body, `name="ids" value="`+id+`" form="ul-bulk-form"`)
}

func (v env202) assetOf(t *testing.T, asID string) string {
	t.Helper()
	var a string
	if err := v.db.QueryRow(`SELECT COALESCE(asset_id,'') FROM as_receipts WHERE as_id=?`, asID).Scan(&a); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestHTTP95_204_BulkTargetJudgment(t *testing.T) {
	v := setup202(t)
	seed204Link(t, v)
	v.as(t, "L-ONE", "C204S", "in_progress", "2026-11-05")  // 자산 1 → 대상
	v.as(t, "L-MANY", "C151", "in_progress", "2026-11-04")  // 자산 3 → 제외
	v.as(t, "L-ZERO", "C151b", "in_progress", "2026-11-03") // 자산 0 → 제외
	v.as(t, "L-DONE", "C204T", "completed", "2026-11-02")   // 완료 건도 접수자(조직관리자=담당 도서관장)는 가능
	v.as(t, "L-LINKED", "C204S", "in_progress", "2026-11-01")
	if _, err := v.db.Exec(`UPDATE as_receipts SET asset_id='A204S' WHERE as_id='L-LINKED'`); err != nil {
		t.Fatal(err)
	}
	if _, err := v.db.Exec(`INSERT INTO as_receipts(as_id,as_number,customer_id,receipt_datetime,status,symptom,org_id) VALUES ('L-OTHER','L-OTHER','C204S','2026-11-06 09:00:00','in_progress','타조직','O02')`); err != nil {
		t.Fatal(err)
	}
	body := v.hx(t, "/as/unlinked/as", nil).Body.String()
	for id, want := range map[string]bool{"L-ONE": true, "L-DONE": true, "L-MANY": false, "L-ZERO": false} {
		if got := hasCheck(body, id); got != want {
			t.Errorf("AS %s 체크박스 %v (기대 %v)", id, got, want)
		}
	}
	if strings.Contains(body, `data-unlinked-row="L-LINKED"`) || strings.Contains(body, `data-unlinked-row="L-OTHER"`) {
		t.Error("연결된 건·타 조직 건이 목록에 있다")
	}
	for _, want := range []string{"고객 자산이 2개 이상입니다", "고객 자산이 없습니다", "자료관리서버"} {
		if !strings.Contains(body, want) {
			t.Errorf("사유·연결될 자산 문구 %q 없음", want)
		}
	}
	if !strings.Contains(body, `data-unlinked-checkall`) || !strings.Contains(body, `data-unlinked-open`) {
		t.Error("일괄 연결 영역(전체 선택·선택 연결)이 없다")
	}
	// 정기점검: 제품 일치·불일치·빈 제품
	v.visit(t, "M-KLAS", "MP202", "C204S", "2026-11-05", "KLAS", 1, 0)  // 자료관리↔KLAS 일치
	v.visit(t, "M-BAD1", "MP202", "C204S", "2026-11-04", "앤로보틱스", 2, 0) // 자료관리↔앤로보틱스 불일치
	v.visit(t, "M-RFID", "MP202", "C204T", "2026-11-03", "앤로보틱스", 1, 0) // RFID↔앤로보틱스 일치
	v.visit(t, "M-BAD2", "MP202", "C204T", "2026-11-02", "KLAS", 2, 0)  // RFID↔KLAS 불일치
	v.visit(t, "M-EMPTY", "MP202", "C204T", "2026-11-01", "", 3, 0)     // 제품 없음은 막지 않는다
	v.visit(t, "M-MANY", "MP202", "C151", "2026-10-30", "KLAS", 1, 0)
	v.visit(t, "M-DONE", "MP202", "C204S", "2026-10-29", "KLAS", 4, 1)
	mb := v.hx(t, "/as/unlinked/mnt", nil).Body.String()
	for id, want := range map[string]bool{"M-KLAS": true, "M-RFID": true, "M-EMPTY": true, "M-DONE": true, "M-BAD1": false, "M-BAD2": false, "M-MANY": false} {
		if got := hasCheck(mb, id); got != want {
			t.Errorf("정기점검 %s 체크박스 %v (기대 %v)", id, got, want)
		}
	}
	if !strings.Contains(mb, "점검 제품과 자산 제품군이 다릅니다") {
		t.Error("제품 불일치 안내가 없다")
	}
}

func TestHTTP95_204_BulkLinkAS(t *testing.T) {
	v := setup202(t)
	seed204Link(t, v)
	v.as(t, "B1", "C204S", "in_progress", "2026-11-05")
	v.as(t, "B2", "C204T", "in_progress", "2026-11-04")
	v.as(t, "B-MANY", "C151", "in_progress", "2026-11-03")
	if _, err := v.db.Exec(`UPDATE as_receipts SET project_id='KEEP' WHERE as_id='B1'`); err != nil {
		t.Fatal(err)
	}
	// 한 건은 건너뛰고(자산 2개 이상), 없는 id 도 섞어도 나머지는 성공한다
	rec := v.post204(t, "?tab=as", linkForm("as", "B1", "B-MANY", "NOPE", "B2"), nil, true)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "연결 2건 완료") || !strings.Contains(body, "건너뜀 <b>2</b>건") {
		t.Fatalf("결과 요약 %d: %s", rec.Code, regexp.MustCompile(`data-unlinked-result[^<]*<p>.*`).FindString(body))
	}
	if v.assetOf(t, "B1") != "A204S" || v.assetOf(t, "B2") != "A204T" || v.assetOf(t, "B-MANY") != "" {
		t.Fatalf("asset_id: %s %s %s", v.assetOf(t, "B1"), v.assetOf(t, "B2"), v.assetOf(t, "B-MANY"))
	}
	var pid string
	_ = v.db.QueryRow(`SELECT COALESCE(project_id,'') FROM as_receipts WHERE as_id='B1'`).Scan(&pid)
	if pid != "KEEP" {
		t.Errorf("project_id 를 다시 계산하지 않아야 한다: %q", pid)
	}
	for _, want := range []string{"고객 자산이 2개 이상입니다", "이미 연결됐거나 대상이 아닙니다"} {
		if !strings.Contains(body, want) {
			t.Errorf("건너뜀 사유 %q 없음", want)
		}
	}
	// 연결된 건은 목록에서 사라지고, 건수 칩이 OOB 로 갱신된다
	if strings.Contains(body, `data-unlinked-row="B1"`) || !strings.Contains(body, `data-unlinked-row="B-MANY"`) {
		t.Error("연결 후 목록 갱신이 틀렸다")
	}
	if !regexp.MustCompile(`hx-swap-oob="outerHTML"[^>]*data-unlinked-count="as">1<`).MatchString(body) {
		t.Errorf("AS 건수 칩 갱신(OOB)이 없다: %s", regexp.MustCompile(`<b id="ul-count-as"[^>]*>[^<]*`).FindString(body))
	}
	// 변경 이력(단건 저장과 같은 방식)과 감사 요약
	var n int
	_ = v.db.QueryRow(`SELECT COUNT(*) FROM data_change_logs WHERE table_name='as_receipts' AND entity_id IN ('B1','B2')`).Scan(&n)
	if n != 2 {
		t.Errorf("건별 변경 이력 %d건(2건이어야 한다)", n)
	}
	_ = v.db.QueryRow(`SELECT COUNT(*) FROM data_change_logs WHERE table_name='as_unlinked_bulk' AND entity_label LIKE '%AS 2건 연결%'`).Scan(&n)
	if n != 1 {
		t.Errorf("일괄 연결 요약 감사 기록 %d건", n)
	}
	// 멱등: 같은 요청을 다시 보내도 오류·중복 없이 건너뜀으로 끝난다
	rec = v.post204(t, "?tab=as", linkForm("as", "B1", "B2"), nil, true)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "연결 0건 완료") || v.assetOf(t, "B1") != "A204S" {
		t.Fatalf("재전송 %d", rec.Code)
	}
	_ = v.db.QueryRow(`SELECT COUNT(*) FROM data_change_logs WHERE table_name='as_receipts' AND entity_id IN ('B1','B2')`).Scan(&n)
	if n != 2 {
		t.Errorf("재전송이 이력을 더했다 %d", n)
	}
	// 95-202 목록: 건수 판정이 연결 후 건수와 같다
	cb := v.hx(t, "/as/unlinked/counts", nil).Body.String()
	if countIn(cb, "as") != 1 {
		t.Errorf("연결 후 AS 건수 %d", countIn(cb, "as"))
	}
	// 연결된 자산은 조치 화면 제품군 표시의 근거(asset_id)로 읽힌다
	var grp string
	_ = v.db.QueryRow(`SELECT COALESCE(pg.type_name,'') FROM as_receipts ar JOIN assets a ON a.asset_id=ar.asset_id JOIN product_groups pg ON pg.group_id=a.product_group_id WHERE ar.as_id='B1'`).Scan(&grp)
	if grp != "자료관리" {
		t.Errorf("연결 자산 제품군 %q", grp)
	}
}

func TestHTTP95_204_BulkLinkRevalidatesAtRequestTime(t *testing.T) {
	v := setup202(t)
	seed204Link(t, v)
	v.as(t, "R1", "C204S", "in_progress", "2026-11-05")
	v.as(t, "R2", "C204T", "in_progress", "2026-11-04")
	if !hasCheck(v.hx(t, "/as/unlinked/as", nil).Body.String(), "R1") {
		t.Fatal("목록에는 대상으로 보여야 한다")
	}
	// 화면을 본 뒤 사이에 자산이 늘고, 다른 화면에서 R2 가 연결됐다
	if _, err := v.db.Exec(`INSERT INTO assets(asset_id,customer_id,product_name,product_type,product_category,model_name,product_group_id,org_id) VALUES ('A204S2','C204S','추가장비','HW','','','PG-MAT','O01')`); err != nil {
		t.Fatal(err)
	}
	if _, err := v.db.Exec(`UPDATE as_receipts SET asset_id='A204T' WHERE as_id='R2'`); err != nil {
		t.Fatal(err)
	}
	rec := v.post204(t, "?tab=as", linkForm("as", "R1", "R2"), nil, true)
	body := rec.Body.String()
	if !strings.Contains(body, "연결 0건 완료") || v.assetOf(t, "R1") != "" || v.assetOf(t, "R2") != "A204T" {
		t.Fatalf("재검증 실패: %s %s", v.assetOf(t, "R1"), v.assetOf(t, "R2"))
	}
	if !strings.Contains(body, "고객 자산이 2개 이상입니다") {
		t.Error("자산이 늘어난 건의 사유가 없다")
	}
	// 쓰는 문장 자체도 조건을 다시 본다(목록 조회 뒤 자산이 늘어난 경쟁 상황을 저장소 함수로 직접 확인)
	r := repository.NewASRepo(v.db)
	if ok, err := r.LinkSingleAS("O01", "R1", "A204S", "R1"); ok || err != nil {
		t.Fatalf("자산 2개 고객에 쓰기 시도가 통과했다 %v %v", ok, err)
	}
	if ok, err := r.LinkSingleAS("O01", "R1", "A204T", "R1"); ok || err != nil { // 다른 고객 자산
		t.Fatalf("다른 고객 자산이 통과했다 %v %v", ok, err)
	}
}

func TestHTTP95_204_BulkLinkGuards(t *testing.T) {
	v := setup202(t)
	seed204Link(t, v)
	v.as(t, "G1", "C204S", "in_progress", "2026-11-05")
	if _, err := v.db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id) VALUES ('C204X','엑스도서관','엑스도서관',1,'O02')`); err != nil {
		t.Fatal(err)
	}
	if _, err := v.db.Exec(`INSERT INTO assets(asset_id,customer_id,product_name,product_type,product_category,model_name,product_group_id,org_id) VALUES ('A204X','C204X','타조직장비','HW','','','PG-MAT','O02')`); err != nil {
		t.Fatal(err)
	}
	if _, err := v.db.Exec(`INSERT INTO as_receipts(as_id,as_number,customer_id,receipt_datetime,status,symptom,org_id) VALUES ('G-OTHER','G-OTHER','C204X','2026-11-06 09:00:00','in_progress','타조직','O02')`); err != nil {
		t.Fatal(err)
	}
	// CSRF 없음 403 / 종류 오류 400 / 빈 선택 400 / 100건 초과 400
	if c := v.post204(t, "", linkForm("as", "G1"), nil, false).Code; c != http.StatusForbidden {
		t.Errorf("CSRF 없음 %d", c)
	}
	if c := v.post204(t, "", linkForm("zz", "G1"), nil, true).Code; c != http.StatusBadRequest {
		t.Errorf("종류 오류 %d", c)
	}
	if c := v.post204(t, "", linkForm("as"), nil, true).Code; c != http.StatusBadRequest {
		t.Errorf("빈 선택 %d", c)
	}
	var many []string
	for i := 0; i < 101; i++ {
		many = append(many, fmt.Sprintf("X%03d", i))
	}
	if c := v.post204(t, "", linkForm("as", many...), nil, true).Code; c != http.StatusBadRequest {
		t.Errorf("101건 %d", c)
	}
	if v.assetOf(t, "G1") != "" {
		t.Fatal("거부된 요청이 연결했다")
	}
	// 타 조직 id 위조는 건너뜀(연결되지 않는다)
	rec := v.post204(t, "", linkForm("as", "G-OTHER", "G1"), nil, true)
	if !strings.Contains(rec.Body.String(), "연결 1건 완료") || v.assetOf(t, "G-OTHER") != "" || v.assetOf(t, "G1") != "A204S" {
		t.Fatalf("타 조직 위조 처리 G-OTHER=%q G1=%q", v.assetOf(t, "G-OTHER"), v.assetOf(t, "G1"))
	}
	// 읽기 전용·C-Level·비담당 기술자는 연결되지 않는다
	if _, err := v.db.Exec(`UPDATE as_receipts SET asset_id=NULL WHERE as_id='G1'`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(v.db)
	mk := func(name, role string, ro bool) *http.Cookie {
		u := &model.User{Username: name, PasswordHash: HashPassword("x"), FullName: name + "님", Role: role, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true, IsReadOnly: ro}
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
		got, _ := users.GetByUsername(name)
		return jwtCookieVerified(t, got)
	}
	ro := mk("ro204", model.RoleTech, true)
	cl := mk("cl204", model.RoleCLevel, false)
	tech := mk("tech204", model.RoleTech, false)
	if c := v.post204(t, "", linkForm("as", "G1"), ro, true).Code; c != http.StatusForbidden {
		t.Errorf("읽기 전용 %d", c)
	}
	if c := v.post204(t, "", linkForm("as", "G1"), cl, true).Code; c != http.StatusForbidden {
		t.Errorf("C-Level %d", c)
	}
	rec = v.post204(t, "", linkForm("as", "G1"), tech, true)
	if v.assetOf(t, "G1") != "" || !strings.Contains(rec.Body.String(), "연결 권한이 없습니다") {
		t.Fatalf("배정되지 않은 기술자가 연결했다 %q %d", v.assetOf(t, "G1"), rec.Code)
	}
	// 같은 기술자는 목록에서도 체크박스가 없다(서버 판정과 같은 함수)
	if hasCheck(v.hx(t, "/as/unlinked/as?mine=0", tech).Body.String(), "G1") {
		t.Error("권한 없는 사용자 화면에 체크박스가 있다")
	}
}

func TestHTTP95_204_BulkLinkMnt(t *testing.T) {
	v := setup202(t)
	seed204Link(t, v)
	v.visit(t, "N-KLAS", "MP202", "C204S", "2026-11-05", "KLAS", 1, 0)
	v.visit(t, "N-BAD", "MP202", "C204S", "2026-11-04", "앤로보틱스", 2, 0)
	v.visit(t, "N-RFID", "MP202", "C204T", "2026-11-03", "앤로보틱스", 1, 0)
	v.visit(t, "N-MANY", "MP202", "C151", "2026-11-02", "KLAS", 1, 0)
	// N-RFID 는 방문 업무가 이미 있고, 직접 입력한 점검 장비 행이 하나 있다
	if _, err := v.db.Exec(`INSERT INTO work_tasks(task_id,work_type,title,source_type,source_id) VALUES ('T-N-RFID','maintenance','티도서관 점검','maintenance','N-RFID')`); err != nil {
		t.Fatal(err)
	}
	if _, err := v.db.Exec(`INSERT INTO maintenance_visit_targets(target_id,visit_id,asset_id,name,memo,sort_order,org_id) VALUES ('mvt_manual','N-RFID','','현장 직접입력 장비','',0,'O01')`); err != nil {
		t.Fatal(err)
	}
	var tasksBefore int
	_ = v.db.QueryRow(`SELECT COUNT(*) FROM work_tasks`).Scan(&tasksBefore)
	rec := v.post204(t, "?tab=mnt", linkForm("mnt", "N-KLAS", "N-BAD", "N-RFID", "N-MANY"), nil, true)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "연결 2건 완료") || !strings.Contains(body, "건너뜀 <b>2</b>건") {
		t.Fatalf("결과 %d", rec.Code)
	}
	for _, want := range []string{"점검 제품과 자산 제품군이 다릅니다", "고객 자산이 2개 이상입니다"} {
		if !strings.Contains(body, want) {
			t.Errorf("사유 %q 없음", want)
		}
	}
	tgt := func(visit string) (n int, assets string) {
		rows, _ := v.db.Query(`SELECT COALESCE(asset_id,'') FROM maintenance_visit_targets WHERE visit_id=? ORDER BY sort_order, target_id`, visit)
		defer rows.Close()
		var as []string
		for rows.Next() {
			var a string
			_ = rows.Scan(&a)
			as = append(as, a)
			n++
		}
		return n, strings.Join(as, ",")
	}
	if n, a := tgt("N-KLAS"); n != 1 || a != "A204S" {
		t.Errorf("N-KLAS 방문 대상 %d %q", n, a)
	}
	if n, a := tgt("N-RFID"); n != 2 || a != ",A204T" { // 직접입력 행은 그대로 두고 자산 행을 더한다
		t.Errorf("N-RFID 방문 대상 %d %q (직접입력 행 보존)", n, a)
	}
	if n, _ := tgt("N-BAD"); n != 0 {
		t.Error("제품 불일치 건이 연결됐다")
	}
	var wta int
	_ = v.db.QueryRow(`SELECT COUNT(*) FROM work_task_assets WHERE task_id='T-N-RFID' AND asset_id='A204T'`).Scan(&wta)
	if wta != 1 {
		t.Errorf("방문 업무가 있으면 work_task_assets 를 채운다 %d", wta)
	}
	var tasksAfter int
	_ = v.db.QueryRow(`SELECT COUNT(*) FROM work_tasks`).Scan(&tasksAfter)
	if tasksAfter != tasksBefore {
		t.Errorf("업무를 새로 만들었다 %d→%d", tasksBefore, tasksAfter)
	}
	// 연결된 건은 정기점검 목록에서 사라지고, 방문 대상 판정(95-202)과 맞는다
	if strings.Contains(body, `data-unlinked-row="N-KLAS"`) || !strings.Contains(body, `data-unlinked-row="N-BAD"`) {
		t.Error("연결 후 목록 갱신이 틀렸다")
	}
	cb := v.hx(t, "/as/unlinked/counts", nil).Body.String()
	if countIn(cb, "mnt") != 2 {
		t.Errorf("연결 후 점검 건수 %d", countIn(cb, "mnt"))
	}
	// 멱등: 재전송해도 방문 대상 행이 늘지 않는다
	rec = v.post204(t, "?tab=mnt", linkForm("mnt", "N-KLAS", "N-RFID"), nil, true)
	if !strings.Contains(rec.Body.String(), "연결 0건 완료") {
		t.Fatal("재전송 결과")
	}
	if n, _ := tgt("N-KLAS"); n != 1 {
		t.Errorf("재전송이 방문 대상을 늘렸다 %d", n)
	}
	// 읽기 전용 계정은 403
	u := &model.User{Username: "ro204m", PasswordHash: HashPassword("x"), FullName: "읽기", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true, IsReadOnly: true}
	if err := repository.NewUserRepo(v.db).Create(u); err != nil {
		t.Fatal(err)
	}
	got, _ := repository.NewUserRepo(v.db).GetByUsername("ro204m")
	if c := v.post204(t, "", linkForm("mnt", "N-BAD"), jwtCookieVerified(t, got), true).Code; c != http.StatusForbidden {
		t.Errorf("읽기 전용 %d", c)
	}
}

func TestHTTP95_204_BulkLinkNoN1(t *testing.T) {
	v := setup202(t)
	seed204Link(t, v)
	var ids []string
	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("Z%03d", i)
		v.as(t, id, "C204S", "in_progress", "2026-10-01")
		ids = append(ids, id)
	}
	before := repository.UnlinkedCandidateQueries()
	rec := v.post204(t, "?tab=as", linkForm("as", ids...), nil, true)
	// 대상 확인 1회 + 갱신된 목록 1회(고객 묶음 집계), 건수에 비례하지 않는다
	if n := repository.UnlinkedCandidateQueries() - before; n > 2 {
		t.Errorf("후보 자산 집계가 %d회(건마다 조회하면 안 된다)", n)
	}
	if !strings.Contains(rec.Body.String(), "연결 100건 완료") {
		t.Fatal("100건 연결 결과")
	}
	var n int
	_ = v.db.QueryRow(`SELECT COUNT(*) FROM as_receipts WHERE as_id LIKE 'Z%' AND asset_id='A204S'`).Scan(&n)
	if n != 100 {
		t.Errorf("연결된 건 %d", n)
	}
}

// T7: 실제 브라우저(Playwright) 마우스 클릭. 화면 너비마다 새 DB 로 실행한다(연결하면 데이터가 바뀐다).
func TestBrowser95_204_BulkLinkAndSort(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	for _, w := range []int{390, 820, 1440} {
		w := w
		t.Run(fmt.Sprint(w), func(t *testing.T) {
			v := setup202(t)
			seed204Link(t, v)
			for i := 1; i <= 6; i++ { // 연결 가능 AS 6건(자산 1개 고객)
				v.as(t, fmt.Sprintf("E%d", i), "C204S", "in_progress", fmt.Sprintf("2026-11-%02d", 10+i))
			}
			v.as(t, "M1", "C151", "in_progress", "2026-11-04")  // 자산 3개
			v.as(t, "M2", "C151", "in_progress", "2026-11-02")  // 자산 3개
			v.as(t, "Z1", "C151b", "in_progress", "2026-11-03") // 자산 0개
			v.visit(t, "K1", "MP202", "C204S", "2026-11-05", "KLAS", 1, 0)
			v.visit(t, "K2", "MP202", "C204T", "2026-11-04", "앤로보틱스", 1, 0)
			v.visit(t, "K3", "MP202", "C204T", "2026-11-03", "", 2, 0)
			v.visit(t, "K4", "MP202", "C204S", "2026-11-02", "앤로보틱스", 2, 0) // 제품 불일치
			g := v.e.Group("")
			g.Use(v.h.Auth.AuthMiddleware)
			g.POST("/as/:id/action-sets", v.h.AS.SaveActionSet)
			staticDir, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
			v.e.Static("/static", staticDir)
			srv := httptest.NewServer(v.e)
			t.Cleanup(srv.Close)
			shot, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-204"))
			cmd := exec.Command("node", "95_204_bulk_browser.mjs")
			cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
			cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+srv.URL, fmt.Sprintf("WIDTH=%d", w),
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
		})
	}
}
