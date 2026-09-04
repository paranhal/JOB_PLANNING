package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

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
	return r.StagesFor(model.SalesDealBuild)
}

func (r *SalesRepo) StagesFor(dealType string) ([]model.SalesStageDef, error) {
	if model.NormalizeSalesDealType(dealType) == model.SalesDealSupply {
		codes, err := r.codeRepo.ActiveByGroup(model.SalesCodeGroupSupplyStage)
		if err != nil {
			return model.LoadSalesSupplyStages(nil), err
		}
		return model.LoadSalesSupplyStages(codes), nil
	}
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

func (r *SalesRepo) allStageDefs() []model.SalesStageDef {
	build, _ := r.StagesFor(model.SalesDealBuild)
	supply, _ := r.StagesFor(model.SalesDealSupply)
	return append(build, supply...)
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
		COALESCE(s.legacy_stage,''), COALESCE(s.won_at,''), COALESCE(s.contracted_at,''),
		COALESCE(s.deal_type,'build'), COALESCE(s.po_no,''), COALESCE(s.delivered_at,''),
		COALESCE(s.created_at,''), COALESCE(s.updated_at,''),
		COALESCE(cu.org_name,'')
	FROM sales_projects s
	LEFT JOIN customers cu ON cu.customer_id = s.customer_id`

func (r *SalesRepo) List(search, status, stage string) ([]model.SalesProject, error) {
	return r.ListFilter(SalesListFilter{Search: search, Status: status, Stage: stage, DealType: model.SalesDealAll})
}

type SalesListFilter struct {
	Search          string
	Status          string
	Stage           string
	Owner           string
	Period          string
	Customer        string
	AmountConfirmed string
	DealType        string
}

func (r *SalesRepo) ListFilter(f SalesListFilter) ([]model.SalesProject, error) {
	q := salesSelect + ` WHERE 1=1`
	var args []interface{}
	if strings.TrimSpace(f.DealType) != model.SalesDealAll {
		deal := model.NormalizeSalesDealType(f.DealType)
		q += ` AND COALESCE(NULLIF(TRIM(s.deal_type),''),'build')=?`
		args = append(args, deal)
	}
	if s := strings.TrimSpace(f.Status); s != "" {
		q += ` AND s.status=?`
		args = append(args, s)
	}
	if s := strings.TrimSpace(f.Stage); s != "" {
		q += ` AND s.stage=?`
		args = append(args, s)
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		like := "%" + s + "%"
		q += ` AND (s.name LIKE ? OR COALESCE(s.prospect_name,'') LIKE ? OR COALESCE(cu.org_name,'') LIKE ?)`
		args = append(args, like, like, like)
	}
	if s := strings.TrimSpace(f.Owner); s != "" {
		q += ` AND (s.sales_owner=? OR s.sales_owner_id=?)`
		args = append(args, s, s)
	}
	if s := strings.TrimSpace(f.Customer); s != "" {
		like := "%" + s + "%"
		q += ` AND (COALESCE(s.prospect_name,'') LIKE ? OR COALESCE(cu.org_name,'') LIKE ?)`
		args = append(args, like, like)
	}
	switch strings.TrimSpace(f.AmountConfirmed) {
	case "1", "yes", "true":
		q += ` AND COALESCE(s.expected_amount_confirmed,0)=1`
	case "0", "no", "false":
		q += ` AND COALESCE(s.expected_amount_confirmed,0)=0`
	}
	if pred, arg := salesPeriodSQL(f.Period); pred != "" {
		q += pred
		args = append(args, arg...)
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

func salesPeriodSQL(period string) (string, []interface{}) {
	period = strings.TrimSpace(period)
	if period == "" {
		return "", nil
	}
	if len(period) == 4 {
		return ` AND COALESCE(s.expected_ym,'') LIKE ?`, []interface{}{period + "%"}
	}
	if len(period) >= 7 && period[4] == '-' && (period[5] == 'Q' || period[5] == 'q') {
		q := 0
		if len(period) >= 7 {
			q = int(period[6] - '0')
		}
		if q < 1 || q > 4 {
			return "", nil
		}
		year := period[:4]
		from := fmt.Sprintf("%s-%02d", year, (q-1)*3+1)
		to := fmt.Sprintf("%s-%02d", year, q*3)
		return ` AND COALESCE(s.expected_ym,'') >= ? AND COALESCE(s.expected_ym,'') <= ?`, []interface{}{from, to}
	}
	if len(period) >= 7 && period[4] == '-' && (period[5] == 'H' || period[5] == 'h') {
		h := 1
		if len(period) >= 7 {
			h = int(period[6] - '0')
		}
		year := period[:4]
		if h == 1 {
			return ` AND COALESCE(s.expected_ym,'') >= ? AND COALESCE(s.expected_ym,'') <= ?`, []interface{}{year + "-01", year + "-06"}
		}
		return ` AND COALESCE(s.expected_ym,'') >= ? AND COALESCE(s.expected_ym,'') <= ?`, []interface{}{year + "-07", year + "-12"}
	}
	return ` AND COALESCE(s.expected_ym,'') LIKE ?`, []interface{}{period + "%"}
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
	p.DealType = model.NormalizeSalesDealType(p.DealType)
	stages, err := r.StagesFor(p.DealType)
	if err != nil && len(stages) == 0 {
		return err
	}
	normalizeSalesProject(p, stages)
	applySupplyAutoStage(p, stages)
	if !p.IsSupply() && model.StageNeedsFullConfirm(p.Stage) {
		if err := p.RequireWonConfirmation(); err != nil {
			return err
		}
	}
	id, err := NextSeq(r.db, "sales_project")
	if err != nil {
		return err
	}
	p.SalesID = fmt.Sprintf("SP-%03d", id)
	if p.Stage == model.SalesStageWon && strings.TrimSpace(p.WonAt) == "" {
		p.WonAt = time.Now().Format("2006-01-02")
	}
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
			status, notes, legacy_stage, won_at, contracted_at,
			deal_type, po_no, delivered_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.SalesID, p.Name, boolToInt(p.IsTentativeName), p.Stage, p.Probability, overrideArg(p),
		nullStr(p.CustomerID), p.ProspectName, p.ProspectRegion, boolToInt(p.CustomerConfirmed),
		p.ExpectedYM, p.ExpectedPrecision, boolToInt(p.ExpectedYMConfirmed),
		p.ExpectedAmount, boolToInt(p.ExpectedAmountConfirmed),
		p.SalesOwner, nullStr(p.SalesOwnerID), p.Competitor, p.LeadSource, p.LostReason,
		p.Status, p.Notes, p.LegacyStage, p.WonAt, p.ContractedAt,
		p.DealType, p.PONo, p.DeliveredAt)
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

func (r *SalesRepo) Update(p *model.SalesProject, byName string) error {
	if p == nil || strings.TrimSpace(p.SalesID) == "" {
		return fmt.Errorf("sales_id 필요")
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("사업명을 입력하세요")
	}
	old, err := r.Get(p.SalesID)
	if err != nil {
		return err
	}
	p.DealType = model.NormalizeSalesDealType(old.DealType)
	stages, _ := r.StagesFor(p.DealType)
	normalizeSalesProject(p, stages)
	applySupplyAutoStage(p, stages)
	err = touchUpdate(r.db, "sales_projects", "sales_id", p.SalesID, p.Name, func() error {
		_, err := r.db.Exec(`
			UPDATE sales_projects SET
				name=?, is_tentative_name=?, probability=?, probability_override=?,
				customer_id=?, prospect_name=?, prospect_region=?, customer_confirmed=?,
				expected_ym=?, expected_precision=?, expected_ym_confirmed=?,
				expected_amount=?, expected_amount_confirmed=?,
				sales_owner=?, sales_owner_id=?, competitor=?, lead_source=?, lost_reason=?,
				status=?, notes=?, contracted_at=?, po_no=?, delivered_at=?, updated_at=CURRENT_TIMESTAMP
			WHERE sales_id=?`,
			p.Name, boolToInt(p.IsTentativeName), p.Probability, overrideArg(p),
			nullStr(p.CustomerID), p.ProspectName, p.ProspectRegion, boolToInt(p.CustomerConfirmed),
			p.ExpectedYM, p.ExpectedPrecision, boolToInt(p.ExpectedYMConfirmed),
			p.ExpectedAmount, boolToInt(p.ExpectedAmountConfirmed),
			p.SalesOwner, nullStr(p.SalesOwnerID), p.Competitor, p.LeadSource, p.LostReason,
			p.Status, p.Notes, p.ContractedAt, p.PONo, p.DeliveredAt, p.SalesID)
		return err
	})
	if err != nil {
		return err
	}
	if err := r.recordProjectChanges(old, p, byName); err != nil {
		return err
	}
	if p.IsSupply() && p.Stage != old.Stage {
		return r.ChangeStage(p.SalesID, p.Stage, "자동", "", byName, true)
	}
	return nil
}

func (r *SalesRepo) Delete(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("sales_id 필요")
	}
	before := rowJSON(r.db, "sales_projects", "sales_id", id)
	if _, err := r.db.Exec(`
		DELETE FROM work_tasks
		WHERE source_type=? AND source_id IN (SELECT activity_id FROM sales_activities WHERE sales_id=?)`,
		model.WBSourceSalesActivity, id); err != nil && !strings.Contains(err.Error(), "no such table") {
		return err
	}
	if _, err := r.db.Exec(`DELETE FROM sales_activities WHERE sales_id=?`, id); err != nil &&
		!strings.Contains(err.Error(), "no such table") {
		return err
	}
	if _, err := r.db.Exec(`DELETE FROM sales_parties WHERE sales_id=?`, id); err != nil &&
		!strings.Contains(err.Error(), "no such table") {
		return err
	}
	if _, err := r.db.Exec(`DELETE FROM sales_changes WHERE sales_id=?`, id); err != nil &&
		!strings.Contains(err.Error(), "no such table") {
		return err
	}
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

func (r *SalesRepo) MarkPromoted(salesID string) error {
	salesID = strings.TrimSpace(salesID)
	if salesID == "" {
		return fmt.Errorf("sales_id 필요")
	}
	p, err := r.Get(salesID)
	if err != nil {
		return err
	}
	if p.Status == model.SalesStatusPromoted {
		return nil
	}
	return touchUpdate(r.db, "sales_projects", "sales_id", salesID, p.Name, func() error {
		_, err := r.db.Exec(`
			UPDATE sales_projects SET status=?, updated_at=CURRENT_TIMESTAMP WHERE sales_id=?`,
			model.SalesStatusPromoted, salesID)
		return err
	})
}

func (r *SalesRepo) ChangeStage(id, toStage, reason, byID, byName string, keepOverride bool) error {
	p, err := r.Get(id)
	if err != nil {
		return err
	}
	stages, err := r.StagesFor(p.DealType)
	if err != nil && len(stages) == 0 {
		return err
	}
	toStage = strings.TrimSpace(toStage)
	from := model.FindSalesStage(stages, p.Stage)
	to := model.FindSalesStage(stages, toStage)
	if to == nil || !model.SalesStageAllowedForDeal(p.DealType, toStage) {
		return fmt.Errorf("알 수 없는 단계입니다")
	}
	if p.Stage == toStage {
		return nil
	}
	if to.IsLost() && strings.TrimSpace(reason) == "" {
		return fmt.Errorf("실주할 때는 실패 사유가 필요합니다")
	}
	if model.IsSalesStageBackward(from, to) && strings.TrimSpace(reason) == "" {
		return fmt.Errorf("단계를 되돌릴 때는 사유가 필요합니다")
	}
	if !p.IsSupply() && model.StageNeedsFullConfirm(toStage) {
		if err := p.RequireWonConfirmation(); err != nil {
			return err
		}
	}
	p.Stage = toStage
	p.Probability = to.Probability
	if toStage == model.SalesStageWon && strings.TrimSpace(p.WonAt) == "" {
		p.WonAt = time.Now().Format("2006-01-02")
	}
	if !keepOverride {
		p.HasOverride = false
		p.OverrideValue = 0
	}
	if to.IsLost() {
		p.Status = model.SalesStatusLost
		p.LostReason = strings.TrimSpace(reason)
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
				status=?, lost_reason=?, won_at=?, updated_at=CURRENT_TIMESTAMP
			WHERE sales_id=?`,
		p.Stage, p.Probability, overrideArg(p), p.Status, p.LostReason, p.WonAt, p.SalesID)
	if err != nil {
		return err
	}
	if err := insertSalesHistory(tx, fmt.Sprintf("SH-%03d", histN), p.SalesID, fromCode(from), toStage, reason, byID, byName); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	fromLbl, toLbl := fromCode(from), toStage
	if from != nil && from.Label != "" {
		fromLbl = from.Label
	}
	if to != nil && to.Label != "" {
		toLbl = to.Label
	}
	_ = r.insertChange(p.SalesID, model.SalesChangeStage, fromLbl, toLbl, byName, reason)
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
	stages := r.allStageDefs()
	var items []model.SalesStageHistory
	for rows.Next() {
		var h model.SalesStageHistory
		if err := rows.Scan(&h.HistoryID, &h.SalesID, &h.FromStage, &h.ToStage, &h.Reason,
			&h.ChangedBy, &h.ChangedByID, &h.ChangedAt); err != nil {
			return nil, err
		}
		h.FromLabel = model.SalesStageDisplayLabel(h.FromStage, stages)
		h.ToLabel = model.SalesStageDisplayLabel(h.ToStage, stages)
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
	p.PONo = strings.TrimSpace(p.PONo)
	p.DeliveredAt = strings.TrimSpace(p.DeliveredAt)
	p.DealType = model.NormalizeSalesDealType(p.DealType)
	p.ExpectedYM = model.NormalizeSalesYM(p.ExpectedYM)
	p.ExpectedPrecision = model.NormalizeSalesPrecision(p.ExpectedPrecision)
	if p.Stage == "" {
		p.Stage = model.DefaultSalesStageCodeFor(p.DealType)
	}
	if !p.IsSupply() {
		p.Stage = model.CurrentSalesStage(p.Stage)
	}
	if def := model.FindSalesStage(stages, p.Stage); def != nil {
		if !p.IsSupply() {
			p.Probability = def.Probability
		} else {
			p.Probability = 0
			p.HasOverride = false
			p.OverrideValue = 0
		}
		p.StageLabel = def.Label
	} else {
		p.Stage = model.DefaultSalesStageCodeFor(p.DealType)
		if def := model.FindSalesStage(stages, p.Stage); def != nil {
			p.StageLabel = def.Label
			if !p.IsSupply() {
				p.Probability = def.Probability
			}
		}
	}
	if p.Status == "" {
		p.Status = model.SalesStatusActive
	}
	if p.Stage == model.SalesStageLost || p.Stage == model.SalesStageDropped {
		p.Status = model.SalesStatusLost
	}
}

func applySupplyAutoStage(p *model.SalesProject, stages []model.SalesStageDef) {
	if p == nil || !p.IsSupply() {
		return
	}
	next := model.SupplyAutoStage(p)
	if next == "" || next == p.Stage {
		return
	}
	from := model.FindSalesStage(stages, p.Stage)
	to := model.FindSalesStage(stages, next)
	if to == nil || model.IsSalesStageBackward(from, to) {
		return
	}
	p.Stage = next
	p.StageLabel = to.Label
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
		&p.Status, &p.Notes, &p.LegacyStage, &p.WonAt, &p.ContractedAt,
		&p.DealType, &p.PONo, &p.DeliveredAt,
		&p.CreatedAt, &p.UpdatedAt, &p.CustomerName,
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
