package repository

import (
	"strings"
	"time"

	"customer-support/internal/model"
)

type SalesLogFilter struct {
	From   string
	To     string
	Search string
	User   string
	Kind   string
	Limit  int
}

type SalesLogRow struct {
	LogID       string
	OccurredAt  string
	UserName    string
	Kind        string
	KindLabel   string
	WhatNo      string
	WhatName    string
	EntityID    string
	TableName   string
	Action      string
	ActionLabel string
	Summary     string
	BeforeJSON  string
	AfterJSON   string
	Reason      string
	Diffs       []model.SalesLogDiffLine
	Legacy      bool
}

func salesLogKindLabel(kind, table string) string {
	switch kind {
	case "project":
		return "사업"
	case "activity":
		return "활동"
	case "quote":
		return "견적"
	case "memo":
		return "메모"
	case "party":
		return "관계자"
	case "group":
		return "대분류"
	case "legacy":
		return "이전 기록"
	}
	return table
}

func salesLogKindOf(table string) string {
	switch table {
	case "sales_projects":
		return "project"
	case "sales_activities":
		return "activity"
	case "sales_quotes":
		return "quote"
	case "sales_memos":
		return "memo"
	case "sales_parties":
		return "party"
	case "sales_groups", "sales_group_members":
		return "group"
	case "labor_rates":
		return "quote"
	default:
		return table
	}
}

func (r *SalesRepo) ListSalesLogs(f SalesLogFilter) ([]SalesLogRow, error) {
	if f.Limit <= 0 || f.Limit > 2000 {
		f.Limit = 500
	}
	from, to := strings.TrimSpace(f.From), strings.TrimSpace(f.To)
	if from == "" {
		from = time.Now().AddDate(0, -3, 0).Format("2006-01-02")
	}
	if to == "" {
		to = time.Now().Format("2006-01-02")
	}
	fromAt := from + " 00:00:00"
	toAt := to + " 23:59:59"
	kind := strings.TrimSpace(f.Kind)
	search := strings.TrimSpace(f.Search)
	user := strings.TrimSpace(f.User)

	tables := []string{
		"sales_projects", "sales_activities", "sales_quotes", "sales_memos",
		"sales_parties", "sales_groups", "sales_group_members", "labor_rates",
	}
	if kind != "" && kind != "legacy" {
		switch kind {
		case "project":
			tables = []string{"sales_projects"}
		case "activity":
			tables = []string{"sales_activities"}
		case "quote":
			tables = []string{"sales_quotes", "labor_rates"}
		case "memo":
			tables = []string{"sales_memos"}
		case "party":
			tables = []string{"sales_parties"}
		case "group":
			tables = []string{"sales_groups", "sales_group_members"}
		}
	}

	var out []SalesLogRow
	if kind != "legacy" {
		ph := make([]string, len(tables))
		args := make([]interface{}, 0, 16)
		for i, t := range tables {
			ph[i] = "?"
			args = append(args, t)
		}
		q := `
		SELECT l.log_id, l.occurred_at,
			COALESCE(NULLIF(TRIM(l.user_name),''), NULLIF(TRIM(l.username),''), '(시스템)'),
			l.action, l.table_name, COALESCE(l.entity_id,''),
			COALESCE(l.summary,''), COALESCE(l.before_json,''), COALESCE(l.after_json,''), COALESCE(l.reason,''),
			COALESCE(sp.sales_no, sq.quote_no, sap.sales_no, smp.sales_no, spp.sales_no, '') AS what_no,
			COALESCE(NULLIF(TRIM(sp.name),''), NULLIF(TRIM(sap.name),''), NULLIF(TRIM(smp.name),''), NULLIF(TRIM(spp.name),''), l.entity_label, '') AS what_name
		FROM data_change_logs l
		LEFT JOIN sales_projects sp ON l.table_name='sales_projects' AND l.entity_id=sp.sales_id
		LEFT JOIN sales_quotes sq ON l.table_name='sales_quotes' AND l.entity_id=sq.quote_id
		LEFT JOIN sales_activities sa ON l.table_name='sales_activities' AND l.entity_id=sa.activity_id
		LEFT JOIN sales_projects sap ON sa.sales_id=sap.sales_id
		LEFT JOIN sales_memos sm ON l.table_name='sales_memos' AND l.entity_id=sm.memo_id
		LEFT JOIN sales_projects smp ON sm.sales_id=smp.sales_id
		LEFT JOIN sales_parties pty ON l.table_name='sales_parties' AND l.entity_id=pty.party_id
		LEFT JOIN sales_projects spp ON pty.sales_id=spp.sales_id
		WHERE l.table_name IN (` + strings.Join(ph, ",") + `)
		  AND l.occurred_at >= ? AND l.occurred_at <= ?`
		args = append(args, fromAt, toAt)
		if user != "" {
			q += ` AND (l.user_name LIKE ? OR l.username LIKE ?)`
			like := "%" + user + "%"
			args = append(args, like, like)
		}
		if search != "" {
			like := "%" + search + "%"
			q += ` AND (COALESCE(sp.sales_no, sq.quote_no, sap.sales_no, smp.sales_no, spp.sales_no, l.entity_label, '') LIKE ?
			OR COALESCE(sp.name, sap.name, smp.name, spp.name, l.entity_label, '') LIKE ?)`
			args = append(args, like, like)
		}
		q += ` ORDER BY l.occurred_at DESC, l.log_id DESC LIMIT ?`
		args = append(args, f.Limit)
		rows, err := r.db.Query(q, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var row SalesLogRow
			if err := rows.Scan(&row.LogID, &row.OccurredAt, &row.UserName, &row.Action, &row.TableName,
				&row.EntityID, &row.Summary, &row.BeforeJSON, &row.AfterJSON, &row.Reason,
				&row.WhatNo, &row.WhatName); err != nil {
				return nil, err
			}
			row.Kind = salesLogKindOf(row.TableName)
			row.KindLabel = salesLogKindLabel(row.Kind, row.TableName)
			switch row.Action {
			case "create":
				row.ActionLabel = "등록"
			case "update":
				row.ActionLabel = "수정"
			case "delete":
				row.ActionLabel = "삭제"
			default:
				row.ActionLabel = row.Action
			}
			row.Diffs = model.DiffSalesLogJSON(row.BeforeJSON, row.AfterJSON)
			out = append(out, row)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	if kind == "" || kind == "legacy" {
		legacy, err := r.listLegacySalesChanges(fromAt, toAt, search, user, f.Limit)
		if err != nil {
			return nil, err
		}
		out = append(out, legacy...)
	}
	if len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

func (r *SalesRepo) listLegacySalesChanges(fromAt, toAt, search, user string, limit int) ([]SalesLogRow, error) {
	q := `
		SELECT c.change_id, COALESCE(c.changed_at,''), COALESCE(c.changed_by,''),
			c.sales_id, COALESCE(p.sales_no,''), COALESCE(p.name,''),
			COALESCE(c.field_key,''), COALESCE(c.old_value,''), COALESCE(c.new_value,''), COALESCE(c.note,'')
		FROM sales_changes c
		LEFT JOIN sales_projects p ON p.sales_id=c.sales_id
		WHERE COALESCE(c.changed_at,'') >= ? AND COALESCE(c.changed_at,'') <= ?`
	args := []interface{}{fromAt, toAt}
	if user != "" {
		q += ` AND c.changed_by LIKE ?`
		args = append(args, "%"+user+"%")
	}
	if search != "" {
		like := "%" + search + "%"
		q += ` AND (p.sales_no LIKE ? OR p.name LIKE ?)`
		args = append(args, like, like)
	}
	q += ` ORDER BY c.changed_at DESC, c.change_id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.Query(q, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []SalesLogRow
	for rows.Next() {
		var row SalesLogRow
		var field, oldV, newV string
		if err := rows.Scan(&row.LogID, &row.OccurredAt, &row.UserName, &row.EntityID, &row.WhatNo, &row.WhatName,
			&field, &oldV, &newV, &row.Reason); err != nil {
			return nil, err
		}
		row.Kind = "legacy"
		row.KindLabel = "이전 기록"
		row.Action = "update"
		row.ActionLabel = "수정"
		row.Legacy = true
		row.TableName = "sales_changes"
		row.Diffs = []model.SalesLogDiffLine{{
			Field: model.SalesLogFieldLabel(field),
			From:  model.SalesLogValueLabel(oldV),
			To:    model.SalesLogValueLabel(newV),
		}}
		out = append(out, row)
	}
	return out, rows.Err()
}
