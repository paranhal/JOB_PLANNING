package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"customer-support/internal/model"
)

type ProjectRepo struct {
	db *sql.DB
}

func NewProjectRepo(db *sql.DB) *ProjectRepo {
	return &ProjectRepo{db: db}
}

const projectSelect = `
	SELECT p.project_id, p.name, COALESCE(p.short_name,''), COALESCE(p.plan_year,0),
		COALESCE(p.is_paid,1), COALESCE(p.sort_order,0),
		COALESCE(p.ordering_party_id,''), COALESCE(p.ordering_party,''), COALESCE(p.customer_id,''),
		COALESCE(p.contract_type,''), COALESCE(p.billing_type,''),
		COALESCE(p.start_date,''), COALESCE(p.end_date,''),
		COALESCE(p.notes,''), COALESCE(p.contact_id,''),
		COALESCE(p.color,'#3B82F6'), COALESCE(p.status,'active'),
		COALESCE(p.project_kind,'maintenance'),
		COALESCE(p.sales_stage,''), COALESCE(p.expected_ym,''), COALESCE(p.expected_precision,'month'),
		COALESCE(p.expected_note,''), COALESCE(p.expected_undated_reason,''),
		COALESCE(p.prospect_name,''), COALESCE(p.prospect_region,''),
		COALESCE(p.prospect_contact_name,''), COALESCE(p.prospect_contact_title,''),
		COALESCE(p.prospect_contact_phone,''), COALESCE(p.prospect_contact_email,''),
		COALESCE(p.sales_owner,''), COALESCE(p.sales_owner_id,''),
		COALESCE(p.expected_amount,0), COALESCE(p.win_probability,0),
		COALESCE(p.competitor,''), COALESCE(p.lead_source,''),
		p.created_at, p.updated_at,
		COALESCE(cu.org_name,''), COALESCE(ct.full_name,''), COALESCE(op.org_name,'')
	FROM work_projects p
	LEFT JOIN customers cu ON cu.customer_id = p.customer_id
	LEFT JOIN contacts ct ON ct.contact_id = p.contact_id
	LEFT JOIN customers op ON op.customer_id = p.ordering_party_id`

func (r *ProjectRepo) List(year int, status string) ([]model.WorkProject, error) {
	return r.ListFiltered("", year, status, model.ProjectKindMaintenance, false)
}

// ListForTab §22.1.1 ⑧ 전체 / 영업(수주 전) / 진행 / 종료
func (r *ProjectRepo) ListForTab(tab, search string, year int, extraStatus string, includeLost bool) ([]model.WorkProject, error) {
	tab = model.NormalizeProjectTab(tab)
	q := projectSelect + ` WHERE 1=1`
	var args []interface{}
	switch tab {
	case model.ProjectTabAll:
		if !includeLost {
			q += ` AND COALESCE(p.sales_stage,'') NOT IN (?,?)`
			args = append(args, model.SalesStageLost, model.SalesStageDropped)
		}
	case model.ProjectTabSales:
		q += ` AND COALESCE(p.project_kind,'maintenance') != ?`
		args = append(args, model.ProjectKindMaintenance)
		if includeLost {
			q += ` AND COALESCE(p.sales_stage,'') != ?`
			args = append(args, model.SalesStageWon)
		} else {
			q += ` AND COALESCE(p.sales_stage,'') NOT IN (?,?,?)`
			args = append(args, model.SalesStageWon, model.SalesStageLost, model.SalesStageDropped)
		}
	case model.ProjectTabClosed:
		q += ` AND (p.status IN (?,?) OR COALESCE(p.sales_stage,'') IN (?,?))`
		args = append(args, model.WBProjectComplete, model.WBProjectArchived, model.SalesStageLost, model.SalesStageDropped)
	default: // 진행
		q += ` AND p.status=? AND (COALESCE(p.project_kind,'maintenance')=? OR COALESCE(p.sales_stage,'')=?)`
		args = append(args, model.WBProjectActive, model.ProjectKindMaintenance, model.SalesStageWon)
	}
	if year > 0 {
		q += ` AND COALESCE(p.plan_year,0)=?`
		args = append(args, year)
	}
	if extraStatus != "" && tab != model.ProjectTabActive && tab != model.ProjectTabClosed {
		q += ` AND p.status=?`
		args = append(args, extraStatus)
	}
	if s := strings.TrimSpace(search); s != "" {
		like := "%" + s + "%"
		q += ` AND (
			p.name LIKE ? OR COALESCE(p.short_name,'') LIKE ?
			OR COALESCE(p.ordering_party,'') LIKE ? OR COALESCE(op.org_name,'') LIKE ?
			OR COALESCE(cu.org_name,'') LIKE ? OR COALESCE(p.prospect_name,'') LIKE ?
		)`
		args = append(args, like, like, like, like, like, like)
	}
	if tab == model.ProjectTabSales {
		q += ` ORDER BY COALESCE(p.expected_ym,''), COALESCE(p.sort_order,0), p.name`
	} else {
		q += ` ORDER BY COALESCE(p.sort_order,0), COALESCE(p.plan_year,0) DESC, p.name`
	}
	rows, err := r.db.Query(q, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return scanProjectRows(rows)
}

func (r *ProjectRepo) SetSalesStage(id, stage string) error {
	id = strings.TrimSpace(id)
	stage = model.NormalizeSalesStage(stage)
	if id == "" || stage == "" {
		return fmt.Errorf("project_id·sales_stage 필요")
	}
	p, err := r.Get(id)
	if err != nil {
		return err
	}
	p.SalesStage = stage
	if err := model.ValidateWorkProject(p); err != nil {
		return err
	}
	return r.Update(p)
}

// ListFiltered 사업명·발주처·고객 검색 + 연도·상태·유형 필터.
// kind 가 비면 유지보수만(§22.1.1 기본). kind=all 이면 유형을 가리지 않는다.
// kind=sales 이면 유지보수 외. 영업 목록은 기본으로 lost·dropped 를 숨긴다.
func (r *ProjectRepo) ListFiltered(search string, year int, status, kind string, includeLost bool) ([]model.WorkProject, error) {
	q := projectSelect + ` WHERE 1=1`
	var args []interface{}
	kind = strings.TrimSpace(kind)
	if kind == "" {
		kind = model.ProjectKindMaintenance
	}
	switch {
	case strings.EqualFold(kind, "all"):
	case strings.EqualFold(kind, "sales"):
		q += ` AND COALESCE(p.project_kind,'maintenance') != ?`
		args = append(args, model.ProjectKindMaintenance)
	default:
		q += ` AND COALESCE(p.project_kind,'maintenance')=?`
		args = append(args, model.NormalizeProjectKind(kind))
	}
	if year > 0 {
		q += ` AND COALESCE(p.plan_year,0)=?`
		args = append(args, year)
	}
	if status != "" {
		q += ` AND p.status=?`
		args = append(args, status)
	}
	hideLost := !includeLost && (strings.EqualFold(kind, "sales") || model.IsSalesKind(kind))
	if hideLost {
		q += ` AND COALESCE(p.sales_stage,'') NOT IN (?,?)`
		args = append(args, model.SalesStageLost, model.SalesStageDropped)
	}
	if s := strings.TrimSpace(search); s != "" {
		like := "%" + s + "%"
		q += ` AND (
			p.name LIKE ? OR COALESCE(p.short_name,'') LIKE ?
			OR COALESCE(p.ordering_party,'') LIKE ? OR COALESCE(op.org_name,'') LIKE ?
			OR COALESCE(cu.org_name,'') LIKE ? OR COALESCE(p.prospect_name,'') LIKE ?
		)`
		args = append(args, like, like, like, like, like, like)
	}
	if strings.EqualFold(kind, "sales") || model.IsSalesKind(kind) {
		q += ` ORDER BY COALESCE(p.expected_ym,''), COALESCE(p.sort_order,0), p.name`
	} else {
		q += ` ORDER BY COALESCE(p.sort_order,0), COALESCE(p.plan_year,0) DESC, p.name`
	}
	rows, err := r.db.Query(q, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return scanProjectRows(rows)
}

// SetStatus 사업 상태만 변경(보관/재개/완료 등)
func (r *ProjectRepo) SetStatus(id, status string) error {
	id = strings.TrimSpace(id)
	status = strings.TrimSpace(status)
	if id == "" || status == "" {
		return fmt.Errorf("project_id·status 필요")
	}
	return touchUpdate(r.db, "work_projects", "project_id", id, "사업", func() error {
		_, err := r.db.Exec(`
			UPDATE work_projects SET status=?, updated_at=CURRENT_TIMESTAMP
			WHERE project_id=?`, status, id)
		return err
	})
}

func (r *ProjectRepo) ListYears() ([]int, error) {
	rows, err := r.db.Query(`
		SELECT DISTINCT plan_year FROM work_projects
		WHERE COALESCE(plan_year,0) > 0
		ORDER BY plan_year DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var years []int
	for rows.Next() {
		var y int
		if err := rows.Scan(&y); err != nil {
			return nil, err
		}
		years = append(years, y)
	}
	return years, rows.Err()
}

func (r *ProjectRepo) Get(id string) (*model.WorkProject, error) {
	row := r.db.QueryRow(projectSelect+` WHERE p.project_id=?`, id)
	p, err := scanProjectRow(row)
	if err != nil {
		return nil, err
	}
	rules, _ := r.ListRules(id)
	p.ScopeRules = rules
	return p, nil
}

func (r *ProjectRepo) Create(p *model.WorkProject) error {
	id, err := NextSeq(r.db, "work_project")
	if err != nil {
		return err
	}
	p.ProjectID = fmt.Sprintf("WP-%03d", id)
	normalizeProject(p)
	args := []interface{}{
		p.ProjectID, p.Name, p.ShortName, p.PlanYear, boolToInt(p.IsPaid), p.SortOrder,
		nullStr(p.OrderingPartyID), p.OrderingParty, nullStr(p.CustomerID),
		p.ContractType, p.BillingType, p.StartDate, p.EndDate, p.Notes, nullStr(p.ContactID),
		p.Color, p.Status, p.ProjectKind,
	}
	args = append(args, salesProjectArgs(p)...)
	_, err = r.db.Exec(`
		INSERT INTO work_projects (
			project_id, name, short_name, plan_year, is_paid, sort_order,
			ordering_party_id, ordering_party, customer_id,
			contract_type, billing_type, start_date, end_date, notes, contact_id,
			color, status, project_kind, `+salesProjectInsertCols()+`
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,`+salesProjectInsertPlaceholders()+`)`,
		args...)
	if err != nil {
		return err
	}
	logCreate(r.db, "work_projects", "project_id", p.ProjectID, p.Name)
	return nil
}

func (r *ProjectRepo) Update(p *model.WorkProject) error {
	if p == nil || p.ProjectID == "" {
		return fmt.Errorf("project_id 필요")
	}
	normalizeProject(p)
	return touchUpdate(r.db, "work_projects", "project_id", p.ProjectID, p.Name, func() error {
		args := []interface{}{
			p.Name, p.ShortName, p.PlanYear, boolToInt(p.IsPaid), p.SortOrder,
			nullStr(p.OrderingPartyID), p.OrderingParty, nullStr(p.CustomerID),
			p.ContractType, p.BillingType, p.StartDate, p.EndDate, p.Notes, nullStr(p.ContactID),
			p.Color, p.Status, p.ProjectKind,
		}
		args = append(args, salesProjectArgs(p)...)
		args = append(args, p.ProjectID)
		_, err := r.db.Exec(`
		UPDATE work_projects SET
			name=?, short_name=?, plan_year=?, is_paid=?, sort_order=?,
			ordering_party_id=?, ordering_party=?, customer_id=?,
			contract_type=?, billing_type=?, start_date=?, end_date=?, notes=?, contact_id=?,
			color=?, status=?, project_kind=?,
			sales_stage=?, expected_ym=?, expected_precision=?, expected_note=?, expected_undated_reason=?,
			prospect_name=?, prospect_region=?,
			prospect_contact_name=?, prospect_contact_title=?, prospect_contact_phone=?, prospect_contact_email=?,
			sales_owner=?, sales_owner_id=?, expected_amount=?, win_probability=?, competitor=?, lead_source=?,
			updated_at=CURRENT_TIMESTAMP
		WHERE project_id=?`, args...)
		return err
	})
}

// LinkCustomer 가등록 고객을 마스터에 연결한다. prospect_name 은 남긴다.
func (r *ProjectRepo) LinkCustomer(projectID, customerID, contactID string) error {
	projectID = strings.TrimSpace(projectID)
	customerID = strings.TrimSpace(customerID)
	if projectID == "" || customerID == "" {
		return fmt.Errorf("project_id·customer_id 필요")
	}
	return touchUpdate(r.db, "work_projects", "project_id", projectID, "사업", func() error {
		if strings.TrimSpace(contactID) != "" {
			_, err := r.db.Exec(`
				UPDATE work_projects SET customer_id=?, contact_id=?, updated_at=CURRENT_TIMESTAMP
				WHERE project_id=?`, customerID, contactID, projectID)
			return err
		}
		_, err := r.db.Exec(`
			UPDATE work_projects SET customer_id=?, updated_at=CURRENT_TIMESTAMP
			WHERE project_id=?`, customerID, projectID)
		return err
	})
}

func (r *ProjectRepo) Delete(id string) error {
	var name string
	_ = r.db.QueryRow(`SELECT name FROM work_projects WHERE project_id=?`, id).Scan(&name)
	return touchDelete(r.db, "work_projects", "project_id", id, name, func() error {
		tx, err := r.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.Exec(`DELETE FROM project_scope_rules WHERE project_id=?`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM work_projects WHERE project_id=?`, id); err != nil {
			return err
		}
		return tx.Commit()
	})
}

func (r *ProjectRepo) CountLinkedTasks(id string) (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE project_id=?`, id).Scan(&n)
	return n, err
}

func (r *ProjectRepo) ListRules(projectID string) ([]model.ProjectScopeRule, error) {
	rows, err := r.db.Query(`
		SELECT r.rule_id, r.project_id, COALESCE(r.parent_customer_id,''),
			COALESCE(r.product_keys,''), COALESCE(r.work_kinds,''), COALESCE(r.notes,''),
			COALESCE(c.org_name,'')
		FROM project_scope_rules r
		LEFT JOIN customers c ON c.customer_id = r.parent_customer_id
		WHERE r.project_id=?
		ORDER BY r.rule_id`, projectID)
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
	return out, rows.Err()
}

func (r *ProjectRepo) ReplaceRules(projectID string, rules []model.ProjectScopeRule) error {
	prepared := make([]model.ProjectScopeRule, 0, len(rules))
	for i, rule := range rules {
		ruleID := strings.TrimSpace(rule.RuleID)
		if ruleID == "" {
			n, err := NextSeq(r.db, "project_scope_rule")
			if err != nil {
				return err
			}
			ruleID = fmt.Sprintf("PSR-%03d", n)
		}
		rule.RuleID = ruleID
		rule.ProjectID = projectID
		prepared = append(prepared, rule)
		_ = i
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM project_scope_rules WHERE project_id=?`, projectID); err != nil {
		return err
	}
	for _, rule := range prepared {
		if _, err := tx.Exec(`
			INSERT INTO project_scope_rules (rule_id, project_id, parent_customer_id, product_keys, work_kinds, notes)
			VALUES (?,?,?,?,?,?)`,
			rule.RuleID, projectID, nullStr(rule.ParentCustomerID),
			strings.TrimSpace(rule.ProductKeys), strings.TrimSpace(rule.WorkKinds), strings.TrimSpace(rule.Notes)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// MatchAS 규칙(상위기관×제품×as)에 맞는 AS 접수. limit>0이면 최근 N건만.
func (r *ProjectRepo) MatchAS(projectID string, limit int) ([]model.ProjectMatchRow, int, error) {
	rules, err := r.ListRules(projectID)
	if err != nil {
		return nil, 0, err
	}
	var asRules []model.ProjectScopeRule
	for _, rule := range rules {
		if rule.HasWorkKind(model.ScopeWorkAS) {
			asRules = append(asRules, rule)
		}
	}
	if len(asRules) == 0 {
		return nil, 0, nil
	}
	conds, args := buildScopeORConds(asRules, "c", "a", "ar.symptom", true)
	if conds == "" {
		return nil, 0, nil
	}
	baseFrom := `
		FROM as_receipts ar
		JOIN customers c ON c.customer_id = ar.customer_id
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE (` + conds + `)`
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) `+baseFrom, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	q := `
		SELECT ar.as_id, COALESCE(ar.as_number,''), COALESCE(date(ar.receipt_datetime),''),
			COALESCE(c.org_name,''), COALESCE(ar.symptom,''), COALESCE(ar.status,''),
			COALESCE(a.product_type,''), COALESCE(a.product_name,'')
		` + baseFrom + `
		ORDER BY ar.receipt_datetime DESC, ar.as_number DESC`
	queryArgs := append([]interface{}{}, args...)
	if limit > 0 {
		q += ` LIMIT ?`
		queryArgs = append(queryArgs, limit)
	}
	rows, err := r.db.Query(q, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []model.ProjectMatchRow
	for rows.Next() {
		var m model.ProjectMatchRow
		var prodType, prodName string
		if err := rows.Scan(&m.ID, &m.Number, &m.Date, &m.OrgName, &m.Title, &m.Status, &prodType, &prodName); err != nil {
			return nil, 0, err
		}
		m.Href = "/as/" + m.ID
		m.StatusLabel = asMatchStatusLabel(m.Status)
		m.ProductHint = firstNonEmpty(prodType, prodName)
		out = append(out, m)
	}
	return out, total, rows.Err()
}

// MatchMaintenance 규칙에 맞는 정기점검 방문.
func (r *ProjectRepo) MatchMaintenance(projectID string, limit int) ([]model.ProjectMatchRow, int, error) {
	rules, err := r.ListRules(projectID)
	if err != nil {
		return nil, 0, err
	}
	var mRules []model.ProjectScopeRule
	for _, rule := range rules {
		if rule.HasWorkKind(model.ScopeWorkMaintenance) {
			mRules = append(mRules, rule)
		}
	}
	if len(mRules) == 0 {
		return nil, 0, nil
	}
	conds, args := buildScopeORConds(mRules, "c", "v", "v.product_type", false)
	if conds == "" {
		return nil, 0, nil
	}
	baseFrom := `
		FROM maintenance_visits v
		JOIN customers c ON c.customer_id = v.customer_id
		WHERE (` + conds + `)`
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) `+baseFrom, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	q := `
		SELECT v.visit_id, COALESCE(v.visit_date,''),
			COALESCE(c.org_name,''), COALESCE(v.notes,''),
			CASE WHEN COALESCE(v.completed,0)=1 THEN 'completed' ELSE 'planned' END,
			COALESCE(v.product_type,''), COALESCE(v.plan_id,'')
		` + baseFrom + `
		ORDER BY v.visit_date DESC, v.visit_id DESC`
	queryArgs := append([]interface{}{}, args...)
	if limit > 0 {
		q += ` LIMIT ?`
		queryArgs = append(queryArgs, limit)
	}
	rows, err := r.db.Query(q, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []model.ProjectMatchRow
	for rows.Next() {
		var m model.ProjectMatchRow
		var planID string
		if err := rows.Scan(&m.ID, &m.Date, &m.OrgName, &m.Title, &m.Status, &m.ProductHint, &planID); err != nil {
			return nil, 0, err
		}
		m.Number = m.ID
		if m.Title == "" {
			m.Title = m.ProductHint
		}
		if m.Status == "completed" {
			m.StatusLabel = "완료"
		} else {
			m.StatusLabel = "예정"
		}
		if planID != "" {
			m.Href = "/maintenance/" + planID
		} else {
			m.Href = "/maintenance"
		}
		out = append(out, m)
	}
	return out, total, rows.Err()
}

// MatchAdminTasks 이 사업에 연결된 행정/지원 일일업무.
func (r *ProjectRepo) MatchAdminTasks(projectID string, limit int) ([]model.ProjectMatchRow, int, error) {
	var total int
	if err := r.db.QueryRow(`
		SELECT COUNT(*) FROM work_tasks
		WHERE project_id=? AND work_type IN ('admin','support')`, projectID).Scan(&total); err != nil {
		return nil, 0, err
	}
	q := `
		SELECT task_id, '', COALESCE(work_date, due_date,''), '',
			COALESCE(title,''), COALESCE(status,''), ''
		FROM work_tasks
		WHERE project_id=? AND work_type IN ('admin','support')
		ORDER BY COALESCE(work_date, due_date) DESC, title`
	args := []interface{}{projectID}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []model.ProjectMatchRow
	for rows.Next() {
		var m model.ProjectMatchRow
		var dummy string
		if err := rows.Scan(&m.ID, &m.Number, &m.Date, &m.OrgName, &m.Title, &m.Status, &dummy); err != nil {
			return nil, 0, err
		}
		m.Number = m.ID
		m.Href = "/workboard/tasks/" + m.ID
		m.StatusLabel = model.WBTaskStatusLabel(m.Status)
		out = append(out, m)
	}
	return out, total, rows.Err()
}

func buildScopeORConds(rules []model.ProjectScopeRule, custAlias, prodAlias, textCol string, asMode bool) (string, []interface{}) {
	var parts []string
	var args []interface{}
	for _, rule := range rules {
		orgSQL, orgArgs := parentOrgSQL(custAlias, rule.ParentCustomerID)
		prodSQL, prodArgs := productKeysSQL(rule.ProductKeys, prodAlias, textCol, asMode)
		inner := "1=1"
		if orgSQL != "" {
			inner += " AND " + orgSQL
			args = append(args, orgArgs...)
		}
		if prodSQL != "" {
			inner += " AND " + prodSQL
			args = append(args, prodArgs...)
		}
		parts = append(parts, "("+inner+")")
	}
	if len(parts) == 0 {
		return "", nil
	}
	return strings.Join(parts, " OR "), args
}

func parentOrgSQL(alias, parentID string) (string, []interface{}) {
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		return "", nil
	}
	return fmt.Sprintf(`(%s.customer_id=? OR %s.parent_customer_id=?)`, alias, alias),
		[]interface{}{parentID, parentID}
}

// matchProductKeys productKeysSQL 과 같은 제품키 판정. ResolveProjectID 가 SQL 없이 쓴다.
func matchProductKeys(keysCSV, text string) bool {
	keys := splitCSV(keysCSV)
	if len(keys) == 0 {
		return true
	}
	u := strings.ToUpper(text)
	for _, k := range keys {
		switch k {
		case model.ProductKeySejongKLAS:
			if strings.Contains(text, "세종") || strings.Contains(u, "SEJONG") {
				return true
			}
		case model.ProductKeyKLAS:
			if (strings.Contains(u, "KLAS") || strings.Contains(u, "K-LAS")) && !strings.Contains(text, "세종") {
				return true
			}
		case model.ProductKeyAnrobotics:
			if strings.Contains(text, "앤로") || strings.Contains(u, "ANROBOT") || strings.Contains(u, "RFID") {
				return true
			}
		}
	}
	return false
}

// productKeysSQL 제품키 매칭. asMode면 asset 컬럼+증상, 아니면 product_type 단일 컬럼(정기점검).
func productKeysSQL(keysCSV, alias, textCol string, asMode bool) (string, []interface{}) {
	keys := splitCSV(keysCSV)
	if len(keys) == 0 {
		return "", nil
	}
	var ors []string
	for _, k := range keys {
		switch k {
		case model.ProductKeySejongKLAS:
			if asMode {
				ors = append(ors, fmt.Sprintf(`(
					COALESCE(%[1]s.product_type,'') LIKE '%%세종%%' OR COALESCE(%[1]s.product_name,'') LIKE '%%세종%%'
					OR COALESCE(%[1]s.manufacturer,'') LIKE '%%세종%%' OR COALESCE(%[2]s,'') LIKE '%%세종%%'
					OR UPPER(COALESCE(%[1]s.product_type,'')) LIKE '%%SEJONG%%'
					OR UPPER(COALESCE(%[1]s.product_name,'')) LIKE '%%SEJONG%%'
				)`, alias, textCol))
			} else {
				ors = append(ors, fmt.Sprintf(`(
					COALESCE(%s,'') LIKE '%%세종%%' OR UPPER(COALESCE(%s,'')) LIKE '%%SEJONG%%'
				)`, textCol, textCol))
			}
		case model.ProductKeyKLAS:
			if asMode {
				ors = append(ors, fmt.Sprintf(`(
					(COALESCE(%[1]s.product_type,'') LIKE '%%KLAS%%' OR COALESCE(%[1]s.product_type,'') LIKE '%%K-LAS%%'
					 OR COALESCE(%[1]s.product_name,'') LIKE '%%KLAS%%' OR COALESCE(%[1]s.manufacturer,'') LIKE '%%KLAS%%'
					 OR COALESCE(%[2]s,'') LIKE '%%KLAS%%' OR COALESCE(%[2]s,'') LIKE '%%K-LAS%%')
					AND COALESCE(%[1]s.product_type,'') NOT LIKE '%%세종%%'
					AND COALESCE(%[1]s.product_name,'') NOT LIKE '%%세종%%'
					AND COALESCE(%[2]s,'') NOT LIKE '%%세종%%'
				)`, alias, textCol))
			} else {
				ors = append(ors, fmt.Sprintf(`(
					(COALESCE(%[1]s,'') LIKE '%%KLAS%%' OR COALESCE(%[1]s,'') LIKE '%%K-LAS%%')
					AND COALESCE(%[1]s,'') NOT LIKE '%%세종%%'
				)`, textCol))
			}
		case model.ProductKeyAnrobotics:
			if asMode {
				ors = append(ors, fmt.Sprintf(`(
					COALESCE(%[1]s.product_type,'') LIKE '%%앤로%%' OR COALESCE(%[1]s.product_name,'') LIKE '%%앤로%%'
					OR COALESCE(%[1]s.manufacturer,'') LIKE '%%앤로%%' OR COALESCE(%[2]s,'') LIKE '%%앤로%%'
					OR UPPER(COALESCE(%[1]s.product_type,'')) LIKE '%%ANROBOT%%'
					OR UPPER(COALESCE(%[1]s.manufacturer,'')) LIKE '%%ANROBOT%%'
					OR COALESCE(%[1]s.product_type,'') LIKE '%%RFID%%' OR COALESCE(%[2]s,'') LIKE '%%RFID%%'
				)`, alias, textCol))
			} else {
				ors = append(ors, fmt.Sprintf(`(
					COALESCE(%[1]s,'') LIKE '%%앤로%%' OR UPPER(COALESCE(%[1]s,'')) LIKE '%%ANROBOT%%'
					OR COALESCE(%[1]s,'') LIKE '%%RFID%%'
				)`, textCol))
			}
		}
	}
	if len(ors) == 0 {
		return "", nil
	}
	return "(" + strings.Join(ors, " OR ") + ")", nil
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func normalizeProject(p *model.WorkProject) {
	if p.Status == "" {
		p.Status = model.WBProjectActive
	}
	p.ProjectKind = model.NormalizeProjectKind(p.ProjectKind)
	p.SalesStage = model.NormalizeSalesStage(p.SalesStage)
	p.ExpectedPrecision = model.NormalizeExpectedPrecision(p.ExpectedPrecision)
	p.ExpectedYM = model.NormalizeExpectedYM(p.ExpectedYM)
	if p.Color == "" {
		p.Color = "#3B82F6"
	}
	if p.PlanYear == 0 && len(p.StartDate) >= 4 {
		var y int
		if _, err := fmt.Sscanf(p.StartDate[:4], "%d", &y); err == nil {
			p.PlanYear = y
		}
	}
}

func asMatchStatusLabel(s string) string {
	m := map[string]string{
		"received": "접수", "assigned": "담당자 배정", "in_progress": "진행중", "hold": "보류",
		"transfer": "이관", "cancelled": "접수취소",
		"partial_complete": "부분완료", "completed": "완료", "closed": "종료",
	}
	if l, ok := m[s]; ok {
		return l
	}
	return s
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

type projectScanner interface {
	Scan(dest ...interface{}) error
}

func scanProjectRow(row projectScanner) (*model.WorkProject, error) {
	var p model.WorkProject
	var created, updated string
	var paid int
	err := row.Scan(
		&p.ProjectID, &p.Name, &p.ShortName, &p.PlanYear, &paid, &p.SortOrder,
		&p.OrderingPartyID, &p.OrderingParty, &p.CustomerID,
		&p.ContractType, &p.BillingType,
		&p.StartDate, &p.EndDate,
		&p.Notes, &p.ContactID,
		&p.Color, &p.Status, &p.ProjectKind,
		&p.SalesStage, &p.ExpectedYM, &p.ExpectedPrecision,
		&p.ExpectedNote, &p.ExpectedUndatedReason,
		&p.ProspectName, &p.ProspectRegion,
		&p.ProspectContactName, &p.ProspectContactTitle,
		&p.ProspectContactPhone, &p.ProspectContactEmail,
		&p.SalesOwner, &p.SalesOwnerID,
		&p.ExpectedAmount, &p.WinProbability,
		&p.Competitor, &p.LeadSource,
		&created, &updated,
		&p.CustomerName, &p.ContactName, &p.OrderingPartyName,
	)
	if err != nil {
		return nil, err
	}
	p.IsPaid = paid != 0
	p.ProjectKind = model.NormalizeProjectKind(p.ProjectKind)
	p.SalesStage = model.NormalizeSalesStage(p.SalesStage)
	p.ExpectedPrecision = model.NormalizeExpectedPrecision(p.ExpectedPrecision)
	p.ExpectedYM = model.NormalizeExpectedYM(p.ExpectedYM)
	p.CreatedAt = parseTime(created)
	p.UpdatedAt = parseTime(updated)
	return &p, nil
}

func scanProjectRows(rows *sql.Rows) ([]model.WorkProject, error) {
	var items []model.WorkProject
	for rows.Next() {
		p, err := scanProjectRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *p)
	}
	return items, rows.Err()
}
