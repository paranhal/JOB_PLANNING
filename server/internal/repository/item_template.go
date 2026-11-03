package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"customer-support/internal/model"
)

type ItemTemplateRepo struct{ db *sql.DB }

func NewItemTemplateRepo(db *sql.DB) *ItemTemplateRepo { return &ItemTemplateRepo{db: db} }

func (r *ItemTemplateRepo) nextID() (string, error) {
	n, err := NextSeq(r.db, "wb_item_tpl")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("WIT-%03d", n), nil
}

func (r *ItemTemplateRepo) Get(id string) (*model.ItemTemplate, error) {
	if r == nil || r.db == nil || strings.TrimSpace(id) == "" {
		return nil, nil
	}
	row := r.db.QueryRow(`SELECT template_id, COALESCE(org_id,''), kind, name, default_qty, COALESCE(unit,'건'),
		COALESCE(is_active,1), COALESCE(sort_order,0), COALESCE(note,''), COALESCE(created_by,'')
		FROM wb_item_templates WHERE template_id=?`, id)
	var t model.ItemTemplate
	var active int
	if err := row.Scan(&t.TemplateID, &t.OrgID, &t.Kind, &t.Name, &t.DefaultQty, &t.Unit, &active, &t.SortOrder, &t.Note, &t.CreatedBy); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	t.IsActive = active != 0
	t.Kind = model.NormalizeItemKind(t.Kind)
	t.KindLabel = model.ItemKindLabel(t.Kind)
	t.Unit = model.NormalizeItemUnit(t.Unit)
	if t.DefaultQty < 1 {
		t.DefaultQty = 1
	}
	links, err := r.listLinks([]string{t.TemplateID})
	if err != nil {
		return nil, err
	}
	t.Links = links[t.TemplateID]
	t.InUse = r.inUse(t.TemplateID)
	return &t, nil
}

func (r *ItemTemplateRepo) List(scopeOrg string, kind, q string, includeInactive, allOrgs bool) ([]model.ItemTemplate, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	sqlText := `SELECT template_id, COALESCE(org_id,''), kind, name, default_qty, COALESCE(unit,'건'),
		COALESCE(is_active,1), COALESCE(sort_order,0), COALESCE(note,''), COALESCE(created_by,'')
		FROM wb_item_templates WHERE 1=1`
	args := []interface{}{}
	if !allOrgs {
		sqlText += ` AND (org_id='' OR org_id=?)`
		args = append(args, strings.TrimSpace(scopeOrg))
	}
	if kind = strings.TrimSpace(kind); kind != "" {
		sqlText += ` AND kind=?`
		args = append(args, kind)
	}
	if !includeInactive {
		sqlText += ` AND is_active=1`
	}
	if q = strings.TrimSpace(q); q != "" {
		sqlText += ` AND name LIKE ?`
		args = append(args, "%"+q+"%")
	}
	sqlText += ` ORDER BY sort_order, name`
	rows, err := r.db.Query(sqlText, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.ItemTemplate
	var ids []string
	for rows.Next() {
		var t model.ItemTemplate
		var active int
		if err := rows.Scan(&t.TemplateID, &t.OrgID, &t.Kind, &t.Name, &t.DefaultQty, &t.Unit, &active, &t.SortOrder, &t.Note, &t.CreatedBy); err != nil {
			return nil, err
		}
		t.IsActive = active != 0
		t.Kind = model.NormalizeItemKind(t.Kind)
		t.KindLabel = model.ItemKindLabel(t.Kind)
		t.Unit = model.NormalizeItemUnit(t.Unit)
		if t.DefaultQty < 1 {
			t.DefaultQty = 1
		}
		if t.OrgID == "" {
			t.OrgLabel = "전사 공통"
		} else {
			t.OrgLabel = t.OrgID
		}
		out = append(out, t)
		ids = append(ids, t.TemplateID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	links, err := r.listLinks(ids)
	if err != nil {
		return nil, err
	}
	used := r.inUseSet(ids)
	for i := range out {
		out[i].Links = links[out[i].TemplateID]
		out[i].InUse = used[out[i].TemplateID]
		out[i].LinkSummary = linkSummary(out[i].Links)
	}
	return out, nil
}

func (r *ItemTemplateRepo) ListActiveForOrg(orgID string) ([]model.ItemTemplate, error) {
	orgID = strings.TrimSpace(orgID)
	all := orgID == "" || orgID == OrgAll
	return r.List(orgID, "", "", false, all)
}

func linkSummary(links []model.ItemTemplateLink) string {
	if len(links) == 0 {
		return ""
	}
	parts := make([]string, 0, len(links))
	for _, l := range links {
		tag := "기본 포함"
		if !l.DefaultChecked {
			tag = "기본 제외"
		}
		parts = append(parts, l.ChildName+"("+tag+")")
	}
	return strings.Join(parts, ", ")
}

func (r *ItemTemplateRepo) listLinks(ids []string) (map[string][]model.ItemTemplateLink, error) {
	out := map[string][]model.ItemTemplateLink{}
	if r == nil || r.db == nil || len(ids) == 0 {
		return out, nil
	}
	ph := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := r.db.Query(`SELECT l.template_id, l.child_template_id, COALESCE(l.default_checked,1), COALESCE(l.sort_order,0),
		COALESCE(c.name,''), COALESCE(c.kind,'etc'), COALESCE(c.unit,'건'), COALESCE(c.default_qty,1)
		FROM wb_item_template_links l
		LEFT JOIN wb_item_templates c ON c.template_id=l.child_template_id
		WHERE l.template_id IN (`+ph+`) ORDER BY l.sort_order, c.name`, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var l model.ItemTemplateLink
		var checked int
		if err := rows.Scan(&l.TemplateID, &l.ChildTemplateID, &checked, &l.SortOrder, &l.ChildName, &l.ChildKind, &l.ChildUnit, &l.DefaultQty); err != nil {
			return nil, err
		}
		l.DefaultChecked = checked != 0
		l.ChildKind = model.NormalizeItemKind(l.ChildKind)
		l.KindLabel = model.ItemKindLabel(l.ChildKind)
		if l.DefaultQty < 1 {
			l.DefaultQty = 1
		}
		out[l.TemplateID] = append(out[l.TemplateID], l)
	}
	return out, rows.Err()
}

func (r *ItemTemplateRepo) inUse(id string) bool {
	if r == nil || r.db == nil || strings.TrimSpace(id) == "" {
		return false
	}
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM work_task_items WHERE template_id=?`, id).Scan(&n)
	return n > 0
}

func (r *ItemTemplateRepo) inUseSet(ids []string) map[string]bool {
	out := map[string]bool{}
	if r == nil || r.db == nil || len(ids) == 0 {
		return out
	}
	ph := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := r.db.Query(`SELECT template_id, COUNT(*) FROM work_task_items WHERE template_id IN (`+ph+`) GROUP BY template_id`, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var n int
		if rows.Scan(&id, &n) == nil && n > 0 {
			out[id] = true
		}
	}
	return out
}

func (r *ItemTemplateRepo) Create(t *model.ItemTemplate, links []model.ItemTemplateLink) error {
	if t == nil {
		return fmt.Errorf("항목이 없습니다")
	}
	if err := r.validate(t, links, ""); err != nil {
		return err
	}
	id, err := r.nextID()
	if err != nil {
		return err
	}
	t.TemplateID = id
	if t.DefaultQty < 1 {
		t.DefaultQty = 1
	}
	t.Unit = model.NormalizeItemUnit(t.Unit)
	if _, err := r.db.Exec(`INSERT INTO wb_item_templates (template_id, org_id, kind, name, default_qty, unit, is_active, sort_order, note, created_by)
		VALUES (?,?,?,?,?,?,1,?,?,?)`,
		id, strings.TrimSpace(t.OrgID), model.NormalizeItemKind(t.Kind), strings.TrimSpace(t.Name),
		t.DefaultQty, t.Unit, t.SortOrder, strings.TrimSpace(t.Note), strings.TrimSpace(t.CreatedBy)); err != nil {
		return err
	}
	return r.replaceLinks(id, links)
}

func (r *ItemTemplateRepo) Update(t *model.ItemTemplate, links []model.ItemTemplateLink) error {
	if t == nil || strings.TrimSpace(t.TemplateID) == "" {
		return fmt.Errorf("항목이 없습니다")
	}
	if err := r.validate(t, links, t.TemplateID); err != nil {
		return err
	}
	if t.DefaultQty < 1 {
		t.DefaultQty = 1
	}
	t.Unit = model.NormalizeItemUnit(t.Unit)
	if _, err := r.db.Exec(`UPDATE wb_item_templates SET org_id=?, kind=?, name=?, default_qty=?, unit=?, sort_order=?, note=?,
		updated_at=CURRENT_TIMESTAMP WHERE template_id=?`,
		strings.TrimSpace(t.OrgID), model.NormalizeItemKind(t.Kind), strings.TrimSpace(t.Name),
		t.DefaultQty, t.Unit, t.SortOrder, strings.TrimSpace(t.Note), t.TemplateID); err != nil {
		return err
	}
	return r.replaceLinks(t.TemplateID, links)
}

func (r *ItemTemplateRepo) Deactivate(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("항목이 없습니다")
	}
	_, err := r.db.Exec(`UPDATE wb_item_templates SET is_active=0, updated_at=CURRENT_TIMESTAMP WHERE template_id=?`, id)
	return err
}

func (r *ItemTemplateRepo) Delete(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("항목이 없습니다")
	}
	if r.inUse(id) {
		return fmt.Errorf("사용 중인 항목은 삭제할 수 없습니다. 비활성만 가능합니다")
	}
	if _, err := r.db.Exec(`DELETE FROM wb_item_template_links WHERE template_id=? OR child_template_id=?`, id, id); err != nil {
		return err
	}
	_, err := r.db.Exec(`DELETE FROM wb_item_templates WHERE template_id=?`, id)
	return err
}

func (r *ItemTemplateRepo) replaceLinks(id string, links []model.ItemTemplateLink) error {
	if _, err := r.db.Exec(`DELETE FROM wb_item_template_links WHERE template_id=?`, id); err != nil {
		return err
	}
	for i, l := range links {
		cid := strings.TrimSpace(l.ChildTemplateID)
		if cid == "" {
			continue
		}
		checked := 0
		if l.DefaultChecked {
			checked = 1
		}
		ord := l.SortOrder
		if ord == 0 {
			ord = i + 1
		}
		if _, err := r.db.Exec(`INSERT INTO wb_item_template_links (template_id, child_template_id, default_checked, sort_order) VALUES (?,?,?,?)`,
			id, cid, checked, ord); err != nil {
			return err
		}
	}
	return nil
}

func (r *ItemTemplateRepo) validate(t *model.ItemTemplate, links []model.ItemTemplateLink, selfID string) error {
	name := strings.TrimSpace(t.Name)
	if name == "" {
		return fmt.Errorf("이름을 입력하세요")
	}
	if !model.ValidItemKindCode(t.Kind) {
		return fmt.Errorf("구분 값이 올바르지 않습니다")
	}
	if t.DefaultQty < 1 {
		return fmt.Errorf("수량은 1 이상이어야 합니다")
	}
	if !model.ValidItemUnitCode(t.Unit, nil) {
		return fmt.Errorf("단위 값이 올바르지 않습니다")
	}
	if err := r.dupName(t.OrgID, t.Kind, name, selfID); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, l := range links {
		cid := strings.TrimSpace(l.ChildTemplateID)
		if cid == "" {
			continue
		}
		if cid == selfID || (selfID == "" && cid == t.TemplateID) {
			return fmt.Errorf("자기 자신을 세트에 넣을 수 없습니다")
		}
		if seen[cid] {
			continue
		}
		seen[cid] = true
		if selfID != "" {
			if ok, err := r.canReach(cid, selfID); err != nil {
				return err
			} else if ok {
				return fmt.Errorf("세트 순환은 저장할 수 없습니다")
			}
		}
		child, err := r.Get(cid)
		if err != nil {
			return err
		}
		if child == nil {
			return fmt.Errorf("세트 항목을 찾을 수 없습니다")
		}
		parentOrg := strings.TrimSpace(t.OrgID)
		childOrg := strings.TrimSpace(child.OrgID)
		if parentOrg == "" && childOrg != "" {
			return fmt.Errorf("전사 공통 항목은 조직 전용 항목을 세트에 넣을 수 없습니다")
		}
		if parentOrg != "" && childOrg != "" && parentOrg != childOrg {
			return fmt.Errorf("다른 조직 항목은 세트에 넣을 수 없습니다")
		}
	}
	return nil
}

func (r *ItemTemplateRepo) dupName(orgID, kind, name, exceptID string) error {
	keyOrg := strings.TrimSpace(orgID)
	keyKind := model.NormalizeItemKind(kind)
	keyName := strings.ToLower(strings.TrimSpace(name))
	rows, err := r.db.Query(`SELECT template_id, COALESCE(org_id,''), kind, name FROM wb_item_templates`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, o, k, n string
		if err := rows.Scan(&id, &o, &k, &n); err != nil {
			return err
		}
		if exceptID != "" && id == exceptID {
			continue
		}
		if model.ItemTemplateNameKey(o, k, n) == model.ItemTemplateNameKey(keyOrg, keyKind, keyName) {
			return fmt.Errorf("같은 구분·범위에 같은 이름이 있습니다")
		}
	}
	return rows.Err()
}

func (r *ItemTemplateRepo) canReach(from, target string) (bool, error) {
	if from == "" || target == "" {
		return false, nil
	}
	seen := map[string]bool{}
	q := []string{from}
	for len(q) > 0 {
		cur := q[0]
		q = q[1:]
		if cur == target {
			return true, nil
		}
		if seen[cur] {
			continue
		}
		seen[cur] = true
		rows, err := r.db.Query(`SELECT child_template_id FROM wb_item_template_links WHERE template_id=?`, cur)
		if err != nil {
			return false, err
		}
		for rows.Next() {
			var cid string
			if rows.Scan(&cid) == nil && cid != "" && !seen[cid] {
				q = append(q, cid)
			}
		}
		rows.Close()
	}
	return false, nil
}

func ItemTemplatePickJSON(list []model.ItemTemplate) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(list))
	for _, t := range list {
		links := make([]map[string]interface{}, 0, len(t.Links))
		for _, l := range t.Links {
			links = append(links, map[string]interface{}{
				"TemplateID":     l.ChildTemplateID,
				"Name":           l.ChildName,
				"Kind":           l.ChildKind,
				"KindLabel":      l.KindLabel,
				"Unit":           l.ChildUnit,
				"DefaultQty":     l.DefaultQty,
				"DefaultChecked": l.DefaultChecked,
			})
		}
		out = append(out, map[string]interface{}{
			"TemplateID": t.TemplateID,
			"Name":       t.Name,
			"Kind":       t.Kind,
			"KindLabel":  t.KindLabel,
			"Unit":       t.Unit,
			"DefaultQty": t.DefaultQty,
			"OrgID":      t.OrgID,
			"Links":      links,
		})
	}
	return out
}
