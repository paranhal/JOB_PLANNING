package repository

import (
	"database/sql"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"customer-support/internal/model"
)

// 95-215 규칙 O 게이트: 담당자가 저장·복사되는 경로는 모두 조직 검사를 거쳐야 한다.
// 새 경로(담당자 열을 쓰는 SQL이 든 소스 파일)가 생기면 아래 assigneeWriteFiles 에 한 줄을 더하고
// 어떤 검사를 거치는지 적어야 통과한다(누락되면 TestAssigneeOrgIsolationMatrix 가 실패).
var assigneeWriteFiles = map[string]string{
	// 정기점검 — 95-215 에서 사이트 조직 검사 적용(assignee_org_rule.go)
	"repository/maintenance_manage.go": "상속·일괄: siteOrgAssigneeFilter",
	"repository/maintenance_repo.go":   "방문 저장: 호출 쪽(상속·기본값)에서 걸러 넘김",
	"repository/mnt_assignees.go":      "기본 담당·그룹 리더·구성원: ownOrgAssignee",
	"repository/wb_mnt_sync.go":        "방문→업무 동기화: 방문 담당을 그대로 따라간다(입력 단계에서 막음)",
	// 일반 업무·AS·사업 작업 — handler 의 checkAssigneeInOrg
	"repository/wb_repo.go":              "handler checkAssigneeInOrg",
	"repository/work_task_members.go":    "handler checkAssigneeInOrg",
	"repository/work_task_item_ext.go":   "handler checkAssigneeInOrg",
	"repository/work_gtd_repo.go":        "handler checkAssigneeInOrg",
	"repository/work_task_split.go":      "업무 분할: 원본 담당 복사",
	"repository/work_task_action.go":     "handler checkAssigneeInOrg",
	"repository/work_live.go":            "내 업무 시작: 로그인 본인",
	"repository/work_board_unplanned.go": "handler checkAssigneeInOrg",
	"repository/assignee_save.go":        "공용 저장 함수: handler checkAssigneeInOrg",
	"repository/assignee_user_id.go":     "이름→ID 매칭 도우미",
	"repository/as_repo.go":              "handler checkAssigneeInOrg(as.go)",
	"repository/as_project_link.go":      "AS 사업 연결: 담당 변경 없음(동기화)",
	"repository/as_admin_move.go":        "AS 조직 이동: 담당 초기화 정책",
	"repository/wb_as_sync.go":           "AS→업무 동기화: AS 담당을 따라간다",
	"repository/company_weekly.go":       "주간보고 집계: 읽기·표시",
	"repository/coop_assign.go":          "협조: 대상 팀 소속만(resolveAssignees)",
	// 95-215 매트릭스가 처음 잡아낸 경로(소스 스캔)
	"repository/assignee_picker_schema.go": "스키마·이관(담당자 선택기): 데이터 정리 대상, 입력 경로 아님",
	"repository/mnt_region.go":             "지역 담당 저장: 읽을 때 siteOrgAssigneeFilter·ownOrgAssignee 로 걸러 쓴다",
	"repository/work_task_member_store.go": "업무 참여자 저장: handler checkAssigneeInOrg",
	"repository/work_task_segments.go":     "업무 구간 담당: handler checkAssigneeInOrg",
}

var assigneeWriteSQL = regexp.MustCompile(`(?s)(UPDATE|INSERT)[^;` + "`" + `]{0,400}assignee`)

func TestAssigneeOrgIsolationMatrix(t *testing.T) {
	// 1) 경로 목록 상수: 담당자 열을 쓰는 소스 파일이 목록에 없으면 실패.
	found := map[string]bool{}
	for _, dir := range []string{".", "../handler"} {
		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if assigneeWriteSQL.Match(b) {
				rel := "repository/" + filepath.Base(f)
				if dir != "." {
					rel = "handler/" + filepath.Base(f)
				}
				found[rel] = true
			}
		}
	}
	var missing []string
	for f := range found {
		if _, ok := assigneeWriteFiles[f]; !ok {
			missing = append(missing, f)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("담당자 열을 쓰는 새 경로가 있다. 조직 검사를 거치게 하고 assigneeWriteFiles 에 적어라: %v", missing)
	}
	for f := range assigneeWriteFiles {
		if !found[f] && !strings.HasPrefix(f, "repository/coop") {
			// 목록에만 남은 낡은 항목은 알려 준다(실패는 아님).
			t.Logf("목록에만 있는 경로(더 이상 담당자 SQL 없음): %s", f)
		}
	}

	// 2) 행동 시험: 정기점검 입력 경로 × {같은 조직 / 타 조직 / 이름만 / 조직 빈 사용자 / 그룹 구성원}
	db, err := InitDB(filepath.Join(t.TempDir(), "matrix.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ex := func(q string, a ...interface{}) {
		t.Helper()
		if _, err := db.Exec(q, a...); err != nil {
			t.Fatal(err)
		}
	}
	ex(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES ('O03','O03','사업관리','사업관리',1,3)`)
	ex(`INSERT INTO users(user_id,username,password_hash,full_name,role,is_active,org_id,profile_done) VALUES
		('u-park','park','x','박노선','tech',1,'O03',1),
		('u-lib','lib','x','도서관직원','tech',1,?,1),
		('u-lead','lead','x','도서관팀장','org_admin',1,?,1)`, model.OrgIDLibrary, model.OrgIDLibrary)
	for _, id := range []string{"c-prev", "c-year", "c-def", "c-reg", "c-grp", "c-name", "c-same"} {
		ex(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES (?,?,?,1,?)`, id, id, id, model.OrgIDLibrary)
	}
	repo := NewMaintenanceRepo(db)
	for _, id := range []string{"c-prev", "c-year", "c-def", "c-reg", "c-grp", "c-name", "c-same"} {
		cfg := &model.MaintenanceSiteConfig{CustomerID: id, ShortName: id, HasKlas: true, InspectionCycle: "monthly",
			OrgID: model.OrgIDLibrary, RoundsPerPeriod: 1}
		if id == "c-reg" {
			cfg.Region = "공주"
		}
		if err := repo.UpsertSiteConfig(cfg); err != nil {
			t.Fatal(err)
		}
	}
	prev, _ := repo.CreatePlan(2025, "prev")
	cur, _ := repo.CreatePlan(2026, "cur")
	mustVisit := func(v model.MaintenanceVisit) string {
		t.Helper()
		id, err := repo.InsertVisitFullID(v)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	assigneeOf := func(id string) string {
		var a string
		_ = db.QueryRow(`SELECT COALESCE(assignee,'') FROM maintenance_visits WHERE visit_id=?`, id).Scan(&a)
		return a
	}

	type tc struct {
		path, name string
		got        func() string
		want       string // 빈 문자열이면 「조직 밖 이름이 없어야 한다」
	}
	var cases []tc
	// 전년 방문 상속
	mustVisit(model.MaintenanceVisit{PlanID: prev.PlanID, VisitDate: "2025-11-01", CustomerID: "c-prev", ProductType: "KLAS", Assignee: "박노선", SortOrder: 1})
	cases = append(cases, tc{"상속-전년", "타 조직", func() string {
		return repo.AssigneeForVisit(cur.PlanID, "c-prev", "KLAS", "2026-11-13")
	}, ""})
	mustVisit(model.MaintenanceVisit{PlanID: prev.PlanID, VisitDate: "2025-11-01", CustomerID: "c-same", ProductType: "KLAS", Assignee: "도서관직원", SortOrder: 1})
	cases = append(cases, tc{"상속-전년", "같은 조직(대조)", func() string {
		return repo.AssigneeForVisit(cur.PlanID, "c-same", "KLAS", "2026-11-13")
	}, "도서관직원"})
	mustVisit(model.MaintenanceVisit{PlanID: prev.PlanID, VisitDate: "2025-11-01", CustomerID: "c-name", ProductType: "KLAS", Assignee: "사용자없는이름", SortOrder: 1})
	cases = append(cases, tc{"상속-전년", "이름만(사용자 미등록)", func() string {
		return repo.AssigneeForVisit(cur.PlanID, "c-name", "KLAS", "2026-11-13")
	}, ""})
	// 올해 같은 사이트 상속
	mustVisit(model.MaintenanceVisit{PlanID: cur.PlanID, VisitDate: "2026-01-05", CustomerID: "c-year", ProductType: "KLAS", Assignee: "박노선", SortOrder: 1})
	cases = append(cases, tc{"상속-올해", "타 조직", func() string {
		return repo.AssigneeForVisit(cur.PlanID, "c-year", "KLAS", "2026-11-13")
	}, ""})
	// 슬롯 배정(AssignSlotID)에서 상속
	cases = append(cases, tc{"슬롯 배정", "타 조직", func() string {
		id, err := repo.AssignSlotID(model.MaintenanceVisit{PlanID: cur.PlanID, VisitDate: "2026-11-14", CustomerID: "c-prev", ProductType: "KLAS", SortOrder: 1})
		if err != nil {
			t.Fatal(err)
		}
		return assigneeOf(id)
	}, ""})
	// 사이트 기본 담당
	if err := repo.ReplaceSiteAssignees("c-def", []model.MaintenanceSiteAssignee{{CustomerID: "c-def", UserID: "u-park", Name: "박노선", Role: model.WBMemberOwner}}); err != nil {
		t.Fatal(err)
	}
	cases = append(cases, tc{"사이트 기본 담당", "타 조직", func() string {
		return assigneeOf(mustVisit(model.MaintenanceVisit{PlanID: cur.PlanID, VisitDate: "2026-11-16", CustomerID: "c-def", ProductType: "KLAS", SortOrder: 1}))
	}, ""})
	cases = append(cases, tc{"사이트 기본 담당", "SiteDefaultOwnerName", func() string { return repo.SiteDefaultOwnerName("c-def") }, ""})
	// 지역 기본 담당
	if err := repo.ReplaceRegionAssignees(model.OrgIDLibrary, "공주", []model.MaintenanceRegionAssignee{{OrgID: model.OrgIDLibrary, Region: "공주", UserID: "u-park", Name: "박노선", Role: model.WBMemberOwner}}); err != nil {
		t.Fatal(err)
	}
	cases = append(cases, tc{"지역 기본 담당", "타 조직", func() string { return repo.SiteDefaultOwnerName("c-reg") }, ""})
	// 업무 그룹 리더·구성원(타 팀 사람이 섞인 그룹)
	// 그룹은 같은 조직 구성원만 넣을 수 있게 막혀 있다. 타 팀 사람이 섞인 그룹은 조직 이동·옛 데이터로만 생기므로 SQL 로 만든다.
	g := &model.WorkGroup{Name: "섞인그룹", OrgID: model.OrgIDLibrary}
	actor := model.WorkGroupActor{UserID: "u-lead", OrgID: model.OrgIDLibrary, Role: "org_admin"}
	if err := NewWorkGroupRepo(db).CreateWithMembers(actor, g, []string{"u-lib", "u-lead"}, "u-lead"); err != nil {
		t.Fatalf("그룹 생성: %v", err)
	}
	ex(`UPDATE work_group_members SET role=? WHERE group_id=? AND user_id='u-lead'`, model.WorkGroupMember, g.GroupID)
	_, groupErr := db.Exec(`INSERT INTO work_group_members(group_id,user_id,role,joined_on,added_by) VALUES(?,?,?,?,?)`, g.GroupID, "u-park", model.WorkGroupLeader, "2020-01-01", "u-lead")
	if groupErr != nil {
		// DB 제약(member org mismatch)이 타 조직 구성원을 막는다. 옛 데이터에 섞인 그룹이 있어도 코드가 막도록 방어 코드는 유지한다.
		t.Logf("타 조직 구성원 그룹은 DB 제약으로 만들 수 없다: %v", groupErr)
	}
	if groupErr == nil {
		cases = append(cases, tc{"업무 그룹 리더·구성원", "타 팀 리더", func() string {
			id := mustVisit(model.MaintenanceVisit{PlanID: cur.PlanID, VisitDate: "2026-11-17", CustomerID: "c-grp", ProductType: "KLAS", SortOrder: 1, WorkGroupID: g.GroupID})
			var n int
			_ = db.QueryRow(`SELECT COUNT(*) FROM maintenance_visit_assignees WHERE visit_id=? AND TRIM(assignee_name)='박노선'`, id).Scan(&n)
			if n > 0 {
				return "박노선(지원자 행)"
			}
			return assigneeOf(id)
		}, ""})
	}

	for _, c := range cases {
		got := strings.TrimSpace(c.got())
		if c.want == "" && strings.Contains(got, "박노선") {
			t.Errorf("[%s × %s] 조직 밖 담당자가 저장·복사됨: %q", c.path, c.name, got)
		}
		if c.want != "" && got != c.want {
			t.Errorf("[%s × %s] 같은 조직은 종전대로 got=%q want=%q", c.path, c.name, got, c.want)
		}
		if c.want == "" && got != "" && !strings.Contains(got, "박노선") {
			t.Logf("[%s × %s] got=%q (조직 안 담당으로 대체됨)", c.path, c.name, got)
		}
	}

	// 3) 표시 쪽: 도서관 칸반·목록 항목 — 미완료는 이름을 감추고 재배정 표시, 완료는 이름 그대로
	wb := NewWorkBoardRepo(db)
	pv := mustVisit(model.MaintenanceVisit{PlanID: cur.PlanID, VisitDate: "2026-11-18", CustomerID: "c-same", ProductType: "KLAS", Assignee: "박노선", SortOrder: 1})
	dv := mustVisit(model.MaintenanceVisit{PlanID: cur.PlanID, VisitDate: "2026-11-02", CustomerID: "c-name", ProductType: "KLAS", Assignee: "박노선", SortOrder: 1, Completed: true, CompletedDate: "2026-11-02"})
	for _, tcase := range []struct {
		visit      string
		wantName   string
		wantReassn bool
	}{{pv, "", true}, {dv, "박노선", false}} {
		items, err := wb.queryMaintenance("mv.visit_id = ?", []interface{}{tcase.visit}, 5)
		if err != nil || len(items) != 1 {
			t.Fatalf("항목 조회 err=%v n=%d", err, len(items))
		}
		kept, _ := wb.filterWorkListForOrg(items, model.OrgIDLibrary)
		if len(kept) != 1 {
			t.Fatalf("도서관 칸반에 카드가 있어야 한다(숨기지 않음) visit=%s", tcase.visit)
		}
		if kept[0].Assignee != tcase.wantName || kept[0].AssigneeReassign != tcase.wantReassn {
			t.Errorf("표시 visit=%s assignee=%q reassign=%v want %q/%v", tcase.visit, kept[0].Assignee, kept[0].AssigneeReassign, tcase.wantName, tcase.wantReassn)
		}
	}
	_ = sql.ErrNoRows
}
