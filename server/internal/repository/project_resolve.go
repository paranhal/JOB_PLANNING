package repository

import (
	"database/sql"
	"log"
	"strings"

	"customer-support/internal/model"
)

const maxCustomerAncestors = 32

// ResolveProjectID 고객의 조상 전체를 타고 올라가 project_scope_rules 와 매칭한다.
// 직속 부모만 보면 세종처럼 2단 상위기관은 빠진다 (§16.6.9).
// 매칭 실패는 ("", nil). DB 오류만 error 로 돌린다.
func (r *ProjectRepo) ResolveProjectID(customerID, productType, workKind string) (string, error) {
	customerID = strings.TrimSpace(customerID)
	workKind = strings.TrimSpace(workKind)
	if customerID == "" || workKind == "" {
		return "", nil
	}
	ancestors, err := r.customerAncestorIDs(customerID)
	if err != nil {
		return "", err
	}
	if len(ancestors) == 0 {
		return "", nil
	}
	rules, err := r.listAllScopeRules()
	if err != nil {
		return "", err
	}
	ancIdx := make(map[string]int, len(ancestors))
	for i, id := range ancestors {
		ancIdx[id] = i
	}

	type cand struct {
		projectID   string
		ancestorIdx int
		productKeys int
	}
	var best *cand
	better := func(c cand) bool {
		if best == nil {
			return true
		}
		if c.ancestorIdx != best.ancestorIdx {
			return c.ancestorIdx < best.ancestorIdx
		}
		if c.productKeys != best.productKeys {
			return c.productKeys < best.productKeys
		}
		return c.projectID < best.projectID
	}
	for _, rule := range rules {
		if !rule.HasWorkKind(workKind) {
			continue
		}
		if !matchProductKeys(rule.ProductKeys, productType) {
			continue
		}
		parent := strings.TrimSpace(rule.ParentCustomerID)
		if parent == "" {
			continue
		}
		idx, ok := ancIdx[parent]
		if !ok {
			continue
		}
		nKeys := len(rule.ProductKeySlice())
		if nKeys == 0 {
			nKeys = 99
		}
		c := cand{projectID: rule.ProjectID, ancestorIdx: idx, productKeys: nKeys}
		if better(c) {
			cp := c
			best = &cp
		}
	}
	if best == nil {
		return "", nil
	}
	return best.projectID, nil
}

// customerAncestorIDs 본인 → 부모 → 조부모 … (순환·깊이 제한).
func (r *ProjectRepo) customerAncestorIDs(customerID string) ([]string, error) {
	customerID = strings.TrimSpace(customerID)
	if customerID == "" {
		return nil, nil
	}
	var chain []string
	seen := map[string]bool{}
	cur := customerID
	for i := 0; i < maxCustomerAncestors; i++ {
		if cur == "" || seen[cur] {
			break
		}
		seen[cur] = true
		var parent string
		err := r.db.QueryRow(`
			SELECT TRIM(COALESCE(parent_customer_id,'')) FROM customers WHERE customer_id=?`, cur).Scan(&parent)
		if err == sql.ErrNoRows {
			break
		}
		if err != nil {
			return chain, err
		}
		chain = append(chain, cur)
		cur = strings.TrimSpace(parent)
	}
	return chain, nil
}

func (r *ProjectRepo) listAllScopeRules() ([]model.ProjectScopeRule, error) {
	rows, err := r.db.Query(`
		SELECT r.rule_id, r.project_id, COALESCE(r.parent_customer_id,''),
			COALESCE(r.product_keys,''), COALESCE(r.work_kinds,''), COALESCE(r.notes,''),
			COALESCE(c.org_name,'')
		FROM project_scope_rules r
		LEFT JOIN customers c ON c.customer_id = r.parent_customer_id
		ORDER BY r.project_id, r.rule_id`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.ProjectScopeRule
	for rows.Next() {
		var rule model.ProjectScopeRule
		if err := rows.Scan(&rule.RuleID, &rule.ProjectID, &rule.ParentCustomerID,
			&rule.ProductKeys, &rule.WorkKinds, &rule.Notes, &rule.ParentOrgName); err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	overlayScopeJunction(r.db, out)
	return out, nil
}

func resolveStoredProjectID(db *sql.DB, customerID, productType, workKind string) string {
	pid, err := NewProjectRepo(db).ResolveProjectID(customerID, productType, workKind)
	if err != nil {
		log.Printf("ResolveProjectID: %v", err)
		return ""
	}
	return pid
}

func lookupASProductText(db *sql.DB, assetID, symptom string) string {
	var pt, pn, mf string
	if id := strings.TrimSpace(assetID); id != "" {
		_ = db.QueryRow(`
			SELECT COALESCE(product_type,''), COALESCE(product_name,''), COALESCE(manufacturer,'')
			FROM assets WHERE asset_id=?`, id).Scan(&pt, &pn, &mf)
	}
	parts := []string{pt, pn, mf, strings.TrimSpace(symptom)}
	return strings.TrimSpace(strings.Join(parts, " "))
}

// applyASReceiptsProjectID 마이그레이션 005. 컬럼 추가 후 빈 project_id 를 백필한다.
func applyASReceiptsProjectID(db *sql.DB) {
	if _, err := db.Exec(`ALTER TABLE as_receipts ADD COLUMN project_id TEXT`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		log.Printf("005 as_receipts.project_id: %v", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_as_receipts_project ON as_receipts(project_id)`); err != nil {
		log.Printf("005 idx_as_receipts_project: %v", err)
	}
	backfillStoredProjectIDs(db)
}

func backfillStoredProjectIDs(db *sql.DB) {
	repo := NewProjectRepo(db)

	type visitRow struct{ id, cust, prod string }
	var visits []visitRow
	vrows, err := db.Query(`
		SELECT visit_id, customer_id, COALESCE(product_type,'')
		FROM maintenance_visits
		WHERE TRIM(COALESCE(project_id,'')) = ''`)
	if err != nil {
		log.Printf("project_id backfill visits query: %v", err)
	} else {
		for vrows.Next() {
			var row visitRow
			if err := vrows.Scan(&row.id, &row.cust, &row.prod); err == nil {
				visits = append(visits, row)
			}
		}
		vrows.Close()
	}
	vFilled, vMiss := 0, 0
	for _, row := range visits {
		pid, err := repo.ResolveProjectID(row.cust, row.prod, model.ScopeWorkMaintenance)
		if err != nil || pid == "" {
			vMiss++
			continue
		}
		if _, err := db.Exec(`UPDATE maintenance_visits SET project_id=? WHERE visit_id=?`, pid, row.id); err != nil {
			vMiss++
			continue
		}
		vFilled++
	}

	type asRow struct{ id, cust, assetID, symptom string }
	var receipts []asRow
	arows, err := db.Query(`
		SELECT as_id, customer_id, COALESCE(asset_id,''), COALESCE(symptom,'')
		FROM as_receipts
		WHERE TRIM(COALESCE(project_id,'')) = ''`)
	if err != nil {
		log.Printf("project_id backfill as query: %v", err)
	} else {
		for arows.Next() {
			var row asRow
			if err := arows.Scan(&row.id, &row.cust, &row.assetID, &row.symptom); err == nil {
				receipts = append(receipts, row)
			}
		}
		arows.Close()
	}
	aFilled, aMiss := 0, 0
	for _, row := range receipts {
		text := lookupASProductText(db, row.assetID, row.symptom)
		pid, err := repo.ResolveProjectID(row.cust, text, model.ScopeWorkAS)
		if err != nil || pid == "" {
			aMiss++
			continue
		}
		if _, err := db.Exec(`UPDATE as_receipts SET project_id=? WHERE as_id=?`, pid, row.id); err != nil {
			aMiss++
			continue
		}
		aFilled++
	}

	var visitNull, asNull int
	_ = db.QueryRow(`SELECT COUNT(*) FROM maintenance_visits WHERE project_id IS NULL OR TRIM(project_id)=''`).Scan(&visitNull)
	_ = db.QueryRow(`SELECT COUNT(*) FROM as_receipts WHERE project_id IS NULL OR TRIM(project_id)=''`).Scan(&asNull)
	log.Printf("project_id backfill visits: filled=%d unresolved=%d", vFilled, vMiss)
	log.Printf("project_id backfill as_receipts: filled=%d unresolved=%d", aFilled, aMiss)
	log.Printf("SELECT COUNT(*) FROM maintenance_visits WHERE project_id IS NULL → %d", visitNull)
	log.Printf("SELECT COUNT(*) FROM as_receipts WHERE project_id IS NULL → %d", asNull)
}

// ListUnresolvedProjectRows 해석 실패(project_id 비어 있음) 목록. 관리 > 데이터 관리.
func (r *ProjectRepo) ListUnresolvedProjectRows() ([]model.UnresolvedProjectRow, error) {
	var out []model.UnresolvedProjectRow

	vrows, err := r.db.Query(`
		SELECT v.visit_id, COALESCE(v.visit_date,''), v.customer_id,
		       COALESCE(c.org_name,''), COALESCE(v.product_type,''), COALESCE(v.plan_id,'')
		FROM maintenance_visits v
		LEFT JOIN customers c ON c.customer_id = v.customer_id
		WHERE TRIM(COALESCE(v.project_id,'')) = ''
		ORDER BY v.visit_date DESC, v.visit_id DESC`)
	if err != nil {
		return nil, err
	}
	defer vrows.Close()
	for vrows.Next() {
		var row model.UnresolvedProjectRow
		var planID string
		if err := vrows.Scan(&row.ID, &row.Date, &row.CustomerID, &row.OrgName, &row.ProductType, &planID); err != nil {
			return nil, err
		}
		row.Kind = model.ScopeWorkMaintenance
		row.KindLabel = "정기점검"
		row.Number = row.ID
		row.Reason = r.unresolvedReason(row.CustomerID, row.ProductType, model.ScopeWorkMaintenance)
		if planID != "" {
			row.Href = "/maintenance/" + planID
		} else {
			row.Href = "/maintenance"
		}
		out = append(out, row)
	}
	if err := vrows.Err(); err != nil {
		return nil, err
	}

	arows, err := r.db.Query(`
		SELECT ar.as_id, COALESCE(ar.as_number,''), COALESCE(date(ar.receipt_datetime),''),
		       ar.customer_id, COALESCE(c.org_name,''),
		       COALESCE(a.product_type,''), COALESCE(a.product_name,''), COALESCE(ar.symptom,'')
		FROM as_receipts ar
		LEFT JOIN customers c ON c.customer_id = ar.customer_id
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE TRIM(COALESCE(ar.project_id,'')) = ''
		ORDER BY ar.receipt_datetime DESC, ar.as_number DESC`)
	if err != nil {
		return nil, err
	}
	defer arows.Close()
	for arows.Next() {
		var row model.UnresolvedProjectRow
		var prodType, prodName, symptom string
		if err := arows.Scan(&row.ID, &row.Number, &row.Date, &row.CustomerID, &row.OrgName,
			&prodType, &prodName, &symptom); err != nil {
			return nil, err
		}
		row.Kind = model.ScopeWorkAS
		row.KindLabel = "AS"
		row.ProductType = firstNonEmpty(prodType, prodName)
		row.Reason = r.unresolvedReason(row.CustomerID, strings.Join([]string{prodType, prodName, symptom}, " "), model.ScopeWorkAS)
		row.Href = "/as/" + row.ID
		out = append(out, row)
	}
	return out, arows.Err()
}

func (r *ProjectRepo) unresolvedReason(customerID, productType, workKind string) string {
	ancestors, err := r.customerAncestorIDs(customerID)
	if err != nil {
		return "조회 오류"
	}
	if len(ancestors) == 0 {
		return "고객 없음"
	}
	rules, err := r.listAllScopeRules()
	if err != nil || len(rules) == 0 {
		return "범위 규칙 없음"
	}
	hasParent, hasKind := false, false
	for _, rule := range rules {
		parent := strings.TrimSpace(rule.ParentCustomerID)
		if parent == "" {
			continue
		}
		inTree := false
		for _, a := range ancestors {
			if a == parent {
				inTree = true
				break
			}
		}
		if !inTree {
			continue
		}
		hasParent = true
		if !rule.HasWorkKind(workKind) {
			continue
		}
		hasKind = true
		if matchProductKeys(rule.ProductKeys, productType) {
			return "해석 재시도 필요"
		}
	}
	if !hasParent {
		if len(ancestors) <= 1 {
			return "상위기관 미연결"
		}
		return "상위기관이 범위 규칙과 다름"
	}
	if !hasKind {
		return "업무유형 불일치"
	}
	return "제품 불일치"
}
