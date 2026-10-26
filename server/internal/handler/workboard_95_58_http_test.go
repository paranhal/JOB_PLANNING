package handler

import (
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func Test95_58_ActionLayoutRender(t *testing.T) {
	e, repo, _ := newProjectWorkServer(t)
	id := pwCreateTask(t, e, repo, "화면개편작업")
	page := doGet(t, e, "/workboard/tasks/"+id+"/action")
	if page.Code != http.StatusOK {
		t.Fatalf("GET %d", page.Code)
	}
	body := page.Body.String()
	if !strings.Contains(body, `data-pw-fixed="top"`) || !strings.Contains(body, `data-pw-scroll="mid"`) || !strings.Contains(body, `data-pw-fixed="bar"`) {
		t.Fatal("고정/스크롤 영역 없음")
	}
	if strings.Contains(body, "임시저장") {
		t.Fatal("임시저장 버튼이 보이면 안 됨")
	}
	if !strings.Contains(body, `id="pw-action-cancel"`) {
		t.Fatal("취소 버튼 없음")
	}
	form := extractHTMLForm(body, "as-action-form")
	if _, ok := htmlControlNames(form)["work_date"]; ok {
		t.Fatal("작업일 입력칸이 있으면 안 됨")
	}
	if !strings.Contains(body, `data-pw-supporters="1"`) {
		t.Fatal("지원 인력 UI 없음")
	}
	if strings.Contains(body, "WorkDesk") || strings.Contains(body, "김관리") {
		t.Fatal("Figma 샘플 문구가 화면에 있음")
	}
	if !strings.Contains(body, "직책 없이 이름만 입력") {
		t.Fatal("이름만 입력 안내 없음")
	}
	if !strings.Contains(body, "pw-action-mid") || !strings.Contains(body, "as-action-bar") {
		t.Fatal("레이아웃 클래스 없음")
	}
	css, err := os.ReadFile(filepath.Join(findTemplateRoot(t), "..", "static", "vendor", "tailwind.css"))
	if err != nil {
		t.Fatal(err)
	}
	bundle := string(css)
	for _, cls := range []string{".pw-action-mid", ".pw-action-left", ".pw-action-right", ".pw-chip", ".pw-type-card"} {
		if !strings.Contains(bundle, cls) {
			t.Fatalf("tailwind 번들에 %s 없음 — npm run build:css 필요", cls)
		}
	}
	idxMid := strings.Index(body, `id="pw-action-mid"`)
	idxBar := strings.Index(body, `id="as-action-bar"`)
	if idxMid < 0 || idxBar < 0 || idxBar < idxMid {
		t.Fatal("저장 바가 스크롤 영역 밖에 있지 않음")
	}
}

func Test95_58_FormKeysPostThreeResults(t *testing.T) {
	e, repo, _ := newProjectWorkServer(t)
	id := pwCreateTask(t, e, repo, "키포스트작업")
	page := doGet(t, e, "/workboard/tasks/"+id+"/action")
	form := htmlFormValues(extractHTMLForm(page.Body.String(), "as-action-form"))
	form.Set("action_taken", "현장 배선 정리 및 점검 완료함")
	form.Set("action_short_ok", "1")
	form.Set("work_place", "field")
	form.Set("work_subtype", "other")
	form.Set("result_code", model.WorkResultDone)
	if rec := doForm(t, e, "/workboard/tasks/"+id+"/action", form); rec.Code != http.StatusSeeOther {
		t.Fatalf("완료 POST %d loc=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}

	id2 := pwCreateTask(t, e, repo, "키일부작업")
	page2 := doGet(t, e, "/workboard/tasks/"+id2+"/action")
	p2 := htmlFormValues(extractHTMLForm(page2.Body.String(), "as-action-form"))
	p2.Set("action_taken", "현장 배선 정리 및 점검 완료함")
	p2.Set("action_short_ok", "1")
	p2.Set("work_place", "field")
	p2.Set("work_subtype", "other")
	p2.Set("result_code", model.WorkResultPartial)
	p2.Set("followup_note", "자재 대기")
	p2.Set("next_date", "2026-10-28")
	if rec := doForm(t, e, "/workboard/tasks/"+id2+"/action", p2); rec.Code != http.StatusSeeOther {
		t.Fatalf("일부 POST %d loc=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}

	id3 := pwCreateTask(t, e, repo, "키이관작업")
	page3 := doGet(t, e, "/workboard/tasks/"+id3+"/action")
	p3 := htmlFormValues(extractHTMLForm(page3.Body.String(), "as-action-form"))
	p3.Set("action_taken", "현장 배선 정리 및 점검 완료함")
	p3.Set("action_short_ok", "1")
	p3.Set("work_place", "field")
	p3.Set("work_subtype", "other")
	p3.Set("result_code", model.WorkResultTransfer)
	p3.Set("transfer_to", "협력사C")
	p3.Set("transfer_reason", "야간 이관")
	if rec := doForm(t, e, "/workboard/tasks/"+id3+"/action", p3); rec.Code != http.StatusSeeOther {
		t.Fatalf("이관 POST %d loc=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
}

func Test95_58_WorkDateFromStartKeepsPlan(t *testing.T) {
	e, repo, _ := newProjectWorkServer(t)
	id := pwCreateTask(t, e, repo, "시작일시작업")
	form := pwActionForm()
	form.Set("action_start_at", "2026-10-23T04:50")
	form.Set("complete_datetime", "2026-10-23T05:20")
	form.Set("confirm_start_differs_plan", "1")
	if rec := doForm(t, e, "/workboard/tasks/"+id+"/action", form); rec.Code != http.StatusSeeOther {
		t.Fatalf("저장 %d loc=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	tk, _ := repo.GetTask(id)
	_ = repo.LoadTaskActionFields(tk)
	if tk.WorkDate != "2026-10-21" {
		t.Fatalf("계획 작업일이 바뀜 %s", tk.WorkDate)
	}
	if !strings.HasPrefix(strings.ReplaceAll(tk.ActionStartAt, " ", "T"), "2026-10-23T04:50") {
		t.Fatalf("시작일시 %q", tk.ActionStartAt)
	}
	procs, _ := repo.ListWorkTaskProcesses(tk.OrgID, id)
	if len(procs) == 0 || !strings.Contains(procs[0].ProcessAt, "2026-10-23") {
		t.Fatalf("이력 작업일 %+v", procs)
	}

	id2 := pwCreateTask(t, e, repo, "완료이전작업")
	bad := pwActionForm()
	bad.Set("action_start_at", "2026-10-21T10:00")
	bad.Set("complete_datetime", "2026-10-21T09:00")
	rec := doForm(t, e, "/workboard/tasks/"+id2+"/action", bad)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "완료 일시가 시작 일시보다 이전입니다") {
		t.Fatalf("완료<시작 code=%d body=%s", rec.Code, rec.Body.String())
	}

	id3 := pwCreateTask(t, e, repo, "시작누락작업")
	miss := pwActionForm()
	miss.Del("action_start_at")
	miss.Set("action_start_at", "")
	miss.Set("action_taken", "시작 없이 적은 조치 내용입니다")
	rec = doForm(t, e, "/workboard/tasks/"+id3+"/action", miss)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "시작 일시를 입력하세요") {
		t.Fatalf("시작 누락 code=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "시작 없이 적은 조치 내용입니다") {
		t.Fatal("시작 누락 시 입력 미보존")
	}
}

func Test95_58_SupportersAndCancel(t *testing.T) {
	e, repo, users := newProjectWorkServer(t)
	id := pwCreateTask(t, e, repo, "지원인력작업")
	u2 := &model.User{Username: "sup58a", PasswordHash: HashPassword("password10"), FullName: "지원갑", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	u3 := &model.User{Username: "sup58b", PasswordHash: HashPassword("password10"), FullName: "지원을", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	if err := users.Create(u2); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(u3); err != nil {
		t.Fatal(err)
	}
	u2, _ = users.GetByUsername("sup58a")
	u3, _ = users.GetByUsername("sup58b")
	form := pwActionForm()
	form.Add("supporter_user_id", u2.UserID)
	form.Add("supporter_name", u2.FullName)
	form.Add("supporter_user_id", u3.UserID)
	form.Add("supporter_name", u3.FullName)
	form.Add("supporter_user_id", "")
	form.Add("supporter_name", "외부갑")
	form.Add("supporter_user_id", "")
	form.Add("supporter_name", "협력을")
	if rec := doForm(t, e, "/workboard/tasks/"+id+"/action", form); rec.Code != http.StatusSeeOther {
		t.Fatalf("지원 저장 %d loc=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	ms, err := repo.ListMembers(id)
	if err != nil {
		t.Fatal(err)
	}
	var sup []model.WorkTaskMember
	for _, m := range ms {
		if m.IsSupport() {
			sup = append(sup, m)
		}
	}
	if len(sup) != 4 {
		t.Fatalf("지원 %d %+v", len(sup), sup)
	}

	page := doGet(t, e, "/workboard/tasks/"+id+"/action")
	if page.Code != http.StatusOK {
		t.Fatalf("재조회 %d", page.Code)
	}
	body := page.Body.String()
	if !strings.Contains(body, "외부갑") || !strings.Contains(body, "협력을") {
		t.Fatal("직접입력 지원 인력 재표시 안 됨")
	}

	admin, _ := users.GetByUsername("admin")
	idDup := pwCreateTask(t, e, repo, "주담당중복작업")
	dup := pwActionForm()
	dup.Set("assigned_to_code", admin.UserID)
	dup.Add("supporter_user_id", admin.UserID)
	dup.Add("supporter_name", admin.FullName)
	rec := doForm(t, e, "/workboard/tasks/"+idDup+"/action", dup)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "처리 담당자는 지원 인력에 넣을 수 없습니다") {
		t.Fatalf("주담당 중복 code=%d body=%s", rec.Code, rec.Body.String())
	}

	id2 := pwCreateTask(t, e, repo, "취소모달작업")
	page2 := doGet(t, e, "/workboard/tasks/"+id2+"/action")
	b2 := page2.Body.String()
	if !strings.Contains(b2, "pw-leave-modal") || !strings.Contains(b2, "작성 중인 내용이 사라집니다") {
		t.Fatal("취소 확인 모달 없음")
	}
	if !strings.Contains(b2, `/workboard/tasks/`+id2) {
		t.Fatal("취소 기본 경로가 작업 상세가 아님")
	}
	tk, _ := repo.GetTask(id2)
	if tk.Status == model.WBTaskComplete {
		t.Fatal("취소 경로 조회만으로 저장되면 안 됨")
	}
}

func Test95_58_ErrorKeepsSupporters(t *testing.T) {
	e, repo, _ := newProjectWorkServer(t)
	id := pwCreateTask(t, e, repo, "오류지원작업")
	form := url.Values{
		"work_subtype":        {"other"},
		"work_place":          {"field"},
		"action_start_at":     {"2026-10-21T09:00"},
		"action_duration_min": {"30"},
		"complete_datetime":   {"2026-10-21T09:30"},
		"result_code":         {model.WorkResultDone},
		"supporter_name":      {"현장보조"},
		"supporter_user_id":   {""},
	}
	rec := doForm(t, e, "/workboard/tasks/"+id+"/action", form)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "조치 내용을 입력하세요") {
		t.Fatalf("조치 내용 오류 code=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "현장보조") {
		t.Fatal("오류 시 지원 인력 칩 미보존")
	}
	if !strings.Contains(rec.Body.String(), `id="pw-action-err-bar"`) {
		t.Fatal("저장 바 위 오류 없음")
	}
}

func Test95_58_TimeHelpersAndJS(t *testing.T) {
	local, day, code := parseActionStartAt("2026-10-23T04:50")
	if code != "" || local != "2026-10-23T04:50" || day != "2026-10-23" {
		t.Fatalf("parse start %s %s %s", local, day, code)
	}
	if !completeDatetimeBeforeStart("2026-10-21T09:00", "2026-10-21T10:00") {
		t.Fatal("완료<시작")
	}
	if completeDatetimeBeforeStart("2026-10-21T10:00", "2026-10-21T09:00") {
		t.Fatal("완료>시작은 오류가 아님")
	}
	js := filepath.Join(findTemplateRoot(t), "..", "static", "js", "pw-action.js")
	out, err := exec.Command("node", "--check", js).CombinedOutput()
	if err != nil {
		t.Fatalf("pw-action.js 구문 %s", out)
	}
	tmp := filepath.Join(t.TempDir(), "pw-logic.js")
	src, err := os.ReadFile(js)
	if err != nil {
		t.Fatal(err)
	}
	logic := "var window = globalThis;\n" + string(src) + `
	if (pwAddMinutes('2026-10-21T09:00', 30) !== '2026-10-21T09:30') process.exit(2);
	if (pwDiffMinutes('2026-10-21T09:00', '2026-10-21T10:00') !== 60) process.exit(3);
	`
	if err := os.WriteFile(tmp, []byte(logic), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("node", tmp).CombinedOutput(); err != nil {
		t.Fatalf("시간 계산 JS %s", out)
	}
}
