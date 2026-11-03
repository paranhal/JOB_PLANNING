package repository

import (
	"fmt"
	"strings"

	"customer-support/internal/model"
)

// CustomerScope — 무엇을 고르는가. 메뉴가 아니라 용도.
type CustomerScope string

const (
	ScopeSite  CustomerScope = "site"  // 활성 party_kind=customer. AS·정기점검·일일업무·작업·자산·연락처
	ScopeParty CustomerScope = "party" // customer+partner+vendor+own. 발주처·계약상대·제조사·공급사
)

// CustomerQuery 선택 목록·검색의 유일한 입력.
type CustomerQuery struct {
	Scope      CustomerScope
	Q          string
	IncludeIDs []string
	ParentID   string
	Limit      int    // 0 = 제한 없음
	Offset     int
}

// CustomerOption 콤보·검색 한 줄.
type CustomerOption struct {
	ID          string
	Name        string
	ParentName  string
	Region      string
	ShortName   string
	PartyKind   string
	Label       string
	Inactive    bool
	Provisional bool
}

func normalizeCustomerScope(s CustomerScope) CustomerScope {
	if s == ScopeParty {
		return ScopeParty
	}
	return ScopeSite
}

func lookupLikePrefix(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(q) + "%"
}

func (r *CustomerRepo) lookupWhere(q CustomerQuery) (string, []interface{}) {
	scope := normalizeCustomerScope(q.Scope)
	kind := `COALESCE(NULLIF(TRIM(c.party_kind),''),'customer')`
	var b strings.Builder
	args := []interface{}{}
	b.WriteString(` WHERE (`)
	if scope == ScopeParty {
		b.WriteString(kind + ` IN ('customer','partner','vendor','own') AND c.is_active=1`)
	} else {
		b.WriteString(kind + `='customer' AND c.is_active=1`)
	}
	ids := uniqueNonEmpty(q.IncludeIDs)
	if len(ids) > 0 {
		b.WriteString(` OR c.customer_id IN (` + lookupPlaceholders(len(ids)) + `)`)
		for _, id := range ids {
			args = append(args, id)
		}
	}
	b.WriteString(`)`)
	if pid := strings.TrimSpace(q.ParentID); pid != "" {
		b.WriteString(` AND c.parent_customer_id=?`)
		args = append(args, pid)
	}
	term := strings.Join(strings.Fields(q.Q), " ")
	if term != "" {
		pat := likeContains(term)
		b.WriteString(` AND (
			c.org_name LIKE ? ESCAPE '\'
			OR c.official_name LIKE ? ESCAPE '\'
			OR COALESCE(c.org_code,'') LIKE ? ESCAPE '\'
			OR c.customer_id LIKE ? ESCAPE '\'
			OR COALESCE(p.org_name,'') LIKE ? ESCAPE '\'
			OR COALESCE(c.addr_sido,'') LIKE ? ESCAPE '\'
			OR COALESCE(c.addr_sigungu,'') LIKE ? ESCAPE '\'
			OR COALESCE(c.address,'') LIKE ? ESCAPE '\'
		)`)
		for i := 0; i < 8; i++ {
			args = append(args, pat)
		}
	}
	return b.String(), args
}

func uniqueNonEmpty(ids []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func lookupPlaceholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func customerOptionLabel(name, parent, region string, ambiguous bool) string {
	if !ambiguous && parent == "" {
		return name
	}
	var extra []string
	if parent != "" {
		extra = append(extra, parent)
	}
	if region != "" {
		extra = append(extra, region)
	}
	if len(extra) == 0 {
		return name
	}
	return name + " (" + strings.Join(extra, " · ") + ")"
}

func assignCustomerLabels(items []CustomerOption) {
	counts := map[string]int{}
	for _, it := range items {
		counts[it.Name]++
	}
	for i := range items {
		items[i].Label = customerOptionLabel(items[i].Name, items[i].ParentName, items[i].Region, counts[items[i].Name] > 1)
	}
}

const customerLookupFrom = `
		FROM customers c
		LEFT JOIN customers p ON p.customer_id = c.parent_customer_id
		LEFT JOIN maintenance_site_config cfg ON cfg.customer_id = c.customer_id`

// Lookup 선택 목록·검색의 유일한 구현. 조직 조건 없음, 하위 기관 포함.
func (r *CustomerRepo) Lookup(q CustomerQuery) ([]CustomerOption, int, error) {
	if r == nil || r.db == nil {
		return nil, 0, nil
	}
	where, args := r.lookupWhere(q)
	var total int
	countSQL := `SELECT COUNT(*) ` + customerLookupFrom + where
	if err := r.db.QueryRow(countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	term := strings.Join(strings.Fields(q.Q), " ")
	sel := `
		SELECT c.customer_id, c.org_name, COALESCE(p.org_name,''),
		       COALESCE(NULLIF(TRIM(cfg.region),''), NULLIF(TRIM(c.addr_sigungu),''), COALESCE(c.addr_sido,'')),
		       COALESCE(cfg.short_name,''),
		       COALESCE(NULLIF(TRIM(c.party_kind),''),'customer'),
		       CASE WHEN c.is_active=1 THEN 0 ELSE 1 END,
		       COALESCE(c.is_provisional,0)` + customerLookupFrom + where
	orderArgs := append([]interface{}{}, args...)
	if term != "" {
		sel += ` ORDER BY CASE
			WHEN c.org_name = ? COLLATE NOCASE THEN 0
			WHEN c.org_name LIKE ? ESCAPE '\' THEN 1
			ELSE 2 END, c.org_name COLLATE NOCASE`
		orderArgs = append(orderArgs, term, lookupLikePrefix(term))
	} else {
		sel += ` ORDER BY c.org_name COLLATE NOCASE`
	}
	if q.Limit > 0 {
		sel += ` LIMIT ?`
		orderArgs = append(orderArgs, q.Limit)
		if q.Offset > 0 {
			sel += ` OFFSET ?`
			orderArgs = append(orderArgs, q.Offset)
		}
	} else if q.Offset > 0 {
		sel += ` LIMIT -1 OFFSET ?`
		orderArgs = append(orderArgs, q.Offset)
	}
	rows, err := r.db.Query(sel, orderArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []CustomerOption
	for rows.Next() {
		var it CustomerOption
		var inactive, prov int
		if err := rows.Scan(&it.ID, &it.Name, &it.ParentName, &it.Region, &it.ShortName, &it.PartyKind, &inactive, &prov); err != nil {
			return nil, 0, err
		}
		it.Inactive = inactive == 1
		it.Provisional = prov == 1
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if out == nil {
		out = []CustomerOption{}
	}
	assignCustomerLabels(out)
	return out, total, nil
}

func (r *CustomerRepo) LookupByIDs(ids []string) (map[string]CustomerOption, error) {
	ids = uniqueNonEmpty(ids)
	out := map[string]CustomerOption{}
	if len(ids) == 0 {
		return out, nil
	}
	opts, _, err := r.Lookup(CustomerQuery{Scope: ScopeParty, IncludeIDs: ids, Limit: len(ids) + 8})
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	for _, o := range opts {
		if want[o.ID] {
			out[o.ID] = o
		}
	}
	return out, nil
}

func optionsToCustomers(opts []CustomerOption) []model.Customer {
	out := make([]model.Customer, 0, len(opts))
	for _, o := range opts {
		out = append(out, model.Customer{
			CustomerID:    o.ID,
			OrgName:       o.Name,
			ParentOrgName: o.ParentName,
			Region:        o.Region,
			ShortName:     o.ShortName,
			PartyKind:     o.PartyKind,
			IsActive:      !o.Inactive,
			IsProvisional: o.Provisional,
		})
	}
	return out
}

// ListSelectable 활성 사이트(customer) 전체. 조직·상하위·건수 제한 없음.
func (r *CustomerRepo) ListSelectable() ([]model.Customer, error) {
	opts, _, err := r.Lookup(CustomerQuery{Scope: ScopeSite})
	if err != nil {
		return nil, err
	}
	return optionsToCustomers(opts), nil
}

// ListParties 거래처 전체(ScopeParty). 영업·사업 선택용.
func (r *CustomerRepo) ListParties() ([]model.Customer, error) {
	return r.listFromLookup(ScopeParty, "")
}

func (r *CustomerRepo) listFromLookup(scope CustomerScope, includeID string) ([]model.Customer, error) {
	q := CustomerQuery{Scope: scope}
	if id := strings.TrimSpace(includeID); id != "" {
		q.IncludeIDs = []string{id}
	}
	opts, _, err := r.Lookup(q)
	if err != nil {
		return nil, err
	}
	return optionsToCustomers(opts), nil
}

// FormatLookupError 잘못된 scope 등.
func FormatLookupError(scope string) error {
	s := strings.TrimSpace(scope)
	if s == "" || s == string(ScopeSite) || s == string(ScopeParty) {
		return nil
	}
	return fmt.Errorf("잘못된 scope")
}
