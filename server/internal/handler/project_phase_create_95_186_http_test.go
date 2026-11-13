package handler

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
)

// 95-186: 「1단계 사업 만들기」 박스가 수행 구조 맨 위에 있고, 맨 아래 폼은 없다.

func TestHTTP95_186_CreateBoxOnTop(t *testing.T) {
	v := setup183(t)
	body := v.get(t, "/projects/"+v.id, v.admin).Body.String()
	if n := strings.Count(body, `action="/projects/`+v.id+`/phases" `); n != 1 {
		t.Fatalf("단계 추가 폼 %d개 want 1", n)
	}
	iSum := strings.Index(body, "수행 구조")
	iChg := strings.Index(body, "data-changes-link")
	iBox := strings.Index(body, "data-phase-create")
	iList := strings.Index(body, `data-phase-id="`+v.phase+`"`)
	iCopy := strings.Index(body, "data-structure-copy")
	if !(iSum >= 0 && iSum < iChg && iChg < iBox && iBox < iList && iBox < iCopy) {
		t.Fatalf("위치 순서 오류 sum=%d chg=%d box=%d list=%d copy=%d", iSum, iChg, iBox, iList, iCopy)
	}
	if !strings.Contains(body, "1단계 사업 만들기") {
		t.Fatal("제목 없음")
	}
	// 단계 추가 폼 뒤(예전 자리)에 단계명 입력이 더 없다: 입력은 박스 안 1개뿐
	if n := strings.Count(body, `placeholder="단계명"`); n != 1 {
		t.Fatalf("단계명 입력 %d개 want 1", n)
	}
}

func TestHTTP95_186_CreateBoxPermissions(t *testing.T) {
	v := setup183(t)
	for _, k := range []struct {
		name string
		see  bool
		code int
		ck   *http.Cookie
	}{
		{"관리자", true, 303, v.admin},
		{"비전관리자", true, 303, cookie183(t, "va", "vision_admin", "O01", false)},
		{"PM", true, 303, cookie183(t, "pm-u", "support", "O01", false, model.PermWorkboard)},
		{"수행 담당", false, 403, cookie183(t, "exec-u", "support", "O01", false, model.PermWorkboard)},
		{"일반", false, 403, cookie183(t, "gen-u", "support", "O01", false, model.PermWorkboard)},
		{"읽기 전용", false, 403, cookie183(t, "ro-u", "org_admin", "O01", true)},
		{"옵저버", false, 403, cookie183(t, "ob", "observer", "O01", false)},
	} {
		r := v.get(t, "/projects/"+v.id, k.ck)
		if r.Code == 200 && strings.Contains(r.Body.String(), "data-phase-create") != k.see {
			t.Fatalf("%s: 박스 표시 want %v", k.name, k.see)
		}
		if code, _ := v.post(t, "/projects/"+v.id+"/phases", url.Values{"name": {"권한 " + k.name}}, k.ck); code != k.code {
			t.Fatalf("%s: POST %d want %d", k.name, code, k.code)
		}
	}
}

func TestHTTP95_186_CreateAndError(t *testing.T) {
	v := setup183(t)
	code, loc := v.post(t, "/projects/"+v.id+"/phases", url.Values{"name": {"신규 단계"}}, v.admin)
	if code != 303 || !strings.Contains(loc, "ok=phase") {
		t.Fatalf("성공 %d %s", code, loc)
	}
	// 오류: 이름 비움 → err 쿼리는 인코딩되어 있고 화면 위쪽에 문구가 나온다
	r := postPhase(t, v.e, "/projects/"+v.id+"/phases", url.Values{"name": {strings.Repeat("가", 101)}}, v.admin)
	raw := r.Header().Get("Location")
	if r.Code != 303 || !strings.Contains(raw, "?err=") || strings.ContainsAny(strings.SplitN(strings.SplitN(raw, "?err=", 2)[1], "#", 2)[0], " ·「」") {
		t.Fatalf("오류 Location이 인코딩되지 않음: %s", raw)
	}
	page := v.get(t, strings.ReplaceAll(raw, "http://localhost", ""), v.admin).Body.String()
	if !strings.Contains(page, `id="phase-flash"`) || strings.Index(page, `id="phase-flash"`) > strings.Index(page, "data-phase-create") {
		t.Fatal("오류 문구가 박스 위에 나오지 않음")
	}
}
