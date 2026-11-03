package repository

import (
	"fmt"
	"strings"

	"customer-support/internal/model"
)

func (r *ProjectRepo) ListAssignments(projectID string) ([]model.ProjectAssignment, error) {
	rows, err := r.db.Query(`
		SELECT a.id, a.project_id, a.user_id, a.duty,
			COALESCE(u.full_name, a.user_id), COALESCE(u.deleted_at,'')
		FROM project_assignments a
		LEFT JOIN users u ON u.user_id = a.user_id
		WHERE a.project_id=?
		ORDER BY a.duty, u.full_name`, projectID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.ProjectAssignment
	for rows.Next() {
		var a model.ProjectAssignment
		var deletedAt string
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.UserID, &a.Duty, &a.UserName, &deletedAt); err != nil {
			return nil, err
		}
		a.Deleted = strings.TrimSpace(deletedAt) != ""
		if a.Deleted && !strings.Contains(a.UserName, "삭제됨") {
			a.UserName = strings.TrimSpace(a.UserName) + " (삭제됨)"
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *ProjectRepo) ReplaceAssignments(projectID, createdBy string, items []model.ProjectAssignment) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return fmt.Errorf("project_id 필요")
	}
	seen := map[string]struct{}{}
	var clean []model.ProjectAssignment
	for _, it := range items {
		uid := strings.TrimSpace(it.UserID)
		duty := strings.TrimSpace(it.Duty)
		if uid == "" || !model.ValidProjectDuty(duty) {
			continue
		}
		key := uid + "\x00" + duty
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		u, err := NewUserRepo(r.db).GetByID(uid)
		if err != nil {
			return err
		}
		if u == nil {
			return fmt.Errorf("사용자를 찾을 수 없습니다")
		}
		if u.IsDeleted() || !u.IsActive {
			return fmt.Errorf("삭제·비활성 사용자는 새로 배정할 수 없습니다")
		}
		clean = append(clean, model.ProjectAssignment{ProjectID: projectID, UserID: uid, Duty: duty})
	}
	if err := r.assertDutyAssigneeScope(projectID, clean); err != nil {
		return err
	}
	before := rowJSON(r.db, "work_projects", "project_id", projectID)
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM project_assignments WHERE project_id=?`, projectID); err != nil {
		return err
	}
	for _, it := range clean {
		if _, err := tx.Exec(`
			INSERT INTO project_assignments (project_id, user_id, duty, created_by)
			VALUES (?,?,?,?)`, it.ProjectID, it.UserID, it.Duty, nullStr(createdBy)); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	logUpdate(r.db, "work_projects", "project_id", projectID, "사업 담당자", before)
	return nil
}

func (r *ProjectRepo) assertDutyAssigneeScope(projectID string, items []model.ProjectAssignment) error {
	if r == nil || len(items) == 0 {
		return nil
	}
	p, err := r.Get(projectID)
	if err != nil || p == nil {
		return fmt.Errorf("사업을 찾을 수 없습니다")
	}
	orgIDs := append([]string{strings.TrimSpace(p.OrgID)}, p.RelatedOrgIDs...)
	users := NewUserRepo(r.db)
	duty, err := users.ListAssignableInOrgs(orgIDs)
	if err != nil {
		return err
	}
	acc, err := users.LookupAccountingAssignees()
	if err != nil {
		return err
	}
	dutyOK := map[string]struct{}{}
	for _, u := range duty {
		dutyOK[u.UserID] = struct{}{}
	}
	accOK := map[string]struct{}{}
	for _, u := range acc {
		accOK[u.UserID] = struct{}{}
	}
	for _, it := range items {
		if it.Duty == model.ProjectDutyAccounting {
			if _, ok := accOK[it.UserID]; !ok {
				return fmt.Errorf("회계 담당은 전 조직 활성 구성원만 지정할 수 있습니다")
			}
			continue
		}
		if _, ok := dutyOK[it.UserID]; !ok {
			return fmt.Errorf("주관·유관 부서 구성원만 지정할 수 있습니다")
		}
	}
	return nil
}
