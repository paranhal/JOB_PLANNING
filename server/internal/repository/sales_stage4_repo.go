package repository

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"customer-support/internal/model"
)

func (r *SalesRepo) ChangeStage(id, toStage, reason, byID, byName string, keepOverride bool) error {
	_ = keepOverride
	p, err := r.Get(id)
	if err != nil {
		return err
	}
	toStage = strings.TrimSpace(toStage)
	from := strings.TrimSpace(p.Stage)
	reason = strings.TrimSpace(reason)
	if from == model.SalesStage4Closed {
		return fmt.Errorf("끝난 사업은 단계를 바꿀 수 없습니다. 새 사업 만들기로 이어 가세요")
	}
	if toStage == model.SalesStage4Closed || toStage == model.SalesCloseLost || toStage == model.SalesCloseDropped || toStage == model.SalesStageLost {
		return fmt.Errorf("사업 종료는 상세의 종료 버튼으로만 할 수 있습니다")
	}

	direct := toStage == model.SalesDirectWin
	if direct {
		if from != model.SalesStage4Discover && from != model.SalesStage4Propose {
			return fmt.Errorf("바로 수주는 발굴·제안에서만 할 수 있습니다")
		}
		p.Stage = model.SalesStage4Bid
		p.BidStatus = model.SalesBidWon
		p.ProbabilityFinal = nil
		if strings.TrimSpace(p.WonAt) == "" {
			p.WonAt = todayYMD()
		}
		p.Status = model.SalesStatusActive
		p.Probability = model.SalesProbability(p)
		return r.commitSalesStage(p, from, model.SalesStage4Bid, reason, model.SalesBidWon, byID, byName)
	}

	if toStage == from && toStage != model.SalesStage4Bid {
		return nil
	}

	switch {
	case from == model.SalesStage4Discover && toStage == model.SalesStage4Propose:
		p.Stage = model.SalesStage4Propose
	case from == model.SalesStage4Propose && toStage == model.SalesStage4Bid:
		if strings.TrimSpace(p.BidEvalMethod) == "" && !model.CanEnterBidWithoutEval(p) {
			return fmt.Errorf("입찰 평가방법을 먼저 정하세요")
		}
		if strings.TrimSpace(p.RFPReceivedAt) == "" && p.WinProb == nil {
			p.WinProb = model.IntPtr(50)
		}
		freeze := model.SalesProbability(p)
		if strings.TrimSpace(p.RFPReceivedAt) == "" {
			if p.WinProb != nil {
				freeze = *p.WinProb
			} else {
				freeze = 50
			}
		}
		p.Stage = model.SalesStage4Bid
		p.BidStatus = model.SalesBidPending
		p.ProbabilityFinal = model.IntPtr(freeze)
		if strings.TrimSpace(p.BidYM) == "" {
			p.BidYM = time.Now().Format("2006-01")
		}
	case from == model.SalesStage4Propose && toStage == model.SalesStage4Discover:
		if reason == "" {
			return fmt.Errorf("단계를 되돌릴 때는 사유가 필요합니다")
		}
		p.Stage = model.SalesStage4Discover
		p.WinProb = nil
		p.RFPReceivedAt = ""
	case from == model.SalesStage4Bid && toStage == model.SalesStage4Propose:
		if reason == "" {
			return fmt.Errorf("단계를 되돌릴 때는 사유가 필요합니다")
		}
		p.Stage = model.SalesStage4Propose
		p.BidStatus = ""
		p.ProbabilityFinal = nil
	default:
		return fmt.Errorf("그 단계로는 옮길 수 없습니다")
	}
	p.Status = model.SalesStatusActive
	p.Probability = model.SalesProbability(p)
	detail := p.BidStatus
	return r.commitSalesStage(p, from, p.Stage, reason, detail, byID, byName)
}

func (r *SalesRepo) SetBidResult(id, result, reason, amount, byID, byName string) error {
	p, err := r.Get(id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(p.Stage) != model.SalesStage4Bid || strings.TrimSpace(p.BidStatus) != model.SalesBidPending {
		return fmt.Errorf("결과 대기 중인 입찰만 수주·실주할 수 있습니다")
	}
	from := p.Stage
	switch strings.TrimSpace(result) {
	case model.SalesBidWon:
		p.BidStatus = model.SalesBidWon
		if strings.TrimSpace(p.WonAt) == "" {
			p.WonAt = todayYMD()
		}
		if n, err := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(amount), ",", "")); err == nil && n > 0 {
			p.AwardedAmount = n
		}
	case model.SalesCloseLost:
		if strings.TrimSpace(reason) == "" {
			return fmt.Errorf("실주 사유가 필요합니다")
		}
		p.Stage = model.SalesStage4Closed
		p.CloseReason = model.SalesCloseLost
		p.Status = model.SalesStatusLost
		p.LostReason = strings.TrimSpace(reason)
		p.BidStatus = ""
	default:
		return fmt.Errorf("수주 또는 실주만 넣을 수 있습니다")
	}
	p.Probability = model.SalesProbability(p)
	detail := p.BidStatus
	if detail == "" {
		detail = p.CloseReason
	}
	return r.commitSalesStage(p, from, p.Stage, reason, detail, byID, byName)
}

func (r *SalesRepo) StartNegotiation(id, byID, byName string) error {
	p, err := r.Get(id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(p.BidStatus) != model.SalesBidWon {
		return fmt.Errorf("수주 뒤에만 협상을 시작할 수 있습니다")
	}
	from := p.Stage
	p.BidStatus = model.SalesBidNegotiating
	p.Probability = model.SalesProbability(p)
	return r.commitSalesStage(p, from, p.Stage, "", model.SalesBidNegotiating, byID, byName)
}

func (r *SalesRepo) CloseContracted(id, contractedAt string, amount int, byID, byName string) error {
	p, err := r.Get(id)
	if err != nil {
		return err
	}
	st := strings.TrimSpace(p.BidStatus)
	if st != model.SalesBidWon && st != model.SalesBidNegotiating {
		return fmt.Errorf("수주 또는 협상 중인 건만 계약으로 끝낼 수 있습니다")
	}
	contractedAt = strings.TrimSpace(contractedAt)
	if contractedAt == "" || amount <= 0 {
		return fmt.Errorf("계약일과 계약금액이 필요합니다")
	}
	from := p.Stage
	p.Stage = model.SalesStage4Closed
	p.CloseReason = model.SalesCloseContracted
	p.Status = model.SalesStatusContracted
	p.ContractedAt = contractedAt
	p.ContractAmount = amount
	if strings.TrimSpace(p.RevenueYM) == "" {
		if ym := model.NormalizeSalesYM(p.RevenueFrom); ym != "" {
			p.RevenueYM = ym
		} else if len(contractedAt) >= 7 {
			p.RevenueYM = contractedAt[:7]
		}
	}
	p.Probability = model.SalesProbability(p)
	return r.commitSalesStage(p, from, p.Stage, "", model.SalesCloseContracted, byID, byName)
}

func (r *SalesRepo) CloseNegotiationFailed(id, reason, byID, byName string) error {
	p, err := r.Get(id)
	if err != nil {
		return err
	}
	st := strings.TrimSpace(p.BidStatus)
	if st != model.SalesBidWon && st != model.SalesBidNegotiating {
		return fmt.Errorf("수주 또는 협상 중인 건만 협상 결렬로 끝낼 수 있습니다")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return fmt.Errorf("결렬 사유가 필요합니다")
	}
	from := p.Stage
	p.Stage = model.SalesStage4Closed
	p.CloseReason = model.SalesCloseLost
	p.Status = model.SalesStatusLost
	p.LostReason = "협상 결렬: " + reason
	p.BidStatus = ""
	p.Probability = model.SalesProbability(p)
	return r.commitSalesStage(p, from, p.Stage, p.LostReason, model.SalesCloseLost, byID, byName)
}

func (r *SalesRepo) SetWinProb(id string, n int, byID, byName string) error {
	p, err := r.Get(id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(p.Stage) == model.SalesStage4Bid {
		return fmt.Errorf("입찰 뒤에는 수주 확도를 바꿀 수 없습니다")
	}
	if strings.TrimSpace(p.Stage) != model.SalesStage4Propose {
		return fmt.Errorf("제안 단계에서만 수주 확도를 바꿀 수 있습니다")
	}
	if strings.TrimSpace(p.RFPReceivedAt) == "" {
		return fmt.Errorf("RFP 접수를 먼저 하세요")
	}
	if !model.ValidWinProb(n) {
		return fmt.Errorf("수주 확도는 10부터 90까지 10 단위입니다")
	}
	from := p.Stage
	p.WinProb = model.IntPtr(n)
	p.Probability = model.SalesProbability(p)
	return r.commitSalesStage(p, from, p.Stage, "", "win_prob", byID, byName)
}

func (r *SalesRepo) SetRFPReceived(id, day, byID, byName string) error {
	p, err := r.Get(id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(p.Stage) != model.SalesStage4Propose {
		return fmt.Errorf("제안 단계에서만 RFP 접수를 기록합니다")
	}
	day = strings.TrimSpace(day)
	if day == "" {
		day = todayYMD()
	}
	from := p.Stage
	p.RFPReceivedAt = day
	if p.WinProb == nil {
		p.WinProb = model.IntPtr(50)
	}
	p.Probability = model.SalesProbability(p)
	return r.commitSalesStage(p, from, p.Stage, "", "rfp_received", byID, byName)
}

func (r *SalesRepo) MarkMigratedChecked(id, byID, byName string) error {
	p, err := r.Get(id)
	if err != nil {
		return err
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
	if err := insertSalesHistory(tx, fmt.Sprintf("SH-%03d", histN), p.SalesID, p.Stage, p.Stage, "이관 확인", "migrated_checked", byID, byName); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *SalesRepo) commitSalesStage(p *model.SalesProject, from, to, reason, detail, byID, byName string) error {
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
	if err := updateSalesStageRow(tx, p); err != nil {
		return err
	}
	if err := insertSalesHistory(tx, fmt.Sprintf("SH-%03d", histN), p.SalesID, from, to, reason, detail, byID, byName); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	logUpdateWithReason(r.db, "sales_projects", "sales_id", p.SalesID, p.Name, before, reason)
	return nil
}

func updateSalesStageRow(tx *sql.Tx, p *model.SalesProject) error {
	_, err := tx.Exec(`
		UPDATE sales_projects SET
			stage=?, bid_status=?, close_reason=?, status=?, probability=?,
			win_prob=?, probability_final=?, rfp_received_at=?,
			won_at=?, contracted_at=?, awarded_amount=?, contract_amount=?,
			lost_reason=?, bid_ym=?, revenue_ym=?, updated_at=CURRENT_TIMESTAMP
		WHERE sales_id=?`,
		p.Stage, p.BidStatus, p.CloseReason, p.Status, p.Probability,
		nullIntPtr(p.WinProb), nullIntPtr(p.ProbabilityFinal), p.RFPReceivedAt,
		p.WonAt, p.ContractedAt, p.AwardedAmount, p.ContractAmount,
		p.LostReason, p.BidYM, p.RevenueYM, p.SalesID)
	return err
}

func nullIntPtr(n *int) interface{} {
	if n == nil {
		return nil
	}
	return *n
}

func todayYMD() string {
	return time.Now().Format("2006-01-02")
}

type SalesDropPreview struct {
	OpenTasks int
	Quotes    int
}

func (r *SalesRepo) DropPreview(id string) (SalesDropPreview, error) {
	var out SalesDropPreview
	_ = r.db.QueryRow(`
		SELECT COUNT(*) FROM work_tasks
		WHERE source_type=? AND source_id IN (SELECT activity_id FROM sales_activities WHERE sales_id=?)
		  AND status NOT IN (?,?)`,
		model.WBSourceSalesActivity, id, model.WBTaskComplete, model.WBTaskCancelled).Scan(&out.OpenTasks)
	_ = r.db.QueryRow(`
		SELECT COUNT(*) FROM sales_quotes
		WHERE sales_id=? AND status NOT IN (?,?)`,
		id, model.QuoteStatusLost, model.QuoteStatusExpired).Scan(&out.Quotes)
	return out, nil
}

func (r *SalesRepo) CountHiddenClosed(f SalesListFilter) (int, error) {
	f.IncludeClosed = false
	vis, err := r.ListFilter(f)
	if err != nil {
		return 0, err
	}
	f.IncludeClosed = true
	all, err := r.ListFilter(f)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, p := range all {
		if p.CloseReason == model.SalesCloseLost || p.CloseReason == model.SalesCloseDropped ||
			p.Status == model.SalesStatusLost || p.Status == model.SalesStatusDropped {
			n++
		}
	}
	_ = vis
	hidden := len(all) - len(vis)
	if hidden < 0 {
		hidden = 0
	}
	if n > hidden {
		return n, nil
	}
	return hidden, nil
}

func (r *SalesRepo) ListMigratedUnchecked() ([]model.SalesProject, error) {
	q := salesSelect + `
		WHERE s.stage=? AND s.bid_status=?
		  AND EXISTS (
			SELECT 1 FROM sales_stage_history h
			WHERE h.sales_id=s.sales_id AND h.from_stage='negotiation' AND h.to_stage=?
		  )
		  AND NOT EXISTS (
			SELECT 1 FROM sales_stage_history h2
			WHERE h2.sales_id=s.sales_id AND h2.detail='migrated_checked'
		  )
		ORDER BY s.updated_at DESC`
	rows, err := r.db.Query(q, model.SalesStage4Bid, model.SalesBidPending, model.SalesStage4Bid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSalesRows(rows)
}

func (r *SalesRepo) Drop(id, code, reason, byID, byName string) error {
	p, err := r.Get(id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(p.Stage) == model.SalesStage4Closed {
		return fmt.Errorf("이미 종료된 사업입니다")
	}
	st := strings.TrimSpace(p.BidStatus)
	if st == model.SalesBidWon || st == model.SalesBidNegotiating {
		return fmt.Errorf("수주 뒤에는 드롭할 수 없습니다. 협상 결렬로 종료하세요")
	}
	code = strings.TrimSpace(code)
	reason = strings.TrimSpace(reason)
	var name string
	var active int
	err = r.db.QueryRow(`SELECT code_name, is_active FROM codes WHERE code_group=? AND code_value=?`,
		model.SalesCodeGroupDropReason, code).Scan(&name, &active)
	if err != nil || active != 1 {
		return fmt.Errorf("드롭 사유를 고르세요")
	}
	if code == "etc" && reason == "" {
		return fmt.Errorf("기타 사유를 적어 주세요")
	}
	from := p.Stage
	p.DroppedFromStage = from
	p.DropReasonCode = code
	p.DropReason = reason
	p.Stage = model.SalesStage4Closed
	p.CloseReason = model.SalesCloseDropped
	p.Status = model.SalesStatusDropped
	p.DroppedAt = todayYMD()
	p.DroppedBy = strings.TrimSpace(byName)
	p.BidStatus = ""
	p.Probability = 0
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
	if err := updateSalesStageRow(tx, p); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		UPDATE sales_projects SET drop_reason_code=?, drop_reason=?, dropped_at=?, dropped_by=?, dropped_from_stage=?
		WHERE sales_id=?`,
		p.DropReasonCode, p.DropReason, p.DroppedAt, p.DroppedBy, p.DroppedFromStage, p.SalesID); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		UPDATE work_tasks SET status=?, cancel_reason=?
		WHERE source_type=? AND source_id IN (SELECT activity_id FROM sales_activities WHERE sales_id=?)
		  AND status NOT IN (?,?)`,
		model.WBTaskCancelled, "사업 드롭으로 취소",
		model.WBSourceSalesActivity, p.SalesID,
		model.WBTaskComplete, model.WBTaskCancelled); err != nil && !strings.Contains(err.Error(), "no such column") && !strings.Contains(err.Error(), "no such table") {
		return err
	}
	detailReason := strings.TrimSpace(name + " " + reason)
	if err := insertSalesHistory(tx, fmt.Sprintf("SH-%03d", histN), p.SalesID, from, p.Stage, detailReason, "dropped", byID, byName); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	logUpdate(r.db, "sales_projects", "sales_id", p.SalesID, p.Name, before)
	return nil
}
