package handler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func asRegisterLookupJS(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(findStaticRoot(t), "js", "as-register-lookup.js"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestASLookupScriptOnBoardAndRegister(t *testing.T) {
	e, _, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "as20c-script.db"))
	ck := jwtCookieUser(t, me)
	src := `/static/js/as-register-lookup.js`
	board := doGetWith(t, e, "/workboard/board?view=day", ck).Body.String()
	if !strings.Contains(board, src) {
		t.Fatal("업무 관리에 공용 스크립트 없음")
	}
	reg := doGetWith(t, e, "/workboard/register?view=day&date=2026-10-24&auto=0", ck).Body.String()
	if !strings.Contains(reg, src) {
		t.Fatal("예전 화면에 공용 스크립트 없음")
	}
	if !strings.Contains(reg, `id="as-lookup-status"`) || !strings.Contains(reg, `id="as-register-q"`) {
		t.Fatal("예전 화면 조회 영역 없음")
	}
}

func TestNewWorkASFormHasLookupMarkup(t *testing.T) {
	e, _, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "as20c-form.db"))
	ck := jwtCookieUser(t, me)
	body := doGetWith(t, e, "/workboard/new-work/form?kind=as&date=2026-10-24", ck).Body.String()
	if !strings.Contains(body, `id="as-lookup-status"`) || !strings.Contains(body, `id="as-register-q"`) {
		t.Fatal("AS 조각에 조회 영역 없음")
	}
	js := asRegisterLookupJS(t)
	if !strings.Contains(js, "/workboard/as-lookup") || !strings.Contains(js, "window.bindASRegisterLookup") {
		t.Fatal("공용 스크립트에 조회 호출 없음")
	}
	if !strings.Contains(js, "missingDialog") {
		t.Fatal("새 모달 팝업 옵션 없음")
	}
}

func TestASLookupMissingDialogMarkup(t *testing.T) {
	js := asRegisterLookupJS(t)
	for _, s := range []string{
		`role`, `alertdialog`,
		"접수된 내용이 없습니다",
		"에 해당하는 AS 접수가 없습니다. 새로 접수하시겠습니까?",
		"접수 추가",
		"닫기",
		"document.createTextNode",
	} {
		if !strings.Contains(js, s) {
			t.Fatalf("팝업 문구 없음: %s", s)
		}
	}
	if strings.Contains(js, "innerHTML") {
		t.Fatal("검색어 innerHTML 삽입 금지")
	}
}

func TestASLookupDialogCreatePermission(t *testing.T) {
	js := asRegisterLookupJS(t)
	idx := strings.Index(js, "function showMissingDialog")
	if idx < 0 {
		t.Fatal("showMissingDialog 없음")
	}
	chunk := js[idx:]
	if i := strings.Index(chunk, "function renderInline"); i > 0 {
		chunk = chunk[:i]
	}
	if !strings.Contains(chunk, "data.can_create") {
		t.Fatal("can_create 분기 없음")
	}
	if !strings.Contains(chunk, "AS 접수 권한이 없습니다. 담당자에게 접수를 요청하세요.") {
		t.Fatal("권한 없음 안내 없음")
	}
}

func TestASLookupAddLinkAttrs(t *testing.T) {
	js := asRegisterLookupJS(t)
	idx := strings.Index(js, "function showMissingDialog")
	chunk := js[idx:]
	if i := strings.Index(chunk, "function renderInline"); i > 0 {
		chunk = chunk[:i]
	}
	if !strings.Contains(chunk, "data.new_href") || !strings.Contains(chunk, "/as/new") {
		t.Fatal("new_href 없음")
	}
	if !strings.Contains(js, `window.open(href, '_blank', 'noopener')`) {
		t.Fatal("window.open 이중 안전 없음")
	}
	if !strings.Contains(js, "as-new-blocked") {
		t.Fatal("팝업 차단 안내 없음")
	}
	if strings.Contains(chunk, `el('a'`) {
		t.Fatal("모달 접수 추가가 <a> 이다")
	}
}

func TestASLookupDismissAndRefocus(t *testing.T) {
	js := asRegisterLookupJS(t)
	if !strings.Contains(js, "dismissed") {
		t.Fatal("같은 검색어 재표시 방지 없음")
	}
	if !strings.Contains(js, "addEventListener('focus'") && !strings.Contains(js, `addEventListener("focus"`) {
		t.Fatal("창 focus 재조회 없음")
	}
	if !strings.Contains(js, "awaitingReturn") {
		t.Fatal("접수 추가 후 재조회 플래그 없음")
	}
	if !strings.Contains(js, "700") {
		t.Fatal("팝업 모드 700ms 디바운스 없음")
	}
}

func TestASLookupNoDialogForPlacedClosedOk(t *testing.T) {
	js := asRegisterLookupJS(t)
	idx := strings.Index(js, "function lookup()")
	if idx < 0 {
		t.Fatal("lookup 없음")
	}
	chunk := js[idx:]
	if i := strings.Index(chunk, "q.addEventListener"); i > 0 {
		chunk = chunk[:i]
	}
	if !strings.Contains(chunk, "kind === 'ok'") {
		t.Fatal("ok 처리 없음")
	}
	if !strings.Contains(chunk, "showMissingDialog") {
		t.Fatal("missing 팝업 분기 없음")
	}
	if !strings.Contains(chunk, "kind === 'placed'") || !strings.Contains(chunk, "kind === 'closed'") {
		t.Fatal("placed/closed 인라인 유지 없음")
	}
	placedIdx := strings.Index(chunk, "kind === 'placed'")
	missIdx := strings.Index(chunk, "showMissingDialog")
	if placedIdx >= 0 && missIdx >= 0 && placedIdx < missIdx {
		// placed 분기가 팝업보다 앞이면 팝업을 타지 않음 — 여기선 missing만 showMissingDialog
	}
	if strings.Count(chunk, "showMissingDialog") != 1 {
		t.Fatalf("showMissingDialog 호출 %d회", strings.Count(chunk, "showMissingDialog"))
	}
}

func TestASLookupOrgIsolationStillMissing(t *testing.T) {
	e, _, o3, _, _ := newASLookupHTTP(t)
	hidden := getASLookup(t, e, "R2610-075", o3)
	if hidden.Kind != "missing" {
		t.Fatalf("타조직 번호는 missing이어야 함 %+v", hidden)
	}
	if len(hidden.Items) != 0 {
		t.Fatal("타조직 존재 여부 구분 불가")
	}
	nameHide := getASLookup(t, e, "서천도서관", o3)
	if nameHide.Kind != "missing" {
		t.Fatalf("타조직 이름 %+v", nameHide)
	}
}

func TestRegisterASLookupLegacyButtonsRemain(t *testing.T) {
	js := asRegisterLookupJS(t)
	if !strings.Contains(js, "txt('추가')") || !strings.Contains(js, "txt('확인')") {
		t.Fatal("예전 화면 추가/확인 버튼 없음")
	}
	if !strings.Contains(js, "입력 중인 내용이 사라집니다") {
		t.Fatal("예전 confirm 없음")
	}
	if !strings.Contains(js, "250") {
		t.Fatal("예전 250ms 디바운스 없음")
	}
	e, _, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "as20c-reg.db"))
	ck := jwtCookieUser(t, me)
	reg := doGetWith(t, e, "/workboard/register?view=day&date=2026-10-24&auto=0", ck).Body.String()
	if strings.Contains(reg, "window.bindASRegisterLookup = function") {
		t.Fatal("예전 화면에 조회 스크립트가 인라인으로 남아 있다")
	}
}
