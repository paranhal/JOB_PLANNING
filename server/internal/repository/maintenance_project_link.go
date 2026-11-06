package repository

import (
	"database/sql"
	"fmt"
	"strings"
)

// SetVisitProject 방문의 project_id 만 바꾼다. 날짜·고객·담당자·완료는 건드리지 않는다.
func (r *MaintenanceRepo) SetVisitProject(orgID, visitID, projectID string) (beforeName, afterName string, err error) {
	if r == nil || r.db == nil {
		return "", "", fmt.Errorf("점검 저장소가 없습니다")
	}
	orgID, err = RequireInsertOrg(orgID)
	if err != nil {
		return "", "", err
	}
	visitID = strings.TrimSpace(visitID)
	if visitID == "" {
		return "", "", fmt.Errorf("업무가 없습니다")
	}
	var beforeID, planOrg string
	err = r.db.QueryRow(`
		SELECT COALESCE(v.project_id,''), COALESCE(mp.org_id,'')
		  FROM maintenance_visits v
		  LEFT JOIN maintenance_plans mp ON mp.plan_id = v.plan_id
		 WHERE v.visit_id=?`, visitID).Scan(&beforeID, &planOrg)
	if err == sql.ErrNoRows {
		return "", "", fmt.Errorf("업무가 없습니다")
	}
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(planOrg) != orgID {
		return "", "", fmt.Errorf("업무가 없습니다")
	}
	beforeName = workProjectName(r.db, beforeID)
	projectID = strings.TrimSpace(projectID)
	if projectID != "" {
		if err := AssertProjectRegisterAccess(r.db, projectID, orgID); err != nil {
			return "", "", err
		}
		afterName = workProjectName(r.db, projectID)
		if afterName == "" {
			var n int
			_ = r.db.QueryRow(`SELECT COUNT(*) FROM work_projects WHERE project_id=?`, projectID).Scan(&n)
			if n == 0 {
				return "", "", fmt.Errorf("사업을 찾을 수 없습니다")
			}
		}
	}
	if _, err := r.db.Exec(`UPDATE maintenance_visits SET project_id=? WHERE visit_id=?`,
		emptyToNullStr(projectID), visitID); err != nil {
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
	logUpdate(r.db, "maintenance_visits", "visit_id", visitID, label, beforeID)
	return beforeName, afterName, nil
}
