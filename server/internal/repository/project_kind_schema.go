package repository

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func applyProjectKindAssignmentsV72(db *sql.DB) {
	if db == nil {
		return
	}
	if !workProjectHasColumn(db, "project_kind") {
		if _, err := db.Exec(`ALTER TABLE work_projects ADD COLUMN project_kind TEXT NOT NULL DEFAULT 'maintenance'`); err != nil &&
			!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			log.Printf("060 project_kind: %v", err)
			return
		}
	}
	if _, err := db.Exec(`UPDATE work_projects SET project_kind='maintenance' WHERE TRIM(COALESCE(project_kind,''))=''`); err != nil {
		log.Printf("060 project_kind backfill: %v", err)
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS project_assignments (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id TEXT NOT NULL,
  user_id    TEXT NOT NULL,
  duty       TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  created_by TEXT,
  UNIQUE (project_id, user_id, duty)
)`,
		`CREATE INDEX IF NOT EXISTS idx_pa_project ON project_assignments(project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_pa_user ON project_assignments(user_id)`,
		`INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order) VALUES
  ('PKC01','project_kind','construction','공사',10),
  ('PKC02','project_kind','build','구축',20),
  ('PKC03','project_kind','dev','개발',30),
  ('PKC04','project_kind','maintenance','유지보수',40)`,
		`UPDATE codes SET code_name='구축'
 WHERE code_group='project_kind' AND code_value='build' AND code_name='신규구축'`,
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			log.Printf("060 project assignments: %v", err)
			return
		}
	}
}

func projectKindMigrationFiles() (up, down string, err error) {
	candidates := []string{
		filepath.Join("..", "..", "migrations"),
		filepath.Join("migrations"),
	}
	var dir string
	for _, c := range candidates {
		if _, e := os.Stat(filepath.Join(c, "060_project_kind_assignments.sql")); e == nil {
			dir = c
			break
		}
	}
	if dir == "" {
		return "", "", os.ErrNotExist
	}
	ub, err := os.ReadFile(filepath.Join(dir, "060_project_kind_assignments.sql"))
	if err != nil {
		return "", "", err
	}
	dbt, err := os.ReadFile(filepath.Join(dir, "060_project_kind_assignments.down.sql"))
	if err != nil {
		return "", "", err
	}
	return string(ub), string(dbt), nil
}
