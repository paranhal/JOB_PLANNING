package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"customer-support/internal/model"
)

const (
	AssigneeTargetASReceipt         = "as_receipt"
	AssigneeTargetASWorkItem        = "as_work_item"
	AssigneeTargetWorkTask          = "work_task"
	AssigneeTargetMaintenanceVisit  = "maintenance_visit"
)

// AssigneePerson 저장할 담당자 1행.
type AssigneePerson struct {
	UserID       string
	Name         string
	Role         string
	WorkGroupID  string
	SortOrder    int
	DurationMin  int
}

func (p AssigneePerson) owner() bool {
	return strings.TrimSpace(p.Role) == model.WBMemberOwner
}

// SQLAssignedToUser 이 사용자가 주·보조 담당인지 SQL 조각. assigneeMatchSQL 위임.
func SQLAssignedToUser(kind, alias string, keys []string) (string, []interface{}) {
	return assigneeMatchKeys(kind, alias, keys)
}

func primaryPerson(people []AssigneePerson) AssigneePerson {
	for _, p := range people {
		if p.owner() && (strings.TrimSpace(p.UserID) != "" || strings.TrimSpace(p.Name) != "") {
			return p
		}
	}
	if len(people) > 0 {
		return people[0]
	}
	return AssigneePerson{}
}

// SaveAssignees 종류별 담당자 행을 교체하고 단일 컬럼을 주담당으로 미러한다.
func SaveAssignees(db *sql.DB, kind, id string, people []AssigneePerson, groupID string) error {
	kind, id = strings.TrimSpace(kind), strings.TrimSpace(id)
	if db == nil || id == "" {
		return nil
	}
	groupID = strings.TrimSpace(groupID)
	switch kind {
	case AssigneeTargetASReceipt:
		return saveNamedAssignees(db, "as_receipt_assignees", "as_id", "as_receipts", "as_id",
			"assigned_to", "assigned_user_id", id, people, groupID, "")
	case AssigneeTargetASWorkItem:
		return saveNamedAssignees(db, "as_work_item_assignees", "work_id", "as_work_items", "work_id",
			"assigned_to", "assigned_user_id", id, people, groupID, "")
	case AssigneeTargetWorkTask:
		return saveWorkTaskAssignees(db, id, people, groupID)
	case AssigneeTargetMaintenanceVisit:
		r := NewMaintenanceRepo(db)
		items := make([]model.MaintenanceVisitAssignee, 0, len(people))
		for i, p := range people {
			role := strings.TrimSpace(p.Role)
			if role != model.WBMemberSupport {
				role = model.WBMemberOwner
			}
			items = append(items, model.MaintenanceVisitAssignee{
				VisitID: id, UserID: p.UserID, AssigneeName: p.Name, Role: role,
				WorkGroupID: p.WorkGroupID, SortOrder: i,
			})
		}
		return r.ReplaceVisitAssignees(id, items, groupID)
	default:
		return fmt.Errorf("알 수 없는 담당자 대상: %s", kind)
	}
}

func saveNamedAssignees(db *sql.DB, childTable, childFK, parentTable, parentPK, nameCol, uidCol, id string, people []AssigneePerson, groupID, unused string) error {
	_ = unused
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM `+childTable+` WHERE `+childFK+`=?`, id); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return nil
		}
		return err
	}
	seen := map[string]bool{}
	var n int
	for i, p := range people {
		name := strings.TrimSpace(p.Name)
		uid := strings.TrimSpace(p.UserID)
		if name == "" && uid != "" {
			name, uid = bindStaff(db, "", uid)
		} else if name != "" {
			name, uid = bindStaff(db, name, uid)
		}
		if name == "" && uid == "" {
			continue
		}
		key := uid
		if key == "" {
			key = "n:" + name
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		role := strings.TrimSpace(p.Role)
		if role != model.WBMemberSupport {
			role = model.WBMemberOwner
		}
		gid := strings.TrimSpace(p.WorkGroupID)
		order := p.SortOrder
		if order <= 0 {
			order = n
		}
		if _, err := tx.Exec(`INSERT INTO `+childTable+`(`+childFK+`, user_id, assignee_name, role, work_group_id, sort_order)
			VALUES (?,?,?,?,?,?)`, id, uid, name, role, nullIfEmpty(gid), order); err != nil {
			return err
		}
		people[i].Name, people[i].UserID, people[i].Role = name, uid, role
		n++
	}
	owner := primaryPerson(people)
	if owner.Name != "" || owner.UserID != "" {
		q := `UPDATE ` + parentTable + ` SET ` + nameCol + `=?, ` + uidCol + `=? WHERE ` + parentPK + `=?`
		if _, err := tx.Exec(q, owner.Name, nullIfEmpty(owner.UserID), id); err != nil {
			return err
		}
	}
	_ = groupID
	return tx.Commit()
}

func saveWorkTaskAssignees(db *sql.DB, taskID string, people []AssigneePerson, groupID string) error {
	owner := primaryPerson(people)
	name, uid := bindStaff(db, owner.Name, owner.UserID)
	if name != "" || uid != "" {
		if _, err := db.Exec(`UPDATE work_tasks SET assignee=?, assignee_user_id=?, work_group_id=COALESCE(NULLIF(?,''), work_group_id) WHERE task_id=?`,
			name, uid, groupID, taskID); err != nil {
			return err
		}
	}
	supports := make([]model.WorkTaskMember, 0)
	for i, p := range people {
		if strings.TrimSpace(p.Role) == model.WBMemberOwner {
			continue
		}
		if strings.TrimSpace(p.UserID) == strings.TrimSpace(uid) && uid != "" {
			continue
		}
		if strings.TrimSpace(p.Name) == strings.TrimSpace(name) && name != "" {
			continue
		}
		supports = append(supports, model.WorkTaskMember{
			Assignee: p.Name, UserID: p.UserID, Role: model.WBMemberSupport,
			DurationMin: p.DurationMin, SortOrder: i + 1,
		})
	}
	return NewWBRepo(db).ReplaceSupportMembers(taskID, supports)
}

// AssigneesOf 일괄 조회. N+1 금지.
func AssigneesOf(db *sql.DB, kind string, ids []string) (map[string][]AssigneePerson, error) {
	out := map[string][]AssigneePerson{}
	if db == nil || len(ids) == 0 {
		return out, nil
	}
	kind = strings.TrimSpace(kind)
	table, fk := "", ""
	switch kind {
	case AssigneeTargetASReceipt:
		table, fk = "as_receipt_assignees", "as_id"
	case AssigneeTargetASWorkItem:
		table, fk = "as_work_item_assignees", "work_id"
	case AssigneeTargetWorkTask:
		return assigneesOfTasks(db, ids)
	case AssigneeTargetMaintenanceVisit:
		r := NewMaintenanceRepo(db)
		m, err := r.ListVisitAssigneesForVisits(ids)
		if err != nil {
			return out, err
		}
		for id, rows := range m {
			for _, a := range rows {
				out[id] = append(out[id], AssigneePerson{
					UserID: a.UserID, Name: a.AssigneeName, Role: a.Role,
					WorkGroupID: a.WorkGroupID, SortOrder: a.SortOrder,
				})
			}
		}
		return out, nil
	default:
		return out, fmt.Errorf("알 수 없는 담당자 대상: %s", kind)
	}
	clean := uniqueAssigneeIDs(ids)
	for i := 0; i < len(clean); i += 200 {
		end := i + 200
		if end > len(clean) {
			end = len(clean)
		}
		chunk := clean[i:end]
		ph := strings.Repeat("?,", len(chunk))
		ph = strings.TrimSuffix(ph, ",")
		args := make([]any, len(chunk))
		for j, id := range chunk {
			args[j] = id
		}
		q := fmt.Sprintf(`SELECT %s, COALESCE(user_id,''), COALESCE(assignee_name,''), COALESCE(role,''),
			COALESCE(work_group_id,''), COALESCE(sort_order,0)
			FROM %s WHERE %s IN (%s) ORDER BY %s, sort_order, role`, fk, table, fk, ph, fk)
		rows, err := db.Query(q, args...)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "no such table") {
				return out, nil
			}
			return out, err
		}
		for rows.Next() {
			var id string
			var p AssigneePerson
			if err := rows.Scan(&id, &p.UserID, &p.Name, &p.Role, &p.WorkGroupID, &p.SortOrder); err != nil {
				rows.Close()
				return out, err
			}
			out[id] = append(out[id], p)
		}
		rows.Close()
	}
	return out, nil
}

func assigneesOfTasks(db *sql.DB, ids []string) (map[string][]AssigneePerson, error) {
	r := NewWBRepo(db)
	m, err := r.ListMembersByTaskIDs(ids)
	out := map[string][]AssigneePerson{}
	if err != nil {
		return out, err
	}
	for id, rows := range m {
		for _, a := range rows {
			out[id] = append(out[id], AssigneePerson{
				UserID: a.UserID, Name: a.Assignee, Role: a.Role,
				DurationMin: a.DurationMin, SortOrder: a.SortOrder,
			})
		}
	}
	return out, nil
}

func uniqueAssigneeIDs(ids []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func FormatAssigneesLabel(people []AssigneePerson) string {
	if len(people) == 0 {
		return ""
	}
	owner := primaryPerson(people)
	name := strings.TrimSpace(owner.Name)
	if name == "" {
		name = owner.UserID
	}
	n := 0
	for _, p := range people {
		if strings.TrimSpace(p.UserID) != "" || strings.TrimSpace(p.Name) != "" {
			n++
		}
	}
	if n <= 1 {
		return name
	}
	return fmt.Sprintf("%s 외 %d명", name, n-1)
}
