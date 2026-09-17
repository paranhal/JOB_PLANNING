package repository

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	"customer-support/internal/model"
)

// applyNF1 마이그레이션 052. 위치 이름 복사본·CSV 다중값·첨부 ref_type. §44.8.5
func applyNF1(db *sql.DB) {
	if db == nil {
		return
	}
	applyAssetLocIDs(db)
	applyCSVJunctions(db)
	applyAttachRefUnify(db)
}

func assetLocNameSQL(alias, locCol, joinExpr string, locCopy bool) string {
	if locCopy {
		return fmt.Sprintf("COALESCE(NULLIF(TRIM(%s),''), NULLIF(TRIM(%s.%s),''), '')", joinExpr, alias, locCol)
	}
	return fmt.Sprintf("COALESCE(%s, '')", joinExpr)
}

func applyAssetLocIDs(db *sql.DB) {
	if !tableHasColumn(db, "assets", "loc_building_name") {
		return
	}
	n := countAssetLocNameOnly(db)
	log.Printf("052 loc copy: ID 없이 이름만 있는 행 %d건", n)
	backfillAssetLocIDs(db)
	left := countAssetLocNameOnly(db)
	if left > 0 {
		log.Printf("052 loc copy: 역채우기 실패 %d건 — loc_* 열을 지우지 않는다", left)
		return
	}
	for _, col := range []string{"loc_building_name", "loc_floor_name", "loc_room_name"} {
		if _, err := db.Exec(`ALTER TABLE assets DROP COLUMN ` + col); err != nil &&
			!strings.Contains(strings.ToLower(err.Error()), "no such column") {
			log.Printf("052 drop assets.%s: %v", col, err)
			return
		}
	}
	log.Printf("052 loc copy: 이름 복사본 열 삭제")
}

func countAssetLocNameOnly(db *sql.DB) int {
	if !tableHasColumn(db, "assets", "loc_building_name") {
		return 0
	}
	var n int
	_ = db.QueryRow(`
		SELECT COUNT(*) FROM assets
		 WHERE (TRIM(COALESCE(building_id,''))='' AND TRIM(COALESCE(loc_building_name,''))!='')
		    OR (TRIM(COALESCE(floor_id,''))='' AND TRIM(COALESCE(loc_floor_name,''))!='')
		    OR (TRIM(COALESCE(room_id,''))='' AND TRIM(COALESCE(loc_room_name,''))!='')`).Scan(&n)
	return n
}

func backfillAssetLocIDs(db *sql.DB) {
	if _, err := db.Exec(`
UPDATE assets SET building_id=(
	SELECT b.building_id FROM customer_buildings b
	 WHERE b.customer_id=assets.customer_id
	   AND TRIM(b.building_name)=TRIM(assets.loc_building_name)
	 LIMIT 1)
 WHERE TRIM(COALESCE(building_id,''))=''
   AND TRIM(COALESCE(loc_building_name,''))!=''`); err != nil {
		log.Printf("052 loc building_id: %v", err)
	}
	if _, err := db.Exec(`
UPDATE assets SET floor_id=(
	SELECT f.floor_id FROM customer_floors f
	 JOIN customer_buildings b ON b.building_id=f.building_id
	 WHERE b.customer_id=assets.customer_id
	   AND TRIM(f.floor_name)=TRIM(assets.loc_floor_name)
	   AND (TRIM(COALESCE(assets.building_id,''))='' OR f.building_id=assets.building_id)
	 LIMIT 1)
 WHERE TRIM(COALESCE(floor_id,''))=''
   AND TRIM(COALESCE(loc_floor_name,''))!=''`); err != nil {
		log.Printf("052 loc floor_id: %v", err)
	}
	if _, err := db.Exec(`
UPDATE assets SET room_id=(
	SELECT r.room_id FROM customer_rooms r
	 JOIN customer_floors f ON f.floor_id=r.floor_id
	 JOIN customer_buildings b ON b.building_id=f.building_id
	 WHERE b.customer_id=assets.customer_id
	   AND TRIM(r.room_name)=TRIM(assets.loc_room_name)
	   AND (TRIM(COALESCE(assets.floor_id,''))='' OR r.floor_id=assets.floor_id)
	 LIMIT 1)
 WHERE TRIM(COALESCE(room_id,''))=''
   AND TRIM(COALESCE(loc_room_name,''))!=''`); err != nil {
		log.Printf("052 loc room_id: %v", err)
	}
}

func applyCSVJunctions(db *sql.DB) {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS project_scope_product_keys (
  rule_id     TEXT NOT NULL,
  product_key TEXT NOT NULL,
  PRIMARY KEY (rule_id, product_key),
  FOREIGN KEY (rule_id) REFERENCES project_scope_rules(rule_id)
)`,
		`CREATE TABLE IF NOT EXISTS project_scope_work_kinds (
  rule_id    TEXT NOT NULL,
  work_kind  TEXT NOT NULL,
  PRIMARY KEY (rule_id, work_kind),
  FOREIGN KEY (rule_id) REFERENCES project_scope_rules(rule_id)
)`,
		`CREATE TABLE IF NOT EXISTS work_task_tags (
  task_id TEXT NOT NULL,
  tag     TEXT NOT NULL,
  PRIMARY KEY (task_id, tag),
  FOREIGN KEY (task_id) REFERENCES work_tasks(task_id)
)`,
		`CREATE TABLE IF NOT EXISTS sales_activity_members (
  activity_id TEXT NOT NULL,
  member      TEXT NOT NULL,
  PRIMARY KEY (activity_id, member),
  FOREIGN KEY (activity_id) REFERENCES sales_activities(activity_id)
)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			log.Printf("052 junction: %v", err)
		}
	}
	backfillScopeKeys(db)
	backfillTaskTags(db)
	backfillSalesMembers(db)
}

func backfillScopeKeys(db *sql.DB) {
	rows, err := db.Query(`SELECT rule_id, COALESCE(product_keys,''), COALESCE(work_kinds,'') FROM project_scope_rules`)
	if err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "no such table") {
			log.Printf("052 scope rules: %v", err)
		}
		return
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id, keys, kinds string
		if err := rows.Scan(&id, &keys, &kinds); err != nil {
			continue
		}
		replaceCSVRows(db, `INSERT OR IGNORE INTO project_scope_product_keys (rule_id, product_key) VALUES (?,?)`, id, splitCSV(keys))
		replaceCSVRows(db, `INSERT OR IGNORE INTO project_scope_work_kinds (rule_id, work_kind) VALUES (?,?)`, id, splitCSV(kinds))
		n++
	}
	log.Printf("052 scope junction: 규칙 %d건", n)
}

func backfillTaskTags(db *sql.DB) {
	rows, err := db.Query(`SELECT task_id, COALESCE(tags,'') FROM work_tasks WHERE TRIM(COALESCE(tags,''))!=''`)
	if err != nil {
		log.Printf("052 task tags: %v", err)
		return
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id, tags string
		if err := rows.Scan(&id, &tags); err != nil {
			continue
		}
		replaceCSVRows(db, `INSERT OR IGNORE INTO work_task_tags (task_id, tag) VALUES (?,?)`, id, splitCSV(tags))
		n++
	}
	log.Printf("052 work_task_tags: 업무 %d건", n)
}

func backfillSalesMembers(db *sql.DB) {
	rows, err := db.Query(`SELECT activity_id, COALESCE(our_members,'') FROM sales_activities WHERE TRIM(COALESCE(our_members,''))!=''`)
	if err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "no such table") {
			log.Printf("052 sales members: %v", err)
		}
		return
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id, members string
		if err := rows.Scan(&id, &members); err != nil {
			continue
		}
		replaceCSVRows(db, `INSERT OR IGNORE INTO sales_activity_members (activity_id, member) VALUES (?,?)`,
			id, model.SplitSalesPeople(members))
		n++
	}
	log.Printf("052 sales_activity_members: 활동 %d건", n)
}

func replaceCSVRows(db *sql.DB, insertSQL, parent string, parts []string) {
	parent = strings.TrimSpace(parent)
	if parent == "" {
		return
	}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, err := db.Exec(insertSQL, parent, p); err != nil {
			log.Printf("052 junction insert: %v", err)
		}
	}
}

func replaceScopeJunction(tx *sql.Tx, ruleID, keysCSV, kindsCSV string) error {
	ruleID = strings.TrimSpace(ruleID)
	if ruleID == "" {
		return nil
	}
	if _, err := tx.Exec(`DELETE FROM project_scope_product_keys WHERE rule_id=?`, ruleID); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "no such table") {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM project_scope_work_kinds WHERE rule_id=?`, ruleID); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "no such table") {
		return err
	}
	for _, k := range splitCSV(keysCSV) {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO project_scope_product_keys (rule_id, product_key) VALUES (?,?)`, ruleID, k); err != nil {
			return err
		}
	}
	for _, k := range splitCSV(kindsCSV) {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO project_scope_work_kinds (rule_id, work_kind) VALUES (?,?)`, ruleID, k); err != nil {
			return err
		}
	}
	return nil
}

func replaceTaskTags(db *sql.DB, taskID, tagsCSV string) {
	taskID = strings.TrimSpace(taskID)
	if db == nil || taskID == "" {
		return
	}
	if _, err := db.Exec(`DELETE FROM work_task_tags WHERE task_id=?`, taskID); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "no such table") {
		log.Printf("052 replace tags: %v", err)
		return
	}
	replaceCSVRows(db, `INSERT OR IGNORE INTO work_task_tags (task_id, tag) VALUES (?,?)`, taskID, splitCSV(tagsCSV))
}

func replaceSalesMembers(db *sql.DB, activityID, membersCSV string) {
	activityID = strings.TrimSpace(activityID)
	if db == nil || activityID == "" {
		return
	}
	if _, err := db.Exec(`DELETE FROM sales_activity_members WHERE activity_id=?`, activityID); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "no such table") {
		log.Printf("052 replace members: %v", err)
		return
	}
	replaceCSVRows(db, `INSERT OR IGNORE INTO sales_activity_members (activity_id, member) VALUES (?,?)`,
		activityID, model.SplitSalesPeople(membersCSV))
}

func applyAttachRefUnify(db *sql.DB) {
	res, err := db.Exec(`UPDATE attachments SET ref_type=? WHERE ref_type=?`,
		model.AttachRefAS, model.AttachFormReceipt)
	if err != nil {
		log.Printf("052 attach ref_type: %v", err)
		return
	}
	n, _ := res.RowsAffected()
	log.Printf("052 attach ref_type: as_receipt → as %d건", n)
}

func overlayScopeJunction(db *sql.DB, rules []model.ProjectScopeRule) {
	if db == nil || len(rules) == 0 {
		return
	}
	for i := range rules {
		id := strings.TrimSpace(rules[i].RuleID)
		if id == "" {
			continue
		}
		if keys := listChildCSV(db, `SELECT product_key FROM project_scope_product_keys WHERE rule_id=? ORDER BY product_key`, id); keys != "" {
			rules[i].ProductKeys = keys
		}
		if kinds := listChildCSV(db, `SELECT work_kind FROM project_scope_work_kinds WHERE rule_id=? ORDER BY work_kind`, id); kinds != "" {
			rules[i].WorkKinds = kinds
		}
	}
}

func listChildCSV(db *sql.DB, q, parent string) string {
	rows, err := db.Query(q, parent)
	if err != nil {
		return ""
	}
	defer rows.Close()
	var parts []string
	for rows.Next() {
		var s string
		if rows.Scan(&s) == nil && strings.TrimSpace(s) != "" {
			parts = append(parts, strings.TrimSpace(s))
		}
	}
	return strings.Join(parts, ",")
}

func hasScopeKey(db *sql.DB, ruleID, key string) bool {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM project_scope_product_keys WHERE rule_id=? AND product_key=?`,
		ruleID, key).Scan(&n)
	if err == nil {
		return n > 0
	}
	return false
}
