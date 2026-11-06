package handler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func Test95_113BNewWorkASRefreshMarkupAndJSON(t *testing.T) {
	e, _, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "p113b-as.db"))
	ck := jwtCookieUser(t, me)
	as := doGetWith(t, e, "/workboard/new-work/form?kind=as&date=2026-11-04", ck).Body.String()
	if !strings.Contains(as, "목록 새로고침") {
		t.Fatal("목록 새로고침 버튼 없음")
	}
	if !strings.Contains(as, `id="as-register-source"`) {
		t.Fatal("대기 AS select id 없음")
	}
	if !strings.Contains(as, `onclick="window.openASReceiptTab`) {
		t.Fatal("AS 접수 추가가 window.open 경로가 아님")
	}
	if strings.Contains(as, `href="/as/new"`) {
		t.Fatal("AS 접수 추가가 <a href> 로 남아 있다")
	}
	if !strings.Contains(as, "팝업이 차단되어 새 탭을 열 수 없습니다") {
		t.Fatal("팝업 차단 안내 없음")
	}
	board := doGetWith(t, e, "/workboard/board?view=day", ck).Body.String()
	if !strings.Contains(board, "/static/js/as-register-lookup.js?v=") {
		t.Fatal("모달 JS AssetQuery 없음")
	}
	rec := doGetWith(t, e, "/workboard/new-work/as-cards", ck)
	if rec.Code != 200 {
		t.Fatalf("as-cards status=%d %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Cards []model.WBCard `json:"cards"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Cards == nil {
		t.Fatal("cards 키 없음")
	}
}

func Test95_113BLookupJSHasRefreshAndOpen(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(findStaticRoot(t), "js", "as-register-lookup.js"))
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	for _, s := range []string{
		"window.refreshASWaitingList",
		"/workboard/new-work/as-cards",
		"window.openASReceiptTab",
		`window.open(href, '_blank', 'noopener')`,
		"as-new-blocked",
	} {
		if !strings.Contains(js, s) {
			t.Fatalf("없음: %s", s)
		}
	}
	if strings.Contains(js, "window.location.href = data.new_href") {
		t.Fatal("같은 탭 이동이 남아 있다")
	}
}
