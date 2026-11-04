package handler

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var handlerPickerCallRe = regexp.MustCompile(`customerRepo\.(ListAll|ListAllInOrg|ListForReceipt|List)\(`)

func TestHandlerCustomerPickerCallsAllowlisted(t *testing.T) {
	allow := map[string]string{
		"customer.go":  "고객현황 폼 상위기관·내보내기 ListAll / List. 화면 자체는 95-22 범위 밖",
		"backup.go":    "백업 전체 열람",
		"as.go":        "AS 접수 옵션 출처 ListForReceipt → Lookup 위임. 초성 검색 UI 유지",
		"work.go":      "업무 목록 필터 ListForReceipt → Lookup",
	}
	root := findHandlerRoot(t)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		base := filepath.Base(path)
		if !handlerPickerCallRe.Match(b) {
			return nil
		}
		if allow[base] != "" {
			return nil
		}
		t.Errorf("%s: customerRepo.ListAll/ListAllInOrg/ListForReceipt/List 는 허용 목록 밖", base)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func findHandlerRoot(t *testing.T) string {
	t.Helper()
	if st, err := os.Stat("as.go"); err == nil && !st.IsDir() {
		return "."
	}
	return filepath.Join("internal", "handler")
}

func TestCustomerComboSelectLint(t *testing.T) {
	allowSelect := map[string]string{
		"admin/org_split.html":         "조직 분할 체크박스 name=customer_id",
		"admin/data.html":              "관리 데이터 보정 화면",
		"maintenance/bulk.html":        "업종별 일괄 등록 체크박스",
		"maintenance/site_list.html":   "지역 일괄 지정 체크박스",
		"maintenance/plan_manage.html": "hidden 이미 고른 사이트",
		"maintenance/site_form.html":   "hidden 현재 고객",
		"as/form.html":                 "hidden + 초성 검색(AS 동작 유지)",
		"quotes/form.html":             "hidden Alpine 견적 수신처",
		"space/edit.html":              "hidden 층·실 저장",
		"project/sites_pick.html":      "체크박스",
		"as/knowledge.html":            "사례 건수 있는 사이트 목록(.Sites) — 마스터 콤보 아님",
		"sales/form.html":              "후속: ScopeParty 콤보",
		"contact/form.html":            "후속",
		"contact_history/list.html":    "후속 필터",
		"asset/form.html":              "후속",
		"asset/list.html":              "후속 필터",
		"relation/form.html":           "후속",
		"space/list.html":              "후속 필터",
		"admin_work/form.html": "후속",
	}
	root := findTemplateRoot(t)
	selRe := regexp.MustCompile(`(?is)<select[^>]*name="(customer_id|ordering_party_id|end_customer_id)"`)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !selRe.Match(b) {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if allowSelect[rel] != "" {
			return nil
		}
		t.Errorf("%s: 고객 선택 <select name=customer_id> — customer_combo 를 쓰세요", rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCustomerComboLintFailsOnSample(t *testing.T) {
	sample := `<select name="customer_id" class="select"></select>`
	if !regexp.MustCompile(`(?is)<select[^>]*name="customer_id"`).MatchString(sample) {
		t.Fatal("양성 샘플이 규칙에 안 걸린다")
	}
}
