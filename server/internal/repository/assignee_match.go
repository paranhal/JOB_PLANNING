package repository

import (
	"fmt"
	"strings"

	"customer-support/internal/model"
)

const (
	AssigneeKindAS          = "as"
	AssigneeKindMaintenance = "maintenance"
	AssigneeKindTask        = "task"
)

// assigneeMatchSQL §38.4. kind=as|maintenance|task. key는 표시명 또는 사용자 ID.
// 이름·ID 한쪽만 있으면 users 로 나머지를 찾아, 배정 방식과 무관하게 같은 사람이 걸리게 한다.
func assigneeMatchSQL(kind, alias, key string) (string, []interface{}) {
	return assigneeMatchSQLID(kind, alias, key, "")
}

// assigneeMatchSQLID 이름과 사용자 ID를 둘 다 받는다. 빈 값은 건너뛴다.
func assigneeMatchSQLID(kind, alias, name, userID string) (string, []interface{}) {
	return assigneeMatchKeys(kind, alias, mergeAssigneeKeys(name, userID))
}

// MergeAssigneeKeys 이름·사용자 ID·추가 표시명을 모아 §38.4 매칭에 넘긴다.
func MergeAssigneeKeys(name, userID string, extra ...string) []string {
	return mergeAssigneeKeys(name, userID, extra...)
}

func mergeAssigneeKeys(name, userID string, extra ...string) []string {
	all := make([]string, 0, 2+len(extra))
	all = append(all, name, userID)
	all = append(all, extra...)
	return cleanAssignees(all)
}

func assigneeMatchKeys(kind, alias string, keys []string) (string, []interface{}) {
	expr, args := assigneeMatchExpr(kind, alias, keys)
	if expr == "" {
		return "", nil
	}
	return " AND (" + expr + ")", args
}

func assigneeMatchExpr(kind, alias string, keys []string) (string, []interface{}) {
	keys = cleanAssignees(keys)
	if len(keys) == 0 {
		return "", nil
	}
	alias = normalizeSQLAlias(alias)
	switch kind {
	case AssigneeKindAS:
		core, args := asColsExpr(alias+"assigned_to", alias+"assigned_user_id", keys)
		mem, mArgs := linkedTaskMembersExpr(alias+"as_id", model.WBSourceAS, keys)
		return core + " OR " + mem, append(args, mArgs...)
	case AssigneeKindMaintenance:
		core, args := asColsExpr(alias+"assignee", alias+"assignee_user_id", keys)
		mem, mArgs := linkedTaskMembersExpr(alias+"visit_id", model.WBSourceMaintenance, keys)
		return core + " OR " + mem, append(args, mArgs...)
	case AssigneeKindTask:
		return taskColsExpr(alias, keys)
	default:
		return nameColsExpr(alias+"assignee", keys)
	}
}

func normalizeSQLAlias(alias string) string {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return ""
	}
	if strings.HasSuffix(alias, ".") {
		return alias
	}
	return alias + "."
}

func sqlInPh(n int) string {
	if n < 1 {
		return ""
	}
	ph := make([]string, n)
	for i := range ph {
		ph[i] = "?"
	}
	return strings.Join(ph, ",")
}

func keysAsArgs(keys []string) []interface{} {
	args := make([]interface{}, len(keys))
	for i, k := range keys {
		args[i] = k
	}
	return args
}

func appendKeyArgs(dst []interface{}, keys []string, times int) []interface{} {
	for i := 0; i < times; i++ {
		dst = append(dst, keysAsArgs(keys)...)
	}
	return dst
}

// usersMatchKeys users.user_id · full_name · username 이 keys 중 하나.
func usersMatchKeys(keys []string) (string, []interface{}) {
	keys = cleanAssignees(keys)
	if len(keys) == 0 {
		return "", nil
	}
	in := sqlInPh(len(keys))
	expr := `(u.user_id IN (` + in + `) OR TRIM(u.full_name) IN (` + in + `) OR TRIM(COALESCE(u.username,'')) IN (` + in + `))`
	return expr, appendKeyArgs(nil, keys, 3)
}

func asColsExpr(toCol, uidCol string, keys []string) (string, []interface{}) {
	keys = cleanAssignees(keys)
	if len(keys) == 0 {
		return "", nil
	}
	in := sqlInPh(len(keys))
	uExpr, uArgs := usersMatchKeys(keys)
	expr := fmt.Sprintf(
		`TRIM(COALESCE(%[1]s,'')) IN (%[3]s)`+
			` OR (TRIM(COALESCE(%[2]s,'')) != '' AND TRIM(COALESCE(%[2]s,'')) IN (%[3]s))`+
			` OR EXISTS (SELECT 1 FROM users u WHERE %[4]s AND (`+
			`(TRIM(COALESCE(%[2]s,'')) != '' AND TRIM(%[2]s) = u.user_id)`+
			` OR TRIM(COALESCE(%[1]s,'')) = TRIM(u.full_name)))`,
		toCol, uidCol, in, uExpr)
	args := appendKeyArgs(nil, keys, 2)
	args = append(args, uArgs...)
	return expr, args
}

func nameColsExpr(col string, keys []string) (string, []interface{}) {
	keys = cleanAssignees(keys)
	if len(keys) == 0 {
		return "", nil
	}
	in := sqlInPh(len(keys))
	uExpr, uArgs := usersMatchKeys(keys)
	expr := fmt.Sprintf(
		`TRIM(COALESCE(%[1]s,'')) IN (%[2]s)`+
			` OR EXISTS (SELECT 1 FROM users u WHERE %[3]s`+
			` AND TRIM(COALESCE(%[1]s,'')) IN (TRIM(u.full_name), TRIM(COALESCE(u.username,'')), u.user_id))`,
		col, in, uExpr)
	args := appendKeyArgs(nil, keys, 1)
	args = append(args, uArgs...)
	return expr, args
}

func taskColsExpr(alias string, keys []string) (string, []interface{}) {
	keys = cleanAssignees(keys)
	if len(keys) == 0 {
		return "", nil
	}
	nameExpr, nameArgs := asColsExpr(alias+"assignee", alias+"assignee_user_id", keys)
	memExpr, memArgs := asColsExpr("m.assignee", "m.user_id", keys)
	expr := fmt.Sprintf(`(%s) OR EXISTS (SELECT 1 FROM work_task_members m WHERE m.task_id = %stask_id AND (%s))`,
		nameExpr, alias, memExpr)
	return expr, append(nameArgs, memArgs...)
}

// linkedTaskMembersExpr 원본(AS·정기점검)에 매달린 일일업무 참여자. §38.4 · §7.7
func linkedTaskMembersExpr(sourceIDCol, sourceType string, keys []string) (string, []interface{}) {
	keys = cleanAssignees(keys)
	if len(keys) == 0 {
		return "", nil
	}
	memExpr, memArgs := asColsExpr("m.assignee", "m.user_id", keys)
	expr := fmt.Sprintf(
		`EXISTS (SELECT 1 FROM work_tasks wt JOIN work_task_members m ON m.task_id = wt.task_id`+
			` WHERE TRIM(COALESCE(wt.source_type,'')) = ?`+
			` AND TRIM(COALESCE(wt.source_id,'')) = TRIM(COALESCE(%s,'')) AND (%s))`,
		sourceIDCol, memExpr)
	return expr, append([]interface{}{sourceType}, memArgs...)
}
