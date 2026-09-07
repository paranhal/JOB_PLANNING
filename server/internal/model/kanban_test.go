package model

import "testing"

func TestParseDisplay(t *testing.T) {
	if ParseDisplay("", "") != "list" {
		t.Fatal("기본은 리스트")
	}
	if ParseDisplay("kanban", "") != "kanban" || ParseDisplay("", "kanban") != "kanban" {
		t.Fatal("display·view=kanban")
	}
	if ParseDisplay("list", "kanban") != "list" {
		t.Fatal("display가 view보다 앞선다")
	}
}

func TestFillWorkKanbanExclusiveAndCounts(t *testing.T) {
	items := []WorkListItem{
		{Prefix: WorkPrefixAS, RefID: "1", RefNumber: "AS-2", Status: "hold", Title: "보류", ScheduledDate: "2026-08-20"},
		{Prefix: WorkPrefixAS, RefID: "2", RefNumber: "AS-1", Status: "received", Title: "대기", ScheduledDate: "2026-08-10"},
		{Prefix: WorkPrefixGeneral, RefID: "3", RefNumber: "W-1", Status: "complete", Title: "끝", ScheduledDate: "2026-08-01"},
		{Prefix: WorkPrefixAS, RefID: "1", RefNumber: "AS-2", Status: "hold", Title: "중복키"},
	}
	var mapped []WorkListItem
	for i := range items {
		it := items[i]
		if it.ApplyKanbanMapping() {
			mapped = append(mapped, it)
		}
	}
	view := FillWorkKanban(ExecKanbanColumnDefs(), mapped)
	if view.Total != 3 {
		t.Fatalf("배타 합 %d want 3", view.Total)
	}
	if len(view.Columns) != 3 {
		t.Fatalf("열 %d", len(view.Columns))
	}
	for _, c := range view.Columns {
		if c.Items == nil {
			t.Fatalf("빈 열 Items nil %s", c.Key)
		}
	}
	if view.Columns[0].Count != 1 || view.Columns[0].Items[0].RefNumber != "AS-1" {
		t.Fatalf("대기 열 정렬 %+v", view.Columns[0])
	}
	if view.Columns[1].Count != 1 || view.Columns[1].Items[0].Title != "보류" {
		t.Fatalf("진행중(보류) %+v", view.Columns[1])
	}
	if view.Columns[2].Count != 1 {
		t.Fatalf("완료 %d", view.Columns[2].Count)
	}
}

func TestFillASListKanbanFourColumns(t *testing.T) {
	items := []ASListItem{
		{ASID: "a", ASNumber: "N2", Status: "received", OrgName: "대기기관", VisitScheduledDate: "2026-08-20"},
		{ASID: "b", ASNumber: "N1", Status: "hold", OrgName: "진행기관", VisitScheduledDate: "2026-08-10"},
		{ASID: "c", ASNumber: "N3", Status: "review", OrgName: "검토기관"},
		{ASID: "d", ASNumber: "N4", Status: "completed", OrgName: "완료기관"},
		{ASID: "e", ASNumber: "N5", Status: "cancelled", OrgName: "취소"},
		{ASID: "a", ASNumber: "N2", Status: "received", OrgName: "중복"},
	}
	view := FillASListKanban(ASListKanbanColumnDefs(), items)
	if view.Total != 4 {
		t.Fatalf("합 %d (취소·중복 제외)", view.Total)
	}
	by := map[string]int{}
	for _, c := range view.Columns {
		by[c.Key] = c.Count
	}
	if by[WBTaskWaiting] != 1 || by[WBTaskInProgress] != 1 || by[WBTaskReview] != 1 || by[WBTaskComplete] != 1 {
		t.Fatalf("4열 배분 %+v", by)
	}
	if view.Columns[1].Items[0].Title != "진행기관" {
		t.Fatal("보류는 진행중 열")
	}
}

func TestAssignASListColumnSharesMapping(t *testing.T) {
	if AssignASListColumn(MapASStatusToWB("hold")) != WBTaskInProgress {
		t.Fatal("보류는 진행중")
	}
	if AssignASListColumn(MapASStatusToWB("review")) != WBTaskReview {
		t.Fatal("검토는 검토 열")
	}
	if AssignASListColumn(MapASStatusToWB("received")) != WBTaskWaiting {
		t.Fatal("접수는 대기")
	}
	exec := WorkListItem{Status: "hold"}
	exec.ApplyKanbanMapping()
	if exec.Bucket != WBTaskInProgress {
		t.Fatal("3열과 AS 4열의 보류 판정이 어긋남")
	}
}

func TestFillAdminKanbanEmptyColumnStays(t *testing.T) {
	items := []WorkTask{
		{TaskID: "t1", Title: "할일", Status: WBTaskWaiting, DueDate: "2026-08-02"},
		{TaskID: "t2", Title: "끝", Status: WBTaskComplete, DueDate: "2026-08-01"},
		{TaskID: "t1", Title: "중복", Status: WBTaskWaiting},
	}
	view := FillAdminKanban(AdminKanbanColumnDefs(), items)
	if view.Total != 2 {
		t.Fatalf("합 %d", view.Total)
	}
	if view.Columns[1].Count != 0 || view.Columns[1].Title != "진행중" {
		t.Fatalf("빈 진행중 열이 사라졌다 %+v", view.Columns[1])
	}
}

func TestFillMeetingKanbanExclusiveOrder(t *testing.T) {
	same := WorkListItem{Prefix: WorkPrefixAS, RefID: "x", RefNumber: "N1", Title: "겹침", Assignee: "양기헌", Status: WBTaskInProgress}
	prev := []WorkListItem{
		{Prefix: WorkPrefixAS, RefID: "y", RefNumber: "Y1", Title: "어제끝", Assignee: "양기헌", Status: "completed", CompleteDate: "2026-08-10"},
		same,
	}
	prog := []WorkListItem{same, {Prefix: WorkPrefixAS, RefID: "p", RefNumber: "P1", Title: "이어짐", Assignee: "최혜영", Status: WBTaskInProgress}}
	today := []WorkListItem{same, {Prefix: WorkPrefixAS, RefID: "t", RefNumber: "T1", Title: "오늘만", Assignee: "양기헌", Status: "received"}}
	unpl := []WorkListItem{
		{Prefix: WorkPrefixAS, RefID: "t", RefNumber: "T1", Title: "오늘만-미계획중복"},
		{Prefix: WorkPrefixAS, RefID: "u", RefNumber: "U1", Title: "미계획", Assignee: "", Status: "received"},
	}
	view := FillMeetingKanban(prev, prog, today, unpl)
	if len(view.Columns) != 4 {
		t.Fatalf("열 %d", len(view.Columns))
	}
	if view.Columns[0].Title != "전일 완료" || view.Columns[1].Title != "오늘 예정" ||
		view.Columns[2].Title != "진행중" || view.Columns[3].Title != "미계획" {
		t.Fatalf("열 제목 %+v", view.Columns)
	}
	if view.Total != 5 {
		t.Fatalf("배타 합 %d want 5", view.Total)
	}
	seen := map[string]int{}
	for _, c := range view.Columns {
		for _, it := range c.Items {
			seen[it.ID]++
			if seen[it.ID] > 1 {
				t.Fatalf("두 열에 %s", it.ID)
			}
		}
	}
	if view.Columns[0].Count != 2 || view.Columns[2].Count != 1 || view.Columns[1].Count != 1 || view.Columns[3].Count != 1 {
		t.Fatalf("배분 전일=%d 예정=%d 진행=%d 미계획=%d",
			view.Columns[0].Count, view.Columns[1].Count, view.Columns[2].Count, view.Columns[3].Count)
	}
	if view.Columns[0].Items[0].ID != "as:x" && view.Columns[0].Items[1].ID != "as:x" {
		t.Fatal("겹친 건은 전일 완료가 먼저")
	}
	if view.Columns[1].Items[0].RefNumber != "T1" {
		t.Fatal("오늘 예정만 남은 건")
	}
	if view.Columns[2].Items[0].RefNumber != "P1" {
		t.Fatal("진행중만 남은 건")
	}
	if view.Columns[3].Count != 1 || view.Columns[3].Items[0].RefNumber != "U1" {
		t.Fatal("미계획만 남은 건")
	}
}

func TestFillUnplannedKanbanFiveColumnsMatchList(t *testing.T) {
	items := []UnplannedItem{
		{WorkListItem: WorkListItem{Prefix: WorkPrefixAS, RefID: "1", RefNumber: "A", Title: "없음"}, ItemKey: "as:1", Kinds: []string{UnplannedNoDate, UnplannedUnassigned}},
		{WorkListItem: WorkListItem{Prefix: WorkPrefixAS, RefID: "2", RefNumber: "B"}, ItemKey: "as:2", Kinds: []string{UnplannedDelayed, UnplannedUnassigned}},
		{WorkListItem: WorkListItem{Prefix: WorkPrefixAS, RefID: "3", RefNumber: "C"}, ItemKey: "as:3", Kinds: []string{UnplannedNext}},
		{WorkListItem: WorkListItem{Prefix: WorkPrefixAS, RefID: "4", RefNumber: "D"}, ItemKey: "as:4", Kinds: []string{UnplannedUnassigned}},
		{WorkListItem: WorkListItem{Prefix: WorkPrefixAS, RefID: "5", RefNumber: "E"}, ItemKey: "as:5", Kinds: []string{UnplannedReview, UnplannedNoDate}},
		{WorkListItem: WorkListItem{Prefix: WorkPrefixSales, RefID: "6", RefNumber: "F"}, ItemKey: "sales:6", Kinds: []string{UnplannedSalesFollow}},
		{WorkListItem: WorkListItem{Prefix: WorkPrefixAS, RefID: "1", RefNumber: "A"}, ItemKey: "as:1", Kinds: []string{UnplannedNoDate}},
	}
	view := FillUnplannedKanban(items)
	if len(view.Columns) != 5 {
		t.Fatalf("열 %d", len(view.Columns))
	}
	if view.Total != 6 {
		t.Fatalf("합 %d want 6 (리스트 고유 건수)", view.Total)
	}
	by := map[string]int{}
	for _, c := range view.Columns {
		by[c.Key] = c.Count
		if c.Items == nil {
			t.Fatalf("빈 열 nil %s", c.Key)
		}
	}
	if by[UnplannedNoDate] != 2 || by[UnplannedDelayed] != 1 || by[UnplannedNext] != 1 || by[UnplannedUnassigned] != 2 || by[UnplannedReview] != 0 {
		t.Fatalf("5열 배분 %+v", by)
	}
	if AssignUnplannedKanbanColumn(items[5]) != UnplannedUnassigned {
		t.Fatal("영업 후속없음은 담당자미배정 열")
	}
}

func TestFillAssetKanbanFiveStatuses(t *testing.T) {
	items := []Asset{
		{AssetID: "1", ProductName: "게이트", OperationStatus: AssetOpOperating, InstallDate: "2020-01-02"},
		{AssetID: "2", ProductName: "점검", OperationStatus: AssetOpMaintenance, InstallDate: "2019-01-01"},
		{AssetID: "3", ProductName: "장애", OperationStatus: AssetOpFault},
		{AssetID: "4", ProductName: "철수", OperationStatus: AssetOpRetired},
		{AssetID: "5", ProductName: "폐기", OperationStatus: AssetOpDisposed},
		{AssetID: "6", ProductName: "빈값", OperationStatus: ""},
		{AssetID: "1", ProductName: "중복", OperationStatus: AssetOpFault},
	}
	view := FillAssetKanban(items)
	if view.Total != 6 {
		t.Fatalf("합 %d", view.Total)
	}
	by := map[string]int{}
	for _, c := range view.Columns {
		by[c.Key] = c.Count
	}
	if by[AssetOpOperating] != 2 || by[AssetOpMaintenance] != 1 || by[AssetOpFault] != 1 || by[AssetOpRetired] != 1 || by[AssetOpDisposed] != 1 {
		t.Fatalf("%+v", by)
	}
	if AssignAssetKanbanColumn("unknown") != AssetOpOperating {
		t.Fatal("모르는 상태는 운영중")
	}
}

func TestWorkMappingUnchangedForSharedKanban(t *testing.T) {
	if MapASStatusToWB("in_progress") != WBTaskInProgress {
		t.Fatal("공유 판정")
	}
	if AssignASListColumn(MapASStatusToWB("hold")) != WBTaskInProgress {
		t.Fatal("AS 칸반과 오늘 내 업무가 모순되면 안 된다")
	}
}
