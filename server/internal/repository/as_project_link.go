package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

// SetASReceiptProject AS 조치 화면과 같은 사업 연결. org 검사·이력. as_work_items 에는 project_id 가 없다.
func (r *ASRepo) SetASReceiptProject(orgID, asID, projectID string) (beforeName, afterName string, err error) {
	if r == nil || r.db == nil {
		return "", "", fmt.Errorf("AS 저장소가 없습니다")
	}
	orgID, err = RequireInsertOrg(orgID)
	if err != nil {
		return "", "", err
	}
	asID = strings.TrimSpace(asID)
	if asID == "" {
		return "", "", fmt.Errorf("업무가 없습니다")
	}
	var beforeID, rowOrg string
	err = r.db.QueryRow(`
		SELECT COALESCE(project_id,''), COALESCE(org_id,'')
		  FROM as_receipts WHERE as_id=?`, asID).Scan(&beforeID, &rowOrg)
	if err == sql.ErrNoRows {
		return "", "", fmt.Errorf("업무가 없습니다")
	}
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(rowOrg) != orgID {
		return "", "", fmt.Errorf("업무가 없습니다")
	}
	beforeName = workProjectName(r.db, beforeID)
	projectID = strings.TrimSpace(projectID)
	if projectID != "" {
		afterName = workProjectName(r.db, projectID)
		if afterName == "" {
			var n int
			_ = r.db.QueryRow(`SELECT COUNT(*) FROM work_projects WHERE project_id=?`, projectID).Scan(&n)
			if n == 0 {
				return "", "", fmt.Errorf("사업을 찾을 수 없습니다")
			}
		}
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	if _, err := r.db.Exec(`UPDATE as_receipts SET project_id=?, updated_at=? WHERE as_id=? AND org_id=?`,
		emptyToNullStr(projectID), now, asID, orgID); err != nil {
		return "", "", err
	}
	label := "사업 연결: "
	if beforeName == "" {
		label += "없음"
	} else {
		label += beforeName
	}
	label += " → "
	if afterName == "" {
		label += "없음"
	} else {
		label += afterName
	}
	logUpdate(r.db, "as_receipts", "as_id", asID, label, beforeID)
	return beforeName, afterName, nil
}

func workProjectName(db *sql.DB, projectID string) string {
	projectID = strings.TrimSpace(projectID)
	if db == nil || projectID == "" {
		return ""
	}
	var name string
	_ = db.QueryRow(`SELECT COALESCE(name,'') FROM work_projects WHERE project_id=?`, projectID).Scan(&name)
	return strings.TrimSpace(name)
}

// SetASPrimaryAssignee AS 주 담당 1명만 바꾼다. AssignUnplanned 의 담당자 UPDATE 와 같다.
func (r *ASRepo) SetASPrimaryAssignee(asID, assignee, assigneeUID string) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("AS 저장소가 없습니다")
	}
	asID = strings.TrimSpace(asID)
	if asID == "" {
		return fmt.Errorf("업무가 없습니다")
	}
	assignee, assigneeUID, ext := bindStaffExternal(r.db, assignee, assigneeUID)
	oldName, oldUID := readAssigneePair(r.db, "as_receipts", "as_id", asID, "assigned_to", "assigned_user_id")
	now := time.Now().Format("2006-01-02 15:04:05")
	if err := r.touchReceipt(asID, func() error {
		_, err := r.db.Exec(`UPDATE as_receipts SET assigned_to=?, assigned_user_id=?, external_assignee=?, updated_at=? WHERE as_id=?`,
			assignee, assigneeUID, ext, now, asID)
		return err
	}); err != nil {
		return err
	}
	stampAssignedIfChanged(r.db, "as_receipts", "as_id", asID, oldName, oldUID, assignee, assigneeUID)
	return nil
}

func (r *ASRepo) ASReceiptOrgAndAssignee(asID string) (orgID, assignedTo, assignedUID, status string, err error) {
	asID = strings.TrimSpace(asID)
	if r == nil || r.db == nil || asID == "" {
		return "", "", "", "", fmt.Errorf("업무가 없습니다")
	}
	err = r.db.QueryRow(`
		SELECT COALESCE(org_id,''), COALESCE(assigned_to,''), COALESCE(assigned_user_id,''), COALESCE(status,'')
		  FROM as_receipts WHERE as_id=?`, asID).Scan(&orgID, &assignedTo, &assignedUID, &status)
	if err == sql.ErrNoRows {
		return "", "", "", "", fmt.Errorf("업무가 없습니다")
	}
	return orgID, assignedTo, assignedUID, status, err
}

func (r *ASRepo) InsertASProjectProcess(asID, orgID, worker, workerUID, content string) error {
	_ = orgID
	p := &model.ASProcess{
		ASID: asID, Worker: worker, WorkContent: content, ProcessDatetime: time.Now(),
	}
	p.ActedByName = worker
	p.ActedByUserID = workerUID
	return NewASProcessRepo(r.db).Create(p)
}
