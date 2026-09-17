package handler

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestASActionCauseCatsReplaceCauseTypeSelect(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	body := getASActionPage(t, e, asID)
	formStart := strings.Index(body, `id="as-action-form"`)
	formEndRel := strings.Index(body[formStart:], "</form>")
	form := body[formStart : formStart+formEndRel]
	if strings.Contains(form, `name="cause_type"`) {
		t.Fatal("원인분류 5종 select 가 남아 있다")
	}
	n := strings.Count(form, `name="cause_cat`)
	if n != 3 {
		t.Fatalf("원인분류 드롭다운 %d개", n)
	}
	if !strings.Contains(body, `"code":"server.patch"`) || !strings.Contains(body, `"label":"패치"`) {
		t.Fatal("시드에 패치가 없다")
	}

	rec := postASAction(t, e, asID, url.Values{
		"work_place":   {"office"},
		"process_type": {"remote"},
		"cause_cat1":   {"server"},
		"cause_cat2":   {"server.patch"},
		"action_taken": {"보안 패치"},
	})
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || strings.Contains(loc, "err=") {
		t.Fatalf("저장 실패: %s", loc)
	}
	got, err := asRepo.GetByID(asID)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if got.CauseCat1 != "server" || got.CauseCat2 != "server.patch" || got.CauseCat3 != "" {
		t.Fatalf("cats=%q/%q/%q", got.CauseCat1, got.CauseCat2, got.CauseCat3)
	}
	if got.CauseType != "sw" {
		t.Fatalf("패치 매핑 cause_type=%q want sw", got.CauseType)
	}

	empty := postASAction(t, e, asID, url.Values{
		"work_place":   {"office"},
		"process_type": {"remote"},
		"cause_cat1":   {"web"},
		"cause_cat2":   {"web.post_edit"},
		"action_taken": {"게시물 수정"},
	})
	if empty.Code != http.StatusSeeOther || strings.Contains(empty.Header().Get("Location"), "err=") {
		t.Fatalf("매핑 없는 2차 저장 실패: %s", empty.Header().Get("Location"))
	}
	got, _ = asRepo.GetByID(asID)
	if got.CauseType != "" {
		t.Fatalf("매핑 없으면 cause_type 빈값이어야 한다: %q", got.CauseType)
	}
}

func TestASActionCauseCat3RequiredWhenChildrenExist(t *testing.T) {
	e, _, _, _, asID := newASActionFixture(t)
	rec := postASAction(t, e, asID, url.Values{
		"work_place":   {"field"},
		"process_type": {"visit"},
		"cause_cat1":   {"rfid"},
		"cause_cat2":   {"rfid.hw"},
		"action_taken": {"부품 점검"},
		"result_code":  {model.ResultDone},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=cause_cat3") {
		t.Fatalf("3차 필수 거절 실패: %s", rec.Header().Get("Location"))
	}
}
