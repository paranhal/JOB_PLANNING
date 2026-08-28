package handler

import (
	"bytes"
	"html/template"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.ParseInLocation(dateLayout, s, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestFilterByAssignee(t *testing.T) {
	tasks := []model.WorkTask{
		{TaskID: "1", Assignee: "김기술"},
		{TaskID: "2", Assignee: "이접수"},
		{TaskID: "3", Assignee: ""},
	}
	if got := filterTasksByAssignee(tasks, ""); len(got) != 3 {
		t.Fatalf("팀전체: %d", len(got))
	}
	got := filterTasksByAssignee(tasks, "김기술")
	if len(got) != 1 || got[0].TaskID != "1" {
		t.Fatalf("담당자 필터: %+v", got)
	}
	if normalizeAssigneeFilter("팀전체") != "" || normalizeAssigneeFilter("all") != "" {
		t.Fatal("팀전체 정규화")
	}
	if !strings.Contains(assigneeQuerySuffix("김기술"), "assignee=") {
		t.Fatal("쿼리 접미사")
	}
}

// 일일·주간·월간 시간표의 열 구성 (2026-08-04는 화요일)
func TestBuildRegisterPeriodColumns(t *testing.T) {
	base := mustDate(t, "2026-08-04")

	day := buildRegisterPeriod(regViewDay, base, "2026-08-04")
	if len(day.Columns) != 1 || day.Columns[0].Date != "2026-08-04" || !day.Columns[0].IsToday {
		t.Fatalf("일간 열: %+v", day.Columns)
	}

	week := buildRegisterPeriod(regViewWeek, base, "2026-08-04")
	if len(week.Columns) != 7 {
		t.Fatalf("주간 열 개수 = %d, want 7", len(week.Columns))
	}
	if week.From != "2026-08-03" || week.To != "2026-08-09" {
		t.Fatalf("주간 기간 = %s~%s, want 2026-08-03~2026-08-09", week.From, week.To)
	}
	if week.Columns[0].Label != "월" || week.Columns[6].Label != "일" {
		t.Fatalf("주간 요일: %s ~ %s", week.Columns[0].Label, week.Columns[6].Label)
	}

	month := buildRegisterPeriod(regViewMonth, base, "2026-08-04")
	if month.From != "2026-08-01" || month.To != "2026-08-31" {
		t.Fatalf("월간 기간 = %s~%s", month.From, month.To)
	}
	if len(month.Columns) != 0 {
		t.Fatalf("월간은 캘린더(MonthWeeks) 사용 — Columns 비어야 함: %d", len(month.Columns))
	}
	weeks := buildRegisterMonthWeeks(base, "2026-08-04", []model.WorkTask{
		{TaskID: "T1", Title: "[AS]테스트", WorkDate: "2026-08-07", StartTime: "09:00", Assignee: "양기헌"},
		{TaskID: "T2", Title: "[점검]일산", WorkDate: "2026-08-07", StartTime: "10:00"},
	}, taskCardFromWork)
	if len(weeks) < 5 {
		t.Fatalf("월간 주 수=%d", len(weeks))
	}
	if len(weeks[0]) != 7 {
		t.Fatalf("한 주 7칸: %d", len(weeks[0]))
	}
	found := false
	for _, w := range weeks {
		for _, d := range w {
			if d.Date == "2026-08-07" {
				found = true
				if d.Total != 2 || d.Unassigned != 1 || d.IsToday {
					t.Fatalf("8/7 day: %+v", d)
				}
			}
		}
	}
	if !found {
		t.Fatal("2026-08-07 칸 없음")
	}
}

// 배치된 업무는 해당 열·시각 칸에만 들어가야 한다.
func TestBuildRegisterRowsPlacesCards(t *testing.T) {
	period := buildRegisterPeriod(regViewWeek, mustDate(t, "2026-08-04"), "")
	tasks := []model.WorkTask{
		{TaskID: "WT-1", Title: "행정업무", WorkDate: "2026-08-04", StartTime: "09:00"},
		{TaskID: "WT-2", Title: "AS방문", WorkDate: "2026-08-06", StartTime: "13:30", SourceType: model.WBSourceAS},
	}

	rows := buildRegisterRows(period.Columns, tasks)
	if rows[0].Time != "07:00" || rows[len(rows)-1].Time != "20:00" {
		t.Fatalf("시간 범위 = %s ~ %s", rows[0].Time, rows[len(rows)-1].Time)
	}

	found := map[string]string{}
	for _, row := range rows {
		for i, cell := range row.Cells {
			for _, card := range cell.Cards {
				found[card.RefID] = period.Columns[i].Date + " " + row.Time
				if card.RefID == "WT-2" && card.Category != model.WBSourceAS {
					t.Errorf("AS 카드 분류 = %s", card.Category)
				}
			}
		}
	}
	if found["WT-1"] != "2026-08-04 09:00" {
		t.Errorf("WT-1 위치 = %q", found["WT-1"])
	}
	if found["WT-2"] != "2026-08-06 13:30" {
		t.Errorf("WT-2 위치 = %q", found["WT-2"])
	}
}

// 파싱만으로는 실행 중 오류(함수 인자 타입 등)를 잡지 못해 실제 데이터로 렌더링한다.
func TestRegisterTemplateRenders(t *testing.T) {
	root := findTemplateRoot(t)
	files := []string{
		filepath.Join(root, "layout", "base.html"),
		filepath.Join(root, "workboard", "register.html"),
	}
	partials, err := filepath.Glob(filepath.Join(root, "workboard", "_*.html"))
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, partials...)

	tmpl, err := template.New("").Funcs(funcMap()).ParseFiles(files...)
	if err != nil {
		t.Fatal(err)
	}

	period := buildRegisterPeriod(regViewWeek, mustDate(t, "2026-08-04"), "2026-08-04")
	cards := []model.WBCard{{
		Kind: model.WBSourceAS, RefID: "R2607-001", Category: model.WBSourceAS,
		Title: "테스트도서관", SubTitle: `게이트 "오류" & 점검`, PlannedDay: "2026-08-05",
	}}

	placed := []model.WorkTask{
		{TaskID: "WT-1", Title: "행정업무", Assignee: "김기술", WorkDate: "2026-08-04", StartTime: "09:00", EndTime: "09:30", DurationMin: 30},
	}
	slotTimes := registerSlotTimes()
	gridH, slotTops := registerGridStyles(len(slotTimes))
	data := map[string]interface{}{
		"Title": "일일 업무 등록", "Active": NavWorkRegister, "UserRole": "admin",
		"View": regViewWeek, "ViewLabel": registerViewLabel(regViewWeek),
		"Date": "2026-08-04", "PeriodLabel": period.Label,
		"PrevDate": period.Prev, "NextDate": period.Next, "Today": "2026-08-04",
		"Columns":    period.Columns,
		"DayColumns": buildRegisterDayColumns(period.Columns, placed, taskCardFromWork),
		"SlotTimes":  slotTimes, "GridHeightStyle": gridH, "SlotTopStyles": slotTops,
		"Rows":           buildRegisterRows(period.Columns, nil),
		"AssigneeLegend": buildAssigneeLegend(placed, nil),
		"ASCards":        cards, "MntCards": []model.WBCard{}, "AdminCards": []model.WBCard(nil),
		"PlacedCount": 1, "Projects": nil, "Assignees": nil,
		"CanWrite": true, "ModalRedirect": "/workboard/register?view=week&date=2026-08-04",
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatalf("content 렌더링 실패: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `/workboard/unschedule`) || !strings.Contains(out, `name="task_id" value="WT-1"`) {
		t.Fatal("배치된 카드의 대기 목록으로 내리기(×) 폼이 없다")
	}
	if strings.Contains(out, `data-time="09:00" data-assignee=`) {
		t.Fatal("주간 드롭 칸에 data-assignee가 있으면 안 됨")
	}
	if err := tmpl.ExecuteTemplate(io.Discard, "base.html", data); err != nil {
		t.Fatalf("base 렌더링 실패: %v", err)
	}
}

func TestRegisterDayTemplateAssigneeColumns(t *testing.T) {
	root := findTemplateRoot(t)
	files := []string{
		filepath.Join(root, "layout", "base.html"),
		filepath.Join(root, "workboard", "register.html"),
	}
	partials, err := filepath.Glob(filepath.Join(root, "workboard", "_*.html"))
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, partials...)
	tmpl, err := template.New("").Funcs(funcMap()).ParseFiles(files...)
	if err != nil {
		t.Fatal(err)
	}

	dateCol := RegisterColumn{Date: "2026-08-18", From: "2026-08-18", To: "2026-08-18"}
	placed := []model.WorkTask{
		{TaskID: "A", Title: "[점검]광진", Assignee: "최혜영", WorkDate: "2026-08-18", StartTime: "07:30", EndTime: "08:00", DurationMin: 30},
		{TaskID: "B", Title: "[점검]양천", Assignee: "최혜영", WorkDate: "2026-08-18", StartTime: "09:00", EndTime: "09:30", DurationMin: 30},
		{TaskID: "C", Title: "[점검]양기헌건", Assignee: "양기헌", WorkDate: "2026-08-18", StartTime: "08:45", EndTime: "09:15", DurationMin: 30},
	}
	slotTimes := registerSlotTimes()
	gridH, slotTops := registerGridStyles(len(slotTimes))
	data := map[string]interface{}{
		"Title": "일일 업무 등록", "Active": NavWorkRegister, "UserRole": "admin",
		"View": regViewDay, "ViewLabel": registerViewLabel(regViewDay),
		"Date": "2026-08-18", "PeriodLabel": "2026-08-18",
		"PrevDate": "2026-08-17", "NextDate": "2026-08-19", "Today": "2026-08-18",
		"Columns":    []RegisterColumn{dateCol},
		"DayColumns": buildRegisterAssigneeColumns(dateCol, placed, taskCardFromWork, []string{"양기헌", "최혜영"}, ""),
		"SlotTimes":  slotTimes, "GridHeightStyle": gridH, "SlotTopStyles": slotTops,
		"AssigneeLegend": buildAssigneeLegend(placed, nil),
		"ASCards":        []model.WBCard{}, "MntCards": []model.WBCard{}, "AdminCards": []model.WBCard(nil),
		"PlacedCount": 3, "Projects": nil, "Assignees": nil,
		"CanWrite": true, "ModalRedirect": "/workboard/register?view=day&date=2026-08-18",
		"ExtraAssignees": []string{"이해진"}, "DayLeaveNames": []string{},
		"AssigneeColMin": registerAssigneeColMinPx,
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatalf("일일 content 렌더링 실패: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `data-time="07:30" data-assignee="최혜영"`) {
		t.Fatal("일일 드롭 칸에 data-assignee가 없다")
	}
	if !strings.Contains(out, "최혜영") || !strings.Contains(out, "양기헌") {
		t.Fatal("일일 열 머리글에 담당자가 없다")
	}
	if !strings.Contains(out, "2건 · 1시간") {
		t.Fatal("열 머리글 건수·소요가 없다")
	}
	if strings.Contains(out, "07:30~08:00 · 최혜영") {
		t.Fatal("일일 카드에 담당자 이름이 남아 있다")
	}
	if !strings.Contains(out, "sticky left-0") {
		t.Fatal("시간 열 sticky 가 없다")
	}
	if !strings.Contains(out, "＋담당자") {
		t.Fatal("＋담당자 버튼이 없다")
	}
	if !strings.Contains(out, "주담당") || !strings.Contains(out, "참여자") {
		t.Fatal("등록 모달에 주담당·참여자 칸이 없다")
	}
}

func TestRegisterDayTemplateSupportCardsNoButtons(t *testing.T) {
	root := findTemplateRoot(t)
	files := []string{
		filepath.Join(root, "layout", "base.html"),
		filepath.Join(root, "workboard", "register.html"),
	}
	partials, err := filepath.Glob(filepath.Join(root, "workboard", "_*.html"))
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, partials...)
	tmpl, err := template.New("").Funcs(funcMap()).ParseFiles(files...)
	if err != nil {
		t.Fatal(err)
	}
	dateCol := RegisterColumn{Date: "2026-08-18", From: "2026-08-18", To: "2026-08-18"}
	placed := []model.WorkTask{
		{TaskID: "T1", Title: "CRM 자산정비", Assignee: "최혜영", WorkDate: "2026-08-18", StartTime: "09:00", EndTime: "11:00", DurationMin: 120},
	}
	members := map[string][]model.WorkTaskMember{
		"T1": {
			{TaskID: "T1", Assignee: "최혜영", Role: model.WBMemberOwner},
			{TaskID: "T1", Assignee: "양기헌", Role: model.WBMemberSupport},
			{TaskID: "T1", Assignee: "태자운", Role: model.WBMemberSupport},
		},
	}
	slotTimes := registerSlotTimes()
	gridH, slotTops := registerGridStyles(len(slotTimes))
	data := map[string]interface{}{
		"Title": "일일 업무 등록", "Active": NavWorkRegister, "UserRole": "admin",
		"View": regViewDay, "ViewLabel": registerViewLabel(regViewDay),
		"Date": "2026-08-18", "PeriodLabel": "2026-08-18",
		"PrevDate": "2026-08-17", "NextDate": "2026-08-19", "Today": "2026-08-18",
		"Columns":    []RegisterColumn{dateCol},
		"DayColumns": buildRegisterAssigneeColumnsMembers(dateCol, placed, taskCardFromWork, []string{"최혜영", "양기헌", "태자운"}, "", members),
		"SlotTimes":  slotTimes, "GridHeightStyle": gridH, "SlotTopStyles": slotTops,
		"AssigneeLegend": nil,
		"ASCards":        []model.WBCard{}, "MntCards": []model.WBCard{}, "AdminCards": []model.WBCard(nil),
		"PlacedCount": 1, "Projects": nil, "Assignees": nil,
		"CanWrite": true, "ModalRedirect": "/workboard/register",
		"ExtraAssignees": []string{}, "DayLeaveNames": []string{},
		"AssigneeColMin": registerAssigneeColMinPx,
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	if n := strings.Count(out, `border-dashed">지원</span>`); n != 2 {
		t.Fatalf("지원 뱃지=%d want 2", n)
	}
	if strings.Count(out, ">조치<") != 1 {
		t.Fatalf("조치 버튼=%d want 1 (주담당만)", strings.Count(out, ">조치<"))
	}
	if !strings.Contains(out, "외 2명") {
		t.Fatal("카드 하단에 외 n명이 없다")
	}
	if strings.Count(out, `draggable="true"`) != 1 {
		t.Fatalf("드래그 가능 카드=%d want 1 (주담당만)", strings.Count(out, `draggable="true"`))
	}
}

func TestProductFromMaintDesc(t *testing.T) {
	if got := productFromMaintDesc("KLAS 정기 점검"); got != "KLAS" {
		t.Fatalf("desc: %q", got)
	}
	if got := productFromMaintDesc("2026-08-05 · 앤로보틱스"); got != "앤로보틱스" {
		t.Fatalf("number: %q", got)
	}
}

func TestAddMinutesHHMM(t *testing.T) {
	cases := map[string]string{
		"09:00": "09:30",
		"19:45": "20:00", // 업무시간 끝을 넘지 않는다
		"20:00": "20:00",
	}
	for in, want := range cases {
		if got := addMinutesHHMM(in, 30); got != want {
			t.Errorf("addMinutesHHMM(%q) = %q, want %q", in, got, want)
		}
	}
}
