package handler

import (
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func Test95_74_OverlapModalAboveAll(t *testing.T) {
	root := findTemplateRoot(t)
	base := string(mustRead(t, filepath.Join(root, "layout/base.html")))
	idx := strings.LastIndex(base, `id="overlap-resolve"`)
	if idx < 0 {
		t.Fatal("겹침 해결 모달이 레이아웃에 없다")
	}
	if !strings.Contains(base[idx:idx+200], "z-[80]") && !strings.Contains(base[idx:idx+240], "z-[80]") {
		t.Fatal("겹침 해결 모달 z-[80] 없음")
	}
	if strings.LastIndex(base, "</body>") < idx {
		t.Fatal("겹침 해결 모달이 body 끝에 없다")
	}
	if strings.Index(base, `id="overlap-resolve"`) != idx {
		t.Fatal("겹침 해결 모달이 두 곳이다")
	}
	js := string(mustRead(t, filepath.Join(filepath.Dir(root), "static/js/overlap_resolve.js")))
	for _, want := range []string{"closeOverlapResolve", "keydown", "Escape", "Tab", "hideOverlay(false)", "document.body.appendChild", "처리 중…", "overlap-resolve-err"} {
		if !strings.Contains(js, want) {
			t.Fatalf("overlap_resolve.js 에 %s 없음", want)
		}
	}
	nw := string(mustRead(t, filepath.Join(root, "workboard/_new_work_modal.html")))
	if !strings.Contains(nw, "closeOverlapResolve") {
		t.Fatal("새 업무 모달 닫기가 겹침 모달을 닫지 않는다")
	}
}

func Test95_74_HandleKeepModalAlwaysVisible(t *testing.T) {
	js := string(mustRead(t, filepath.Join(findTemplateRoot(t), "..", "static/js/overlap_resolve.js")))
	if strings.Contains(js, "if (j && j.cancelled) return;") {
		t.Fatal("409/ok cancelled 가 조용히 끝난다")
	}
	if !strings.Contains(js, "flash(") {
		t.Fatal("flash 헬퍼 없음")
	}
	if strings.Count(js, "flash(") < 8 {
		t.Fatal("오류 경로 flash 가 부족하다")
	}
	nw := string(mustRead(t, filepath.Join(findTemplateRoot(t), "workboard/_new_work_modal.html")))
	modals := string(mustRead(t, filepath.Join(findTemplateRoot(t), "workboard/_modals.html")))
	if strings.Contains(nw, "form.submit()") || strings.Contains(modals, "form.submit()") {
		t.Fatal("네트워크 오류 시 form.submit 우회가 남아 있다")
	}
}

func Test95_74_AssigneeNeverShowsUSR(t *testing.T) {
	js := string(mustRead(t, filepath.Join(findTemplateRoot(t), "..", "static/js/assignee_picker.js")))
	if !strings.Contains(js, "이름 미등록") {
		t.Fatal("nameOf 최종 fallback 이 이름 미등록이 아니다")
	}
	if strings.Contains(js, "return '담당자'") {
		t.Fatal("nameOf 가 이름 대신 담당자를 반환한다")
	}
	if strings.Contains(js, "if (!el) return id") {
		t.Fatal("nameOf 가 id 를 그대로 반환한다")
	}
	e, _, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d74-picker.db"))
	ck := jwtCookieUser(t, me)
	req := doGetWith(t, e, "/workboard/new-work/form?kind=admin&date="+time.Now().Format(dateLayout), ck)
	if req.Code != http.StatusOK {
		t.Fatalf("폼 %d", req.Code)
	}
	body := req.Body.String()
	if !strings.Contains(body, "js-picker-names") {
		t.Fatal("PickerNames JSON 없음")
	}
	if !strings.Contains(body, me.UserID) {
		t.Fatal("선택된 현재 사용자가 목록에 없다")
	}
	if strings.Contains(body, `data-name="USR-`) {
		t.Fatal("담당자 칸 data-name 이 USR- 이다")
	}
}

func Test95_74_PickerDisplayName(t *testing.T) {
	u := model.User{UserID: "USR-1792139370211635000", Username: "hmyang", FullName: "양현모"}
	if pickerDisplayName(u) != "양현모" {
		t.Fatalf("got %s", pickerDisplayName(u))
	}
	u.FullName = "USR-1792139370211635000"
	if pickerDisplayName(u) != "hmyang" {
		t.Fatalf("username fallback %s", pickerDisplayName(u))
	}
	u.Username = "USR-x"
	if pickerDisplayName(u) != u.UserID {
		t.Fatalf("both USR %s", pickerDisplayName(u))
	}
	if pickerDisplayName(model.User{}) != "이름 미등록" {
		t.Fatalf("empty %s", pickerDisplayName(model.User{}))
	}
	if pickerDisplayName(u) == "담당자" {
		t.Fatal("담당자 단독 반환")
	}
}

func Test95_74_Overlap409JSONShowsConflicts(t *testing.T) {
	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d74-ov.db"))
	ck := jwtCookieUser(t, me)
	day := time.Now().AddDate(0, 0, 2).Format(dateLayout)
	mustCreateBoardTask(t, repo, "테스트_작업_03_공사", day, "09:00", "11:40", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	rec := doFormJSONWith(t, e, "/workboard/tasks", url.Values{
		"work_type": {"admin"}, "title": {"겹침재현"}, "work_date": {day}, "due_date": {day},
		"start_time": {"09:00"}, "end_time": {"09:30"}, "assignee": {me.FullName},
		"assignee_user_id": {me.UserID},
	}, ck)
	if rec.Code != http.StatusConflict {
		t.Fatalf("409 아님 %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "conflicts") {
		t.Fatalf("conflicts 없음 %s", rec.Body.String())
	}
}

func Test95_74_KeepModalRequiredFlashJS(t *testing.T) {
	js := string(mustRead(t, filepath.Join(findTemplateRoot(t), "..", "static/js/overlap_resolve.js")))
	if !strings.Contains(js, "필수 항목을 확인해 주세요") {
		t.Fatal("필수 누락 안내 없음")
	}
	if !strings.Contains(js, "네트워크를 확인해 주세요") {
		t.Fatal("네트워크 오류 안내 없음")
	}
}

func Test95_74_MobileOverlapButtons(t *testing.T) {
	root := findTemplateRoot(t)
	base := string(mustRead(t, filepath.Join(root, "layout/base.html")))
	if !strings.Contains(base, `grid grid-cols-1 sm:grid-cols-2`) {
		t.Fatal("360/390에서 겹침 해결 버튼이 한 줄로 쌓이지 않는다")
	}
}

func Test95_74_NodeNameOfAndPlaywright(t *testing.T) {
	skipIfNoPlaywrightBrowser(t)
	cmd := exec.Command("node", "95_74_picker_test.js")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("js 테스트 실패: %v\n%s", err, out)
	}
	play := exec.Command("node", "95_74_browser_test.mjs")
	play.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	play.Env = append(os.Environ(),
		"PLAYWRIGHT_HTML="+filepath.Join(findTemplateRoot(t), "..", "static", "review-95-74", "overlap-stack.html"),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
	)
	out, err = play.CombinedOutput()
	if err != nil {
		t.Fatalf("브라우저 테스트 실패: %v\n%s", err, out)
	}
}
