package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"customer-support/internal/model"
)

type SalesRepo struct {
	db       *sql.DB
	codeRepo *CodeRepo
}

func NewSalesRepo(db *sql.DB) *SalesRepo {
	return &SalesRepo{db: db, codeRepo: NewCodeRepo(db)}
}

func (r *SalesRepo) Stages() ([]model.SalesStageDef, error) {
	stageCodes, err := r.codeRepo.ActiveByGroup(model.SalesCodeGroupStage)
	if err != nil {
		return model.LoadSalesStages(nil, nil), err
	}
	probCodes, err := r.codeRepo.ActiveByGroup(model.SalesCodeGroupProb)
	if err != nil {
		return model.LoadSalesStages(stageCodes, nil), err
	}
	return model.LoadSalesStages(stageCodes, probCodes), nil
}

const salesSelect = `
	SELECT s.sales_id, s.name, COALESCE(s.is_tentative_name,0), COALESCE(s.stage,'lead'),
		COALESCE(s.probability,0), s.probability_override,
		COALESCE(s.customer_id,''), COALESCE(s.prospect_name,''), COALESCE(s.prospect_region,''),
		COALESCE(s.customer_confirmed,0),
		COALESCE(s.expected_ym,''), COALESCE(s.expected_precision,'month'), COALESCE(s.expected_ym_confirmed,0),
		COALESCE(s.expected_amount,0), COALESCE(s.expected_amount_confirmed,0),
		COALESCE(s.sales_owner,''), COALESCE(s.sales_owner_id,''),
		COALESCE(s.competitor,''), COALESCE(s.lead_source,''), COALESCE(s.lost_reason,''),
		COALESCE(s.status,'active'), COALESCE(s.notes,''),
		COALESCE(s.created_at,''), COALESCE(s.updated_at,''),
		COALESCE(cu.org_name,'')
	FROM sales_projects s
	LEFT JOIN customers cu ON cu.customer_id = s.customer_id`

func (r *SalesRepo) List(search, status, stage string) ([]model.SalesProject, error) {
	q := salesSelect + ` WHERE 1=1`
	var args []interface{}
	if s := strings.TrimSpace(status); s != "" {
		q += ` AND s.status=?`
		args = append(args, s)
	}
	if s := strings.TrimSpace(stage); s != "" {
		q += ` AND s.stage=?`
		args = append(args, s)
	}
	if s := strings.TrimSpace(search); s != "" {
		like := "%" + s + "%"
		q += ` AND (s.name LIKE ? OR COALESCE(s.prospect_name,'') LIKE ? OR COALESCE(cu.org_name,'') LIKE ?)`
		args = append(args, like, like, like)
	}
	q += ` ORDER BY COALESCE(s.expected_ym,'') DESC, s.updated_at DESC, s.name`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return scanSalesRows(rows)
}

func (r *SalesRepo) Get(id string) (*model.SalesProject, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, sql.ErrNoRows
	}
	row := r.db.QueryRow(salesSelect+` WHERE s.sales_id=?`, id)
	p, err := scanSalesRow(row)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (r *SalesRepo) Create(p *model.SalesProject) error {
	if p == nil {
		return fmt.Errorf("sales project 필요")
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("사업명을 입력하세요")
	}
	stages, err := r.Stages()
	if err != nil && len(stages) == 0 {
		return err
	}
	normalizeSalesProject(p, stages)
	if model.StageNeedsFullConfirm(p.Stage) {
		if err := p.RequireWonConfirmation(); err != nil {
			return err
		}
	}
	id, err := NextSeq(r.db, "sales_project")
	if err != nil {
		return err
	}
	p.SalesID = fmt.Sprintf("SP-%03d", id)
	histN, err := NextSeq(r.db, "sales_stage_history")
	if err != nil {
		return err
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`
		INSERT INTO sales_projects (
			sales_id, name, is_tentative_name, stage, probability, probability_override,
			customer_id, prospect_name, prospect_region, customer_confirmed,
			expected_ym, expected_precision, expected_ym_confirmed,
			expected_amount, expected_amount_confirmed,
			sales_owner, sales_owner_id, competitor, lead_source, lost_reason,
			status, notes
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.SalesID, p.Name, boolToInt(p.IsTentativeName), p.Stage, p.Probability, overrideArg(p),
		nullStr(p.CustomerID), p.ProspectName, p.ProspectRegion, boolToInt(p.CustomerConfirmed),
		p.ExpectedYM, p.ExpectedPrecision, boolToInt(p.ExpectedYMConfirmed),
		p.ExpectedAmount, boolToInt(p.ExpectedAmountConfirmed),
		p.SalesOwner, nullStr(p.SalesOwnerID), p.Competitor, p.LeadSource, p.LostReason,
		p.Status, p.Notes)
	if err != nil {
		return err
	}
	if err := insertSalesHistory(tx, fmt.Sprintf("SH-%03d", histN), p.SalesID, "", p.Stage, "등록", "", ""); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	logCreate(r.db, "sales_projects", "sales_id", p.SalesID, p.Name)
	return nil
}

func (r *SalesRepo) Update(p *model.SalesProject) error {
	if p == nil || strings.TrimSpace(p.SalesID) == "" {
		return fmt.Errorf("sales_id 필요")
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("사업명을 입력하세요")
	}
	stages, _ := r.Stages()
	normalizeSalesProject(p, stages)
	return touchUpdate(r.db, "sales_projects", "sales_id", p.SalesID, p.Name, func() error {
		_, err := r.db.Exec(`
			UPDATE sales_projects SET
				name=?, is_tentative_name=?, probability=?, probability_override=?,
				customer_id=?, prospect_name=?, prospect_region=?, customer_confirmed=?,
				expected_ym=?, expected_precision=?, expected_ym_confirmed=?,
				expected_amount=?, expected_amount_confirmed=?,
				sales_owner=?, sales_owner_id=?, competitor=?, lead_source=?, lost_reason=?,
				status=?, notes=?, updated_at=CURRENT_TIMESTAMP
			WHERE sales_id=?`,
			p.Name, boolToInt(p.IsTentativeName), p.Probability, overrideArg(p),
			nullStr(p.CustomerID), p.ProspectName, p.ProspectRegion, boolToInt(p.CustomerConfirmed),
			p.ExpectedYM, p.ExpectedPrecision, boolToInt(p.ExpectedYMConfirmed),
			p.ExpectedAmount, boolToInt(p.ExpectedAmountConfirmed),
			p.SalesOwner, nullStr(p.SalesOwnerID), p.Competitor, p.LeadSource, p.LostReason,
			p.Status, p.Notes, p.SalesID)
		return err
	})
}

func (r *SalesRepo) Delete(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("sales_id 필요")
	}
	before := rowJSON(r.db, "sales_projects", "sales_id", id)
	if _, err := r.db.Exec(`DELETE FROM sales_stage_history WHERE sales_id=?`, id); err != nil {
		return err
	}
	res, err := r.db.Exec(`DELETE FROM sales_projects WHERE sales_id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	logDelete(r.db, "sales_projects", "sales_id", id, "영업 사업", before)
	return nil
}

func (r *SalesRepo) ChangeStage(id, toStage, reason, byID, byName string, keepOverride bool) error {
	p, err := r.Get(id)
	if err != nil {
		return err
	}
	stages, err := r.Stages()
	if err != nil && len(stages) == 0 {
		return err
	}
	toStage = strings.TrimSpace(toStage)
	from := model.FindSalesStage(stages, p.Stage)
	to := model.FindSalesStage(stages, toStage)
	if to == nil {
		return fmt.Errorf("알 수 없는 단계입니다")
	}
	if p.Stage == toStage {
		return nil
	}
	if model.IsSalesStageBackward(from, to) && strings.TrimSpace(reason) == "" {
		return fmt.Errorf("단계를 되돌릴 때는 사유가 필요합니다")
	}
	if model.StageNeedsFullConfirm(toStage) {
		if err := p.RequireWonConfirmation(); err != nil {
			return err
		}
	}
	p.Stage = toStage
	p.Probability = to.Probability
	if !keepOverride {
		p.HasOverride = false
		p.OverrideValue = 0
	}
	if to.IsLost() {
		p.Status = model.SalesStatusLost
		if strings.TrimSpace(p.LostReason) == "" {
			p.LostReason = strings.TrimSpace(reason)
		}
	} else if p.Status == model.SalesStatusLost {
		p.Status = model.SalesStatusActive
	}
	before := rowJSON(r.db, "sales_projects", "sales_id", p.SalesID)
	histN, err := NextSeq(r.db, "sales_stage_history")
	if err != nil {
		return err
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`
			UPDATE sales_projects SET
				stage=?, probability=?, probability_override=?,
				status=?, lost_reason=?, updated_at=CURRENT_TIMESTAMP
			WHERE sales_id=?`,
		p.Stage, p.Probability, overrideArg(p), p.Status, p.LostReason, p.SalesID)
	if err != nil {
		return err
	}
	if err := insertSalesHistory(tx, fmt.Sprintf("SH-%03d", histN), p.SalesID, fromCode(from), toStage, reason, byID, byName); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	logUpdate(r.db, "sales_projects", "sales_id", p.SalesID, p.Name, before)
	return nil
}

func (r *SalesRepo) ListHistory(salesID string) ([]model.SalesStageHistory, error) {
	rows, err := r.db.Query(`
		SELECT history_id, sales_id, COALESCE(from_stage,''), to_stage, COALESCE(reason,''),
			COALESCE(changed_by,''), COALESCE(changed_by_id,''), COALESCE(changed_at,'')
		FROM sales_stage_history
		WHERE sales_id=?
		ORDER BY changed_at DESC, history_id DESC`, salesID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	stages, _ := r.Stages()
	var items []model.SalesStageHistory
	for rows.Next() {
		var h model.SalesStageHistory
		if err := rows.Scan(&h.HistoryID, &h.SalesID, &h.FromStage, &h.ToStage, &h.Reason,
			&h.ChangedBy, &h.ChangedByID, &h.ChangedAt); err != nil {
			return nil, err
		}
		if d := model.FindSalesStage(stages, h.FromStage); d != nil {
			h.FromLabel = d.Label
		} else {
			h.FromLabel = h.FromStage
		}
		if d := model.FindSalesStage(stages, h.ToStage); d != nil {
			h.ToLabel = d.Label
		} else {
			h.ToLabel = h.ToStage
		}
		items = append(items, h)
	}
	return items, rows.Err()
}

func insertSalesHistory(tx *sql.Tx, historyID, salesID, from, to, reason, byID, byName string) error {
	_, err := tx.Exec(`
		INSERT INTO sales_stage_history (history_id, sales_id, from_stage, to_stage, reason, changed_by, changed_by_id)
		VALUES (?,?,?,?,?,?,?)`,
		historyID, salesID, from, to, strings.TrimSpace(reason), strings.TrimSpace(byName), strings.TrimSpace(byID))
	return err
}

func normalizeSalesProject(p *model.SalesProject, stages []model.SalesStageDef) {
	p.Name = strings.TrimSpace(p.Name)
	p.ProspectName = strings.TrimSpace(p.ProspectName)
	p.ProspectRegion = strings.TrimSpace(p.ProspectRegion)
	p.CustomerID = strings.TrimSpace(p.CustomerID)
	p.SalesOwner = strings.TrimSpace(p.SalesOwner)
	p.SalesOwnerID = strings.TrimSpace(p.SalesOwnerID)
	p.Competitor = strings.TrimSpace(p.Competitor)
	p.LeadSource = strings.TrimSpace(p.LeadSource)
	p.LostReason = strings.TrimSpace(p.LostReason)
	p.Notes = strings.TrimSpace(p.Notes)
	p.ExpectedYM = model.NormalizeSalesYM(p.ExpectedYM)
	p.ExpectedPrecision = model.NormalizeSalesPrecision(p.ExpectedPrecision)
	if p.Stage == "" {
		p.Stage = model.DefaultSalesStageCode()
	}
	if def := model.FindSalesStage(stages, p.Stage); def != nil {
		p.Probability = def.Probability
		p.StageLabel = def.Label
	}
	if p.Status == "" {
		p.Status = model.SalesStatusActive
	}
	if p.Stage == model.SalesStageLost {
		p.Status = model.SalesStatusLost
	}
}

func overrideArg(p *model.SalesProject) interface{} {
	if p == nil || !p.HasOverride {
		return nil
	}
	return p.OverrideValue
}

func fromCode(d *model.SalesStageDef) string {
	if d == nil {
		return ""
	}
	return d.Code
}

type salesScanner interface {
	Scan(dest ...interface{}) error
}

func scanSalesRow(row salesScanner) (*model.SalesProject, error) {
	var p model.SalesProject
	var override sql.NullInt64
	var tentative, custConf, ymConf, amtConf int
	err := row.Scan(
		&p.SalesID, &p.Name, &tentative, &p.Stage, &p.Probability, &override,
		&p.CustomerID, &p.ProspectName, &p.ProspectRegion, &custConf,
		&p.ExpectedYM, &p.ExpectedPrecision, &ymConf,
		&p.ExpectedAmount, &amtConf,
		&p.SalesOwner, &p.SalesOwnerID, &p.Competitor, &p.LeadSource, &p.LostReason,
		&p.Status, &p.Notes, &p.CreatedAt, &p.UpdatedAt, &p.CustomerName,
	)
	if err != nil {
		return nil, err
	}
	p.IsTentativeName = tentative != 0
	p.CustomerConfirmed = custConf != 0
	p.ExpectedYMConfirmed = ymConf != 0
	p.ExpectedAmountConfirmed = amtConf != 0
	if override.Valid {
		p.HasOverride = true
		p.OverrideValue = int(override.Int64)
	}
	return &p, nil
}

func scanSalesRows(rows *sql.Rows) ([]model.SalesProject, error) {
	var items []model.SalesProject
	for rows.Next() {
		p, err := scanSalesRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *p)
	}
	return items, rows.Err()
}
