package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_85_ASActionDropsWorkListAndSegments(t *testing.T) {
	e, h, _, _, asID := newASActionFixture(t)
	body := getASActionPage(t, e, asID)
	if strings.Contains(body, `name="seg_start"`) || strings.Contains(body, "data-work-items") || strings.Contains(body, `name="item_title"`) {
		t.Fatal("AS 조치에 구간·작업 리스트가 있다")
	}
	wa := httptest.NewRecorder()
	w := &model.ASWorkItem{ASID: asID, WorkKind: model.WorkKindConfirm, AssignedTo: "관리자", ScheduledDate: time.Now().Format("2006-01-02"), Status: "open"}
	if err := h.AS.workRepo.Create(w); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/work/"+w.WorkID+"/action", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(wa, req)
	if wa.Code == http.StatusOK {
		b := wa.Body.String()
		if strings.Contains(b, `name="seg_start"`) || strings.Contains(b, "data-work-items") {
			t.Fatal("하부 조치에 구간·리스트가 있다")
		}
	}
}

func Test95_85_SetRenderAndResultSelect(t *testing.T) {
	e, _, _, _, asID := newASActionFixture(t)
	body := getASActionPage(t, e, asID)
	if !strings.Contains(body, "＋ 조치 추가") || !strings.Contains(body, `id="as-add-set"`) {
		t.Fatal("조치 추가 버튼 없음")
	}
	if !strings.Contains(body, "조치 1") || !strings.Contains(body, "접기 ▲") {
		t.Fatal("조치 1 펼침 헤더 없음")
	}
	if !strings.Contains(body, `name="s1_result_code"`) {
		t.Fatal("세트 결과 select 없음")
	}
	formStart := strings.Index(body, `id="as-action-form"`)
	formEnd := strings.Index(body[formStart:], "</form>")
	form := body[formStart : formStart+formEnd]
	if strings.Contains(form, `type="radio"`) && strings.Contains(form, `name="result_code"`) && strings.Contains(form, `name="result_code" value=`) {
		if strings.Contains(form, `type="radio" name="result_code"`) || strings.Contains(form, `name="result_code" type="radio"`) {
			t.Fatal("결과 라디오가 남아 있다")
		}
	}
	selStart := strings.Index(form, `id="as-result-code"`)
	if selStart < 0 {
		t.Fatal("최종 결과 select 없음")
	}
	selEnd := strings.Index(form[selStart:], "</select>")
	sel := form[selStart : selStart+selEnd]
	n := strings.Count(sel, "<option")
	if n != 4 { // 선택하세요 + 3종
		t.Fatalf("결과 옵션 %d (선택하세요+3종이어야 함) %s", n, sel)
	}
	for _, v := range []string{`value="done"`, `value="partial"`, `value="transfer"`} {
		if !strings.Contains(sel, v) {
			t.Fatalf("%s 없음", v)
		}
	}
	if strings.Contains(sel, `value="hold"`) || strings.Contains(sel, `value="temporary"`) {
		t.Fatal("비허용 결과가 선택지에 있다")
	}
	if !strings.Contains(body, "작업 시간") || !strings.Contains(body, "철수 시간 동일") {
		t.Fatal("작업 시간 없음")
	}
	if strings.Contains(body, `type="time"`) {
		t.Fatal("input type=time 사용")
	}
	if !strings.Contains(body, "오전 09:00") || !strings.Contains(body, `value="09:00"`) {
		t.Fatal("시작 드롭다운 오전/오후·24시간 value 없음")
	}
	if strings.Count(body, `value="00:00"`) < 1 || strings.Count(body, `value="23:50"`) < 1 {
		t.Fatal("시작 슬롯 경계 없음")
	}
	if !strings.Contains(body, `name="kw_manual"`) || !strings.Contains(body, "w-full") {
		t.Fatal("키워드 전체 폭 없음")
	}
	if !strings.Contains(body, "자동 키워드 후보가 없습니다.") {
		t.Fatal("빈 후보 안내 없음")
	}
}

func Test95_85_LegacyHoldShownReadonlyOption(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	got, _ := asRepo.GetByID(repository.OrgAll, asID)
	got.ResultCode = model.ResultHold
	got.ActionTaken = "대기 중 기존 값"
	if err := asRepo.Update(got); err != nil {
		t.Fatal(err)
	}
	body := getASActionPage(t, e, asID)
	if !strings.Contains(body, "대기") || !strings.Contains(body, "3종 중 하나로 바꾸세요") {
		t.Fatal("기존 비허용 값 읽기 전용 옵션 없음")
	}
	rec := postASAction(t, e, asID, url.Values{
		"work_place": {"field"}, "process_type": {"visit"}, "cause_type": {"hw"},
		"action_taken": {"그대로 저장"}, "result_code": {model.ResultHold},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("hold 저장 %d %s", rec.Code, rec.Body.String())
	}
}

func Test95_85_TempSaveNoSideEffects(t *testing.T) {
	e, h, asRepo, _, asID := newASActionFixture(t)
	before, _ := asRepo.GetByID(repository.OrgAll, asID)
	asActionSideEffectRuns = 0
	form := url.Values{
		"seq": {"1"},
		"s1_work_place": {"field"}, "s1_process_type": {"visit"}, "s1_visit_date": {"2026-08-10"},
		"s1_action_taken": {"1차 점검 완료 내용입니다"}, "s1_result_code": {model.ResultDone},
		"s1_cause_cat1": {"server"}, "s1_cause_cat2": {"server.hw"},
		"s1_work_date": {"2026-08-10"}, "s1_start_time": {"09:00"}, "s1_duration_min": {"90"},
		"s1_leave_same": {"1"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+asID+"/action-sets", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("임시 저장 %d %s", rec.Code, rec.Body.String())
	}
	after, _ := asRepo.GetByID(repository.OrgAll, asID)
	if after.Status != before.Status {
		t.Fatalf("상태 변경 %s→%s", before.Status, after.Status)
	}
	if asActionSideEffectRuns != 0 {
		t.Fatalf("부수효과 %d", asActionSideEffectRuns)
	}
	works, _ := h.AS.workRepo.ListByAS(asID)
	if len(works) != 0 {
		t.Fatalf("W01 %d", len(works))
	}
	sets, _ := h.AS.setRepo.ListByAS(asID)
	if len(sets) != 1 || sets[0].State != model.ActionSetStateSaved {
		t.Fatalf("%+v", sets)
	}
	if sets[0].EndTime != "10:30" {
		t.Fatalf("완료 %s", sets[0].EndTime)
	}

	bad := url.Values{"seq": {"1"}, "s1_work_place": {"field"}, "s1_process_type": {"visit"}, "s1_result_code": {model.ResultDone}}
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+asID+"/action-sets", strings.NewReader(bad.Encode()))
	req2.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req2.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("내용 없음 %d", rec2.Code)
	}

	hold := url.Values{
		"seq": {"1"}, "s1_work_place": {"field"}, "s1_process_type": {"visit"}, "s1_visit_date": {"2026-08-10"},
		"s1_action_taken": {"대기 저장 시도 내용입니다"}, "s1_result_code": {model.ResultHold},
	}
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+asID+"/action-sets", strings.NewReader(hold.Encode()))
	req3.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req3.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusBadRequest {
		t.Fatalf("hold 400 아님 %d %s", rec3.Code, rec3.Body.String())
	}

	ro := httptest.NewRecorder()
	reqRO := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+asID+"/action-sets", strings.NewReader(form.Encode()))
	reqRO.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	reqRO.AddCookie(jwtCookieReadOnly(t, "c-level"))
	e.ServeHTTP(ro, reqRO)
	if ro.Code != http.StatusForbidden && ro.Code != http.StatusUnauthorized {
		t.Fatalf("읽기 전용 %d", ro.Code)
	}
}

func Test95_85_TempSaveOtherOrg404(t *testing.T) {
	e, _, asRepo, _, _ := newASActionFixture(t)
	other := &model.ASReceipt{
		CustomerID: "cust_a", Symptom: "타조직", Status: "in_progress",
		ReceiptDatetime: time.Now(), OrgID: "O03", AssignedTo: "관리자",
	}
	if err := asRepo.Create(other); err != nil {
		t.Fatal(err)
	}
	form := url.Values{"seq": {"1"}, "s1_action_taken": {"내용입니다 충분히"}, "s1_result_code": {model.ResultDone}, "s1_work_place": {"office"}, "s1_process_type": {"remote"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+other.ASID+"/action-sets", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, model.OrgIDLibrary))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusForbidden {
		t.Fatalf("타 조직 %d %s", rec.Code, rec.Body.String())
	}
}

func Test95_85_FinalSaveLastSetOnly(t *testing.T) {
	e, h, asRepo, _, asID := newASActionFixture(t)
	asActionSideEffectRuns = 0
	form := url.Values{
		"s1_work_place": {"field"}, "s1_process_type": {"visit"}, "s1_visit_date": {"2026-08-10"},
		"s1_action_taken": {"임시 조치 내용입니다 충분히"}, "s1_result_code": {model.ResultTransfer},
		"s1_cause_cat1": {"server"}, "s1_cause_cat2": {"server.hw"},
		"s2_work_place": {"field"}, "s2_process_type": {"visit"}, "s2_visit_date": {"2026-08-11"},
		"s2_action_taken": {"추가 조치 필요 내용입니다 충분히"}, "s2_result_code": {model.ResultPartial},
		"s2_cause_cat1": {"server"}, "s2_cause_cat2": {"server.hw"},
		"s3_work_place": {"office"}, "s3_process_type": {"remote"},
		"s3_action_taken": {"최종 원격 완료 내용입니다 충분히"}, "s3_result_code": {model.ResultDone},
		"s3_cause_cat1": {"server"}, "s3_cause_cat2": {"server.hw"},
		"s3_work_date": {"2026-08-12"}, "s3_start_time": {"13:00"}, "s3_duration_min": {"40"},
		"s3_leave_same": {"0"}, "s3_leave_time": {"14:00"},
		"work_place": {"office"}, "process_type": {"remote"}, "cause_type": {"hw"},
		"action_taken": {"최종 원격 완료 내용입니다 충분히"}, "result_code": {model.ResultDone},
		"action_short_ok": {"1"},
	}
	rec := postASAction(t, e, asID, form)
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("최종 저장 %d %s", rec.Code, rec.Header().Get("Location")+rec.Body.String())
	}
	got, _ := asRepo.GetByID(repository.OrgAll, asID)
	if got.Status != "completed" {
		t.Fatalf("상태 %s", got.Status)
	}
	if got.ActionTaken != "최종 원격 완료 내용입니다 충분히" {
		t.Fatalf("본문 %s", got.ActionTaken)
	}
	if got.ResultCode != model.ResultDone {
		t.Fatalf("결과 %s", got.ResultCode)
	}
	if asActionSideEffectRuns != 1 {
		t.Fatalf("부수효과 %d", asActionSideEffectRuns)
	}
	sets, _ := h.AS.setRepo.ListByAS(asID)
	if len(sets) != 3 {
		t.Fatalf("세트 %d", len(sets))
	}
	if got.CompleteDatetime == nil {
		t.Fatal("완료일시 동기화 없음")
	}
	page := getASActionPage(t, e, asID)
	if !strings.Contains(page, "철수") || !strings.Contains(page, "오후 02:00") && !strings.Contains(page, "오후 14:00") {
		if !strings.Contains(page, "철수") {
			t.Fatal("철수 표시 없음")
		}
	}
}

func Test95_85_WorkTimeServerReject(t *testing.T) {
	e, _, _, _, asID := newASActionFixture(t)
	form := url.Values{
		"s1_work_place": {"field"}, "s1_process_type": {"visit"}, "s1_visit_date": {"2026-08-10"},
		"s1_action_taken": {"자정 넘김 내용입니다 충분히"}, "s1_result_code": {model.ResultDone},
		"s1_cause_cat1": {"server"}, "s1_cause_cat2": {"server.hw"},
		"s1_start_time": {"23:50"}, "s1_duration_min": {"10"},
		"work_place": {"field"}, "process_type": {"visit"}, "cause_type": {"hw"},
		"action_taken": {"자정 넘김 내용입니다 충분히"}, "result_code": {model.ResultDone},
	}
	rec := postASAction(t, e, asID, form)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "work_time_midnight") {
		t.Fatalf("자정 거부 %d %s", rec.Code, rec.Header().Get("Location"))
	}
}

func Test95_85_KeywordSuggestAndManual(t *testing.T) {
	e, _, _, _, asID := newASActionFixture(t)
	rec := httptest.NewRecorder()
	body := url.Values{"text": {"게이트를 교체 했음 완료 오류"}}
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as/keywords/suggest", strings.NewReader(body.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("suggest %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"new"`) {
		t.Fatal("new 없음")
	}
	bad := postASAction(t, e, asID, url.Values{
		"work_place": {"office"}, "process_type": {"remote"}, "cause_type": {"hw"},
		"action_taken": {"키워드 오류 저장 내용입니다"}, "result_code": {model.ResultDone},
		"kw_manual": {"나쁜(없음)"},
	})
	if bad.Code != http.StatusSeeOther || !strings.Contains(bad.Header().Get("Location"), "keyword") {
		t.Fatalf("수동 키워드 거절 %d %s", bad.Code, bad.Header().Get("Location"))
	}
}

func Test95_85_SegmentsUnchangedWithoutFields(t *testing.T) {
	e, db, h, ck, repo := new78A(t)
	day := time.Now().AddDate(0, 0, 27).Format(dateLayout)
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c85seg','도서관S','도서관S',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	as := &model.ASReceipt{
		CustomerID: "c85seg", ReceiptChannel: "phone", Requester: "홍", Symptom: "게이트",
		Urgency: "normal", Priority: "normal", AssignedTo: "관리자",
		ReceiptDatetime: time.Now(), Status: "in_progress", OrgID: model.OrgIDLibrary, VisitScheduledDate: day,
	}
	if err := h.AS.repo.Create(as); err != nil {
		t.Fatal(err)
	}
	_ = repo.SyncASDailyTask(as)
	daily, _ := repo.GetTaskBySource(model.WBSourceAS, as.ASID)
	_ = repo.ReplacePlanSegments(daily.TaskID, []model.WorkTaskSegment{
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "09:00", EndTime: "10:00"},
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "14:00", EndTime: "16:00"},
	})
	form := url.Values{
		"work_place": {"field"}, "process_type": {"visit"}, "visit_date": {day},
		"cause_type": {"hw"}, "cause_cat1": {"server"}, "cause_cat2": {"server.hw"},
		"action_taken": {"구간 유지 저장 내용입니다"}, "result_code": {model.ResultDone},
		"action_short_ok": {"1"},
	}
	ra := doFormWith(t, e, "/as/"+as.ASID+"/update", form, ck)
	if ra.Code != http.StatusSeeOther || strings.Contains(ra.Header().Get("Location"), "err=") {
		t.Fatalf("저장 %d %s", ra.Code, ra.Header().Get("Location"))
	}
	segs, _ := repo.ListPlanSegmentsOrVirtual(*mustGetTask(t, repo, daily.TaskID))
	if len(segs) != 2 {
		t.Fatalf("구간이 지워짐 %d", len(segs))
	}
}

func Test95_85_OverlapWarnNotBlock(t *testing.T) {
	sets := []model.ASActionSet{
		{Seq: 1, WorkDate: "2026-08-10", StartTime: "09:00", EndTime: "11:00"},
		{Seq: 2, WorkDate: "2026-08-10", StartTime: "10:00", EndTime: "12:00"},
	}
	out := model.ActionSetOverlapWarning(sets)
	if out[1].OverlapWarn == "" {
		t.Fatal("겹침 경고 없음")
	}
}

func Test95_85_JoinReport(t *testing.T) {
	s := []model.ASActionSet{
		{Seq: 1, ActionTaken: "A", LeaveTime: "11:00", LeaveSame: false},
		{Seq: 2, ActionTaken: "B"},
	}
	j := model.JoinActionSetsForReport(s)
	if !strings.Contains(j, "[조치 1]") || !strings.Contains(j, "[조치 2]") {
		t.Fatal(j)
	}
}
