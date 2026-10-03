package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"customer-support/internal/audit"
)

var assigneeNameTargets = []struct {
	table, nameCol, idCol string
}{
	{"as_receipts", "assigned_to", "assigned_user_id"},
	{"as_processes", "worker", "worker_user_id"},
	{"maintenance_visits", "assignee", "assignee_user_id"},
	{"work_tasks", "assignee", "assignee_user_id"},
	{"work_task_members", "assignee", "user_id"},
	{"work_actions", "assignee", "assignee_user_id"},
	{"work_activities", "actor", "actor_user_id"},
	{"as_work_items", "assigned_to", "assigned_user_id"},
}

func (r *UserRepo) UsernameTaken(username, exceptUserID string) (bool, error) {
	username = strings.TrimSpace(username)
	if r == nil || r.db == nil || username == "" {
		return false, fmt.Errorf("아이디가 비었습니다")
	}
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM users WHERE username=? AND user_id!=?`, username, strings.TrimSpace(exceptUserID)).Scan(&n)
	return n > 0, err
}

func (r *UserRepo) RenameUsername(userID, newUsername string) error {
	userID = strings.TrimSpace(userID)
	newUsername = strings.TrimSpace(newUsername)
	if r == nil || r.db == nil || userID == "" || newUsername == "" {
		return fmt.Errorf("아이디가 비었습니다")
	}
	taken, err := r.UsernameTaken(newUsername, userID)
	if err != nil {
		return err
	}
	if taken {
		return fmt.Errorf("이미 쓰는 아이디입니다")
	}
	u, err := r.GetByID(userID)
	if err != nil {
		return err
	}
	if u == nil {
		return fmt.Errorf("계정이 없습니다")
	}
	if u.Username == newUsername {
		return nil
	}
	old := u.Username
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err = r.db.Exec(`UPDATE users SET username=?, username_changed_at=? WHERE user_id=?`, newUsername, now, userID)
	if err != nil {
		return err
	}
	audit.Use(r.db)
	audit.LogWithReason("update", "users", "user_id", userID, u.FullName, old, newUsername, "아이디 변경")
	return nil
}

func (r *UserRepo) RewriteAssigneeNames(userID, oldName, newName string) (int, []UnmatchedAssignee, error) {
	if r == nil || r.db == nil {
		return 0, nil, fmt.Errorf("db")
	}
	userID = strings.TrimSpace(userID)
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if userID == "" || newName == "" || oldName == newName {
		return 0, nil, nil
	}
	var total int
	for _, t := range assigneeNameTargets {
		if !sqliteTableExists(r.db, t.table) || !tableHasColumn(r.db, t.table, t.nameCol) {
			continue
		}
		if tableHasColumn(r.db, t.table, t.idCol) {
			res, err := r.db.Exec(`UPDATE "`+t.table+`" SET "`+t.nameCol+`"=? WHERE "`+t.idCol+`"=?`, newName, userID)
			if err != nil {
				return total, nil, err
			}
			n, _ := res.RowsAffected()
			total += int(n)
		}
	}
	if sqliteTableExists(r.db, "sales_activities") && tableHasColumn(r.db, "sales_activities", "our_members") {
		res, err := r.db.Exec(`UPDATE sales_activities SET our_members=? WHERE TRIM(our_members)=?`, newName, oldName)
		if err != nil {
			return total, nil, err
		}
		n, _ := res.RowsAffected()
		total += int(n)
	}
	if sqliteTableExists(r.db, "sales_activity_members") {
		res, err := r.db.Exec(`UPDATE sales_activity_members SET member=? WHERE member=?`, newName, oldName)
		if err == nil {
			n, _ := res.RowsAffected()
			total += int(n)
		}
	}
	rememberUserName(userID, newName)
	return total, unmatchedAfterNameChange(r.db, oldName), nil
}

func (r *UserRepo) UnmatchedAfterNameChange(oldName string) []UnmatchedAssignee {
	if r == nil {
		return nil
	}
	return unmatchedAfterNameChange(r.db, oldName)
}

func leftoverNameRows(db *sql.DB, oldName string) []UnmatchedAssignee {
	oldName = strings.TrimSpace(oldName)
	if oldName == "" {
		return nil
	}
	var out []UnmatchedAssignee
	for _, t := range assigneeNameTargets {
		if !sqliteTableExists(db, t.table) || !tableHasColumn(db, t.table, t.nameCol) {
			continue
		}
		q := `SELECT COUNT(*) FROM "` + t.table + `" WHERE TRIM("` + t.nameCol + `")=?`
		if tableHasColumn(db, t.table, t.idCol) {
			q += ` AND TRIM(COALESCE("` + t.idCol + `",''))=''`
		}
		var n int
		if err := db.QueryRow(q, oldName).Scan(&n); err != nil || n == 0 {
			continue
		}
		out = append(out, UnmatchedAssignee{Table: t.table, Column: t.nameCol, Name: oldName, Count: n})
	}
	if sqliteTableExists(db, "sales_activities") && tableHasColumn(db, "sales_activities", "our_members") {
		var n int
		_ = db.QueryRow(`SELECT COUNT(*) FROM sales_activities WHERE TRIM(our_members)=?`, oldName).Scan(&n)
		if n > 0 {
			out = append(out, UnmatchedAssignee{Table: "sales_activities", Column: "our_members", Name: oldName, Count: n})
		}
	}
	return out
}

func unmatchedAfterNameChange(db *sql.DB, oldName string) []UnmatchedAssignee {
	oldName = strings.TrimSpace(oldName)
	out := leftoverNameRows(db, oldName)
	seen := map[string]struct{}{}
	for _, it := range out {
		seen[it.Table+"\t"+it.Column+"\t"+it.Name] = struct{}{}
	}
	for _, it := range ListUnmatchedAssignees(db) {
		if strings.TrimSpace(it.Name) != oldName {
			continue
		}
		key := it.Table + "\t" + it.Column + "\t" + it.Name
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, it)
	}
	return out
}
