package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func doFormJSON(t *testing.T, e *echo.Echo, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set("Accept", "application/json")
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	return rec
}

func TestSalesHTTP_ActivityLogTimelineAndPanel(t *testing.T) {
	e, db := newSalesServerDB(t)

	empty := doGet(t, e, "/sales/activities")
	if empty.Code != http.StatusOK {
		t.Fatalf("빈 로그 status=%d", empty.Code)
	}
	eb := empty.Body.String()
	if !strings.Contains(eb, "아직 등록된 활동이 없습니다") || !strings.Contains(eb, "첫 활동 등록하기") {
		t.Fatalf("빈 상태 없음: %s", clipBody(eb))
	}
	if strings.Contains(eb, "+ 활동 추가") || strings.Contains(eb, "사업 목록") {
		t.Fatal("빈 화면에 헤더 등록·사업 목록이 남았다")
	}
	if strings.Count(eb, `@click="startCreate()"`) != 1 {
		t.Fatalf("주황 등록이 하나가 아니다: %s", clipBody(eb))
	}
	for _, label := range []string{"방문", "전화", "이메일", "온라인미팅", "내부회의", "정보수집"} {
		if !strings.Contains(eb, label) {
			t.Fatalf("칩 라벨 없음 %s: %s", label, clipBody(eb))
		}
	}
	if strings.Contains(eb, "방문미팅") || strings.Contains(eb, ">메일<") {
		t.Fatal("옛 라벨이 칩에 남았다")
	}
	if !strings.Contains(eb, "자료송부") || !strings.Contains(eb, "견적제출") ||
		!strings.Contains(eb, "제안서제출") || !strings.Contains(eb, "입찰") {
		t.Fatalf("드롭다운 유형이 없다: %s", clipBody(eb))
	}
	if !strings.Contains(eb, "max-w-md") || !strings.Contains(eb, "활동 저장") || !strings.Contains(eb, "right-0") {
		t.Fatalf("슬라이드오버가 아니다: %s", clipBody(eb))
	}

	rec := doForm(t, e, "/sales", url.Values{"name": {"세종 RFID 증설"}, "is_tentative_name": {"1"}})
	id := salesIDFromRedirect(t, rec.Header().Get("Location"))

	rec = doForm(t, e, "/sales/"+id+"/activities", url.Values{
		"activity_date":    {"2026-08-09"},
		"start_time":       {"10:00"},
		"duration_min":     {"90"},
		"activity_type":    {"visit"},
		"title":            {"현장 방문"},
		"place":            {"세종청사"},
		"counterparts":     {"김담당"},
		"content":          {"본문 한 줄\n두 번째 줄\n세 번째는 잘린다"},
		"next_action":      {"제안서 작성 및 제출"},
		"next_action_date": {"2026-09-15"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("활동1: %d", rec.Code)
	}
	rec = doForm(t, e, "/sales/"+id+"/activities", url.Values{
		"activity_date": {"2026-08-20"},
		"start_time":    {"14:00"},
		"duration_min":  {"60"},
		"activity_type": {"internal"},
		"title":         {"내부 검토"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("활동2: %d", rec.Code)
	}

	logPage := doGet(t, e, "/sales/activities?month=2026-08")
	body := logPage.Body.String()
	if i1, i2 := strings.Index(body, "2026-08-20"), strings.Index(body, "2026-08-09"); i1 < 0 || i2 < 0 || i1 > i2 {
		t.Fatalf("날짜 그룹 최신이 위가 아니다: %s", clipBody(body))
	}
	if !strings.Contains(body, "color:#DC2626") {
		t.Fatalf("휴일 빨강이 없다: %s", clipBody(body))
	}
	if !strings.Contains(body, "2시간 30분") || !strings.Contains(body, "90분") || !strings.Contains(body, "60분") {
		t.Fatalf("요약·카드 분이 다르다: %s", clipBody(body))
	}
	if strings.Contains(body, "첫 활동 등록하기") || strings.Count(body, `@click="startCreate()"`) != 1 {
		t.Fatal("목록이 있는데 빈 상태 CTA 가 남았거나 등록 버튼이 둘이다")
	}
	if !strings.Contains(body, "+ 활동 추가") {
		t.Fatal("목록이 있는데 헤더 등록이 없다")
	}

	sepEmpty := doGet(t, e, "/sales/activities?month=2026-09")
	sepBody := sepEmpty.Body.String()
	if !strings.Contains(sepBody, "2026년 9월에 등록된 활동이 없습니다") || !strings.Contains(sepBody, "전체 기간 보기") {
		t.Fatalf("이 달만 빈 안내가 없다: %s", clipBody(sepBody))
	}
	if strings.Contains(sepBody, "아직 등록된 활동이 없습니다") || strings.Contains(sepBody, "첫 활동 등록하기") {
		t.Fatal("이 달만 비었는데 하나도 없다고 나왔다")
	}
	if !strings.Contains(sepBody, "+ 활동 추가") || strings.Count(sepBody, `@click="startCreate()"`) != 1 {
		t.Fatal("이 달 빈 화면의 등록 버튼이 하나가 아니다")
	}
	if !strings.Contains(body, "text-orange-800") || !strings.Contains(body, "다음: 제안서 작성 및 제출 · 2026-09-15") {
		t.Fatalf("다음 행동 주황 칩 없음: %s", clipBody(body))
	}
	if !strings.Contains(body, "다음 행동 없음") || !strings.Contains(body, "text-gray-500") {
		t.Fatalf("다음 행동 없음이 숨겨졌다: %s", clipBody(body))
	}

	past := doForm(t, e, "/sales/"+id+"/activities", url.Values{
		"activity_date":    {"2026-08-21"},
		"duration_min":     {"30"},
		"activity_type":    {"call"},
		"title":            {"지난 다음행동"},
		"next_action":      {"재통화"},
		"next_action_date": {"2026-08-01"},
	})
	if past.Code != http.StatusSeeOther {
		t.Fatalf("활동3: %d", past.Code)
	}
	logPage = doGet(t, e, "/sales/activities?month=2026-08")
	if !strings.Contains(logPage.Body.String(), "text-red-700") {
		t.Fatalf("지난 다음 행동이 빨강이 아니다: %s", clipBody(logPage.Body.String()))
	}

	visitOnly := doGet(t, e, "/sales/activities?month=2026-08&type=visit")
	vb := visitOnly.Body.String()
	if !strings.Contains(vb, "1시간 30분") || strings.Contains(vb, "내부 검토") {
		t.Fatalf("유형 칩 필터가 요약을 안 바꾼다: %s", clipBody(vb))
	}

	show := doGet(t, e, "/sales/"+id)
	sb := show.Body.String()
	if strings.Contains(sb, `href="/sales/`+id+`"`) && strings.Count(sb, "세종 RFID 증설") < 1 {
		t.Fatal("상세에 사업명이 없다")
	}
	if strings.Contains(sb, `@click="tab='changes'"`) {
		t.Fatal("상세에서 변경 이력을 탭으로 나눴다")
	}
	xData, panel, script := strings.Index(sb, `salesActPanel(`), strings.Index(sb, `typeof open === 'boolean' && open`), strings.Index(sb, `function salesActPanel`)
	if xData < 0 || panel < 0 || script < 0 || !(xData < panel && panel < script) {
		t.Fatal("활동 패널이 salesActPanel 스코프 밖에 있다")
	}
	if !strings.Contains(sb, `@keydown.escape.window="if (typeof open === 'boolean') open=false"`) {
		t.Fatal("Esc 닫기가 없다")
	}

	js := doFormJSON(t, e, "/sales/activities", url.Values{
		"sales_id":      {id},
		"activity_date": {"2026-08-22"},
		"duration_min":  {"45"},
		"activity_type": {"mail"},
		"title":         {"메일 회신"},
	})
	if js.Code != http.StatusOK {
		t.Fatalf("JSON 저장 status=%d body=%s", js.Code, js.Body.String())
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(js.Body.Bytes(), &payload); err != nil || payload["ok"] != true {
		t.Fatalf("JSON=%s", js.Body.String())
	}

	task, err := repository.NewWBRepo(db).GetTaskBySource(model.WBSourceSalesActivity, payload["activity_id"].(string))
	if err != nil || task == nil {
		t.Fatalf("일일 업무 미생성: %v", err)
	}
	if task.DurationMin != 45 {
		t.Fatalf("소요 시간 불일치 duration=%d", task.DurationMin)
	}
	reg := doGet(t, e, "/workboard/register?view=day&date=2026-08-22")
	if !strings.Contains(reg.Body.String(), "메일 회신") {
		t.Fatalf("일정표에 활동 없음: %s", clipBody(reg.Body.String()))
	}
}

func TestSalesHTTP_ActivityTypesSeeded(t *testing.T) {
	_, db := newSalesServerDB(t)
	codes := repository.NewCodeRepo(db)
	list, err := codes.ActiveByGroup(model.SalesCodeGroupActivityType)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, c := range list {
		got[c.CodeValue] = c.CodeName
	}
	if got["visit"] != "방문" || got["mail"] != "이메일" || got["internal"] != "내부회의" {
		t.Fatalf("유형 라벨 %+v", got)
	}
	if got["quote"] != "견적제출" || got["proposal"] != "제안서제출" || got["bid"] != "입찰" {
		t.Fatalf("진척 열쇠 유형이 없다 %+v", got)
	}
}

func TestSalesHTTP_UpdateActivitySyncsWorkTask(t *testing.T) {
	e, db := newSalesServerDB(t)
	rec := doForm(t, e, "/sales", url.Values{"name": {"수정 연동"}, "is_tentative_name": {"1"}})
	id := salesIDFromRedirect(t, rec.Header().Get("Location"))
	js := doFormJSON(t, e, "/sales/activities", url.Values{
		"sales_id": {id}, "activity_date": {"2026-08-22"}, "start_time": {"10:00"},
		"duration_min": {"45"}, "activity_type": {"mail"}, "title": {"메일 회신"},
		"counterparts": {"김담당"},
	})
	if js.Code != http.StatusOK {
		t.Fatalf("등록 status=%d body=%s", js.Code, js.Body.String())
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(js.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	aid, _ := payload["activity_id"].(string)
	if aid == "" {
		t.Fatal("activity_id 없음")
	}

	page := doGet(t, e, "/sales/activities?month=2026-08")
	body := page.Body.String()
	if !strings.Contains(body, ">수정</button>") || !strings.Contains(body, ">삭제</button>") {
		t.Fatal("수정·삭제 버튼이 없다")
	}
	if !strings.Contains(body, "되돌릴 수 없습니다") {
		t.Fatal("삭제 확인 문구가 없다")
	}

	upd := doFormJSON(t, e, "/sales/activities/"+aid, url.Values{
		"activity_date": {"2026-08-25"}, "start_time": {"14:00"},
		"duration_min": {"90"}, "activity_type": {"visit"}, "title": {"방문으로 변경"},
		"counterparts": {"김담당"},
	})
	if upd.Code != http.StatusOK {
		t.Fatalf("수정 status=%d body=%s", upd.Code, upd.Body.String())
	}
	wb := repository.NewWBRepo(db)
	task, err := wb.GetTaskBySource(model.WBSourceSalesActivity, aid)
	if err != nil || task == nil {
		t.Fatalf("일정표 동기화 실패: %v", err)
	}
	if task.Title != "방문으로 변경" || task.WorkDate != "2026-08-25" || task.DurationMin != 90 {
		t.Fatalf("work_tasks 미반영 title=%q date=%s dur=%d", task.Title, task.WorkDate, task.DurationMin)
	}
	reg := doGet(t, e, "/workboard/register?view=day&date=2026-08-25")
	if !strings.Contains(reg.Body.String(), "방문으로 변경") {
		t.Fatalf("바꾼 날이 일정표에 없다: %s", clipBody(reg.Body.String()))
	}
	old := doGet(t, e, "/workboard/register?view=day&date=2026-08-22")
	if strings.Contains(old.Body.String(), "방문으로 변경") || strings.Contains(old.Body.String(), "메일 회신") {
		t.Fatal("옛 날짜 일정표에 유령이 남았다")
	}
}

func TestSalesHTTP_DeleteActivityRemovesWorkTaskKeepsAutoParty(t *testing.T) {
	e, db := newSalesServerDB(t)
	rec := doForm(t, e, "/sales", url.Values{"name": {"삭제 연동"}, "is_tentative_name": {"1"}, "prospect_name": {"세종시립도서관"}})
	id := salesIDFromRedirect(t, rec.Header().Get("Location"))
	js := doFormJSON(t, e, "/sales/activities", url.Values{
		"sales_id": {id}, "activity_date": {"2026-08-22"},
		"duration_min": {"30"}, "activity_type": {"visit"}, "title": {"지울 방문"},
		"counterparts": {"박상대"},
	})
	var payload map[string]interface{}
	if err := json.Unmarshal(js.Body.Bytes(), &payload); err != nil || payload["ok"] != true {
		t.Fatalf("등록: %s", js.Body.String())
	}
	aid, _ := payload["activity_id"].(string)
	sales := repository.NewSalesRepo(db)
	before, err := sales.ListParties(id, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.CustomerPartyHintNames(before)) == 0 {
		t.Fatal("자동 관계자가 안 생겼다")
	}

	del := httptestPostAs(t, e, "/sales/activities/"+aid+"/delete", url.Values{}, "admin")
	if del.Code != http.StatusSeeOther && del.Code != http.StatusOK {
		t.Fatalf("삭제 status=%d body=%s", del.Code, del.Body.String())
	}
	got, err := sales.GetActivity(aid)
	if err == nil && got != nil {
		t.Fatal("활동이 남아 있다")
	}
	task, err := repository.NewWBRepo(db).GetTaskBySource(model.WBSourceSalesActivity, aid)
	if err != nil {
		t.Fatal(err)
	}
	if task != nil {
		t.Fatal("일정표 유령이 남았다")
	}
	after, _ := sales.ListParties(id, true)
	if len(model.CustomerPartyHintNames(after)) == 0 {
		t.Fatal("자동 생성 관계자가 지워졌다")
	}
	var auto bool
	for _, p := range after {
		if p.PersonName == "박상대" && p.IsAuto {
			auto = true
		}
	}
	if !auto {
		t.Fatalf("is_auto 관계자가 없다: %+v", after)
	}
	reg := doGet(t, e, "/workboard/register?view=day&date=2026-08-22")
	if strings.Contains(reg.Body.String(), "지울 방문") {
		t.Fatal("삭제한 활동이 일정표에 남았다")
	}
}

func TestSalesHTTP_ForeignActivityMutationsForbidden(t *testing.T) {
	e, db := newSalesServerDB(t)
	rec := doForm(t, e, "/sales", url.Values{"name": {"남의 활동"}, "is_tentative_name": {"1"}})
	id := salesIDFromRedirect(t, rec.Header().Get("Location"))
	js := doFormJSON(t, e, "/sales/activities", url.Values{
		"sales_id": {id}, "activity_date": {"2026-08-22"},
		"duration_min": {"30"}, "activity_type": {"call"}, "title": {"관리자 통화"},
	})
	var payload map[string]interface{}
	if err := json.Unmarshal(js.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	aid, _ := payload["activity_id"].(string)
	upd := httptestPostAs(t, e, "/sales/activities/"+aid, url.Values{
		"activity_date": {"2026-08-23"}, "duration_min": {"60"},
		"activity_type": {"call"}, "title": {"가로채기"},
	}, "sales")
	if upd.Code != http.StatusForbidden {
		t.Fatalf("남의 수정 status=%d want 403", upd.Code)
	}
	del := httptestPostAs(t, e, "/sales/activities/"+aid+"/delete", url.Values{}, "sales")
	if del.Code != http.StatusForbidden {
		t.Fatalf("남의 삭제 status=%d want 403", del.Code)
	}
	got, err := repository.NewSalesRepo(db).GetActivity(aid)
	if err != nil || got == nil || got.Title != "관리자 통화" {
		t.Fatalf("403 인데 내용이 바뀌었다: %+v err=%v", got, err)
	}
}
