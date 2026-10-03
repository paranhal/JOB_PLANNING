package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"customer-support/internal/audit"
	"customer-support/internal/model"
)

type OrgCounts struct {
	Users     int `json:"users"`
	Customers int `json:"customers"`
	Assets    int `json:"assets"`
	AS        int `json:"as"`
	Sales     int `json:"sales"`
	Work      int `json:"work"`
	Visits    int `json:"visits"`
}

type OrgBackupMeta struct {
	Kind     string    `json:"kind"`
	OrgID    string    `json:"org_id"`
	OrgName  string    `json:"org_name"`
	BackupAt string    `json:"backup_at"`
	Counts   OrgCounts `json:"counts"`
}

func (r *OrgRepo) CountOrg(orgID string) (OrgCounts, error) {
	var c OrgCounts
	orgID = strings.TrimSpace(orgID)
	if orgID == "" || orgID == OrgAll {
		return c, ErrEmptyOrgID
	}
	q := func(sqlStr string, dest *int) error {
		return r.db.QueryRow(sqlStr, orgID).Scan(dest)
	}
	if err := q(`SELECT COUNT(*) FROM users WHERE org_id=? AND COALESCE(role,'')<>'vision_admin'`, &c.Users); err != nil {
		return c, err
	}
	if err := q(`SELECT COUNT(*) FROM customers WHERE org_id=?`, &c.Customers); err != nil {
		return c, err
	}
	if err := q(`SELECT COUNT(*) FROM assets WHERE org_id=?`, &c.Assets); err != nil {
		return c, err
	}
	if err := q(`SELECT COUNT(*) FROM as_receipts WHERE org_id=?`, &c.AS); err != nil {
		return c, err
	}
	if err := q(`SELECT COUNT(*) FROM sales_projects WHERE org_id=?`, &c.Sales); err != nil {
		return c, err
	}
	if err := q(`SELECT COUNT(*) FROM work_tasks WHERE org_id=?`, &c.Work); err != nil {
		return c, err
	}
	if sqliteTableExists(r.db, "maintenance_visits") {
		_ = r.db.QueryRow(`SELECT COUNT(*) FROM maintenance_visits v
			JOIN customers c ON c.customer_id=v.customer_id WHERE c.org_id=?`, orgID).Scan(&c.Visits)
	}
	return c, nil
}

func countSQL(db *sql.DB, q string, args ...interface{}) int {
	var n int
	_ = db.QueryRow(q, args...).Scan(&n)
	return n
}

func (r *OrgRepo) WriteOrgSnapshot(destDir, orgID, kind string) (*OrgBackupMeta, error) {
	org, err := r.Get(orgID)
	if err != nil {
		return nil, err
	}
	if org == nil {
		return nil, fmt.Errorf("조직을 찾지 못했습니다")
	}
	counts, err := r.CountOrg(orgID)
	if err != nil {
		return nil, err
	}
	now := time.Now().In(time.Local)
	meta := &OrgBackupMeta{
		Kind: kind, OrgID: org.OrgID, OrgName: org.OrgName,
		BackupAt: now.Format(time.RFC3339), Counts: counts,
	}
	dbPath := filepath.Join(destDir, "org.db")
	if err := vacuumOrgInto(r.db, dbPath); err != nil {
		return nil, fmt.Errorf("org.db: %w", err)
	}
	snap, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(0)")
	if err != nil {
		return nil, err
	}
	defer snap.Close()
	if err := pruneSnapshotToOrg(snap, orgID); err != nil {
		return nil, err
	}
	jb, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(destDir, "org.json"), jb, 0644); err != nil {
		return nil, err
	}
	txt := fmt.Sprintf("backup_at=%s\nkind=%s\norg_id=%s\norg_name=%s\ndb=org.db\nusers=%d\ncustomers=%d\nas=%d\nsales=%d\nwork=%d\n",
		meta.BackupAt, kind, org.OrgID, org.OrgName, counts.Users, counts.Customers, counts.AS, counts.Sales, counts.Work)
	if err := os.WriteFile(filepath.Join(destDir, "backup.txt"), []byte(txt), 0644); err != nil {
		return nil, err
	}
	return meta, nil
}

func vacuumOrgInto(db *sql.DB, dest string) error {
	abs, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return err
	}
	_ = os.Remove(abs)
	var src string
	if err := db.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&src); err != nil || strings.TrimSpace(src) == "" {
		q := "VACUUM INTO '" + strings.ReplaceAll(filepath.ToSlash(abs), "'", "''") + "'"
		_, err = db.Exec(q)
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(abs)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	for _, suf := range []string{"-wal", "-shm"} {
		sb, err := os.ReadFile(src + suf)
		if err != nil {
			continue
		}
		_ = os.WriteFile(abs+suf, sb, 0644)
	}
	return nil
}

func pruneSnapshotToOrg(db *sql.DB, orgID string) error {
	dropWorkTaskMemberTriggers(db)
	if _, err := db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		return err
	}
	for _, t := range tablesWithColumn(db, "org_id") {
		if t == "orgs" {
			continue
		}
		if _, err := execDelete(db, t, `DELETE FROM `+t+` WHERE TRIM(COALESCE(org_id,'')) != ?`, orgID); err != nil {
			return fmt.Errorf("%s: %w", t, err)
		}
	}
	if _, err := execDelete(db, "users", `DELETE FROM users WHERE TRIM(COALESCE(org_id,'')) != ?`, orgID); err != nil {
		return err
	}
	if _, err := execDelete(db, "orgs", `DELETE FROM orgs WHERE org_id != ?`, orgID); err != nil {
		return err
	}
	if _, err := pruneOrphans(db); err != nil {
		return err
	}
	return nil
}

func (r *OrgRepo) PurgeOrg(orgID string) (map[string]int64, error) {
	org, err := r.Get(orgID)
	if err != nil {
		return nil, err
	}
	if org == nil {
		return nil, fmt.Errorf("조직을 찾지 못했습니다")
	}
	if org.IsActive {
		return nil, fmt.Errorf("활성 조직은 지울 수 없습니다. 먼저 숨기세요")
	}
	counts, _ := r.CountOrg(orgID)
	dropWorkTaskMemberTriggers(r.db)
	defer installWorkTaskMemberTriggers(r.db)
	ctx := context.Background()
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
		return nil, err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	deleted := map[string]int64{}
	for _, t := range tablesWithColumn(r.db, "org_id") {
		if t == "orgs" || t == "users" {
			continue
		}
		res, err := tx.Exec(`DELETE FROM `+t+` WHERE TRIM(COALESCE(org_id,'')) = ?`, orgID)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t, err)
		}
		n, _ := res.RowsAffected()
		if n > 0 {
			deleted[t] = n
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	orphans, err := pruneOrphansConn(ctx, conn)
	if err != nil {
		return nil, err
	}
	for k, v := range orphans {
		deleted[k] += v
	}
	if _, err := conn.ExecContext(ctx, `UPDATE users SET org_id='' WHERE org_id=?`, orgID); err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM orgs WHERE org_id=?`, orgID); err != nil {
		return nil, err
	}
	before, _ := json.Marshal(counts)
	after, _ := json.Marshal(deleted)
	audit.Use(r.db)
	audit.LogWithReason(audit.ActionDelete, "orgs", "org_id", orgID, org.OrgName, string(before), string(after), "조직 완전삭제 표별 건수")
	return deleted, nil
}

var ErrOrgAlive = fmt.Errorf("같은 조직번호가 아직 있습니다. 덮어쓰지 않습니다")

func (r *OrgRepo) RestoreOrg(orgDBPath string) error {
	orgDBPath = strings.TrimSpace(orgDBPath)
	if orgDBPath == "" {
		return fmt.Errorf("백업 파일이 없습니다")
	}
	src, err := sql.Open("sqlite", orgDBPath+"?mode=ro")
	if err != nil {
		return err
	}
	defer src.Close()
	var orgID string
	if err := src.QueryRow(`SELECT org_id FROM orgs LIMIT 1`).Scan(&orgID); err != nil {
		return fmt.Errorf("org.db 에 조직이 없습니다: %w", err)
	}
	live, err := r.Get(orgID)
	if err != nil {
		return err
	}
	if live != nil {
		return ErrOrgAlive
	}
	abs, err := filepath.Abs(orgDBPath)
	if err != nil {
		return err
	}
	ctx := context.Background()
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	attach := `ATTACH DATABASE '` + strings.ReplaceAll(filepath.ToSlash(abs), "'", "''") + `' AS src`
	if _, err := conn.ExecContext(ctx, attach); err != nil {
		return err
	}
	defer func() { _, _ = conn.ExecContext(ctx, `DETACH DATABASE src`) }()

	skip := map[string]bool{}
	for _, t := range orgSharedTables {
		skip[t] = true
	}
	for _, t := range []string{
		"sqlite_sequence", "data_change_logs", "data_backups", "data_log_archives",
		"access_logs", "as_edit_unlocks", "as_edit_unlock_log",
		"as_keyword_links", "as_keyword_stopwords", "as_keywords",
	} {
		skip[t] = true
	}
	srcTables, err := listTables(src)
	if err != nil {
		return err
	}
	order := append([]string{"orgs", "users"}, orgRootTables...)
	seen := map[string]bool{}
	var ordered []string
	for _, t := range order {
		seen[t] = true
		ordered = append(ordered, t)
	}
	for _, t := range srcTables {
		if skip[t] || seen[t] {
			continue
		}
		ordered = append(ordered, t)
	}
	dropWorkTaskMemberTriggers(r.db)
	defer installWorkTaskMemberTriggers(r.db)
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		return err
	}
	for _, t := range ordered {
		if skip[t] || !sqliteTableExists(r.db, t) {
			continue
		}
		var srcHas int
		_ = tx.QueryRow(`SELECT COUNT(*) FROM src.sqlite_master WHERE type='table' AND name=?`, t).Scan(&srcHas)
		if srcHas == 0 {
			continue
		}
		cols := intersectColumns(tx, t)
		if len(cols) == 0 {
			continue
		}
		colSQL := strings.Join(cols, ",")
		q := fmt.Sprintf(`INSERT OR IGNORE INTO %s (%s) SELECT %s FROM src.%s`, t, colSQL, colSQL, t)
		if _, err := tx.Exec(q); err != nil {
			return fmt.Errorf("복구 %s: %w", t, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	audit.Use(r.db)
	audit.LogWithReason(audit.ActionCreate, "orgs", "org_id", orgID, orgID, "", "", "조직 복구")
	return nil
}

func listTables(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func intersectColumns(tx *sql.Tx, table string) []string {
	live := pragmaCols(tx, `PRAGMA table_info(`+table+`)`)
	src := pragmaCols(tx, `PRAGMA src.table_info(`+table+`)`)
	srcSet := map[string]bool{}
	for _, c := range src {
		srcSet[c] = true
	}
	var out []string
	for _, c := range live {
		if srcSet[c] {
			out = append(out, c)
		}
	}
	return out
}

func pragmaCols(tx *sql.Tx, q string) []string {
	rows, err := tx.Query(q)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return out
		}
		out = append(out, name)
	}
	return out
}

type OrgSplitPreview struct {
	FromOrg     string `json:"from_org"`
	ToOrg       string `json:"to_org"`
	Customers   int    `json:"customers"`
	Assets      int    `json:"assets"`
	AS          int    `json:"as"`
	Sales       int    `json:"sales"`
	Work        int    `json:"work"`
	Visits      int    `json:"visits"`
	CustomerIDs []string
}

func (r *OrgRepo) ListOrgCustomers(orgID string) ([]model.Customer, error) {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" || orgID == OrgAll {
		return nil, ErrEmptyOrgID
	}
	rows, err := r.db.Query(`SELECT customer_id, COALESCE(org_name,''), COALESCE(official_name,'')
		FROM customers WHERE org_id=? ORDER BY official_name, org_name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Customer
	for rows.Next() {
		var c model.Customer
		if err := rows.Scan(&c.CustomerID, &c.OrgName, &c.OfficialName); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *OrgRepo) PreviewSplit(fromOrg, toOrg string, customerIDs []string) (*OrgSplitPreview, error) {
	fromOrg, err := RequireInsertOrg(fromOrg)
	if err != nil {
		return nil, err
	}
	toOrg, err = RequireInsertOrg(toOrg)
	if err != nil {
		return nil, err
	}
	if fromOrg == toOrg {
		return nil, fmt.Errorf("같은 조직으로는 옮길 수 없습니다")
	}
	ids := uniqIDs(customerIDs)
	if len(ids) == 0 {
		return nil, fmt.Errorf("옮길 고객을 고르세요")
	}
	src, err := r.Get(fromOrg)
	dst, err2 := r.Get(toOrg)
	if err != nil || err2 != nil {
		if err != nil {
			return nil, err
		}
		return nil, err2
	}
	if src == nil || dst == nil || !dst.IsActive {
		return nil, fmt.Errorf("조직이 없거나 비활성입니다")
	}
	ph, args := inArgs(ids)
	args = append([]interface{}{fromOrg}, args...)
	p := &OrgSplitPreview{FromOrg: fromOrg, ToOrg: toOrg, CustomerIDs: ids}
	p.Customers = countSQL(r.db, `SELECT COUNT(*) FROM customers WHERE org_id=? AND customer_id IN (`+ph+`)`, args...)
	p.Assets = countSQL(r.db, `SELECT COUNT(*) FROM assets WHERE org_id=? AND customer_id IN (`+ph+`)`, args...)
	p.AS = countSQL(r.db, `SELECT COUNT(*) FROM as_receipts WHERE org_id=? AND customer_id IN (`+ph+`)`, args...)
	p.Sales = countSQL(r.db, `SELECT COUNT(*) FROM sales_projects WHERE org_id=? AND customer_id IN (`+ph+`)`, args...)
	p.Work = countSQL(r.db, `SELECT COUNT(*) FROM work_tasks WHERE org_id=? AND customer_id IN (`+ph+`)`, args...)
	p.Visits = countSQL(r.db, `SELECT COUNT(*) FROM maintenance_visits WHERE customer_id IN (`+ph+`)`, args[1:]...)
	return p, nil
}

func (r *OrgRepo) ApplySplit(fromOrg, toOrg string, customerIDs []string) (*OrgSplitPreview, error) {
	p, err := r.PreviewSplit(fromOrg, toOrg, customerIDs)
	if err != nil {
		return nil, err
	}
	ph, idArgs := inArgs(p.CustomerIDs)
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	up := func(q string, args ...interface{}) error {
		_, err := tx.Exec(q, args...)
		return err
	}
	base := append([]interface{}{toOrg, fromOrg}, idArgs...)
	stmts := []string{
		`UPDATE customers SET org_id=? WHERE org_id=? AND customer_id IN (` + ph + `)`,
		`UPDATE assets SET org_id=? WHERE org_id=? AND customer_id IN (` + ph + `)`,
		`UPDATE as_receipts SET org_id=? WHERE org_id=? AND customer_id IN (` + ph + `)`,
		`UPDATE work_projects SET org_id=? WHERE org_id=? AND customer_id IN (` + ph + `)`,
		`UPDATE work_tasks SET org_id=? WHERE org_id=? AND customer_id IN (` + ph + `)`,
		`UPDATE sales_projects SET org_id=? WHERE org_id=? AND customer_id IN (` + ph + `)`,
	}
	if tableHasColumn(r.db, "maintenance_site_config", "org_id") {
		stmts = append(stmts, `UPDATE maintenance_site_config SET org_id=? WHERE org_id=? AND customer_id IN (`+ph+`)`)
	}
	if tableHasColumn(r.db, "sales_quotes", "org_id") {
		stmts = append(stmts, `UPDATE sales_quotes SET org_id=? WHERE org_id=? AND customer_id IN (`+ph+`)`)
	}
	if tableHasColumn(r.db, "sales_orders", "org_id") {
		stmts = append(stmts, `UPDATE sales_orders SET org_id=? WHERE org_id=? AND customer_id IN (`+ph+`)`)
	}
	for _, q := range stmts {
		if err := up(q, base...); err != nil {
			return nil, err
		}
	}
	if sqliteTableExists(r.db, "maintenance_visits") {
		rows, err := tx.Query(`SELECT v.visit_id, p.plan_year FROM maintenance_visits v
			JOIN maintenance_plans p ON p.plan_id=v.plan_id
			WHERE v.customer_id IN (`+ph+`)`, idArgs...)
		if err != nil {
			return nil, err
		}
		type mv struct {
			id   string
			year int
		}
		var visits []mv
		for rows.Next() {
			var it mv
			if err := rows.Scan(&it.id, &it.year); err != nil {
				rows.Close()
				return nil, err
			}
			visits = append(visits, it)
		}
		rows.Close()
		planByYear := map[int]string{}
		for _, v := range visits {
			pid, ok := planByYear[v.year]
			if !ok {
				_ = tx.QueryRow(`SELECT plan_id FROM maintenance_plans WHERE plan_year=? AND org_id=?`, v.year, toOrg).Scan(&pid)
				if pid == "" {
					pid = fmt.Sprintf("MP-%s-%d", toOrg, v.year)
					if _, err := tx.Exec(`INSERT INTO maintenance_plans(plan_id,plan_year,title,status,org_id)
						VALUES(?,?,?,?,?)`, pid, v.year, fmt.Sprintf("%d년 정기점검", v.year), "active", toOrg); err != nil {
						return nil, err
					}
				}
				planByYear[v.year] = pid
			}
			if _, err := tx.Exec(`UPDATE maintenance_visits SET plan_id=? WHERE visit_id=?`, pid, v.id); err != nil {
				return nil, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	after, _ := json.Marshal(p)
	audit.Use(r.db)
	audit.LogWithReason(audit.ActionUpdate, "orgs", "org_id", fromOrg+"→"+toOrg, srcLabel(p), "", string(after), "조직 분리")
	return p, nil
}

func srcLabel(p *OrgSplitPreview) string {
	if p == nil {
		return "분리"
	}
	return fmt.Sprintf("고객 %d · 자산 %d · AS %d · 사업 %d · 업무 %d", p.Customers, p.Assets, p.AS, p.Sales, p.Work)
}

func uniqIDs(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func inArgs(ids []string) (string, []interface{}) {
	ph := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args[i] = id
	}
	return strings.Join(ph, ","), args
}
