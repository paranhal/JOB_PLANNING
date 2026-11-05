package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

var ErrCustomerNameTaken = fmt.Errorf("이미 등록된 거래처입니다")

type QuickQuoteInput struct {
	OrgID         string
	NewCustomer   bool
	CustomerID    string
	CustomerName  string
	ContactID     string
	AttnName      string
	ContactLine   string
	AddContact    bool
	UpdateContact bool
	Project       *model.SalesProject
	Quote         *model.SalesQuote
}

type QuickQuoteResult struct {
	Project    *model.SalesProject
	Quote      *model.SalesQuote
	MasterNote string
	ContactID  string
	CustomerID string
}

func SaveQuickQuote(db *sql.DB, in QuickQuoteInput) (*QuickQuoteResult, error) {
	if db == nil {
		return nil, fmt.Errorf("저장소가 없습니다")
	}
	if in.Project == nil || in.Quote == nil {
		return nil, fmt.Errorf("사업과 견적이 필요합니다")
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	cust := NewCustomerRepo(db)
	note, customerID, contactID, err := applyQuickQuoteCustomer(tx, cust, in)
	if err != nil {
		return nil, err
	}

	in.Project.CustomerID = customerID
	in.Project.CustomerConfirmed = true
	in.Project.ProspectName = ""
	in.Project.Name = model.CollapseNewlines(in.Project.Name)
	in.Project.DealType = model.SalesDealSupply
	in.Project.BizType = model.SalesBizGoods
	in.Project.Stage = model.SalesStageInquiry
	in.Project.CreatedVia = model.SalesCreatedViaQuickQuote
	in.Project.IsTentativeName = false
	in.Project.ExpectedAmount = in.Quote.Total
	in.Project.ExpectedAmountConfirmed = in.Quote.Total > 0
	if err := insertSupplyProjectTx(tx, db, in.Project); err != nil {
		return nil, err
	}

	in.Quote.SalesID = in.Project.SalesID
	in.Quote.CustomerID = customerID
	in.Quote.RecipientName = model.CollapseNewlines(in.Quote.RecipientName)
	if in.Quote.RecipientName == "" {
		in.Quote.RecipientName = model.CollapseNewlines(in.CustomerName)
	}
	in.Quote.AttnName = model.CollapseNewlines(in.AttnName)
	parsed := model.ParseContactLine(in.ContactLine)
	in.Quote.AttnPhone = parsed.Phone
	in.Quote.AttnEmail = parsed.Email
	in.Quote.QuoteKind = model.QuoteKindSolution
	in.Quote.FormType = model.QuoteFormA
	in.Quote.Status = model.QuoteStatusDraft
	in.Quote.Purpose = model.QuotePurposeDeal
	in.Quote.OrgID = in.Project.OrgID
	qr := NewQuoteRepo(db)
	if err := qr.createInTx(tx, in.Quote); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	rememberCustomerName(customerID, in.Quote.RecipientName)
	byID, _ := stampCreatedBy()
	ids := in.Project.SalesOwnerIDs
	if len(ids) == 0 && strings.TrimSpace(in.Project.SalesOwnerID) != "" {
		ids = []string{in.Project.SalesOwnerID}
	}
	_ = NewSalesRepo(db).ReplaceSalesOwners(in.Project.SalesID, in.Project.SalesOwnerID, byID, ids)
	logCreate(db, "sales_projects", "sales_id", in.Project.SalesID, in.Project.Name)
	logCreate(db, "sales_quotes", "quote_id", in.Quote.QuoteID, in.Quote.QuoteNo)
	return &QuickQuoteResult{
		Project: in.Project, Quote: in.Quote, MasterNote: note,
		ContactID: contactID, CustomerID: customerID,
	}, nil
}

func applyQuickQuoteCustomer(tx *sql.Tx, cust *CustomerRepo, in QuickQuoteInput) (note, customerID, contactID string, err error) {
	line := model.ParseContactLine(in.ContactLine)
	attn := model.CollapseNewlines(in.AttnName)
	if in.NewCustomer {
		name := model.CollapseNewlines(in.CustomerName)
		if name == "" || attn == "" {
			return "", "", "", fmt.Errorf("거래처명·담당자명·담당자 연락처를 입력하세요")
		}
		exist, err := cust.ListActiveNameMatches(name, "")
		if err != nil {
			return "", "", "", err
		}
		if len(exist) > 0 {
			return "", "", "", ErrCustomerNameTaken
		}
		c := &model.Customer{
			OrgName: name, OfficialName: name, IsActive: true,
			PartyKind: model.PartyKindCustomer, OrgID: in.OrgID,
		}
		if err := createCustomerTx(tx, c); err != nil {
			return "", "", "", err
		}
		ct := newQuickContact(c.CustomerID, attn, line)
		if err := createContactTx(tx, ct); err != nil {
			return "", "", "", err
		}
		return "신규 거래처 등록", c.CustomerID, ct.ContactID, nil
	}

	customerID = strings.TrimSpace(in.CustomerID)
	if customerID == "" {
		return "", "", "", fmt.Errorf("고객을 목록에서 선택하세요")
	}
	if in.AddContact || strings.TrimSpace(in.ContactID) == "" {
		ct := newQuickContact(customerID, attn, line)
		if err := createContactTx(tx, ct); err != nil {
			return "", "", "", err
		}
		return "새 연락처 추가", customerID, ct.ContactID, nil
	}
	contactID = strings.TrimSpace(in.ContactID)
	if in.UpdateContact {
		prev, err := loadContactTx(tx, contactID)
		if err != nil {
			return "", "", "", err
		}
		oldLine := model.FormatContactLine(prev.Mobile, prev.Phone, prev.Email)
		applyParsedToContact(prev, line)
		if err := updateContactTx(tx, prev); err != nil {
			return "", "", "", err
		}
		return "연락처 수정(" + oldLine + "→" + model.FormatContactLine(prev.Mobile, prev.Phone, prev.Email) + ")", customerID, contactID, nil
	}
	return "기존 연락처 그대로", customerID, contactID, nil
}

func newQuickContact(customerID, name string, line model.ParsedContactLine) *model.Contact {
	ct := &model.Contact{
		CustomerID: customerID, FullName: name, Status: "active",
		ContactRole: "regular", Affiliation: "institution",
	}
	applyParsedToContact(ct, line)
	return ct
}

func applyParsedToContact(ct *model.Contact, line model.ParsedContactLine) {
	if ct == nil {
		return
	}
	ct.Email = line.Email
	if model.IsMobilePhone(line.Phone) {
		ct.Mobile = line.Phone
		if strings.TrimSpace(ct.Phone) == "" {
			ct.Phone = ""
		}
	} else {
		ct.Phone = line.Phone
	}
}

func createCustomerTx(tx *sql.Tx, c *model.Customer) error {
	orgID, err := RequireInsertOrg(c.OrgID)
	if err != nil {
		if c.OrgID == "" {
			c.OrgID = model.OrgIDLibrary
		} else {
			return err
		}
	} else {
		c.OrgID = orgID
	}
	id, err := nextCustomerIDTx(tx, c.MainPhone)
	if err != nil {
		return err
	}
	c.CustomerID = id
	c.SyncCombinedAddress()
	now := time.Now().Format("2006-01-02 15:04:05")
	byID, byName := stampCreatedBy()
	_, err = tx.Exec(`
		INSERT INTO customers (
			customer_id, org_name, official_name, org_email, main_phone,
			website, business_number, representative, industry,
			has_parent, parent_customer_id,
			postal_code, addr_sido, addr_sigungu, addr_dong,
			address, address_detail,
			is_active, notes, party_kind, org_id, created_by_user_id, created_by_name, created_at, updated_at,
			is_provisional, provisional_by, provisional_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.CustomerID, c.OrgName, c.OfficialName, c.OrgEmail, c.MainPhone,
		c.Website, nullStr(strings.TrimSpace(c.BusinessNumber)), c.Representative, c.Industry,
		boolToInt(c.HasParent), nullStr(c.ParentCustomerID),
		c.PostalCode, c.AddrSido, c.AddrSigungu, c.AddrDong,
		c.Address, c.AddressDetail,
		boolToInt(c.IsActive), c.Notes, model.NormalizePartyKind(c.PartyKind), c.OrgID, byID, byName, now, now,
		boolToInt(c.IsProvisional), c.ProvisionalBy, c.ProvisionalAt,
	)
	return err
}

func createContactTx(tx *sql.Tx, ct *model.Contact) error {
	ct.ContactID = fmt.Sprintf("CONT-%d", time.Now().UnixNano())
	ct.ContactRole = normalizeContactRole(ct.ContactRole, ct.IsPrimary)
	ct.IsPrimary = ct.ContactRole == "primary"
	ct.Affiliation = normalizeAffiliation(ct.Affiliation)
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := tx.Exec(`
		INSERT INTO contacts (
			contact_id, customer_id, full_name, affiliation, job_role, title, job_grade,
			phone, mobile, email, start_date, end_date,
			status, contact_role, is_primary, notes, created_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		ct.ContactID, ct.CustomerID, ct.FullName, ct.Affiliation, ct.JobRole, ct.Title, ct.JobGrade,
		ct.Phone, ct.Mobile, ct.Email, ct.StartDate, ct.EndDate,
		ct.Status, ct.ContactRole, boolToInt(ct.IsPrimary), ct.Notes, now,
	)
	return err
}

func loadContactTx(tx *sql.Tx, id string) (*model.Contact, error) {
	var ct model.Contact
	var isPrimary int
	var role, affiliation string
	err := tx.QueryRow(`
		SELECT contact_id, customer_id, full_name,
		       COALESCE(affiliation,'institution'),
		       COALESCE(job_role,''), COALESCE(title,''), COALESCE(job_grade,''),
		       COALESCE(phone,''), COALESCE(mobile,''),
		       COALESCE(email,''), COALESCE(start_date,''),
		       COALESCE(end_date,''), COALESCE(status,'active'),
		       COALESCE(contact_role,''), is_primary, COALESCE(notes,'')
		FROM contacts WHERE contact_id=?`, id).Scan(
		&ct.ContactID, &ct.CustomerID, &ct.FullName, &affiliation,
		&ct.JobRole, &ct.Title, &ct.JobGrade, &ct.Phone, &ct.Mobile,
		&ct.Email, &ct.StartDate, &ct.EndDate, &ct.Status, &role, &isPrimary, &ct.Notes,
	)
	if err != nil {
		return nil, err
	}
	applyContactFlags(&ct, isPrimary, role, affiliation)
	return &ct, nil
}

func updateContactTx(tx *sql.Tx, ct *model.Contact) error {
	ct.ContactRole = normalizeContactRole(ct.ContactRole, ct.IsPrimary)
	ct.IsPrimary = ct.ContactRole == "primary"
	ct.Affiliation = normalizeAffiliation(ct.Affiliation)
	_, err := tx.Exec(`
		UPDATE contacts SET
			customer_id=?, full_name=?, affiliation=?, job_role=?, title=?, job_grade=?,
			phone=?, mobile=?, email=?, start_date=?, end_date=?,
			status=?, contact_role=?, is_primary=?, notes=?
		WHERE contact_id=?`,
		ct.CustomerID, ct.FullName, ct.Affiliation, ct.JobRole, ct.Title, ct.JobGrade,
		ct.Phone, ct.Mobile, ct.Email, ct.StartDate, ct.EndDate,
		ct.Status, ct.ContactRole, boolToInt(ct.IsPrimary), ct.Notes, ct.ContactID,
	)
	return err
}

func insertSupplyProjectTx(tx *sql.Tx, db *sql.DB, p *model.SalesProject) error {
	orgID, err := RequireInsertOrg(p.OrgID)
	if err != nil {
		return fmt.Errorf("소속 조직을 선택하세요")
	}
	p.OrgID = orgID
	p.Name = model.CollapseNewlines(p.Name)
	if p.Name == "" {
		return fmt.Errorf("사업명을 입력하세요")
	}
	p.DealType = model.SalesDealSupply
	p.Stage = model.SalesStageInquiry
	p.Status = model.SalesStatusActive
	p.BudgetStatus = model.SalesBudgetUnknown
	p.BizType = model.SalesBizGoods
	p.CreatedVia = model.SalesCreatedViaQuickQuote
	p.Probability = 0
	id, err := nextSeqTx(tx, "sales_project")
	if err != nil {
		return err
	}
	p.SalesID = fmt.Sprintf("SP-%03d", id)
	if strings.TrimSpace(p.SalesNo) == "" {
		no, err := nextSalesNoTx(tx, time.Now())
		if err != nil {
			return err
		}
		p.SalesNo = no
	}
	histN, err := nextSeqTx(tx, "sales_stage_history")
	if err != nil {
		return err
	}
	p.SalesOwner, p.SalesOwnerID = bindStaff(db, p.SalesOwner, p.SalesOwnerID)
	byID, byName := stampCreatedBy()
	_, err = tx.Exec(`
		INSERT INTO sales_projects (
			sales_id, sales_no, name, is_tentative_name, tentative_name, stage, probability, probability_override,
			customer_id, prospect_name, prospect_region, customer_confirmed,
			expected_ym, expected_precision, expected_ym_confirmed,
			expected_amount, expected_amount_confirmed,
			sales_owner, sales_owner_id, competitor, lead_source, lost_reason,
			status, notes, legacy_stage, won_at, contracted_at,
			deal_type, po_no, delivered_at,
			bid_status, close_reason, rfp_received_at, win_prob, probability_final,
			awarded_amount, contract_amount, contract_target, procurement_route, contract_method,
			bid_eval_method, mall_contract_type, prev_sales_id,
			biz_type, budget_year, budget_status,
			bid_ym, revenue_ym, revenue_from, revenue_to, billing_cycle, amount_vat_included, org_id,
			created_by_user_id, created_by_name, created_via
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.SalesID, p.SalesNo, p.Name, 0, "", p.Stage, p.Probability, nil,
		nullStr(p.CustomerID), "", "", 1,
		p.ExpectedYM, p.ExpectedPrecision, boolToInt(p.ExpectedYMConfirmed),
		p.ExpectedAmount, boolToInt(p.ExpectedAmountConfirmed),
		p.SalesOwner, nullStr(p.SalesOwnerID), "", "", "",
		p.Status, p.Notes, "", "", "",
		p.DealType, "", "",
		"", "", "", nil, nil,
		0, 0, "", "", "",
		"", "", "",
		p.BizType, p.BudgetYear, p.BudgetStatus,
		"", "", "", "", "", boolToInt(p.AmountVATIncluded), p.OrgID, byID, byName, p.CreatedVia)
	if err != nil {
		return err
	}
	return insertSalesHistory(tx, fmt.Sprintf("SH-%03d", histN), p.SalesID, "", p.Stage, "등록", "빠른 견적", byID, byName)
}

func (r *QuoteRepo) createInTx(tx *sql.Tx, q *model.SalesQuote) error {
	if q == nil {
		return fmt.Errorf("견적이 필요합니다")
	}
	orgID, err := RequireInsertOrg(q.OrgID)
	if err != nil {
		if q.OrgID == "" {
			q.OrgID = model.OrgIDLibrary
		} else {
			return err
		}
	} else {
		q.OrgID = orgID
	}
	if strings.TrimSpace(q.QuoteDate) == "" {
		q.QuoteDate = time.Now().Format("2006-01-02")
	}
	seq, err := r.nextSeqForDayTx(tx, q.QuoteDate)
	if err != nil {
		return err
	}
	q.QuoteNo = model.FormatQuoteNo(q.QuoteDate, seq)
	q.Rev = 0
	if err := r.prepareSave(q, false); err != nil {
		return err
	}
	n, err := nextSeqTx(tx, "sales_quotes")
	if err != nil {
		return err
	}
	q.QuoteID = fmt.Sprintf("Q-%03d", n)
	byID, byName := stampCreatedBy()
	_, err = tx.Exec(`
		INSERT INTO sales_quotes (
			quote_id, sales_id, quote_no, rev, form_type, recipient_kind,
			customer_id, recipient_name, attn_name, attn_title, quote_date, title,
			valid_until_text, due_text, place_text, payment_text, vat_mode, round_rule,
			subtotal, vat, total, owner_user_id, owner_name, owner_phone, remarks,
			purpose, budget_year, overhead_rate, tech_fee_rate, status, is_legacy,
			rev_reason, is_reverse_calc, target_total, maint_block, quote_kind, org_id,
			created_by_user_id, created_by_name,
			attn_phone, attn_email, downloaded_at, sent_at, cut_scope
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		q.QuoteID, q.SalesID, q.QuoteNo, q.Rev, q.FormType, q.RecipientKind,
		q.CustomerID, q.RecipientName, q.AttnName, q.AttnTitle, q.QuoteDate, q.Title,
		q.ValidUntilText, q.DueText, q.PlaceText, q.PaymentText, q.VATMode, q.RoundRule,
		q.Subtotal, q.VAT, q.Total, q.OwnerUserID, q.OwnerName, q.OwnerPhone, q.Remarks,
		q.Purpose, q.BudgetYear, q.OverheadRate, q.TechFeeRate, q.Status, boolToInt(q.IsLegacy),
		q.RevReason, boolToInt(q.IsReverseCalc), q.TargetTotal, boolToInt(q.MaintBlock), q.QuoteKind, q.OrgID, byID, byName,
		q.AttnPhone, q.AttnEmail, q.DownloadedAt, q.SentAt, model.NormalizeCutScope(q.CutScope))
	if err != nil {
		if isUniqueErr(err) {
			return model.ErrQuoteNoDuplicate
		}
		return err
	}
	return r.replaceLinesTx(tx, q)
}

func (r *QuoteRepo) nextSeqForDayTx(tx *sql.Tx, iso string) (int, error) {
	prefix := model.FormatQuoteNo(iso, 1)
	if prefix == "" {
		return 0, fmt.Errorf("견적일이 올바르지 않습니다")
	}
	dayPart := strings.TrimSuffix(prefix, "-001")
	like := dayPart + "-%"
	rows, err := tx.Query(`SELECT quote_no FROM sales_quotes WHERE quote_no LIKE ?`, like)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	max := 0
	for rows.Next() {
		var no string
		if err := rows.Scan(&no); err != nil {
			return 0, err
		}
		i := strings.LastIndex(no, "-")
		if i < 0 {
			continue
		}
		var seq int
		fmt.Sscanf(no[i+1:], "%d", &seq)
		if seq > max {
			max = seq
		}
	}
	return max + 1, rows.Err()
}

func (r *QuoteRepo) replaceLinesTx(tx *sql.Tx, q *model.SalesQuote) error {
	if _, err := tx.Exec(`DELETE FROM sales_quote_lines WHERE quote_id=?`, q.QuoteID); err != nil {
		return err
	}
	for i := range q.Lines {
		ln := &q.Lines[i]
		n, err := nextSeqTx(tx, "sales_quote_lines")
		if err != nil {
			return err
		}
		ln.LineID = fmt.Sprintf("QL-%03d", n)
		ln.QuoteID = q.QuoteID
		ln.Seq = i + 1
		ln.Name = model.CollapseNewlines(ln.Name)
		ln.Spec = model.CollapseNewlines(ln.Spec)
		ln.Amount = model.LineAmount(*ln)
		if _, err := tx.Exec(`
			INSERT INTO sales_quote_lines (
				line_id, quote_id, seq, item_id, group_label, name, spec, spec_id, qty, unit,
				unit_price, amount, gov_price, mm_rate, discount_rate, note,
				rate_id, labor_year, price_overridden
			) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			ln.LineID, ln.QuoteID, ln.Seq, ln.ItemID, ln.GroupLabel, ln.Name, ln.Spec, ln.SpecID,
			ln.Qty, ln.Unit, ln.UnitPrice, ln.Amount, ln.GovPrice, ln.MMRate, ln.DiscountRate, ln.Note,
			ln.RateID, ln.LaborYear, boolToInt(ln.PriceOverridden)); err != nil {
			return err
		}
	}
	var sum int
	if err := tx.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM sales_quote_lines WHERE quote_id=?`, q.QuoteID).Scan(&sum); err != nil {
		return err
	}
	want := 0
	for i := range q.Lines {
		want += model.LineAmount(q.Lines[i])
	}
	if sum != want {
		return model.ErrQuoteLineSum
	}
	return nil
}

func (r *QuoteRepo) MarkDownloaded(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return sql.ErrNoRows
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := r.db.Exec(`UPDATE sales_quotes SET downloaded_at=CASE WHEN TRIM(COALESCE(downloaded_at,''))='' THEN ? ELSE downloaded_at END, updated_at=CURRENT_TIMESTAMP WHERE quote_id=?`, now, id)
	return err
}

func (r *QuoteRepo) MarkSent(id string) (*model.SalesQuote, error) {
	q, err := r.Get(id)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(q.DownloadedAt) == "" {
		return nil, model.ErrQuoteNotDownloaded
	}
	if model.NormalizeQuoteStatus(q.Status) == model.QuoteStatusSent && strings.TrimSpace(q.SentAt) != "" {
		return q, model.ErrQuoteAlreadySent
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	q.Status = model.QuoteStatusSent
	q.SentAt = now
	if err := r.SetStatus(q.QuoteID, model.QuoteStatusSent); err != nil {
		return nil, err
	}
	if _, err := r.db.Exec(`UPDATE sales_quotes SET sent_at=? WHERE quote_id=?`, now, q.QuoteID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(q.SalesID) != "" {
		_ = NewSalesRepo(r.db).AdvanceSupplyQuoted(q.SalesID)
	}
	q.DownloadedAt = q.DownloadedAt
	got, err := r.Get(q.QuoteID)
	if err != nil {
		return q, nil
	}
	return got, nil
}

func (r *SalesRepo) AdvanceSupplyQuoted(salesID string) error {
	p, err := r.Get(salesID)
	if err != nil || p == nil || !p.IsSupply() {
		return err
	}
	if p.Stage == model.SalesStageDropped || p.Stage == model.SalesStageQuoted ||
		p.Stage == model.SalesStageOrdered || p.Stage == model.SalesStageDelivered {
		return nil
	}
	byID, byName := stampCreatedBy()
	histN, err := NextSeq(r.db, "sales_stage_history")
	if err != nil {
		return err
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE sales_projects SET stage=?, updated_at=CURRENT_TIMESTAMP WHERE sales_id=?`, model.SalesStageQuoted, salesID); err != nil {
		return err
	}
	if err := insertSalesHistory(tx, fmt.Sprintf("SH-%03d", histN), salesID, p.Stage, model.SalesStageQuoted, "발송 처리", "", byID, byName); err != nil {
		return err
	}
	return tx.Commit()
}
