package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"customer-support/internal/repository"
)

// 95-204 ① 정렬: sort 화이트리스트, 기본은 현행 유지, 페이지·필터 유지, 쿼리 수는 건수에 비례하지 않는다.

func seed204Sort(t *testing.T, v env202) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO customers(customer_id,org_name,official_name,is_active) VALUES ('C204A','가나도서관','가나도서관',1),('C204B','다라도서관','다라도서관',1),('C204C','나다도서관','나다도서관',1)`,
		`INSERT INTO assets(asset_id,customer_id,product_name,product_type,product_category,model_name,product_group_id,org_id) VALUES
			('A204-A1','C204A','자료검색PC','HW','materials','','PG-MAT','O01'),
			('A204-B1','C204B','무인반납기','HW','rfid','','PG-RFID','O01'),('A204-B2','C204B','자가대출기','HW','rfid','','PG-RFID','O01')`,
	} {
		if _, err := v.db.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	mk := func(id, cust, day, who string) {
		v.as(t, id, cust, "in_progress", day)
		_, _ = v.db.Exec(`UPDATE as_receipts SET assigned_to=? WHERE as_id=?`, who, id)
	}
	mk("S1", "C151", "2026-11-05", "다담당")  // 자산 3
	mk("S2", "C204A", "2026-11-03", "가담당") // 자산 1
	mk("S3", "C204B", "2026-11-04", "")    // 자산 2, 담당자 없음
	mk("S4", "C204C", "2026-11-02", "나담당") // 자산 0
	v.visit(t, "V1", "MP202", "C151", "2026-11-05", "KLAS", 3, 0)
	v.visit(t, "V2", "MP202", "C204A", "2026-11-03", "앤로보틱스", 1, 0)
	v.visit(t, "V3", "MP202", "C204B", "2026-11-04", "KLAS", 2, 0)
	v.visit(t, "V4", "MP202", "C204C", "2026-11-02", "", 4, 0)
}

func orderOf(body string, ids ...string) []string {
	type pos struct {
		id string
		i  int
	}
	var ps []pos
	for _, id := range ids {
		if i := strings.Index(body, `data-unlinked-row="`+id+`"`); i >= 0 {
			ps = append(ps, pos{id, i})
		}
	}
	for a := 0; a < len(ps); a++ {
		for b := a + 1; b < len(ps); b++ {
			if ps[b].i < ps[a].i {
				ps[a], ps[b] = ps[b], ps[a]
			}
		}
	}
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.id
	}
	return out
}

func TestHTTP95_204_SortAllKeys(t *testing.T) {
	v := setup202(t)
	seed204Sort(t, v)
	asWant := map[string]string{
		"":          "S1,S3,S2,S4", // 기본 = 현행(접수일 최신순)
		"recv_desc": "S1,S3,S2,S4",
		"recv_asc":  "S4,S2,S3,S1",
		"customer":  "S2,S4,S3,S1", // 가나·나다·다라·성환
		"assignee":  "S2,S4,S1,S3", // 담당자 없음은 맨 아래
		"cand":      "S1,S3,S2,S4", // 후보 자산 수 많은 순(3·2·1·0)
	}
	mntWant := map[string]string{
		"":           "V1,V3,V2,V4",
		"visit_desc": "V1,V3,V2,V4",
		"visit_asc":  "V4,V2,V3,V1",
		"customer":   "V2,V4,V3,V1",
		"product":    "V1,V3,V2,V4", // KLAS < 앤로보틱스, 빈값은 맨 아래
		"round":      "V2,V3,V1,V4",
		"cand":       "V2,V3,V4,V1", // 1개 먼저, 이어서 1에 가까운 수(일괄 연결 대상 우선)
	}
	for key, want := range asWant {
		before := repository.UnlinkedCandidateQueries()
		body := v.hx(t, "/as/unlinked/as?sort="+key, nil).Body.String()
		if got := strings.Join(orderOf(body, "S1", "S2", "S3", "S4"), ","); got != want {
			t.Fatalf("AS sort=%q 순서 %s (기대 %s)", key, got, want)
		}
		if n := repository.UnlinkedCandidateQueries() - before; n != 1 {
			t.Fatalf("AS sort=%q 후보 자산 집계 %d회(1회여야 한다)", key, n)
		}
	}
	for key, want := range mntWant {
		before := repository.UnlinkedCandidateQueries()
		body := v.hx(t, "/as/unlinked/mnt?sort="+key, nil).Body.String()
		if got := strings.Join(orderOf(body, "V1", "V2", "V3", "V4"), ","); got != want {
			t.Fatalf("정기점검 sort=%q 순서 %s (기대 %s)", key, got, want)
		}
		if n := repository.UnlinkedCandidateQueries() - before; n != 1 {
			t.Fatalf("정기점검 sort=%q 후보 자산 집계 %d회", key, n)
		}
	}
}

func TestHTTP95_204_SortInvalidFallsBackToDefault(t *testing.T) {
	v := setup202(t)
	seed204Sort(t, v)
	// 화이트리스트 밖 값·인젝션 시도는 기본 정렬로 돌아가고 오류가 나지 않는다(정기점검 키를 AS 에 보내도 기본)
	for _, bad := range []string{"zzz", "recv_desc;DROP TABLE as_receipts", "recv_desc,(SELECT 1)", "c.org_name", "visit_asc", "%27%20OR%201=1--", "../x", ""} {
		body := v.hx(t, "/as/unlinked/as?sort="+url.QueryEscape(bad), nil)
		if body.Code != http.StatusOK || strings.Contains(body.Body.String(), "data-unlinked-error") {
			t.Fatalf("AS 잘못된 sort %q → %d", bad, body.Code)
		}
		if got := strings.Join(orderOf(body.Body.String(), "S1", "S2", "S3", "S4"), ","); got != "S1,S3,S2,S4" {
			t.Fatalf("AS 잘못된 sort %q 가 기본 정렬이 아니다: %s", bad, got)
		}
	}
	for _, bad := range []string{"recv_asc", "assignee", "x", "visit_desc;--"} {
		body := v.hx(t, "/as/unlinked/mnt?sort="+url.QueryEscape(bad), nil)
		if body.Code != http.StatusOK {
			t.Fatalf("정기점검 잘못된 sort %q → %d", bad, body.Code)
		}
		if got := strings.Join(orderOf(body.Body.String(), "V1", "V2", "V3", "V4"), ","); got != "V1,V3,V2,V4" {
			t.Fatalf("정기점검 잘못된 sort %q 가 기본 정렬이 아니다: %s", bad, got)
		}
	}
	var n int
	if err := v.db.QueryRow(`SELECT COUNT(*) FROM as_receipts`).Scan(&n); err != nil || n != 4 {
		t.Fatalf("테이블/데이터가 훼손됐다 %d %v", n, err)
	}
}

func TestHTTP95_204_SortPreservedAcrossPagesFiltersAndHeaders(t *testing.T) {
	v := setup202(t)
	seed204Sort(t, v)
	for i := 1; i <= 25; i++ { // 2쪽이 생기도록 같은 고객의 건을 더한다
		v.as(t, fmt.Sprintf("P%02d", i), "C151", "completed", fmt.Sprintf("2026-10-%02d", i))
	}
	body := v.hx(t, "/as/unlinked/as?sort=customer&status=done&who=도서관장", nil).Body.String()
	// 현재 정렬: 선택 상자·머리말 방향 표시·aria-sort
	if !regexp.MustCompile(`<option value="customer" selected>`).MatchString(body) {
		t.Fatal("정렬 선택 상자가 현재 정렬을 가리키지 않는다")
	}
	if !strings.Contains(body, `aria-sort="ascending"`) || !strings.Contains(body, `data-unlinked-sort="customer"`) {
		t.Fatal("머리말 정렬 방향 표시(aria-sort)가 없다")
	}
	// 페이지 링크는 sort·필터를 이어 가고 주소(ul_*)에도 남긴다
	for _, want := range []string{"sort=customer", "status=done", "who=", `ul_sort=customer`} {
		if !strings.Contains(body, want) {
			t.Fatalf("페이지 링크에 %q 가 없다", want)
		}
	}
	// 머리말 링크는 1쪽으로 가되 필터는 유지하고, 같은 키를 다시 누르면 방향이 바뀐다(접수일)
	def := v.hx(t, "/as/unlinked/as?status=done", nil).Body.String()
	m := regexp.MustCompile(`hx-get="([^"]*)"[^>]*data-unlinked-sort="recv"`).FindStringSubmatch(def)
	if m == nil || !strings.Contains(m[1], "sort=recv_asc") || !strings.Contains(m[1], "status=done") || strings.Contains(m[1], "page=") {
		t.Fatalf("접수일 머리말 링크가 다음 정렬(오래된순)·필터 유지·1쪽이 아니다: %v", m)
	}
	if !strings.Contains(def, "▼") {
		t.Fatal("기본 정렬(접수일 최신순) 방향 표시가 없다")
	}
	asc := v.hx(t, "/as/unlinked/as?status=done&sort=recv_asc", nil).Body.String()
	m = regexp.MustCompile(`hx-get="([^"]*)"[^>]*data-unlinked-sort="recv"`).FindStringSubmatch(asc)
	if m == nil || strings.Contains(m[1], "sort=") { // 기본으로 돌아가는 링크는 sort 가 주소에 없다
		t.Fatalf("오래된순에서 다시 누르면 기본(최신순)으로 가야 한다: %v", m)
	}
	// 카운트 응답이 주소의 ul_sort 를 본문 요청에 옮긴다(새로고침 유지)
	cb := v.hx(t, "/as/unlinked/counts?ul_tab=mnt&ul_sort=round", nil).Body.String()
	if !strings.Contains(cb, "sort=round") {
		t.Fatalf("새로고침 때 정렬이 본문 요청에 옮겨지지 않았다: %s", regexp.MustCompile(`id="as-unlinked-body"[^>]*`).FindString(cb))
	}
	// 정렬 결과가 페이지 경계를 넘어도 한결같다(고객명 정렬 1쪽 20건 + 2쪽 나머지, 전체 합 유지)
	p1 := v.hx(t, "/as/unlinked/as?sort=cand", nil).Body.String()
	p2 := v.hx(t, "/as/unlinked/as?sort=cand&page=2", nil).Body.String()
	if strings.Count(p1, "data-unlinked-row=")/2 != 20 || strings.Count(p2, "data-unlinked-row=")/2 != 9 {
		t.Fatalf("정렬 상태에서 페이지 분할 %d/%d", strings.Count(p1, "data-unlinked-row=")/2, strings.Count(p2, "data-unlinked-row=")/2)
	}
}
