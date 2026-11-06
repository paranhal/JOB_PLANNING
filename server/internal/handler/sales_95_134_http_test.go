package handler

import (
	"database/sql"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func ensure95_134Users(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs (org_id, org_no, org_name, is_active, sort_order) VALUES ('O03','03','타조직',1,3)`); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(`INSERT OR IGNORE INTO users (user_id, username, password_hash, full_name, role, is_active, org_id)
		VALUES ('admin-id','jp-admin-id','x','관리자','org_admin',1,'O01'),
		       ('o03-sales','o03-sales','x','타조직영업','sales',1,'O03'),
		       ('vision_admin-id','jp-vision','x','비젼관리자','vision_admin',1,'O01')`)
	if err != nil {
		t.Fatal(err)
	}
}

var salesFormFields95_134 = []string{
	`name="name"`, `name="is_tentative_name"`, `name="tentative_name"`, `name="customer_id"`,
	`name="sales_owner_id"`, `name="prospect_name"`, `name="prospect_region"`, `name="customer_confirmed"`,
	`name="expected_amount"`, `name="expected_amount_confirmed"`, `name="amount_vat_included"`,
	`name="lead_source"`, `name="contract_target"`, `name="procurement_route"`, `name="contract_method"`,
	`name="bid_eval_method"`, `name="mall_contract_type"`, `name="budget_status"`, `name="biz_type"`,
	`name="expected_precision"`, `name="expected_ym"`, `name="expected_year"`, `name="expected_quarter"`,
	`name="expected_half"`, `name="expected_ym_confirmed"`, `name="billing_cycle"`,
	`name="revenue_from"`, `name="revenue_to"`, `name="bid_ym"`, `name="revenue_ym"`,
	`name="competitor"`, `name="notes"`, `name="initial_files"`,
}

func Test95_134NameOnlyCreateOwnerAndForm(t *testing.T) {
	e, db := newSalesServerDB(t)
	ensure95_134Users(t, db)

	page := doGet(t, e, "/sales/new")
	if page.Code != 200 {
		t.Fatalf("new %d", page.Code)
	}
	body := page.Body.String()
	if strings.Contains(body, `:required="isTentative"`) {
		t.Fatal("등록 화면에 가칭 required 가 남아 있다")
	}
	if !strings.Contains(body, `name="name"`) || !strings.Contains(body, "required") {
		t.Fatal("사업명 required 없음")
	}
	if !strings.Contains(body, "자세한 정보") || !strings.Contains(body, "제안·협상 때 권장") ||
		!strings.Contains(body, "계약 전환 때 필요") || !strings.Contains(body, "진행 중 입력") ||
		!strings.Contains(body, "있을 때 첨부") {
		t.Fatal("필요 시점 라벨 없음")
	}
	if strings.Contains(body, "사업 기회 정보") {
		t.Fatal("등록 화면에 수정용 카드 01 제목이 있다")
	}
	for _, f := range salesFormFields95_134 {
		if !strings.Contains(body, f) {
			t.Fatalf("등록 필드 없음 %s", f)
		}
	}

	blank := doForm(t, e, "/sales", url.Values{"name": {""}})
	if blank.Code != 200 || !strings.Contains(blank.Body.String(), "사업명을 입력하세요") {
		t.Fatalf("빈 이름: %d %s", blank.Code, blank.Body.String())
	}

	rec := doForm(t, e, "/sales", url.Values{"name": {"정식만사업"}})
	if rec.Code != 303 {
		t.Fatalf("이름만 저장 %d %s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "ok=created") {
		t.Fatalf("loc=%s", loc)
	}
	id := salesIDFromRedirect(t, loc)
	var name, stage, org, owner, ownerID, tent string
	var flag int
	if err := db.QueryRow(`SELECT name, stage, COALESCE(org_id,''), COALESCE(sales_owner,''), COALESCE(sales_owner_id,''), COALESCE(is_tentative_name,0), COALESCE(tentative_name,'')
		FROM sales_projects WHERE sales_id=?`, id).Scan(&name, &stage, &org, &owner, &ownerID, &flag, &tent); err != nil {
		t.Fatal(err)
	}
	if name != "정식만사업" || stage != model.SalesStage4Discover || org != model.OrgIDLibrary {
		t.Fatalf("저장값 name=%s stage=%s org=%s", name, stage, org)
	}
	if ownerID != "admin-id" || owner == "" {
		t.Fatalf("담당 자동 owner=%s id=%s", owner, ownerID)
	}
	if flag != 0 {
		t.Fatal("가칭이 켜졌다")
	}

	show := doGet(t, e, loc)
	if show.Code != 200 {
		t.Fatalf("상세 %d", show.Code)
	}
	sb := show.Body.String()
	if !strings.Contains(sb, "사업을 등록했습니다") || !strings.Contains(sb, ">닫기<") {
		t.Fatal("등록 팝업·닫기 없음")
	}
	if !strings.Contains(sb, "보완할 정보") || !strings.Contains(sb, "자세한 정보 입력") {
		t.Fatal("상세 안내 박스 없음")
	}

	tentRec := doForm(t, e, "/sales", url.Values{"name": {"가칭사업"}, "is_tentative_name": {"1"}})
	if tentRec.Code != 303 {
		t.Fatalf("가칭 체크 %d", tentRec.Code)
	}
	tid := salesIDFromRedirect(t, tentRec.Header().Get("Location"))
	if err := db.QueryRow(`SELECT name, COALESCE(is_tentative_name,0), COALESCE(tentative_name,'') FROM sales_projects WHERE sales_id=?`, tid).
		Scan(&name, &flag, &tent); err != nil {
		t.Fatal(err)
	}
	if flag != 1 || tent != "가칭사업" || name != "가칭사업" {
		t.Fatalf("가칭 복사 name=%s tent=%s flag=%d", name, tent, flag)
	}

	picked := doForm(t, e, "/sales", url.Values{"name": {"지정담당"}, "sales_owner_id": {"admin-id"}})
	if picked.Code != 303 {
		t.Fatalf("선택 유지 %d", picked.Code)
	}
	pid := salesIDFromRedirect(t, picked.Header().Get("Location"))
	if err := db.QueryRow(`SELECT COALESCE(sales_owner_id,'') FROM sales_projects WHERE sales_id=?`, pid).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	if ownerID != "admin-id" {
		t.Fatalf("선택 담당 %s", ownerID)
	}

	bad := doForm(t, e, "/sales", url.Values{"name": {"타조직담당"}, "sales_owner_id": {"o03-sales"}})
	if bad.Code != 200 || !strings.Contains(bad.Body.String(), "소속 부서가 다른 담당자는 넣을 수 없습니다") {
		t.Fatalf("타조직 담당: %d %s", bad.Code, bad.Body.String())
	}

	skip := httptestPostAs(t, e, "/sales", url.Values{"name": {"조직불일치"}, "org_id": {"O03"}}, "vision_admin")
	if skip.Code != 303 {
		t.Fatalf("자동지정 생략 %d %s", skip.Code, skip.Body.String())
	}
	if !strings.Contains(skip.Header().Get("Location"), "owner_skip=1") {
		t.Fatalf("owner_skip loc=%s", skip.Header().Get("Location"))
	}
	sid := salesIDFromRedirect(t, skip.Header().Get("Location"))
	if err := db.QueryRow(`SELECT COALESCE(sales_owner_id,''), COALESCE(org_id,'') FROM sales_projects WHERE sales_id=?`, sid).Scan(&ownerID, &org); err != nil {
		t.Fatal(err)
	}
	if ownerID != "" || org != "O03" {
		t.Fatalf("스킵 후 owner=%s org=%s", ownerID, org)
	}
	skipShow := doGetWith(t, e, skip.Header().Get("Location"), jwtCookieRole(t, "vision_admin"))
	if !strings.Contains(skipShow.Body.String(), "영업담당이 지정되지 않았습니다(조직이 달라 자동 지정되지 않음)") {
		t.Fatal("스킵 안내 없음")
	}

	edit := doGet(t, e, "/sales/"+id+"/edit")
	if edit.Code != 200 {
		t.Fatalf("수정 %d", edit.Code)
	}
	eb := edit.Body.String()
	if !strings.Contains(eb, "사업 기회 정보") || !strings.Contains(eb, `:required="isTentative"`) {
		t.Fatal("수정 화면 카드 01·가칭 required 없음")
	}
	for _, f := range salesFormFields95_134 {
		if !strings.Contains(eb, f) {
			t.Fatalf("수정 필드 없음 %s", f)
		}
	}
}

func Test95_134ListMissingFilter(t *testing.T) {
	e, db := newSalesServerDB(t)
	ensure95_134Users(t, db)
	if doForm(t, e, "/sales", url.Values{"name": {"보완대상"}}).Code != 303 {
		t.Fatal("보완대상")
	}
	full := doForm(t, e, "/sales", url.Values{
		"name": {"채워진사업"}, "expected_amount": {"1000"}, "expected_ym": {"2026-11"},
		"expected_precision": {"month"}, "biz_type": {model.SalesBizBuild},
	})
	if full.Code != 303 {
		t.Fatalf("채움 %d", full.Code)
	}
	fid := salesIDFromRedirect(t, full.Header().Get("Location"))
	cust := "C-FILL"
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?, '채워진고객', '채워진고객', 1)`, cust); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sales_projects SET customer_id=?, expected_amount=1000, expected_ym='2026-11', biz_type=? WHERE sales_id=?`,
		cust, model.SalesBizBuild, fid); err != nil {
		t.Fatal(err)
	}

	list := doGet(t, e, "/sales?display=list")
	lb := list.Body.String()
	if !strings.Contains(lb, "보완 필요") || !strings.Contains(lb, "name=\"missing\"") {
		t.Fatal("목록 필터 없음")
	}
	pipe := doGet(t, e, "/sales?view=pipeline")
	if strings.Contains(pipe.Body.String(), `name="missing"`) {
		t.Fatal("파이프라인에 보완 필요 필터가 있다")
	}

	miss := doGet(t, e, "/sales?display=list&missing=1")
	mb := miss.Body.String()
	if !strings.Contains(mb, "보완대상") {
		t.Fatal("보완 필요 목록에 빈 건이 없다")
	}
	if strings.Contains(mb, "채워진사업") {
		t.Fatal("채워진 건이 보완 필요에 남았다")
	}
}

func Test95_134ClosedHidesBoxKeepsPopup(t *testing.T) {
	e, db := newSalesServerDB(t)
	ensure95_134Users(t, db)
	rec := doForm(t, e, "/sales", url.Values{"name": {"종료안내"}})
	id := salesIDFromRedirect(t, rec.Header().Get("Location"))
	if _, err := db.Exec(`UPDATE sales_projects SET stage=?, close_reason=?, status=? WHERE sales_id=?`,
		model.SalesStage4Closed, model.SalesCloseContracted, model.SalesStatusContracted, id); err != nil {
		t.Fatal(err)
	}
	show := doGet(t, e, "/sales/"+id+"?ok=created")
	b := show.Body.String()
	if !strings.Contains(b, "사업을 등록했습니다") {
		t.Fatal("종료여도 팝업")
	}
	if strings.Contains(b, "보완할 정보") {
		t.Fatal("종료 상세 박스가 보인다")
	}
}
