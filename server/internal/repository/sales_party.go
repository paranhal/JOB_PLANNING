package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"customer-support/internal/model"
)

func (r *SalesRepo) PartyTypes() ([]model.Code, error) {
	return r.codeRepo.ActiveByGroup(model.SalesCodeGroupPartyType)
}

func (r *SalesRepo) PartyRoles() ([]model.Code, error) {
	return r.codeRepo.ActiveByGroup(model.SalesCodeGroupPartyRole)
}

func (r *SalesRepo) codeLabel(group, value string) string {
	value = strings.TrimSpace(value)
	if value == "" || r.codeRepo == nil {
		return value
	}
	codes, err := r.codeRepo.ActiveByGroup(group)
	if err != nil {
		return value
	}
	for _, c := range codes {
		if c.CodeValue == value {
			return c.CodeName
		}
	}
	return value
}

func (r *SalesRepo) fillPartyLabels(p *model.SalesParty) {
	if p == nil {
		return
	}
	p.TypeLabel = r.codeLabel(model.SalesCodeGroupPartyType, p.PartyType)
	if p.TypeLabel == "" || p.TypeLabel == p.PartyType {
		p.TypeLabel = model.SalesPartyTypeLabel(p.PartyType)
	}
	p.RoleLabel = r.codeLabel(model.SalesCodeGroupPartyRole, p.PartyRole)
}

const salesPartySelect = `
	SELECT party_id, sales_id, COALESCE(party_type,'own'), COALESCE(org_name,''),
		COALESCE(person_name,''), COALESCE(title,''), COALESCE(phone,''), COALESCE(email,''),
		COALESCE(party_role,''), COALESCE(is_primary,0), COALESCE(user_id,''), COALESCE(contact_id,''),
		COALESCE(is_active,1), COALESCE(replaced_by,''), COALESCE(replaced_reason,''), COALESCE(note,''),
		COALESCE(created_at,''), COALESCE(is_auto,0)
	FROM sales_parties`

func (r *SalesRepo) ListParties(salesID string, includeInactive bool) ([]model.SalesParty, error) {
	salesID = strings.TrimSpace(salesID)
	if salesID == "" {
		return nil, nil
	}
	q := salesPartySelect + ` WHERE sales_id=?`
	if !includeInactive {
		q += ` AND COALESCE(is_active,1)=1`
	}
	q += ` ORDER BY CASE party_type WHEN 'own' THEN 0 WHEN 'partner' THEN 1 ELSE 2 END,
		CASE WHEN COALESCE(is_active,1)=1 THEN 0 ELSE 1 END,
		COALESCE(is_primary,0) DESC, created_at, party_id`
	rows, err := r.db.Query(q, salesID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.SalesParty
	for rows.Next() {
		p, err := scanSalesParty(rows)
		if err != nil {
			return nil, err
		}
		r.fillPartyLabels(p)
		out = append(out, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	names := map[string]string{}
	for i := range out {
		names[out[i].PartyID] = out[i].DisplayName()
	}
	for i := range out {
		if out[i].ReplacedBy != "" {
			out[i].ReplacedByName = names[out[i].ReplacedBy]
		}
	}
	return out, nil
}

func (r *SalesRepo) GetParty(id string) (*model.SalesParty, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, sql.ErrNoRows
	}
	row := r.db.QueryRow(salesPartySelect+` WHERE party_id=?`, id)
	p, err := scanSalesParty(row)
	if err != nil {
		return nil, err
	}
	r.fillPartyLabels(p)
	return p, nil
}

func (r *SalesRepo) CreateParty(p *model.SalesParty, byName string) error {
	if p == nil {
		return fmt.Errorf("관계자가 필요합니다")
	}
	if _, err := r.Get(p.SalesID); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("영업 사업을 찾을 수 없습니다")
		}
		return err
	}
	normalizeSalesParty(p)
	if p.PersonName == "" && p.OrgName == "" {
		return fmt.Errorf("이름 또는 기관명을 입력하세요")
	}
	n, err := NextSeq(r.db, "sales_party")
	if err != nil {
		return err
	}
	p.PartyID = fmt.Sprintf("PT-%03d", n)
	p.IsActive = true
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if p.IsPrimary {
		if _, err := tx.Exec(`UPDATE sales_parties SET is_primary=0 WHERE sales_id=? AND party_type=? AND COALESCE(is_active,1)=1`,
			p.SalesID, p.PartyType); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`
		INSERT INTO sales_parties (
			party_id, sales_id, party_type, org_name, person_name, title, phone, email,
			party_role, is_primary, user_id, contact_id, is_active, note, is_auto)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,1,?,0)`,
		p.PartyID, p.SalesID, p.PartyType, p.OrgName, p.PersonName, p.Title, p.Phone, p.Email,
		p.PartyRole, boolToInt(p.IsPrimary), nullStr(p.UserID), nullStr(p.ContactID), p.Note); err != nil {
		return err
	}
	r.fillPartyLabels(p)
	if err := tx.Commit(); err != nil {
		return err
	}
	logCreate(r.db, "sales_parties", "party_id", p.PartyID, p.ChangeLabel())
	return nil
}

// ReplaceParty 옛 사람을 지우지 않고 비활성으로 내린 뒤 새 사람으로 잇는다 (§32.7).
func (r *SalesRepo) ReplaceParty(oldID string, neu *model.SalesParty, reason, byName string) error {
	old, err := r.GetParty(oldID)
	if err != nil {
		return err
	}
	if !old.IsActive {
		return fmt.Errorf("이미 교체된 관계자입니다")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return fmt.Errorf("교체 사유가 필요합니다")
	}
	neu.SalesID = old.SalesID
	if strings.TrimSpace(neu.PartyType) == "" {
		neu.PartyType = old.PartyType
	}
	normalizeSalesParty(neu)
	if neu.PersonName == "" && neu.OrgName == "" {
		return fmt.Errorf("이름 또는 기관명을 입력하세요")
	}
	if old.IsPrimary {
		neu.IsPrimary = true
	}
	n, err := NextSeq(r.db, "sales_party")
	if err != nil {
		return err
	}
	neu.PartyID = fmt.Sprintf("PT-%03d", n)
	neu.IsActive = true
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if neu.IsPrimary {
		if _, err := tx.Exec(`UPDATE sales_parties SET is_primary=0 WHERE sales_id=? AND party_type=? AND COALESCE(is_active,1)=1`,
			neu.SalesID, neu.PartyType); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`
		INSERT INTO sales_parties (
			party_id, sales_id, party_type, org_name, person_name, title, phone, email,
			party_role, is_primary, user_id, contact_id, is_active, note, is_auto)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,1,?,0)`,
		neu.PartyID, neu.SalesID, neu.PartyType, neu.OrgName, neu.PersonName, neu.Title, neu.Phone, neu.Email,
		neu.PartyRole, boolToInt(neu.IsPrimary), nullStr(neu.UserID), nullStr(neu.ContactID), neu.Note); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		UPDATE sales_parties SET is_active=0, replaced_by=?, replaced_reason=?, is_primary=0
		WHERE party_id=?`, neu.PartyID, reason, old.PartyID); err != nil {
		return err
	}
	r.fillPartyLabels(old)
	r.fillPartyLabels(neu)
	if err := tx.Commit(); err != nil {
		return err
	}
	logUpdateWithReason(r.db, "sales_parties", "party_id", neu.PartyID, neu.ChangeLabel(),
		rowJSON(r.db, "sales_parties", "party_id", old.PartyID), reason)
	return nil
}

func (r *SalesRepo) LinkPartyContact(partyID, contactID, byName string) error {
	_ = byName
	p, err := r.GetParty(partyID)
	if err != nil {
		return err
	}
	contactID = strings.TrimSpace(contactID)
	if contactID == "" {
		return fmt.Errorf("담당자를 선택하세요")
	}
	ct, err := NewContactRepo(r.db).GetByID(contactID)
	if err != nil || ct == nil {
		return fmt.Errorf("담당자를 찾을 수 없습니다")
	}
	if p.PersonName == "" {
		p.PersonName = ct.FullName
	}
	if p.Title == "" {
		p.Title = ct.Title
	}
	if p.Phone == "" {
		p.Phone = firstNonEmpty(ct.Mobile, ct.Phone)
	}
	if p.Email == "" {
		p.Email = ct.Email
	}
	if p.OrgName == "" {
		p.OrgName = ct.OrgName
	}
	return touchUpdate(r.db, "sales_parties", "party_id", p.PartyID, p.ChangeLabel(), func() error {
		_, err = r.db.Exec(`
			UPDATE sales_parties SET contact_id=?, person_name=?, title=?, phone=?, email=?, org_name=?, is_auto=0
			WHERE party_id=?`,
			contactID, p.PersonName, p.Title, p.Phone, p.Email, p.OrgName, p.PartyID)
		return err
	})
}

func (r *SalesRepo) ListChanges(salesID string) ([]model.SalesChange, error) {
	salesID = strings.TrimSpace(salesID)
	if salesID == "" {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT change_id, sales_id, field_key, COALESCE(old_value,''), COALESCE(new_value,''),
			COALESCE(changed_at,''), COALESCE(changed_by,''), COALESCE(note,'')
		FROM sales_changes
		WHERE sales_id=?
		ORDER BY changed_at DESC, change_id DESC`, salesID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.SalesChange
	for rows.Next() {
		var c model.SalesChange
		if err := rows.Scan(&c.ChangeID, &c.SalesID, &c.FieldKey, &c.OldValue, &c.NewValue,
			&c.ChangedAt, &c.ChangedBy, &c.Note); err != nil {
			return nil, err
		}
		c.FieldLabel = model.SalesChangeFieldLabel(c.FieldKey)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *SalesRepo) recordProjectChanges(oldP, newP *model.SalesProject, byName string) error {
	if oldP == nil || newP == nil {
		return nil
	}
	type pair struct{ key, oldV, newV string }
	var diffs []pair
	oldPeriod := strings.TrimSpace(oldP.PeriodLabel())
	newPeriod := strings.TrimSpace(newP.PeriodLabel())
	if oldPeriod != newPeriod {
		diffs = append(diffs, pair{model.SalesChangeExpectedYM, oldPeriod, newPeriod})
	}
	oldAmt := strings.TrimSpace(oldP.AmountLabel())
	newAmt := strings.TrimSpace(newP.AmountLabel())
	if oldAmt != newAmt {
		diffs = append(diffs, pair{model.SalesChangeAmount, oldAmt, newAmt})
	}
	oldCust := strings.TrimSpace(oldP.CustomerValue())
	newCust := strings.TrimSpace(newP.CustomerValue())
	if oldCust != newCust {
		diffs = append(diffs, pair{model.SalesChangeCustomer, oldCust, newCust})
	}
	for _, d := range diffs {
		if err := r.insertChange(oldP.SalesID, d.key, d.oldV, d.newV, byName, ""); err != nil {
			return err
		}
	}
	return nil
}

func (r *SalesRepo) insertChange(salesID, field, oldV, newV, by, note string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := insertSalesChangeTx(tx, salesID, field, oldV, newV, by, note); err != nil {
		return err
	}
	return tx.Commit()
}

func insertSalesChangeTx(tx *sql.Tx, salesID, field, oldV, newV, by, note string) error {
	_ = tx
	_ = salesID
	_ = field
	_ = oldV
	_ = newV
	_ = by
	_ = note
	return nil
}

func normalizeSalesParty(p *model.SalesParty) {
	p.SalesID = strings.TrimSpace(p.SalesID)
	p.PartyType = strings.TrimSpace(p.PartyType)
	if p.PartyType == "" {
		p.PartyType = model.SalesPartyOwn
	}
	p.OrgName = strings.TrimSpace(p.OrgName)
	p.PersonName = strings.TrimSpace(p.PersonName)
	p.Title = strings.TrimSpace(p.Title)
	p.Phone = strings.TrimSpace(p.Phone)
	p.Email = strings.TrimSpace(p.Email)
	p.PartyRole = strings.TrimSpace(p.PartyRole)
	p.UserID = strings.TrimSpace(p.UserID)
	p.ContactID = strings.TrimSpace(p.ContactID)
	p.Note = strings.TrimSpace(p.Note)
}

type partyScanner interface {
	Scan(dest ...interface{}) error
}

func scanSalesParty(sc partyScanner) (*model.SalesParty, error) {
	var p model.SalesParty
	var primary, active, isAuto int
	err := sc.Scan(
		&p.PartyID, &p.SalesID, &p.PartyType, &p.OrgName, &p.PersonName, &p.Title, &p.Phone, &p.Email,
		&p.PartyRole, &primary, &p.UserID, &p.ContactID, &active, &p.ReplacedBy, &p.ReplacedReason, &p.Note,
		&p.CreatedAt, &isAuto)
	if err != nil {
		return nil, err
	}
	p.IsPrimary = primary == 1
	p.IsActive = active == 1
	p.IsAuto = isAuto == 1
	return &p, nil
}
