package repository

import (
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	ResetScopeTest = "test"
	ResetScopeOrg  = "org"
	ResetScopeAll  = "all"
)

type ResetCount struct {
	Table string
	N     int64
}

var resetIdent = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func NormalizeResetScope(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case ResetScopeOrg, "2", "조직", "조직전체":
		return ResetScopeOrg
	case ResetScopeAll, "3", "전조직", "전 조직":
		return ResetScopeAll
	default:
		return ResetScopeTest
	}
}

func skipResetTable(name string) bool {
	switch name {
	case "data_change_logs", "data_backups", "data_log_archives",
		"admin_unlocks", "admin_unlock_log", "as_edit_unlocks", "as_edit_unlock_log",
		"users", "orgs", "codes", "as_cause_categories", "as_keywords",
		"app_settings", "app_versions", "schema_migrations", "id_sequences",
		"sqlite_sequence":
		return true
	}
	return strings.HasPrefix(name, "sqlite_")
}

func PreviewReset(db *sql.DB, scope, orgID string) ([]ResetCount, error) {
	return resetCounts(db, scope, orgID, false)
}

func ExecuteReset(db *sql.DB, scope, orgID string) ([]ResetCount, error) {
	return resetCounts(db, scope, orgID, true)
}

func resetCounts(db *sql.DB, scope, orgID string, apply bool) ([]ResetCount, error) {
	if db == nil {
		return nil, fmt.Errorf("DB 연결이 없습니다")
	}
	scope = NormalizeResetScope(scope)
	orgID = strings.TrimSpace(orgID)
	if scope == ResetScopeOrg && (orgID == "" || orgID == OrgAll) {
		return nil, fmt.Errorf("조직 전체 초기화는 조직을 고른 뒤에만 됩니다")
	}

	var tables []string
	switch scope {
	case ResetScopeTest:
		tables = tablesWithColumn(db, "is_test")
	case ResetScopeOrg:
		tables = tablesWithColumn(db, "org_id")
	default:
		seen := map[string]bool{}
		for _, t := range append(append([]string{}, orgRootTables...), "maintenance_visits", "as_work_items") {
			if sqliteTableExists(db, t) {
				seen[t] = true
			}
		}
		for _, t := range tablesWithColumn(db, "org_id") {
			seen[t] = true
		}
		for t := range seen {
			tables = append(tables, t)
		}
		sort.Strings(tables)
	}

	if apply {
		if _, err := db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
			return nil, err
		}
		defer func() { _, _ = db.Exec(`PRAGMA foreign_keys=ON`) }()
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	out := make([]ResetCount, 0, len(tables))
	for _, t := range tables {
		if skipResetTable(t) || !resetIdent.MatchString(t) {
			continue
		}
		where, args, ok := resetWhere(tx, t, scope, orgID)
		if !ok {
			continue
		}
		qIdent := `"` + t + `"`
		var n int64
		if apply {
			res, err := tx.Exec(`DELETE FROM `+qIdent+` WHERE `+where, args...)
			if err != nil {
				return nil, fmt.Errorf("%s 삭제: %w", t, err)
			}
			n, _ = res.RowsAffected()
		} else {
			if err := tx.QueryRow(`SELECT COUNT(*) FROM `+qIdent+` WHERE `+where, args...).Scan(&n); err != nil {
				return nil, fmt.Errorf("%s 건수: %w", t, err)
			}
		}
		if n > 0 {
			out = append(out, ResetCount{Table: t, N: n})
		}
	}

	if apply {
		if err := pruneOrphansTx(tx); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func resetWhere(tx *sql.Tx, table, scope, orgID string) (string, []interface{}, bool) {
	has := func(col string) bool {
		var n int
		err := tx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('`+table+`') WHERE name=?`, col).Scan(&n)
		return err == nil && n > 0
	}
	switch scope {
	case ResetScopeTest:
		if !has("is_test") {
			return "", nil, false
		}
		return `COALESCE(is_test,0)=1`, nil, true
	case ResetScopeOrg:
		if !has("org_id") {
			return "", nil, false
		}
		return `org_id=?`, []interface{}{orgID}, true
	default:
		return `1=1`, nil, true
	}
}

func pruneOrphansTx(tx *sql.Tx) error {
	has := func(table string) bool {
		var n int
		err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n)
		return err == nil && n > 0
	}
	for _, r := range orgOrphanDeletes {
		if !has(r.table) {
			continue
		}
		if _, err := tx.Exec(r.sql); err != nil {
			return fmt.Errorf("%s 고아 정리: %w", r.table, err)
		}
	}
	if has("attachments") {
		if _, err := tx.Exec(`
			DELETE FROM attachments WHERE
			  (ref_id NOT IN (SELECT as_id FROM as_receipts)
			   AND ref_id NOT IN (SELECT asset_id FROM assets)
			   AND ref_id NOT IN (SELECT customer_id FROM customers)
			   AND ref_id NOT IN (SELECT task_id FROM work_tasks)
			   AND ref_id NOT IN (SELECT quote_id FROM sales_quotes)
			   AND ref_id NOT IN (SELECT order_id FROM sales_orders)
			   AND ref_id NOT IN (SELECT process_id FROM as_processes)
			   AND ref_id NOT IN (SELECT sales_id FROM sales_projects))`); err != nil {
			return fmt.Errorf("attachments 고아 정리: %w", err)
		}
	}
	return nil
}
