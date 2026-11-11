package handler

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func Test95_169_SiteImportSearchRender(t *testing.T) {
	e, repo, users, db := newProjectWorkHarness(t)
	if _, err := db.Exec(`UPDATE customers SET addr_sigungu='천안시' WHERE customer_id='C-mid'`); err != nil {
		t.Fatal(err)
	}
	id := pwCreateTask(t, e, repo, "사이트검색169")
	link := doForm(t, e, "/workboard/tasks/"+id+"/action/project", url.Values{"project_id": {"WP-O01"}})
	if link.Code != 303 {
		t.Fatalf("연결 %d", link.Code)
	}
	page := doGet(t, e, "/workboard/tasks/"+id+"/action")
	if page.Code != 200 {
		t.Fatalf("GET %d", page.Code)
	}
	body := page.Body.String()
	for _, need := range []string{
		"사업 대상 사이트 불러오기",
		`data-site-import-q`,
		"사이트명·상위기관·지역 검색",
		`data-site-import-region`,
		`data-site-import-count`,
		"검색 결과 전체 선택",
		"검색 결과 해제",
		"선택 모두 해제",
		"선택한 사이트 추가",
		`action="/workboard/tasks/` + id + `/action/sites"`,
		`name="site_id"`,
		`@keydown.enter.prevent.stop`,
		`max-h-80 overflow-y-auto overflow-x-hidden`,
		"일치하는 사이트가 없습니다",
		"계룡고",
		"충남교육청",
		"천안시",
		`x-show="match(s)"`,
	} {
		if !strings.Contains(body, need) {
			t.Fatalf("없음 %q", need)
		}
	}
	if strings.Contains(body, `name="site_id" required`) || strings.Contains(body, `required name="site_id"`) {
		t.Fatal("숨은 체크박스 required")
	}
	card := body[strings.Index(body, "data-site-import"):strings.Index(body, "data-site-import-add")]
	if strings.Contains(card, "x-if") {
		t.Fatal("체크 목록을 x-if로 제거하면 숨은 선택이 제출되지 않는다")
	}

	add := doForm(t, e, "/workboard/tasks/"+id+"/action/sites", url.Values{"site_id": {"C-high"}})
	if add.Code != 303 {
		t.Fatalf("추가 %d %s", add.Code, add.Body.String())
	}
	sites, err := repo.ListTaskSites(id)
	if err != nil || len(sites) != 1 || sites[0].SiteID != "C-high" {
		t.Fatalf("반영 %+v %v", sites, err)
	}
	again := doGet(t, e, "/workboard/tasks/"+id+"/action").Body.String()
	if strings.Contains(again, "사업 대상 사이트 불러오기") {
		t.Fatal("사이트가 있으면 불러오기 카드가 남아 있다")
	}

	emptyID := pwCreateTask(t, e, repo, "사이트없음169")
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id) VALUES('WP-empty169','빈사업','active','O01')`); err != nil {
		t.Fatal(err)
	}
	if rec := doForm(t, e, "/workboard/tasks/"+emptyID+"/action/project", url.Values{"project_id": {"WP-empty169"}}); rec.Code != 303 {
		t.Fatalf("빈 사업 연결 %d", rec.Code)
	}
	emptyPage := doGet(t, e, "/workboard/tasks/"+emptyID+"/action").Body.String()
	if strings.Contains(emptyPage, "사업 대상 사이트 불러오기") {
		t.Fatal("사업 사이트가 0곳인데 카드가 있다")
	}

	oa, err := users.GetByUsername("oa03")
	if err != nil {
		t.Fatal(err)
	}
	foreign := doFormUser(t, e, "/workboard/tasks/"+id+"/action/sites", url.Values{"site_id": {"C-mid"}}, jwtCookieUser(t, oa))
	if foreign.Code != 303 && foreign.Code != 403 && foreign.Code != 404 {
		t.Fatalf("타조직 %d", foreign.Code)
	}
	after, _ := repo.ListTaskSites(id)
	if len(after) != 1 || after[0].SiteID != "C-high" {
		t.Fatalf("타조직이 사이트를 바꿈 %+v", after)
	}
}

func Test95_169_SiteImportFilterJS(t *testing.T) {
	js := filepath.Join(findTemplateRoot(t), "..", "static", "js", "pw-action.js")
	src, err := os.ReadFile(js)
	if err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(t.TempDir(), "pw-site.js")
	logic := "var window = globalThis;\n" + string(src) + `
var rows = [
  {id:'a', name:'계룡고등학교', parent:'충남교육청', region:'계룡시'},
  {id:'b', name:'천안초등학교', parent:'충남교육청', region:'천안시'},
  {id:'c', name:'Library Hall', parent:'City', region:'Seoul'}
];
function assert(cond, msg) { if (!cond) { console.error(msg); process.exit(1); } }
assert(pwSiteImportMatch(rows[0], '계', ''), '1글자');
assert(!pwSiteImportMatch(rows[0], 'ㄱ', ''), '초성 제외');
assert(pwSiteImportMatch(rows[0], '교육청', ''), '상위기관');
assert(pwSiteImportMatch(rows[0], '계룡시', ''), '지역');
assert(pwSiteImportMatch(rows[0], '계 룡 고', ''), '공백 무시');
assert(pwSiteImportMatch(rows[2], 'library', ''), '대소문자');
assert(!pwSiteImportMatch(rows[1], '없는곳', ''), '0건');
assert(pwSiteImportMatch(rows[0], '계', '계룡시'), '지역 AND');
assert(!pwSiteImportMatch(rows[0], '계', '천안시'), '다른 지역');
var box = pwSiteImport(rows);
box.toggle('b', true);
box.q = '계룡';
box.selectVisible();
assert(box.picked.a && box.picked.b && !box.picked.c, '보이는 항목만 추가하고 숨은 선택은 유지');
box.clearVisible();
assert(!box.picked.a && box.picked.b, '보이는 항목만 해제');
box.q = '';
box.clearAll();
assert(box.pickedCount() === 0, '모두 해제');
assert(box.regions.indexOf('계룡시') >= 0 && box.regions.indexOf('천안시') >= 0, '지역 목록');
`
	if err := os.WriteFile(tmp, []byte(logic), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("node", "--check", js).CombinedOutput(); err != nil {
		t.Fatalf("구문 %s", out)
	}
	if out, err := exec.Command("node", tmp).CombinedOutput(); err != nil {
		t.Fatalf("필터 %s", out)
	}
}

func Test95_169_SiteImportRenderCost(t *testing.T) {
	e, repo, _, db := newProjectWorkHarness(t)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	const n = 3000
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("C169-%04d", i)
		region := "천안시"
		if i%3 == 0 {
			region = "공주시"
		}
		if _, err := tx.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id,party_kind,addr_sigungu)
			VALUES(?,?,?,1,'O01','customer',?)`, id, "학교"+id, "학교"+id, region); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,org_id,is_active)
			VALUES(?,?,?,?,'planned','O01',1)`, "PS169-"+id, "WP-O01", id, "학교"+id); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	id := pwCreateTask(t, e, repo, "대량사이트169")
	if rec := doForm(t, e, "/workboard/tasks/"+id+"/action/project", url.Values{"project_id": {"WP-O01"}}); rec.Code != 303 {
		t.Fatalf("연결 %d", rec.Code)
	}
	start := time.Now()
	page := doGet(t, e, "/workboard/tasks/"+id+"/action")
	elapsed := time.Since(start)
	if page.Code != 200 {
		t.Fatalf("GET %d", page.Code)
	}
	size := page.Body.Len()
	t.Logf("sites=%d render=%s html=%d", n, elapsed, size)
	if elapsed > 2*time.Second {
		t.Fatalf("렌더 %s", elapsed)
	}
	if size > 2*1024*1024 {
		t.Fatalf("html %d", size)
	}
	if !strings.Contains(page.Body.String(), "C169-0000") {
		t.Fatal("대량 사이트가 응답에 없다")
	}
}
