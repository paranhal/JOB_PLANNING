package repository

import (
	"strings"

	"customer-support/internal/model"
)

func (r *WorkBoardRepo) ListBucketOnForOrg(bucket, date, mineUserID string, mineKeys []string, limit int, orgID string) ([]model.WorkListItem, error) {
	items, err := r.ListBucketOn(bucket, date, mineUserID, mineKeys, limit)
	if err != nil {
		return nil, err
	}
	return r.filterWorkListForOrg(items, orgID)
}

func (r *WorkBoardRepo) filterWorkListForOrg(items []model.WorkListItem, orgID string) ([]model.WorkListItem, error) {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" || orgID == OrgAll || len(items) == 0 {
		return items, nil
	}
	var asIDs, visitIDs, taskIDs []string
	for _, it := range items {
		id := strings.TrimSpace(it.RefID)
		if id == "" {
			continue
		}
		switch it.Prefix {
		case model.WorkPrefixAS, model.WorkPrefixConfirm, model.WorkPrefixSupport:
			asIDs = append(asIDs, id)
		case model.WorkPrefixMaintenance:
			visitIDs = append(visitIDs, id)
		default:
			taskIDs = append(taskIDs, id)
		}
	}
	// 접수 org_id가 비면 고객 소속(레거시 INSERT)으로 본다. 방문 슬롯과 같은 COALESCE.
	asOK := r.orgIDSet(`SELECT ar.as_id FROM as_receipts ar
		LEFT JOIN customers c ON c.customer_id=ar.customer_id
		WHERE ar.as_id IN (`+placeholders(len(asIDs))+`)
		  AND COALESCE(NULLIF(TRIM(ar.org_id),''), NULLIF(TRIM(c.org_id),''))=?`,
		append(iface(asIDs), orgID)...)
	visitOK := r.orgIDSet(`SELECT mv.visit_id FROM maintenance_visits mv
		JOIN customers c ON c.customer_id=mv.customer_id
		LEFT JOIN maintenance_site_config cfg ON cfg.customer_id=mv.customer_id
		WHERE mv.visit_id IN (`+placeholders(len(visitIDs))+`)
		  AND COALESCE(NULLIF(TRIM(cfg.org_id),''), NULLIF(TRIM(c.org_id),''))=?`,
		append(iface(visitIDs), orgID)...)
	taskOK := map[string]bool{}
	if len(taskIDs) > 0 {
		frag, extra := workTaskVisibleOrgSQL("t", orgID)
		q := `SELECT t.task_id FROM work_tasks t WHERE t.task_id IN (` + placeholders(len(taskIDs)) + `)` + frag
		taskOK = r.orgIDSet(q, append(iface(taskIDs), extra...)...)
	}
	out := make([]model.WorkListItem, 0, len(items))
	for _, it := range items {
		id := strings.TrimSpace(it.RefID)
		keep := false
		switch it.Prefix {
		case model.WorkPrefixAS, model.WorkPrefixConfirm, model.WorkPrefixSupport:
			keep = asOK[id]
		case model.WorkPrefixMaintenance:
			keep = visitOK[id]
		default:
			// 납품 시리얼 미입력은 work_tasks 가 아니다. 조직 필터로 지우면 §36.10 미계획함이 비게 된다.
			if it.Status == "serial" {
				keep = true
			} else {
				keep = taskOK[id]
			}
		}
		if keep {
			out = append(out, it)
		}
	}
	r.maskForeignAssignees(out, orgID)
	return out, nil
}

// maskForeignAssignees 95-215 규칙 O: 미완료 항목의 담당자가 이 조직 소속이 아니면 이름을 비우고
// AssigneeReassign 으로 표시한다(카드는 남긴다). 완료·취소는 실적 이력이라 그대로 둔다. 저장 데이터는 바꾸지 않는다.
func (r *WorkBoardRepo) maskForeignAssignees(items []model.WorkListItem, orgID string) {
	if r == nil || r.db == nil || len(items) == 0 {
		return
	}
	chk := NewAssigneeOrgChecker(r.db)
	for i := range items {
		done := items[i].Status == "done" || items[i].Status == "complete" || items[i].Status == "cancelled"
		if chk.AssigneeNeedsReassign(orgID, items[i].Assignee, done) {
			items[i].Assignee = ""
			items[i].AssigneeReassign = true
		}
	}
}

func placeholders(n int) string {
	if n <= 0 {
		return "NULL"
	}
	s := strings.Repeat("?,", n)
	return s[:len(s)-1]
}

func iface(ids []string) []interface{} {
	out := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		out = append(out, id)
	}
	return out
}

func (r *WorkBoardRepo) orgIDSet(q string, args ...interface{}) map[string]bool {
	out := map[string]bool{}
	if r == nil || r.db == nil || strings.Contains(q, "IN (NULL)") {
		return out
	}
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil && strings.TrimSpace(id) != "" {
			out[id] = true
		}
	}
	return out
}

// maskItemsOwnOrg 95-215 후속(규칙 O): 항목 자신의 조직 기준으로, 미완료 항목의 담당이 다른 조직 직원이면 이름을 비우고
// AssigneeReassign 을 켠다. 목록을 만드는 질의 5곳(AS·업무·점검·일반·작업)이 모두 이 함수를 거치므로 일정표·목록·회의·
// 요약·대시보드가 한 규칙을 쓴다. 완료·취소는 실적 이력이라 그대로 둔다. 표시용 복사본에만 적용되고 저장값은 바꾸지 않는다.
func (r *WorkBoardRepo) maskItemsOwnOrg(items []model.WorkListItem) {
	if r == nil || r.db == nil || len(items) == 0 {
		return
	}
	var chk *AssigneeOrgChecker
	for i := range items {
		it := &items[i]
		if strings.TrimSpace(it.Assignee) == "" || strings.TrimSpace(it.OrgID) == "" {
			continue
		}
		switch it.Status {
		case "done", "complete", "completed", "cancelled", "closed":
			continue
		}
		if chk == nil {
			chk = assigneeOrgSnap(r.db).chk // 1초 캐시: 한 요청에서 질의가 여러 번 불러도 쿼리는 한 번
		}
		if chk.AssigneeNeedsReassign(it.OrgID, it.Assignee, false) {
			it.Assignee = ""
			it.AssigneeReassign = true
		}
	}
}
