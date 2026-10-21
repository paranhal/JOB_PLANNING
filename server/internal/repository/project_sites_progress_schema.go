package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
)

func applyProjectSitesProgress(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := projectSitesProgressMigrationSQL()
	if err != nil {
		log.Printf("075 project_sites_progress: %v", err)
		return
	}
	if err := execSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("075 project_sites_progress apply: %v", err)
		return
	}
	if err := rebuildProjectSitesStatusCheck(db); err != nil {
		log.Printf("075 project_sites status rebuild: %v", err)
		return
	}
	if err := applyProjectSiteOrgTriggers(db); err != nil {
		log.Printf("075 project_sites org trigger: %v", err)
	}
}

func applyProjectSiteOrgTriggers(db *sql.DB) error {
	stmts := []string{
		`DROP TRIGGER IF EXISTS trg_ps_org_ins`,
		`DROP TRIGGER IF EXISTS trg_ps_org_upd`,
		`CREATE TRIGGER trg_ps_org_ins
BEFORE INSERT ON project_sites
WHEN NEW.customer_id IS NOT NULL AND TRIM(NEW.customer_id) != ''
BEGIN
  SELECT CASE
    WHEN (SELECT org_id FROM customers WHERE customer_id=NEW.customer_id) IS NULL
      THEN RAISE(ABORT, 'unknown customer')
    WHEN (SELECT org_id FROM customers WHERE customer_id=NEW.customer_id) != NEW.org_id
      THEN RAISE(ABORT, 'org mismatch')
    WHEN (SELECT org_id FROM work_projects WHERE project_id=NEW.project_id) != NEW.org_id
      THEN RAISE(ABORT, 'org mismatch')
  END;
END`,
		`CREATE TRIGGER trg_ps_org_upd
BEFORE UPDATE OF customer_id, org_id, project_id ON project_sites
WHEN NEW.customer_id IS NOT NULL AND TRIM(NEW.customer_id) != ''
BEGIN
  SELECT CASE
    WHEN (SELECT org_id FROM customers WHERE customer_id=NEW.customer_id) IS NULL
      THEN RAISE(ABORT, 'unknown customer')
    WHEN (SELECT org_id FROM customers WHERE customer_id=NEW.customer_id) != NEW.org_id
      THEN RAISE(ABORT, 'org mismatch')
    WHEN (SELECT org_id FROM work_projects WHERE project_id=NEW.project_id) != NEW.org_id
      THEN RAISE(ABORT, 'org mismatch')
  END;
END`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

func projectSitesProgressMigrationSQL() (string, error) {
	for _, p := range workGroupSQLPaths("075_project_sites_progress.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("075_project_sites_progress.sql 없음")
}

func projectSitesProgressDownSQL() (string, error) {
	for _, p := range workGroupSQLPaths("075_project_sites_progress.down.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("075_project_sites_progress.down.sql 없음")
}

func rebuildProjectSitesStatusCheck(db *sql.DB) error {
	var ddl sql.NullString
	err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='project_sites'`).Scan(&ddl)
	if err == sql.ErrNoRows || !ddl.Valid {
		return nil
	}
	if err != nil {
		return err
	}
	if strings.Contains(ddl.String, "in_progress") {
		return nil
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		return err
	}
	defer db.Exec(`PRAGMA foreign_keys=ON`)
	script := `
CREATE TABLE project_sites__075 (
  id                 TEXT PRIMARY KEY,
  project_id         TEXT NOT NULL REFERENCES work_projects(project_id),
  customer_id        TEXT,
  site_name          TEXT NOT NULL,
  region             TEXT NOT NULL DEFAULT '',
  address            TEXT NOT NULL DEFAULT '',
  contact_note       TEXT NOT NULL DEFAULT '',
  status             TEXT NOT NULL DEFAULT 'planned' CHECK (status IN ('planned','in_progress','installed','inspected','excluded')),
  sort_order         INTEGER NOT NULL DEFAULT 0,
  is_active          INTEGER NOT NULL DEFAULT 1,
  note               TEXT NOT NULL DEFAULT '',
  org_id             TEXT NOT NULL,
  installed_at       TEXT NOT NULL DEFAULT '',
  inspected_at       TEXT NOT NULL DEFAULT '',
  excluded_reason    TEXT NOT NULL DEFAULT '',
  status_changed_at  TEXT NOT NULL DEFAULT '',
  status_changed_by  TEXT NOT NULL DEFAULT '',
  UNIQUE (project_id, site_name)
);
INSERT INTO project_sites__075 (
  id, project_id, customer_id, site_name, region, address, contact_note,
  status, sort_order, is_active, note, org_id,
  installed_at, inspected_at, excluded_reason, status_changed_at, status_changed_by
)
SELECT
  id, project_id, customer_id, site_name, region, address, contact_note,
  CASE status WHEN 'active' THEN 'in_progress' WHEN 'done' THEN 'inspected' ELSE status END,
  sort_order, is_active, note, org_id,
  COALESCE(installed_at,''), COALESCE(inspected_at,''), COALESCE(excluded_reason,''),
  COALESCE(status_changed_at,''), COALESCE(status_changed_by,'')
FROM project_sites;
DROP TABLE project_sites;
ALTER TABLE project_sites__075 RENAME TO project_sites;
CREATE INDEX IF NOT EXISTS ix_ps_project ON project_sites(project_id, is_active, sort_order);
CREATE INDEX IF NOT EXISTS ix_ps_org ON project_sites(org_id);
CREATE INDEX IF NOT EXISTS ix_ps_region ON project_sites(project_id, region);
CREATE UNIQUE INDEX IF NOT EXISTS ix_ps_project_customer
  ON project_sites(project_id, customer_id)
  WHERE customer_id IS NOT NULL AND TRIM(customer_id) != '';
`
	return execSQLIgnoreDupCol(db, script)
}
