package handler

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestResolveProjectWorkDateFallback(t *testing.T) {
	if got := resolveProjectWorkDate("2026-10-01", "2026-10-21", "2026-10-23T04:50"); got != "2026-10-01" {
		t.Fatalf("form 우선 got=%s", got)
	}
	if got := resolveProjectWorkDate("", "2026-10-21", "2026-10-23T04:50"); got != "2026-10-21" {
		t.Fatalf("task 작업일 got=%s", got)
	}
	if got := resolveProjectWorkDate("", "", "2026-10-23T04:50"); got != "2026-10-23" {
		t.Fatalf("완료일 날짜 got=%s", got)
	}
	if got := resolveProjectWorkDate("", "", ""); got != "" {
		t.Fatalf("전부 비면 빈값 got=%s", got)
	}
	if !completeDatetimeBeforeWorkDate("2026-10-23T04:50", "2026-10-26") {
		t.Fatal("10-23은 10-26보다 이전이어야 함")
	}
	if completeDatetimeBeforeWorkDate("2026-10-23T04:50", "2026-10-23") {
		t.Fatal("같은 날은 이전이 아님")
	}
}

func Test95_56_RenderedFormPostWithoutWorkDate(t *testing.T) {
	e, repo, _ := newProjectWorkServer(t)
	id := pwCreateTask(t, e, repo, "폼재현작업")
	page := doGet(t, e, "/workboard/tasks/"+id+"/action")
	if page.Code != http.StatusOK {
		t.Fatalf("GET %d", page.Code)
	}
	body := page.Body.String()
	formHTML := extractHTMLForm(body, "as-action-form")
	if formHTML == "" {
		t.Fatal("as-action-form 없음")
	}
	names := htmlControlNames(formHTML)
	if _, ok := names["work_date"]; ok {
		t.Fatal("조치 폼에 work_date 가 다시 생겼다")
	}
	form := htmlFormValues(formHTML)
	t.Logf("REPRO_BEFORE form_has_work_date=false names=%v posted_keys=%v", nameList(names), formKeys(form))
	legacy := strings.TrimSpace(form.Get("work_date"))
	if legacy == "" {
		t.Logf("REPRO_BEFORE 수정 전 검사: FormValue(work_date)=%q → pwActionErr{work_date} → Location=/workboard/tasks/%s/action?err=work_date", legacy, id)
	}
	if form.Get("work_place") == "" {
		form.Set("work_place", "field")
	}
	if form.Get("action_taken") == "" {
		form.Set("action_taken", "현장 배선 정리 및 점검 완료함")
	}
	if form.Get("result_code") == "" {
		form.Set("result_code", model.WorkResultDone)
	}
	form.Set("action_short_ok", "1")
	if form.Get("work_date") != "" {
		t.Fatal("재현 POST에 work_date를 넣지 않아야 함")
	}
	rec := doForm(t, e, "/workboard/tasks/"+id+"/action", form)
	t.Logf("REPRO_AFTER status=%d loc=%s", rec.Code, rec.Header().Get("Location"))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("렌더 폼 POST 실패 code=%d loc=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	if strings.Contains(rec.Header().Get("Location"), "err=work_date") {
		t.Fatalf("수정 후에도 err=work_date loc=%s", rec.Header().Get("Location"))
	}
	tk, err := repo.GetTask(id)
	if err != nil {
		t.Fatal(err)
	}
	_ = repo.LoadTaskActionFields(tk)
	if tk.Status != model.WBTaskComplete || tk.WorkDate != "2026-10-21" {
		t.Fatalf("저장 후 status=%s work_date=%s", tk.Status, tk.WorkDate)
	}
}

func Test95_56_ActionFormKeysMatchServer(t *testing.T) {
	e, repo, _ := newProjectWorkServer(t)
	id := pwCreateTask(t, e, repo, "키일치작업")
	page := doGet(t, e, "/workboard/tasks/"+id+"/action")
	if page.Code != http.StatusOK {
		t.Fatalf("GET %d", page.Code)
	}
	names := htmlControlNames(extractHTMLForm(page.Body.String(), "as-action-form"))
	required := []string{"work_place", "work_subtype", "result_code", "action_taken", "assigned_to_code", "complete_datetime"}
	var missing []string
	for _, k := range required {
		if _, ok := names[k]; !ok {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("작업 조치 폼에 서버 필수 키 없음: %v names=%v", missing, nameList(names))
	}
	if _, ok := names["work_date"]; ok {
		t.Fatal("작업 조치 폼에 work_date 가 있으면 안 됨(표시만)")
	}

	asE, _, _, _, asID := newASActionFixture(t)
	asPage := doGet(t, asE, "/as/"+asID+"/action")
	if asPage.Code != http.StatusOK {
		t.Fatalf("AS GET %d body=%s", asPage.Code, asPage.Body.String())
	}
	asNames := htmlControlNames(extractHTMLForm(asPage.Body.String(), "as-action-form"))
	if len(asNames) == 0 {
		asNames = htmlControlNames(asPage.Body.String())
	}
	asRequired := []string{"work_place", "process_type", "action_taken", "result_code"}
	for _, k := range asRequired {
		if _, ok := asNames[k]; !ok {
			t.Fatalf("AS 조치 폼에 %s 없음 names=%v", k, nameList(asNames))
		}
	}
	if _, ok := asNames["assignee_multi"]; !ok {
		if _, ok2 := asNames["assigned_to_code"]; !ok2 {
			t.Fatalf("AS 담당자 키(assignee_multi|assigned_to_code) 없음 names=%v", nameList(asNames))
		}
	}
	if _, ok := asNames["work_date"]; ok {
		t.Fatal("AS 조치 메인 폼에 work_date 필수처럼 있으면 안 됨")
	}

	mntE, db, _ := newMnt47Server(t)
	seedMnt47Visit(t, db, "v56k")
	mntPage := doMnt47Get(t, mntE, "/maintenance/visits/v56k/action")
	if mntPage.Code != http.StatusOK {
		t.Fatalf("정기점검 GET %d", mntPage.Code)
	}
	mntNames := htmlControlNames(mntPage.Body.String())
	mntRequired := []string{"visit_date", "start_time", "end_time", "status", "notes"}
	for _, k := range mntRequired {
		if _, ok := mntNames[k]; !ok {
			t.Fatalf("정기점검 조치 폼에 %s 없음 names=%v", k, nameList(mntNames))
		}
	}
	if _, ok := mntNames["assignee_multi"]; !ok {
		if _, ok2 := mntNames["assignee"]; !ok2 {
			t.Fatalf("정기점검 담당자 키 없음 names=%v", nameList(mntNames))
		}
	}
}

func Test95_56_CompleteDatetimeBeforeWorkDateConfirm(t *testing.T) {
	e, repo, _ := newProjectWorkServer(t)
	pwSlot++
	cr := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"이전완료일시"}, "work_subtype": {"other"},
		"work_date": {"2026-10-26"}, "assignee": {"관리자"},
		"start_time": {"16:00"}, "end_time": {"16:30"},
	})
	if cr.Code != http.StatusSeeOther {
		t.Fatalf("create %d %s", cr.Code, cr.Body.String())
	}
	id := pwFindTitle(t, repo, "이전완료일시")
	ask := pwActionForm()
	ask.Set("complete_datetime", "2026-10-23T04:50")
	ask.Set("work_place", "office")
	rec := doForm(t, e, "/workboard/tasks/"+id+"/action", ask)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "완료일시가 작업일(2026-10-26)보다 이전입니다") {
		t.Fatalf("확인 안내 없음 code=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `value="2026-10-23T04:50"`) {
		t.Fatal("확인 안내 시 완료일시가 유지되지 않음")
	}
	if !strings.Contains(rec.Body.String(), `name="confirm_complete_before_work"`) {
		t.Fatal("확인 후 저장 히든 없음")
	}
	tk, _ := repo.GetTask(id)
	_ = repo.LoadTaskActionFields(tk)
	if tk.Status == model.WBTaskComplete {
		t.Fatal("확인 전에 완료 저장됨")
	}

	ask.Set("confirm_complete_before_work", "1")
	ok := doForm(t, e, "/workboard/tasks/"+id+"/action", ask)
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("확인 후 저장 %d loc=%s body=%s", ok.Code, ok.Header().Get("Location"), ok.Body.String())
	}
	tk, _ = repo.GetTask(id)
	_ = repo.LoadTaskActionFields(tk)
	if tk.CompleteDatetime != "2026-10-23T04:50" {
		t.Fatalf("완료일시 그대로 저장되지 않음 %q", tk.CompleteDatetime)
	}
	if tk.WorkDate != "2026-10-26" {
		t.Fatalf("작업일이 바뀜 %s", tk.WorkDate)
	}
}

func Test95_56_ErrorRedisplayKeepsInput(t *testing.T) {
	e, repo, _ := newProjectWorkServer(t)
	id := pwCreateTask(t, e, repo, "오류유지")
	form := url.Values{
		"work_subtype":      {"build"},
		"action_taken":      {"입력한 조치 초안입니다"},
		"complete_datetime": {"2026-10-21T16:40"},
		"result_code":       {model.WorkResultDone},
		"customer_confirmer_custom": {"박선생"},
	}
	rec := doForm(t, e, "/workboard/tasks/"+id+"/action", form)
	if rec.Code != http.StatusOK {
		t.Fatalf("근무구분 오류 code=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "근무 구분을 선택하세요") || !strings.Contains(body, `id="pw-action-err-bar"`) {
		t.Fatal("저장 바 위 오류 박스 없음")
	}
	if !strings.Contains(body, "입력한 조치 초안입니다") {
		t.Fatal("조치내용 미유지")
	}
	if !strings.Contains(body, `value="2026-10-21T16:40"`) {
		t.Fatal("완료일시 미유지")
	}
	if !strings.Contains(body, `value="build" selected`) {
		t.Fatal("작업 구분 미유지")
	}
}

func Test95_56_NAReasonPartsTransferAndDefaultComplete(t *testing.T) {
	e, repo, _ := newProjectWorkServer(t)
	id := pwCreateTask(t, e, repo, "해당없음작업")
	emptyNA := pwActionForm()
	emptyNA.Set("action_na", "1")
	emptyNA.Del("action_taken")
	emptyNA.Set("action_taken", "")
	bad := doForm(t, e, "/workboard/tasks/"+id+"/action", emptyNA)
	if bad.Code != http.StatusOK || !strings.Contains(bad.Body.String(), "해당 없음 사유를 입력하세요") {
		t.Fatalf("해당없음 사유 code=%d", bad.Code)
	}

	okNA := pwActionForm()
	okNA.Set("action_na", "1")
	okNA.Set("action_na_reason", "현장 출입 불가")
	okNA.Set("action_taken", "")
	if rec := doForm(t, e, "/workboard/tasks/"+id+"/action", okNA); rec.Code != http.StatusSeeOther {
		t.Fatalf("해당없음 저장 %d %s", rec.Code, rec.Body.String())
	}

	id2 := pwCreateTask(t, e, repo, "기본완료일시")
	def := pwActionForm()
	def.Del("complete_datetime")
	if rec := doForm(t, e, "/workboard/tasks/"+id2+"/action", def); rec.Code != http.StatusSeeOther {
		t.Fatalf("기본 완료일시 %d loc=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	tk, _ := repo.GetTask(id2)
	_ = repo.LoadTaskActionFields(tk)
	if strings.TrimSpace(tk.CompleteDatetime) == "" {
		t.Fatal("빈 완료일시가 현재 시각으로 안 채워짐")
	}

	id3 := pwCreateTask(t, e, repo, "이관본문")
	tr := pwActionForm()
	tr.Set("result_code", model.WorkResultTransfer)
	tr.Set("transfer_to", "협력사B")
	tr.Set("transfer_reason", "야간 당직 이관")
	if rec := doForm(t, e, "/workboard/tasks/"+id3+"/action", tr); rec.Code != http.StatusSeeOther {
		t.Fatalf("이관 %d %s", rec.Code, rec.Body.String())
	}

	id4 := pwCreateTask(t, e, repo, "자재포함")
	parts := pwActionForm()
	parts.Set("result_code", model.WorkResultPartial)
	parts.Set("followup_note", "자재 사용")
	parts.Set("parts_form", "1")
	parts.Set("parts_enabled", "1")
	parts.Add("part_name", "커넥터")
	parts.Add("part_code", "K01")
	parts.Add("part_qty", "3")
	parts.Add("part_item_id", "")
	if rec := doForm(t, e, "/workboard/tasks/"+id4+"/action", parts); rec.Code != http.StatusSeeOther {
		t.Fatalf("자재 %d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	tk4, _ := repo.GetTask(id4)
	rows, err := repo.ListWorkTaskParts(tk4.OrgID, id4)
	if err != nil || len(rows) != 1 || rows[0].PartName != "커넥터" {
		t.Fatalf("자재 %+v err=%v", rows, err)
	}
}

func Test95_56_MediaOrgAndLockedUnlock(t *testing.T) {
	e, repo, users, db := newProjectWorkHarness(t)
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order)
		VALUES('O02','O02','미디어','business',1,20)`); err != nil {
		t.Fatal(err)
	}
	oa02 := &model.User{Username: "oa02-56", PasswordHash: HashPassword("password10"), FullName: "미디어관리자", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O02", ProfileDone: true}
	tech := &model.User{Username: "md56", PasswordHash: HashPassword("password10"), FullName: "미디어담당", Role: model.RoleTech, IsActive: true, OrgID: "O02", ProfileDone: true}
	if err := users.Create(oa02); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(tech); err != nil {
		t.Fatal(err)
	}
	oa02, _ = users.GetByUsername("oa02-56")
	tech, _ = users.GetByUsername("md56")
	cr := doFormUser(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"미디어작업56"}, "work_subtype": {"other"},
		"work_date": {"2026-10-21"}, "assignee": {oa02.FullName},
		"start_time": {"14:00"}, "end_time": {"14:30"},
	}, jwtCookieUser(t, oa02))
	if cr.Code != http.StatusSeeOther {
		t.Fatalf("미디어 작업 생성 %d %s", cr.Code, cr.Body.String())
	}
	id := pwFindTitle(t, repo, "미디어작업56")
	ok := doFormUser(t, e, "/workboard/tasks/"+id+"/action", pwActionForm(), jwtCookieUser(t, oa02))
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("미디어 조직관리자 저장 %d loc=%s body=%s", ok.Code, ok.Header().Get("Location"), ok.Body.String())
	}
	cr2 := doFormUser(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"미디어담당작업56"}, "work_subtype": {"other"},
		"work_date": {"2026-10-21"}, "assignee": {tech.FullName},
		"start_time": {"15:00"}, "end_time": {"15:30"},
	}, jwtCookieUser(t, oa02))
	if cr2.Code != http.StatusSeeOther {
		t.Fatalf("미디어 담당 작업 생성 %d", cr2.Code)
	}
	id2 := pwFindTitle(t, repo, "미디어담당작업56")
	techOK := doFormUser(t, e, "/workboard/tasks/"+id2+"/action", pwActionForm(), jwtCookieUser(t, tech))
	if techOK.Code != http.StatusSeeOther {
		t.Fatalf("미디어 담당자 저장 %d loc=%s body=%s", techOK.Code, techOK.Header().Get("Location"), techOK.Body.String())
	}

	libID := pwCreateTask(t, e, repo, "잠금대상")
	if rec := doForm(t, e, "/workboard/tasks/"+libID+"/action", pwActionForm()); rec.Code != http.StatusSeeOther {
		t.Fatalf("완료 %d", rec.Code)
	}
	again := doForm(t, e, "/workboard/tasks/"+libID+"/action", pwActionForm())
	if again.Code != http.StatusOK || !strings.Contains(again.Body.String(), "읽기 전용") {
		t.Fatalf("잠금 code=%d loc=%s", again.Code, again.Header().Get("Location"))
	}
	unlock := doForm(t, e, "/workboard/tasks/"+libID+"/action/unlock-edit", url.Values{"unlock_password": {"nope"}})
	if unlock.Code != http.StatusSeeOther || !strings.Contains(unlock.Header().Get("Location"), "err=") {
		t.Fatalf("잘못된 잠금해제 loc=%s code=%d", unlock.Header().Get("Location"), unlock.Code)
	}
}

func extractHTMLForm(body, id string) string {
	needle := `id="` + id + `"`
	i := strings.Index(body, needle)
	if i < 0 {
		return ""
	}
	start := strings.LastIndex(body[:i], "<form")
	if start < 0 {
		return ""
	}
	rest := body[start:]
	end := strings.Index(strings.ToLower(rest), "</form>")
	if end < 0 {
		return rest
	}
	return rest[:end+7]
}

func htmlControlNames(html string) map[string]struct{} {
	out := map[string]struct{}{}
	re := regexp.MustCompile(`(?is)<(?:input|select|textarea)\b[^>]*\bname="([^"]+)"`)
	for _, m := range re.FindAllStringSubmatch(html, -1) {
		if n := strings.TrimSpace(m[1]); n != "" {
			out[n] = struct{}{}
		}
	}
	return out
}

func htmlFormValues(formHTML string) url.Values {
	vals := url.Values{}
	inRe := regexp.MustCompile(`(?is)<input\b([^>]*)/?>`)
	for _, m := range inRe.FindAllStringSubmatch(formHTML, -1) {
		attrs := htmlAttrs(m[1])
		name := attrs["name"]
		if name == "" {
			continue
		}
		typ := strings.ToLower(attrs["type"])
		if typ == "submit" || typ == "button" || typ == "image" {
			continue
		}
		if typ == "checkbox" || typ == "radio" {
			if _, ok := attrs["checked"]; !ok {
				continue
			}
		}
		vals.Add(name, attrs["value"])
	}
	taRe := regexp.MustCompile(`(?is)<textarea\b([^>]*)>(.*?)</textarea>`)
	for _, m := range taRe.FindAllStringSubmatch(formHTML, -1) {
		attrs := htmlAttrs(m[1])
		name := attrs["name"]
		if name == "" {
			continue
		}
		vals.Set(name, strings.TrimSpace(m[2]))
	}
	selRe := regexp.MustCompile(`(?is)<select\b([^>]*)>(.*?)</select>`)
	optRe := regexp.MustCompile(`(?is)<option\b([^>]*)>(.*?)</option>`)
	for _, m := range selRe.FindAllStringSubmatch(formHTML, -1) {
		attrs := htmlAttrs(m[1])
		name := attrs["name"]
		if name == "" {
			continue
		}
		chosen := ""
		first := ""
		for i, o := range optRe.FindAllStringSubmatch(m[2], -1) {
			oa := htmlAttrs(o[1])
			v := oa["value"]
			if i == 0 {
				first = v
			}
			if _, ok := oa["selected"]; ok {
				chosen = v
			}
		}
		if chosen == "" {
			chosen = first
		}
		vals.Set(name, chosen)
	}
	return vals
}

func htmlAttrs(raw string) map[string]string {
	out := map[string]string{}
	re := regexp.MustCompile(`(?i)([a-zA-Z_:][-a-zA-Z0-9:._]*)(?:\s*=\s*"([^"]*)"|\s*=\s*'([^']*)'|\s*=\s*([^\s>]+))?`)
	for _, m := range re.FindAllStringSubmatch(raw, -1) {
		k := strings.ToLower(m[1])
		v := m[2]
		if v == "" {
			v = m[3]
		}
		if v == "" {
			v = m[4]
		}
		if v == "" {
			v = k
		}
		out[k] = v
	}
	return out
}

func nameList(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func formKeys(v url.Values) []string {
	out := make([]string, 0, len(v))
	for k := range v {
		out = append(out, k)
	}
	return out
}
