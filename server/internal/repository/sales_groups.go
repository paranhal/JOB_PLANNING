package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"customer-support/internal/model"
)

func applySalesGroupsSchema(db *sql.DB) {
	if db == nil {
		return
	}
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS sales_groups (
			group_id TEXT PRIMARY KEY,
			group_no TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL DEFAULT '',
			year INTEGER NOT NULL DEFAULT 0,
			owner_id TEXT NOT NULL DEFAULT '',
			owner_name TEXT NOT NULL DEFAULT '',
			group_kind TEXT NOT NULL DEFAULT 'manual',
			filter_json TEXT NOT NULL DEFAULT '',
			pinned INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'active',
			notes TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
			updated_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
			closed_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS sales_group_members (
			group_id TEXT NOT NULL,
			sales_id TEXT NOT NULL,
			sort_order INTEGER NOT NULL DEFAULT 0,
			added_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
			added_by TEXT NOT NULL DEFAULT '',
			member_kind TEXT NOT NULL DEFAULT 'manual',
			PRIMARY KEY (group_id, sales_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_group_members_sales ON sales_group_members(sales_id)`,
	} {
		if _, err := db.Exec(q); err != nil {
			log.Printf("44-P sales_groups: %v", err)
		}
	}
}

func NextGroupNo(db *sql.DB, t time.Time) (string, error) {
	if t.IsZero() {
		t = time.Now()
	}
	yy := t.Format("06")
	n, err := NextSeq(db, "sales_group_no:"+yy)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("G%s-%s", yy, fmtSeq3(n)), nil
}

func FilterJSONToList(f model.SalesListFilterJSON) SalesListFilter {
	return SalesListFilter{
		BizType:          f.BizType,
		BudgetYear:       f.BudgetYear,
		BudgetStatus:     f.BudgetStatus,
		Owner:            f.Owner,
		Stage:            f.Stage,
		ContractTarget:   f.ContractTarget,
		ProcurementRoute: f.ProcurementRoute,
		ContractMethod:   f.ContractMethod,
		BidEvalMethod:    f.BidEvalMethod,
		MallContractType: f.MallContractType,
		ExpectedFrom:     f.ExpectedFrom,
		ExpectedTo:       f.ExpectedTo,
	}
}

func (r *SalesRepo) CreateGroup(g *model.SalesGroup) error {
	if g == nil || strings.TrimSpace(g.Name) == "" {
		return fmt.Errorf("대분류 이름이 필요합니다")
	}
	if g.GroupKind != model.SalesGroupFilter {
		g.GroupKind = model.SalesGroupManual
	}
	g.Status = model.SalesGroupActive
	n, err := NextSeq(r.db, "sales_group")
	if err != nil {
		return err
	}
	g.GroupID = fmt.Sprintf("SG-%03d", n)
	no, err := NextGroupNo(r.db, time.Now())
	if err != nil {
		return err
	}
	g.GroupNo = no
	_, err = r.db.Exec(`
		INSERT INTO sales_groups (group_id, group_no, name, year, owner_id, owner_name, group_kind, filter_json, pinned, status, notes)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		g.GroupID, g.GroupNo, strings.TrimSpace(g.Name), g.Year, g.OwnerID, g.OwnerName, g.GroupKind, g.FilterJSON, boolToInt(g.Pinned), g.Status, g.Notes)
	if err != nil {
		return err
	}
	logCreate(r.db, "sales_groups", "group_id", g.GroupID, g.Name)
	return nil
}

func (r *SalesRepo) UpdateGroup(g *model.SalesGroup) error {
	if g == nil || strings.TrimSpace(g.GroupID) == "" {
		return fmt.Errorf("group_id 필요")
	}
	before := rowJSON(r.db, "sales_groups", "group_id", g.GroupID)
	if g.GroupKind != model.SalesGroupFilter {
		g.GroupKind = model.SalesGroupManual
	}
	_, err := r.db.Exec(`
		UPDATE sales_groups SET name=?, year=?, owner_id=?, owner_name=?, group_kind=?, filter_json=?, pinned=?, notes=?, updated_at=CURRENT_TIMESTAMP
		WHERE group_id=?`,
		strings.TrimSpace(g.Name), g.Year, g.OwnerID, g.OwnerName, g.GroupKind, g.FilterJSON, boolToInt(g.Pinned), g.Notes, g.GroupID)
	if err != nil {
		return err
	}
	logUpdate(r.db, "sales_groups", "group_id", g.GroupID, g.Name, before)
	return nil
}

func (r *SalesRepo) CloseGroup(id string) error {
	g, err := r.GetGroup(id)
	if err != nil {
		return err
	}
	before := rowJSON(r.db, "sales_groups", "group_id", id)
	_, err = r.db.Exec(`UPDATE sales_groups SET status=?, closed_at=?, updated_at=CURRENT_TIMESTAMP WHERE group_id=?`,
		model.SalesGroupClosed, time.Now().Format("2006-01-02 15:04:05"), id)
	if err != nil {
		return err
	}
	logUpdate(r.db, "sales_groups", "group_id", id, g.Name, before)
	return nil
}

func (r *SalesRepo) GetGroup(id string) (*model.SalesGroup, error) {
	row := r.db.QueryRow(`
		SELECT group_id, group_no, name, year, owner_id, owner_name, group_kind, filter_json, pinned, status, notes,
			created_at, updated_at, closed_at FROM sales_groups WHERE group_id=?`, id)
	return scanSalesGroup(row)
}

func (r *SalesRepo) ListGroups(includeClosed bool) ([]model.SalesGroup, error) {
	q := `SELECT group_id, group_no, name, year, owner_id, owner_name, group_kind, filter_json, pinned, status, notes,
		created_at, updated_at, closed_at FROM sales_groups`
	if !includeClosed {
		q += ` WHERE status='active'`
	}
	q += ` ORDER BY year DESC, group_no`
	rows, err := r.db.Query(q)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.SalesGroup
	for rows.Next() {
		g, err := scanSalesGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}

func (r *SalesRepo) ListPinnedGroups() ([]model.SalesGroup, error) {
	rows, err := r.db.Query(`
		SELECT group_id, group_no, name, year, owner_id, owner_name, group_kind, filter_json, pinned, status, notes,
			created_at, updated_at, closed_at FROM sales_groups WHERE pinned=1 AND status='active' ORDER BY group_no`)
	if err != nil {
		return nil, nil
	}
	defer rows.Close()
	var out []model.SalesGroup
	for rows.Next() {
		g, err := scanSalesGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}

func scanSalesGroup(row salesScanner) (*model.SalesGroup, error) {
	var g model.SalesGroup
	var pinned int
	err := row.Scan(&g.GroupID, &g.GroupNo, &g.Name, &g.Year, &g.OwnerID, &g.OwnerName, &g.GroupKind, &g.FilterJSON, &pinned, &g.Status, &g.Notes,
		&g.CreatedAt, &g.UpdatedAt, &g.ClosedAt)
	if err != nil {
		return nil, err
	}
	g.Pinned = pinned != 0
	return &g, nil
}

func (r *SalesRepo) AddGroupMembers(groupID string, salesIDs []string, kind, by string) error {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		kind = model.SalesMemberManual
	}
	g, err := r.GetGroup(groupID)
	if err != nil {
		return err
	}
	for _, sid := range salesIDs {
		sid = strings.TrimSpace(sid)
		if sid == "" {
			continue
		}
		_, err := r.db.Exec(`INSERT OR IGNORE INTO sales_group_members (group_id, sales_id, added_by, member_kind) VALUES (?,?,?,?)`,
			groupID, sid, by, kind)
		if err != nil {
			return err
		}
		logCreate(r.db, "sales_group_members", "sales_id", sid, g.Name)
	}
	return nil
}

func (r *SalesRepo) RemoveGroupMember(groupID, salesID string) error {
	before := rowJSON(r.db, "sales_group_members", "sales_id", salesID)
	g, _ := r.GetGroup(groupID)
	label := salesID
	if g != nil {
		label = g.Name
	}
	_, err := r.db.Exec(`DELETE FROM sales_group_members WHERE group_id=? AND sales_id=?`, groupID, salesID)
	if err != nil {
		return err
	}
	logDelete(r.db, "sales_group_members", "sales_id", salesID, label, before)
	return nil
}

func (r *SalesRepo) ListGroupMemberRows(groupID string) ([]model.SalesGroupMember, error) {
	rows, err := r.db.Query(`SELECT group_id, sales_id, sort_order, added_at, added_by, COALESCE(member_kind,'manual') FROM sales_group_members WHERE group_id=? ORDER BY sort_order, sales_id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.SalesGroupMember
	for rows.Next() {
		var m model.SalesGroupMember
		if err := rows.Scan(&m.GroupID, &m.SalesID, &m.SortOrder, &m.AddedAt, &m.AddedBy, &m.MemberKind); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *SalesRepo) ResolveGroupProjects(g *model.SalesGroup) ([]model.SalesProject, error) {
	if g == nil {
		return nil, sql.ErrNoRows
	}
	members, err := r.ListGroupMemberRows(g.GroupID)
	if err != nil {
		return nil, err
	}
	exclude := map[string]bool{}
	includeIDs := []string{}
	manualIDs := []string{}
	for _, m := range members {
		switch m.MemberKind {
		case model.SalesMemberExclude:
			exclude[m.SalesID] = true
		case model.SalesMemberInclude:
			includeIDs = append(includeIDs, m.SalesID)
		default:
			manualIDs = append(manualIDs, m.SalesID)
		}
	}
	var items []model.SalesProject
	if g.GroupKind == model.SalesGroupFilter {
		items, err = r.ListFilter(FilterJSONToList(g.Filter()))
		if err != nil {
			return nil, err
		}
	} else {
		for _, id := range manualIDs {
			p, err := r.Get(id)
			if err != nil {
				continue
			}
			items = append(items, *p)
		}
	}
	kept := items[:0]
	have := map[string]bool{}
	for _, p := range items {
		if exclude[p.SalesID] {
			continue
		}
		kept = append(kept, p)
		have[p.SalesID] = true
	}
	items = kept
	for _, id := range includeIDs {
		if have[id] || exclude[id] {
			continue
		}
		p, err := r.Get(id)
		if err != nil {
			continue
		}
		items = append(items, *p)
	}
	return items, nil
}

func (r *SalesRepo) GroupsForSales(salesID string) ([]model.SalesGroup, error) {
	rows, err := r.db.Query(`
		SELECT g.group_id, g.group_no, g.name, g.year, g.owner_id, g.owner_name, g.group_kind, g.filter_json, g.pinned, g.status, g.notes,
			g.created_at, g.updated_at, g.closed_at
		FROM sales_groups g
		JOIN sales_group_members m ON m.group_id=g.group_id
		WHERE m.sales_id=? AND g.status='active'
		ORDER BY g.group_no`, salesID)
	if err != nil {
		return nil, nil
	}
	defer rows.Close()
	var out []model.SalesGroup
	for rows.Next() {
		g, err := scanSalesGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}

func (r *SalesRepo) ReplaceSalesGroups(salesID string, groupIDs []string, by string) error {
	p, err := r.Get(salesID)
	if err != nil {
		return err
	}
	cur, _ := r.GroupsContaining(p)
	want := map[string]bool{}
	for _, id := range groupIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			want[id] = true
		}
	}
	have := map[string]bool{}
	for _, g := range cur {
		have[g.GroupID] = true
		if !want[g.GroupID] {
			if g.GroupKind == model.SalesGroupFilter {
				_, _ = r.db.Exec(`DELETE FROM sales_group_members WHERE group_id=? AND sales_id=?`, g.GroupID, salesID)
				if err := r.AddGroupMembers(g.GroupID, []string{salesID}, model.SalesMemberExclude, by); err != nil {
					return err
				}
			} else if err := r.RemoveGroupMember(g.GroupID, salesID); err != nil {
				return err
			}
		}
	}
	for id := range want {
		if have[id] {
			continue
		}
		g, err := r.GetGroup(id)
		if err != nil {
			continue
		}
		kind := model.SalesMemberManual
		if g.GroupKind == model.SalesGroupFilter {
			kind = model.SalesMemberInclude
		}
		if err := r.AddGroupMembers(id, []string{salesID}, kind, by); err != nil {
			return err
		}
	}
	return nil
}

func EncodeGroupFilter(f model.SalesListFilterJSON) string {
	b, err := json.Marshal(f)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func ProjectMatchesListFilter(p model.SalesProject, f SalesListFilter) bool {
	if s := strings.TrimSpace(f.BizType); s != "" && p.BizType != s {
		return false
	}
	if f.BudgetYear > 0 && p.BudgetYear != f.BudgetYear {
		return false
	}
	if s := strings.TrimSpace(f.BudgetStatus); s != "" && p.BudgetStatus != s {
		return false
	}
	if s := strings.TrimSpace(f.Owner); s != "" && p.SalesOwner != s && p.SalesOwnerID != s {
		return false
	}
	if s := strings.TrimSpace(f.Stage); s != "" && p.Stage != s {
		return false
	}
	if s := strings.TrimSpace(f.ContractTarget); s != "" && p.ContractTarget != s {
		return false
	}
	if s := strings.TrimSpace(f.ProcurementRoute); s != "" && p.ProcurementRoute != s {
		return false
	}
	if s := strings.TrimSpace(f.ContractMethod); s != "" && p.ContractMethod != s {
		return false
	}
	if s := strings.TrimSpace(f.BidEvalMethod); s != "" && p.BidEvalMethod != s {
		return false
	}
	if s := strings.TrimSpace(f.MallContractType); s != "" && p.MallContractType != s {
		return false
	}
	ym := model.NormalizeSalesYM(p.ExpectedYM)
	if from := model.NormalizeSalesYM(f.ExpectedFrom); from != "" && ym < from {
		return false
	}
	if to := model.NormalizeSalesYM(f.ExpectedTo); to != "" && (ym == "" || ym > to) {
		return false
	}
	return true
}

func (r *SalesRepo) CountHiddenDormant(f SalesListFilter) (int, error) {
	f.IncludeDormant = false
	vis, err := r.ListFilter(f)
	if err != nil {
		return 0, err
	}
	f.IncludeDormant = true
	all, err := r.ListFilter(f)
	if err != nil {
		return 0, err
	}
	n := len(all) - len(vis)
	if n < 0 {
		return 0, nil
	}
	return n, nil
}

func (r *SalesRepo) GroupsContaining(p *model.SalesProject) ([]model.SalesGroup, error) {
	if p == nil {
		return nil, nil
	}
	groups, err := r.ListGroups(false)
	if err != nil {
		return nil, err
	}
	var out []model.SalesGroup
	for i := range groups {
		ps, err := r.ResolveGroupProjects(&groups[i])
		if err != nil {
			continue
		}
		for _, x := range ps {
			if x.SalesID == p.SalesID {
				out = append(out, groups[i])
				break
			}
		}
	}
	return out, nil
}

func (r *SalesRepo) MemberSalesIDSet() (map[string]bool, error) {
	out := map[string]bool{}
	rows, err := r.db.Query(`SELECT DISTINCT sales_id FROM sales_group_members`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return out, err
		}
		out[id] = true
	}
	return out, rows.Err()
}
