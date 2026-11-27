package repository

import (
	"fmt"
	"strings"
)

// RecommendASProject 완료·진행 AS 의 사업 연결 후보 중 「같은 담당자가 같은 기관(또는 상위 기관)의 접수를 가장 많이 연결한 사업」을 추천한다.
// 근거가 없거나 1·2위가 같으면 추천하지 않는다(임의로 고르지 않는다). 규칙을 코드에 박지 않고 과거 연결 이력에서 읽는다.
// 돌려주는 것: 추천 사업 ID, 사람이 읽는 근거 문구(없으면 빈 문자열).
func (r *ProjectRepo) RecommendASProject(customerID, assignedTo string, candidateIDs []string) (string, string) {
	customerID, assignedTo = strings.TrimSpace(customerID), strings.TrimSpace(assignedTo)
	if r == nil || r.db == nil || customerID == "" || assignedTo == "" || len(candidateIDs) == 0 {
		return "", ""
	}
	rows, err := r.db.Query(`
		SELECT ar.project_id, COUNT(*) AS n
		  FROM as_receipts ar
		  JOIN customers c ON c.customer_id = ar.customer_id
		 WHERE ar.assigned_to = ?
		   AND TRIM(COALESCE(ar.project_id,'')) <> ''
		   AND ar.project_id IN (`+placeholders(len(candidateIDs))+`)
		   AND (ar.customer_id = ? OR c.parent_customer_id = (SELECT parent_customer_id FROM customers WHERE customer_id = ?) AND TRIM(COALESCE(c.parent_customer_id,'')) <> '')
		 GROUP BY ar.project_id
		 ORDER BY n DESC`, append(append([]interface{}{assignedTo}, iface(candidateIDs)...), customerID, customerID)...)
	if err != nil {
		return "", ""
	}
	defer rows.Close()
	type cnt struct {
		id string
		n  int
	}
	var list []cnt
	for rows.Next() {
		var c cnt
		if rows.Scan(&c.id, &c.n) == nil {
			list = append(list, c)
		}
	}
	if len(list) == 0 || (len(list) > 1 && list[0].n == list[1].n) {
		return "", ""
	}
	return list[0].id, fmt.Sprintf("담당 %s이(가) 같은 기관 접수 %d건을 이 사업에 연결했습니다", assignedTo, list[0].n)
}

// RecommendVisitProject 점검 방문의 연결 후보 중 「같은 담당자가 같은 기관(또는 상위 기관)의 방문을 가장 많이 연결한 사업」을 추천한다.
// 근거가 없거나 1·2위가 같으면 추천하지 않는다.
func (r *ProjectRepo) RecommendVisitProject(customerID, assignee string, candidateIDs []string) (string, string) {
	customerID, assignee = strings.TrimSpace(customerID), strings.TrimSpace(assignee)
	if r == nil || r.db == nil || customerID == "" || assignee == "" || len(candidateIDs) == 0 {
		return "", ""
	}
	rows, err := r.db.Query(`
		SELECT mv.project_id, COUNT(*) AS n
		  FROM maintenance_visits mv
		  JOIN customers c ON c.customer_id = mv.customer_id
		 WHERE mv.assignee = ?
		   AND TRIM(COALESCE(mv.project_id,'')) <> ''
		   AND mv.project_id IN (`+placeholders(len(candidateIDs))+`)
		   AND (mv.customer_id = ? OR (TRIM(COALESCE(c.parent_customer_id,'')) <> '' AND c.parent_customer_id = (SELECT parent_customer_id FROM customers WHERE customer_id = ?)))
		 GROUP BY mv.project_id
		 ORDER BY n DESC`, append(append([]interface{}{assignee}, iface(candidateIDs)...), customerID, customerID)...)
	if err != nil {
		return "", ""
	}
	defer rows.Close()
	type cnt struct {
		id string
		n  int
	}
	var list []cnt
	for rows.Next() {
		var c cnt
		if rows.Scan(&c.id, &c.n) == nil {
			list = append(list, c)
		}
	}
	if len(list) == 0 || (len(list) > 1 && list[0].n == list[1].n) {
		return "", ""
	}
	return list[0].id, fmt.Sprintf("담당 %s이(가) 같은 기관 점검 %d건을 이 사업에 연결했습니다", assignee, list[0].n)
}
