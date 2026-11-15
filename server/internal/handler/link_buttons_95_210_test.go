package handler

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// 95-210: 링크 → 버튼 통일. ① ui_lint 규칙 표 ② 변환 대상은 버튼 클래스·밑줄 없음 ③ 링크로 남긴(제외·애매) 마크업은 해시 불변.

func TestUILintLinkUnderlineRule95_210(t *testing.T) {
	cases := []struct {
		name string
		html string
		want int
	}{
		{"밑줄 단독 링크는 위반", `<a href="/x" class="text-xs text-primary underline">목록</a>`, 1},
		{"hover:underline 도 위반", `<a href="/x" class="text-primary hover:underline">목록</a>`, 1},
		{"버튼 링크는 통과", `<a href="/x" class="btn-primary h-11 px-4 text-ui-sm whitespace-nowrap">목록</a>`, 0},
		{"no-underline 은 통과", `<a href="/x" class="no-underline text-ink">x</a>`, 0},
		{"data-link=text 표식은 허용(표 안 이름·문장 속)", `<a href="/x" data-link="text" class="text-primary hover:underline">사업명</a>`, 0},
		{"여러 줄 태그도 센다", "<a\n  href=\"/x\"\n  class=\"underline\">x</a>", 1},
		{"밑줄 없는 일반 링크는 통과", `<a href="/x" class="text-primary">x</a>`, 0},
		{"둘 섞이면 위반만", `<a class="underline" href="/a">a</a> <a class="btn-secondary btn-sm" href="/b">b</a> <a data-link="text" class="underline" href="/c">c</a>`, 1},
	}
	for _, c := range cases {
		if got := countLinkUnderline(c.html); got != c.want {
			t.Errorf("%s: %d want %d", c.name, got, c.want)
		}
	}
}

var converted95210 = [][2]string{
	{"_partials/project_picker.html", "사업 등록 화면"},
	{"admin/data_reset.html", "데이터 관리로"},
	{"admin/orgs.html", "분리·정리·복구"},
	{"admin_work/stats.html", "행정관련업무등록/처리 →"},
	{"admin_work/stats.html", "목록 보기"},
	{"as/action.html", "예전 화면으로"},
	{"as/action.html", "대기 목록"},
	{"as/action.html", "접수취소 목록"},
	{"as/kb_history.html", "사례 검색으로"},
	{"as/knowledge.html", "수정 이력 페이지"},
	{"as/knowledge.html", "원본 열기"},
	{"as/knowledge.html", "원본 열기"},
	{"as/show.html", "조치 화면 →"},
	{"as/show.html", "{{if .ASReports}}보고서 재발급{{else}}조치완료보고서 생성{{end}} →"},
	{"as/stats.html", "AS 처리 목록 →"},
	{"as/stats.html", "AS 처리 목록 →"},
	{"asset/import.html", "결과 CSV 내려받기"},
	{"auth/admin_unlock.html", "대시보드로"},
	{"contracts/show.html", "사업관리에서 수정"},
	{"contracts/show.html", "사업 대시보드로"},
	{"customer/tab_as.html", "AS 접수하기"},
	{"customer/tab_assets.html", "첫 자산 등록하기"},
	{"customer/tab_contacts.html", "첫 담당자 등록하기"},
	{"dashboard.html", "영업 대시보드로"},
	{"dashboard.html", "통계에서 보기 →"},
	{"dashboard.html", "전체 →"},
	{"dashboard.html", "전체 →"},
	{"dashboard.html", "팀 전체 →"},
	{"dashboard.html", "전체 →"},
	{"dashboard.html", "전체 →"},
	{"maintenance/plan_generate.html", "휴무일 등록"},
	{"maintenance/plan_manage.html", "점검 사이트 등록"},
	{"maintenance/plan_manage.html", "점검 사이트"},
	{"meeting/board.html", "일일 (예전)"},
	{"meeting/board.html", "주간 팀 (예전)"},
	{"meeting/board.html", "주간 내 자료 (예전)"},
	{"meeting/weekly.html", "휴가 화면"},
	{"meeting/weekly.html", "결재함"},
	{"project/import.html", "영업관리에서 확인"},
	{"sales/_activity_log.html", "전체 기간 보기"},
	{"sales/activity_show.html", "사업으로"},
	{"sales/dashboard.html", "기술 대시보드 보기"},
	{"sales/dashboard.html", "계약으로 전환"},
	{"sales/dashboard.html", "전체 보기 →"},
	{"sales/form.html", "빠른 견적"},
	{"sales/quick_quote.html", "영업 사업(기존)으로"},
	{"sales/show.html", "자세한 정보 입력"},
	{"sales/show.html", "수주 관리에서 보기"},
	{"stats/_weekly_card.html", "업무 열기"},
	{"stats/_weekly_unassigned.html", "사업 연결"},
	{"stats/_weekly_unassigned.html", "사업 연결"},
	{"stats/company_weekly.html", "다른 출력물"},
	{"stats/company_weekly.html", "예전 화면으로"},
	{"stats/company_weekly.html", "행 관리"},
	{"stats/reports.html", "행 관리"},
	{"workboard/_modals.html", "계획 관리에서 등록"},
	{"workboard/_modals.html", "발주처 상세 보기 →"},
	{"workboard/_modals.html", "고객 상세 보기 →"},
	{"workboard/_modals.html", "담당자 상세 보기 →"},
	{"workboard/task_show.html", "조치 화면 열기 →"},
	{"workboard/task_show.html", "조치 완료 화면 열기 →"},
	{"workboard/task_show.html", "수정"},
}

func readTemplate95210(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("web", "templates", rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// 변환한 링크: 같은 문구의 <a> 가 btn-primary/btn-secondary 클래스이고 밑줄이 없다(또는 link_button 파셜).
func TestConvertedLinksAreButtons95_210(t *testing.T) {
	if len(converted95210) != 62 {
		t.Fatalf("변환 표 %d건 (전수표 62건과 달라짐)", len(converted95210))
	}
	for _, c := range converted95210 {
		src := readTemplate95210(t, c[0])
		label := regexp.QuoteMeta(c[1])
		anchor := regexp.MustCompile(`(?s)<a\b[^>]*class="([^"]*)"[^>]*>\s*` + label + `\s*</a>`)
		found := false
		for _, m := range anchor.FindAllStringSubmatch(src, -1) {
			if strings.Contains(m[1], "btn-primary") || strings.Contains(m[1], "btn-secondary") {
				if strings.Contains(m[1], "underline") {
					t.Errorf("%s %q: 버튼 클래스에 underline 이 남았다 %q", c[0], c[1], m[1])
				}
				found = true
			}
		}
		if !found && !strings.Contains(src, `template "link_button"`) {
			t.Errorf("%s %q: 버튼 링크를 찾지 못했다", c[0], c[1])
		}
		if !found && strings.Contains(src, `template "link_button"`) && !regexp.MustCompile(`link_button" dict[^}]*"Label" "`+label+`"`).MatchString(src) {
			t.Errorf("%s %q: 파셜 호출을 찾지 못했다", c[0], c[1])
		}
	}
}

// 링크로 남긴 것(제외·애매): 밑줄 링크 태그의 해시 목록이 골든과 같다(마크업 불변). 새 밑줄 링크가 생기면 이 시험이 알려 준다.
func TestExcludedLinksUnchanged95_210(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("internal", "handler", "testdata", "link_underline_golden_95_210.json"))
	if err != nil {
		t.Fatal(err)
	}
	golden := map[string][]string{}
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	anc := regexp.MustCompile(`(?is)<a\b[^>]*>`)
	ul := regexp.MustCompile(`(?is)\bclass\s*=\s*"(?:[^"]*[\s:])?underline\b[^"]*"`)
	allow := regexp.MustCompile(`(?i)\bdata-link\s*=\s*"text"`)
	ws := regexp.MustCompile(`\s+`)
	actual := map[string][]string{}
	root := filepath.Join("web", "templates")
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".html") {
			return nil
		}
		b, _ := os.ReadFile(p)
		rel, _ := filepath.Rel(root, p)
		for _, tag := range anc.FindAllString(string(b), -1) {
			if allow.MatchString(tag) || !ul.MatchString(tag) {
				continue
			}
			sum := sha1.Sum([]byte(ws.ReplaceAllString(tag, " ")))
			actual[filepath.ToSlash(rel)] = append(actual[filepath.ToSlash(rel)], hex.EncodeToString(sum[:])[:12])
		}
		return nil
	})
	for k := range actual {
		sort.Strings(actual[k])
	}
	for rel, want := range golden {
		got := actual[rel]
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s: 링크로 남긴 밑줄 링크가 바뀌었다 (골든 %d개 → 지금 %d개)", rel, len(want), len(got))
		}
	}
	for rel, got := range actual {
		if _, ok := golden[rel]; !ok {
			t.Errorf("%s: 골든에 없는 새 밑줄 링크 %d개 — 독립된 행동·이동이면 btn-* 버튼으로, 링크로 남기는 종류면 data-link=\"text\"", rel, len(got))
		}
	}
}
