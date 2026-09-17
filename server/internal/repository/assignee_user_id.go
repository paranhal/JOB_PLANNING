package repository

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
)

// UnmatchedAssignee users 에 없는 담당자 이름. 우리 직원이 아니면 users 에 넣지 않는다. §44.8.3
type UnmatchedAssignee struct {
	Table  string
	Column string
	Name   string
	Count  int
}

// applyAssigneeUserIDs 마이그레이션 050. 1걸음: ID 열을 채우고 실패 목록을 남긴다. 3걸음(이름 열 삭제)은 하지 않는다. §44.8.3
func applyAssigneeUserIDs(db *sql.DB) {
	if db == nil {
		return
	}
	addTextColumn(db, "work_task_members", "user_id")
	addTextColumn(db, "work_tasks", "assignee_user_id")
	addTextColumn(db, "maintenance_visits", "assignee_user_id")
	addTextColumn(db, "work_actions", "assignee_user_id")
	addTextColumn(db, "as_processes", "worker_user_id")
	addTextColumn(db, "work_activities", "actor_user_id")
	addTextColumn(db, "as_receipts", "external_assignee")
	if tableHasColumn(db, "work_projects", "assignee") {
		addTextColumn(db, "work_projects", "assignee_user_id")
	}

	backfillNameToUserID(db, "work_task_members", "assignee", "user_id")
	backfillNameToUserID(db, "work_tasks", "assignee", "assignee_user_id")
	backfillNameToUserID(db, "maintenance_visits", "assignee", "assignee_user_id")
	backfillNameToUserID(db, "work_actions", "assignee", "assignee_user_id")
	backfillNameToUserID(db, "as_processes", "worker", "worker_user_id")
	backfillNameToUserID(db, "work_activities", "actor", "actor_user_id")
	backfillNameToUserID(db, "as_receipts", "assigned_to", "assigned_user_id")
	backfillNameToUserID(db, "as_work_items", "assigned_to", "assigned_user_id")
	backfillNameToUserID(db, "sales_projects", "sales_owner", "sales_owner_id")
	if tableHasColumn(db, "work_projects", "assignee_user_id") {
		backfillNameToUserID(db, "work_projects", "assignee", "assignee_user_id")
	}

	copyUnmatchedToExternal(db)
	dropWorkTaskMemberTriggers(db)
	installWorkTaskMemberTriggers(db)

	items := ListUnmatchedAssignees(db)
	if len(items) == 0 {
		log.Printf("050 assignee user_id: 매칭 실패 없음")
		return
	}
	log.Printf("050 assignee user_id: 매칭 실패 %d종 — 우리 직원이 아니면 users 에 넣지 말고 외부 담당자로 둔다", len(items))
	limit := 30
	if len(items) < limit {
		limit = len(items)
	}
	for _, it := range items[:limit] {
		log.Printf("050 unmatched %s.%s %q (%d건)", it.Table, it.Column, it.Name, it.Count)
	}
}

func addTextColumn(db *sql.DB, table, col string) {
	if _, err := db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + col + ` TEXT`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		log.Printf("050 add %s.%s: %v", table, col, err)
	}
}

func tableHasColumn(db *sql.DB, table, col string) bool {
	if db == nil {
		return false
	}
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('`+table+`') WHERE name=?`, col).Scan(&n)
	return err == nil && n > 0
}

func backfillNameToUserID(db *sql.DB, table, nameCol, idCol string) {
	if !tableHasColumn(db, table, nameCol) || !tableHasColumn(db, table, idCol) {
		return
	}
	q := fmt.Sprintf(`
UPDATE %s SET %s=(
	SELECT u.user_id FROM users u
	 WHERE TRIM(u.full_name)=TRIM(%s.%s)
	    OR TRIM(COALESCE(u.username,''))=TRIM(%s.%s)
	 LIMIT 1
)
 WHERE TRIM(COALESCE(%s,''))=''
   AND TRIM(COALESCE(%s,''))!=''`,
		table, idCol, table, nameCol, table, nameCol, idCol, nameCol)
	if _, err := db.Exec(q); err != nil {
		log.Printf("050 backfill %s.%s: %v", table, idCol, err)
	}
}

func copyUnmatchedToExternal(db *sql.DB) {
	if !tableHasColumn(db, "as_receipts", "external_assignee") {
		return
	}
	_, err := db.Exec(`
UPDATE as_receipts
   SET external_assignee=TRIM(assigned_to)
 WHERE TRIM(COALESCE(assigned_user_id,''))=''
   AND TRIM(COALESCE(assigned_to,''))!=''
   AND TRIM(COALESCE(external_assignee,''))=''
   AND NOT EXISTS (
		SELECT 1 FROM users u
		 WHERE TRIM(u.full_name)=TRIM(as_receipts.assigned_to)
		    OR TRIM(COALESCE(u.username,''))=TRIM(as_receipts.assigned_to)
		    OR u.user_id=TRIM(as_receipts.assigned_to)
   )`)
	if err != nil {
		log.Printf("050 external_assignee: %v", err)
	}
}

// ListUnmatchedAssignees 이름 열이 users.full_name/username/user_id 어디에도 없는 값. 관리 화면·로그용.
func ListUnmatchedAssignees(db *sql.DB) []UnmatchedAssignee {
	if db == nil {
		return nil
	}
	sources := [][2]string{
		{"work_task_members", "assignee"},
		{"work_tasks", "assignee"},
		{"maintenance_visits", "assignee"},
		{"work_actions", "assignee"},
		{"work_projects", "assignee"},
		{"as_receipts", "assigned_to"},
		{"as_work_items", "assigned_to"},
		{"as_processes", "worker"},
		{"work_activities", "actor"},
		{"sales_projects", "sales_owner"},
	}
	var parts []string
	var args []interface{}
	for _, s := range sources {
		table, col := s[0], s[1]
		if !tableHasColumn(db, table, col) {
			continue
		}
		parts = append(parts, fmt.Sprintf(`
SELECT '%s' AS src, '%s' AS col, TRIM(%s) AS name, COUNT(*) AS n
  FROM %s
 WHERE TRIM(COALESCE(%s,''))!=''
   AND NOT EXISTS (
		SELECT 1 FROM users u
		 WHERE TRIM(u.full_name)=TRIM(%s.%s)
		    OR TRIM(COALESCE(u.username,''))=TRIM(%s.%s)
		    OR u.user_id=TRIM(%s.%s)
   )
 GROUP BY TRIM(%s)`, table, col, col, table, col, table, col, table, col, table, col, col))
	}
	if len(parts) == 0 {
		return nil
	}
	rows, err := db.Query(strings.Join(parts, "\nUNION ALL\n") + ` ORDER BY src, col, name`, args...)
	if err != nil {
		log.Printf("050 unmatched list: %v", err)
		return nil
	}
	defer rows.Close()
	var out []UnmatchedAssignee
	for rows.Next() {
		var it UnmatchedAssignee
		if rows.Scan(&it.Table, &it.Column, &it.Name, &it.Count) != nil {
			continue
		}
		out = append(out, it)
	}
	return out
}

// ResolveStaff 이름 또는 user_id 로 우리 직원을 찾는다. 없으면 ok=false (외부인).
func ResolveStaff(db *sql.DB, name, userID string) (id, fullName string, ok bool) {
	name = strings.TrimSpace(name)
	userID = strings.TrimSpace(userID)
	if db == nil || (name == "" && userID == "") {
		return "", "", false
	}
	if userID != "" {
		var uid, nm string
		err := db.QueryRow(`SELECT user_id, COALESCE(full_name,'') FROM users WHERE user_id=? LIMIT 1`, userID).Scan(&uid, &nm)
		if err == nil && uid != "" {
			return uid, strings.TrimSpace(nm), true
		}
	}
	key := name
	if key == "" {
		key = userID
	}
	var uid, nm string
	err := db.QueryRow(`
		SELECT user_id, COALESCE(full_name,'') FROM users
		 WHERE TRIM(full_name)=? OR TRIM(COALESCE(username,''))=? OR user_id=?
		 LIMIT 1`, key, key, key).Scan(&uid, &nm)
	if err == nil && uid != "" {
		return uid, strings.TrimSpace(nm), true
	}
	return "", "", false
}

// bindStaff 저장용. 직원이면 정규 이름+ID, 아니면 이름은 남기고 ID 는 비운다.
func bindStaff(db *sql.DB, name, userID string) (display, uid string) {
	display = strings.TrimSpace(name)
	uid = strings.TrimSpace(userID)
	if id, nm, ok := ResolveStaff(db, display, uid); ok {
		if nm != "" {
			return nm, id
		}
		return display, id
	}
	return display, ""
}

func bindStaffExternal(db *sql.DB, name, userID string) (display, uid, external string) {
	display, uid = bindStaff(db, name, userID)
	if uid != "" {
		return display, uid, ""
	}
	if display != "" {
		return display, "", display
	}
	return "", "", ""
}

func ownerMemberUserIDSQL(taskAlias string) string {
	if taskAlias == "" {
		taskAlias = "NEW"
	}
	return `COALESCE(
		NULLIF(TRIM(COALESCE(` + taskAlias + `.assignee_user_id,'')), ''),
		(SELECT u.user_id FROM users u
		  WHERE TRIM(COALESCE(` + taskAlias + `.assignee,'')) != ''
		    AND (TRIM(u.full_name)=TRIM(` + taskAlias + `.assignee)
		         OR TRIM(COALESCE(u.username,''))=TRIM(` + taskAlias + `.assignee))
		  LIMIT 1),
		'')`
}

// rebuildWorkTaskMembersByUserID 3걸음. 이름 열을 지우고 PK 를 (task_id, user_id) 로 바꾼다.
// 건수가 다르면 원본 테이블을 그대로 둔다. InitDB 에서는 호출하지 않는다. §44.8.3
func rebuildWorkTaskMembersByUserID(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("db 없음")
	}
	var before int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_task_members`).Scan(&before); err != nil {
		return err
	}
	dropWorkTaskMemberTriggers(db)
	if _, err := db.Exec(`DROP TABLE IF EXISTS work_task_members_new`); err != nil {
		installWorkTaskMemberTriggers(db)
		return err
	}
	if _, err := db.Exec(`
CREATE TABLE work_task_members_new (
  task_id      TEXT NOT NULL,
  user_id      TEXT NOT NULL,
  member_role  TEXT NOT NULL CHECK (member_role IN ('owner','support')),
  duration_min INTEGER,
  sort_order   INTEGER NOT NULL DEFAULT 0,
  created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (task_id, user_id),
  FOREIGN KEY (task_id) REFERENCES work_tasks(task_id),
  FOREIGN KEY (user_id) REFERENCES users(user_id)
)`); err != nil {
		installWorkTaskMemberTriggers(db)
		return err
	}
	if _, err := db.Exec(`
INSERT INTO work_task_members_new
SELECT m.task_id, u.user_id, m.member_role, m.duration_min, m.sort_order, m.created_at
  FROM work_task_members m JOIN users u ON u.full_name = m.assignee`); err != nil {
		_, _ = db.Exec(`DROP TABLE IF EXISTS work_task_members_new`)
		installWorkTaskMemberTriggers(db)
		return err
	}
	var after int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_task_members_new`).Scan(&after); err != nil {
		_, _ = db.Exec(`DROP TABLE IF EXISTS work_task_members_new`)
		installWorkTaskMemberTriggers(db)
		return err
	}
	if before != after {
		_, _ = db.Exec(`DROP TABLE IF EXISTS work_task_members_new`)
		installWorkTaskMemberTriggers(db)
		return fmt.Errorf("이관 중단: 원본 %d행 → 이관 %d행. 매칭 실패 담당자를 먼저 정리하라", before, after)
	}
	if _, err := db.Exec(`DROP TABLE work_task_members`); err != nil {
		_, _ = db.Exec(`DROP TABLE IF EXISTS work_task_members_new`)
		installWorkTaskMemberTriggers(db)
		return err
	}
	if _, err := db.Exec(`ALTER TABLE work_task_members_new RENAME TO work_task_members`); err != nil {
		return err
	}
	return nil
}
