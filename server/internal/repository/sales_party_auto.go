package repository

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	"customer-support/internal/model"
)

const salesPartyAutoMetaKey = "__meta:sales_party_auto_v227"

// applySalesPartyAutoV227 마이그레이션 033. §32.7.1 활동 상대 → sales_parties 자동 생성.
func applySalesPartyAutoV227(db *sql.DB) {
	if db == nil {
		return
	}
	_, err := db.Exec(`ALTER TABLE sales_parties ADD COLUMN is_auto INTEGER NOT NULL DEFAULT 0`)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		if !strings.Contains(err.Error(), "no such table") {
			log.Printf("033 sales_parties.is_auto: %v", err)
		}
		return
	}
	if metaDone(db, salesPartyAutoMetaKey) {
		return
	}
	if err := NewSalesRepo(db).BackfillActivityCounterpartsToParties("migration"); err != nil {
		log.Printf("033 counterparts backfill: %v", err)
		return
	}
	markMetaDone(db, salesPartyAutoMetaKey)
}

// EnsureCustomerPartiesFromCounterparts 활동 상대 이름을 고객 관계자 행으로 잇는다. 같은 이름은 재사용한다.
func (r *SalesRepo) EnsureCustomerPartiesFromCounterparts(salesID, counterparts, byName string) error {
	_ = byName
	names := model.SplitSalesPeople(counterparts)
	if len(names) == 0 {
		return nil
	}
	p, err := r.Get(salesID)
	if err != nil {
		return err
	}
	org := p.CustomerValue()
	for _, name := range names {
		if existing, err := r.findCustomerPartyByName(salesID, name); err != nil {
			return err
		} else if existing != nil {
			continue
		}
		if err := r.insertAutoCustomerParty(salesID, org, name); err != nil {
			return err
		}
	}
	return nil
}

func (r *SalesRepo) findCustomerPartyByName(salesID, name string) (*model.SalesParty, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	parties, err := r.ListParties(salesID, true)
	if err != nil {
		return nil, err
	}
	var inactive *model.SalesParty
	for i := range parties {
		if parties[i].PartyType != model.SalesPartyCustomer {
			continue
		}
		if !sameSalesPersonName(parties[i].PersonName, name) {
			continue
		}
		if parties[i].IsActive {
			return &parties[i], nil
		}
		cp := parties[i]
		inactive = &cp
	}
	return inactive, nil
}

func sameSalesPersonName(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func (r *SalesRepo) insertAutoCustomerParty(salesID, orgName, personName string) error {
	p := &model.SalesParty{
		SalesID:    salesID,
		PartyType:  model.SalesPartyCustomer,
		OrgName:    strings.TrimSpace(orgName),
		PersonName: strings.TrimSpace(personName),
		IsAuto:     true,
		IsActive:   true,
	}
	normalizeSalesParty(p)
	if p.PersonName == "" {
		return nil
	}
	n, err := NextSeq(r.db, "sales_party")
	if err != nil {
		return err
	}
	p.PartyID = fmt.Sprintf("PT-%03d", n)
	_, err = r.db.Exec(`
		INSERT INTO sales_parties (
			party_id, sales_id, party_type, org_name, person_name, title, phone, email,
			party_role, is_primary, user_id, contact_id, is_active, note, is_auto)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,1,?,1)`,
		p.PartyID, p.SalesID, p.PartyType, p.OrgName, p.PersonName, p.Title, p.Phone, p.Email,
		p.PartyRole, 0, nil, nil, p.Note)
	return err
}

// BackfillActivityCounterpartsToParties 16-B counterparts 글자를 사업별 관계자로 옮긴다.
func (r *SalesRepo) BackfillActivityCounterpartsToParties(byName string) error {
	rows, err := r.db.Query(`
		SELECT sales_id, COALESCE(counterparts,'')
		FROM sales_activities
		WHERE TRIM(COALESCE(counterparts,'')) != ''`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	defer rows.Close()
	type pair struct{ salesID, counterparts string }
	var items []pair
	for rows.Next() {
		var it pair
		if err := rows.Scan(&it.salesID, &it.counterparts); err != nil {
			return err
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, it := range items {
		if err := r.EnsureCustomerPartiesFromCounterparts(it.salesID, it.counterparts, byName); err != nil {
			return err
		}
	}
	return nil
}

// ListCustomerPartyHintNames 자동완성용 고객 담당자 이름. 교체된 이름도 포함한다.
func (r *SalesRepo) ListCustomerPartyHintNames(salesID string) ([]string, error) {
	parties, err := r.ListParties(salesID, true)
	if err != nil {
		return nil, err
	}
	return model.CustomerPartyHintNames(parties), nil
}
