package repository

import (
	"context"
	"database/sql"
	"strings"
)

func sqliteTableExists(db *sql.DB, name string) bool {
	if db == nil || strings.TrimSpace(name) == "" {
		return false
	}
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n)
	return err == nil && n > 0
}

func tablesWithColumn(db *sql.DB, col string) []string {
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return out
		}
		if tableHasColumn(db, name, col) {
			out = append(out, name)
		}
	}
	return out
}

func execDelete(db *sql.DB, table, q string, args ...interface{}) (int64, error) {
	if !sqliteTableExists(db, table) {
		return 0, nil
	}
	res, err := db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// orgOrphanDeletes 자식 → 부모. 표가 없으면 건너뛴다.
var orgOrphanDeletes = []struct {
	table string
	sql   string
}{
	{"customer_rooms", `DELETE FROM customer_rooms WHERE floor_id NOT IN (SELECT floor_id FROM customer_floors)`},
	{"customer_floors", `DELETE FROM customer_floors WHERE building_id NOT IN (SELECT building_id FROM customer_buildings)`},
	{"customer_buildings", `DELETE FROM customer_buildings WHERE customer_id NOT IN (SELECT customer_id FROM customers)`},
	{"contact_history", `DELETE FROM contact_history WHERE contact_id NOT IN (SELECT contact_id FROM contacts)`},
	{"contacts", `DELETE FROM contacts WHERE customer_id NOT IN (SELECT customer_id FROM customers)`},
	{"asset_sw_details", `DELETE FROM asset_sw_details WHERE asset_id NOT IN (SELECT asset_id FROM assets)`},
	{"access_info_references", `DELETE FROM access_info_references WHERE asset_id NOT IN (SELECT asset_id FROM assets)`},
	{"performance_relations", `DELETE FROM performance_relations WHERE (customer_id IS NOT NULL AND TRIM(customer_id)!='' AND customer_id NOT IN (SELECT customer_id FROM customers)) OR (asset_id IS NOT NULL AND TRIM(asset_id)!='' AND asset_id NOT IN (SELECT asset_id FROM assets))`},
	{"as_processes", `DELETE FROM as_processes WHERE as_id NOT IN (SELECT as_id FROM as_receipts)`},
	{"as_work_items", `DELETE FROM as_work_items WHERE as_id NOT IN (SELECT as_id FROM as_receipts)`},
	{"as_keyword_links", `DELETE FROM as_keyword_links WHERE as_id NOT IN (SELECT as_id FROM as_receipts)`},
	{"as_case_votes", `DELETE FROM as_case_votes WHERE as_id NOT IN (SELECT as_id FROM as_receipts)`},
	{"as_kb_entries", `DELETE FROM as_kb_entries WHERE as_id IS NOT NULL AND TRIM(as_id)!='' AND as_id NOT IN (SELECT as_id FROM as_receipts)`},
	{"as_kb_gaps", `DELETE FROM as_kb_gaps WHERE as_id IS NOT NULL AND TRIM(as_id)!='' AND as_id NOT IN (SELECT as_id FROM as_receipts)`},
	{"maintenance_visits", `DELETE FROM maintenance_visits WHERE customer_id NOT IN (SELECT customer_id FROM customers) OR plan_id NOT IN (SELECT plan_id FROM maintenance_plans)`},
	{"work_actions", `DELETE FROM work_actions WHERE task_id NOT IN (SELECT task_id FROM work_tasks)`},
	{"work_activities", `DELETE FROM work_activities WHERE task_id NOT IN (SELECT task_id FROM work_tasks)`},
	{"work_task_members", `DELETE FROM work_task_members WHERE task_id NOT IN (SELECT task_id FROM work_tasks)`},
	{"work_task_tags", `DELETE FROM work_task_tags WHERE task_id NOT IN (SELECT task_id FROM work_tasks)`},
	{"work_task_assets", `DELETE FROM work_task_assets WHERE task_id NOT IN (SELECT task_id FROM work_tasks)`},
	{"work_assign_notices", `DELETE FROM work_assign_notices WHERE source_id NOT IN (SELECT task_id FROM work_tasks) AND source_id NOT IN (SELECT as_id FROM as_receipts)`},
	{"work_recurrence", `DELETE FROM work_recurrence WHERE task_id NOT IN (SELECT task_id FROM work_tasks)`},
	{"project_scope_product_keys", `DELETE FROM project_scope_product_keys WHERE rule_id NOT IN (SELECT rule_id FROM project_scope_rules)`},
	{"project_scope_work_kinds", `DELETE FROM project_scope_work_kinds WHERE rule_id NOT IN (SELECT rule_id FROM project_scope_rules)`},
	{"project_scope_rules", `DELETE FROM project_scope_rules WHERE project_id NOT IN (SELECT project_id FROM work_projects)`},
	{"sales_quote_lines", `DELETE FROM sales_quote_lines WHERE quote_id NOT IN (SELECT quote_id FROM sales_quotes)`},
	{"sales_order_lines", `DELETE FROM sales_order_lines WHERE order_id NOT IN (SELECT order_id FROM sales_orders)`},
	{"sales_purchases", `DELETE FROM sales_purchases WHERE order_id NOT IN (SELECT order_id FROM sales_orders)`},
	{"sales_deliveries", `DELETE FROM sales_deliveries WHERE order_id NOT IN (SELECT order_id FROM sales_orders)`},
	{"sales_activity_members", `DELETE FROM sales_activity_members WHERE activity_id NOT IN (SELECT activity_id FROM sales_activities)`},
	{"sales_activities", `DELETE FROM sales_activities WHERE sales_id NOT IN (SELECT sales_id FROM sales_projects)`},
	{"sales_stage_history", `DELETE FROM sales_stage_history WHERE sales_id NOT IN (SELECT sales_id FROM sales_projects)`},
	{"sales_parties", `DELETE FROM sales_parties WHERE sales_id NOT IN (SELECT sales_id FROM sales_projects)`},
	{"sales_changes", `DELETE FROM sales_changes WHERE sales_id NOT IN (SELECT sales_id FROM sales_projects)`},
	{"sales_memos", `DELETE FROM sales_memos WHERE sales_id NOT IN (SELECT sales_id FROM sales_projects)`},
	{"sales_group_members", `DELETE FROM sales_group_members WHERE group_id NOT IN (SELECT group_id FROM sales_groups) OR sales_id NOT IN (SELECT sales_id FROM sales_projects)`},
	{"sales_item_specs", `DELETE FROM sales_item_specs WHERE item_id NOT IN (SELECT item_id FROM sales_items)`},
	{"sales_item_supplier_prices", `DELETE FROM sales_item_supplier_prices WHERE item_id NOT IN (SELECT item_id FROM sales_items)`},
}

func pruneOrphans(db *sql.DB) (map[string]int64, error) {
	out := map[string]int64{}
	for _, r := range orgOrphanDeletes {
		n, err := execDelete(db, r.table, r.sql)
		if err != nil {
			return out, err
		}
		if n > 0 {
			out[r.table] += n
		}
	}
	if sqliteTableExists(db, "attachments") {
		n, err := execDelete(db, "attachments", `
			DELETE FROM attachments WHERE
			  (ref_id NOT IN (SELECT as_id FROM as_receipts)
			   AND ref_id NOT IN (SELECT asset_id FROM assets)
			   AND ref_id NOT IN (SELECT customer_id FROM customers)
			   AND ref_id NOT IN (SELECT task_id FROM work_tasks)
			   AND ref_id NOT IN (SELECT quote_id FROM sales_quotes)
			   AND ref_id NOT IN (SELECT order_id FROM sales_orders)
			   AND ref_id NOT IN (SELECT process_id FROM as_processes)
			   AND ref_id NOT IN (SELECT sales_id FROM sales_projects))`)
		if err != nil {
			return out, err
		}
		if n > 0 {
			out["attachments"] += n
		}
	}
	return out, nil
}

func pruneOrphansConn(ctx context.Context, conn *sql.Conn) (map[string]int64, error) {
	out := map[string]int64{}
	has := func(table string) bool {
		var n int
		err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n)
		return err == nil && n > 0
	}
	run := func(table, q string) error {
		if !has(table) {
			return nil
		}
		res, err := conn.ExecContext(ctx, q)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n > 0 {
			out[table] += n
		}
		return nil
	}
	for _, r := range orgOrphanDeletes {
		if err := run(r.table, r.sql); err != nil {
			return out, err
		}
	}
	if err := run("attachments", `
			DELETE FROM attachments WHERE
			  (ref_id NOT IN (SELECT as_id FROM as_receipts)
			   AND ref_id NOT IN (SELECT asset_id FROM assets)
			   AND ref_id NOT IN (SELECT customer_id FROM customers)
			   AND ref_id NOT IN (SELECT task_id FROM work_tasks)
			   AND ref_id NOT IN (SELECT quote_id FROM sales_quotes)
			   AND ref_id NOT IN (SELECT order_id FROM sales_orders)
			   AND ref_id NOT IN (SELECT process_id FROM as_processes)
			   AND ref_id NOT IN (SELECT sales_id FROM sales_projects))`); err != nil {
		return out, err
	}
	return out, nil
}
