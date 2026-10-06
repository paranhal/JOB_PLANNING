package repository

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"customer-support/internal/model"
)

const projectMergesV59MetaKey = "project_merges_v59"

func applyProjectMergesV59(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS project_merges (
			merge_id     TEXT PRIMARY KEY,
			keep_id      TEXT NOT NULL,
			drop_id      TEXT NOT NULL,
			backup_name  TEXT NOT NULL DEFAULT '',
			created_at   TEXT NOT NULL,
			undone_at    TEXT
		)`); err != nil {
		log.Printf("project_merges schema: %v", err)
		return
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS project_merge_moves (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			merge_id       TEXT NOT NULL,
			table_name     TEXT NOT NULL,
			pk             TEXT NOT NULL,
			old_project_id TEXT NOT NULL,
			new_project_id TEXT NOT NULL
		)`); err != nil {
		log.Printf("project_merge_moves schema: %v", err)
		return
	}
	if !metaDone(db, projectMergesV59MetaKey) {
		markMetaDone(db, projectMergesV59MetaKey)
	}
}

type ProjectMergePreview struct {
	Keep   *model.WorkProject
	Drop   *model.WorkProject
	AS     int
	Mnt    int
	Tasks  int
	Assets int
	Weekly int
}

func (r *ProjectRepo) MergePreview(keepID, dropID string) (ProjectMergePreview, error) {
	var out ProjectMergePreview
	keep, err := r.Get(keepID)
	if err != nil {
		return out, err
	}
	drop, err := r.Get(dropID)
	if err != nil {
		return out, err
	}
	out.Keep, out.Drop = keep, drop
	out.AS = countProjectCol(r.db, "as_receipts", "as_id", dropID)
	out.Mnt = countProjectCol(r.db, "maintenance_visits", "visit_id", dropID)
	out.Tasks = countProjectCol(r.db, "work_tasks", "task_id", dropID)
	out.Assets = countProjectCol(r.db, "assets", "asset_id", dropID)
	out.Weekly = countProjectCol(r.db, "weekly_report_rows", "row_key", dropID)
	return out, nil
}

func countProjectCol(db *sql.DB, table, pk, projectID string) int {
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE TRIM(COALESCE(project_id,''))=?`, projectID).Scan(&n)
	return n
}

func (r *ProjectRepo) MergeProjects(keepID, dropID, backupName string) (string, error) {
	keepID = strings.TrimSpace(keepID)
	dropID = strings.TrimSpace(dropID)
	if keepID == "" || dropID == "" || keepID == dropID {
		return "", fmt.Errorf("남길 사업과 없앨 사업을 다르게 고르세요")
	}
	if _, err := r.Get(keepID); err != nil {
		return "", err
	}
	if _, err := r.Get(dropID); err != nil {
		return "", err
	}
	mergeID := "PM-" + uuid.New().String()[:12]
	tx, err := r.db.Begin()
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	tables := [][2]string{
		{"as_receipts", "as_id"},
		{"maintenance_visits", "visit_id"},
		{"work_tasks", "task_id"},
		{"assets", "asset_id"},
		{"weekly_report_rows", "row_key"},
	}
	now := time.Now().Format(time.RFC3339)
	if _, err := tx.Exec(`
		INSERT INTO project_merges (merge_id, keep_id, drop_id, backup_name, created_at)
		VALUES (?,?,?,?,?)`, mergeID, keepID, dropID, backupName, now); err != nil {
		return "", err
	}
	for _, t := range tables {
		rows, err := tx.Query(`SELECT `+t[1]+` FROM `+t[0]+` WHERE TRIM(COALESCE(project_id,''))=?`, dropID)
		if err != nil {
			return "", err
		}
		var pks []string
		for rows.Next() {
			var pk string
			if err := rows.Scan(&pk); err != nil {
				rows.Close()
				return "", err
			}
			pks = append(pks, pk)
		}
		rows.Close()
		for _, pk := range pks {
			if _, err := tx.Exec(`
				INSERT INTO project_merge_moves (merge_id, table_name, pk, old_project_id, new_project_id)
				VALUES (?,?,?,?,?)`, mergeID, t[0], pk, dropID, keepID); err != nil {
				return "", err
			}
			if _, err := tx.Exec(`UPDATE `+t[0]+` SET project_id=? WHERE `+t[1]+`=?`, keepID, pk); err != nil {
				return "", err
			}
		}
	}
	if _, err := tx.Exec(`UPDATE work_projects SET status=?, updated_at=CURRENT_TIMESTAMP WHERE project_id=?`,
		model.WBProjectArchived, dropID); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return mergeID, nil
}

func (r *ProjectRepo) LatestMergeFor(keepID, dropID string) (string, error) {
	var id string
	err := r.db.QueryRow(`
		SELECT merge_id FROM project_merges
		WHERE keep_id=? AND drop_id=? AND undone_at IS NULL
		ORDER BY created_at DESC LIMIT 1`, keepID, dropID).Scan(&id)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("되돌릴 합치기 기록이 없습니다")
	}
	return id, err
}

func (r *ProjectRepo) UndoMerge(mergeID string) error {
	mergeID = strings.TrimSpace(mergeID)
	if mergeID == "" {
		return fmt.Errorf("merge_id 필요")
	}
	var keepID, dropID string
	var undone sql.NullString
	err := r.db.QueryRow(`SELECT keep_id, drop_id, undone_at FROM project_merges WHERE merge_id=?`, mergeID).
		Scan(&keepID, &dropID, &undone)
	if err != nil {
		return err
	}
	if undone.Valid && strings.TrimSpace(undone.String) != "" {
		return fmt.Errorf("이미 되돌린 합치기입니다")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.Query(`SELECT table_name, pk, old_project_id FROM project_merge_moves WHERE merge_id=?`, mergeID)
	if err != nil {
		return err
	}
	type mv struct{ table, pk, old string }
	var moves []mv
	for rows.Next() {
		var m mv
		if err := rows.Scan(&m.table, &m.pk, &m.old); err != nil {
			rows.Close()
			return err
		}
		moves = append(moves, m)
	}
	rows.Close()
	pkCol := map[string]string{
		"as_receipts": "as_id", "maintenance_visits": "visit_id",
		"work_tasks": "task_id", "assets": "asset_id", "weekly_report_rows": "row_key",
	}
	for _, m := range moves {
		col := pkCol[m.table]
		if col == "" {
			continue
		}
		if _, err := tx.Exec(`UPDATE `+m.table+` SET project_id=? WHERE `+col+`=?`, m.old, m.pk); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE work_projects SET status=?, updated_at=CURRENT_TIMESTAMP WHERE project_id=?`,
		model.WBProjectActive, dropID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE project_merges SET undone_at=? WHERE merge_id=?`,
		time.Now().Format(time.RFC3339), mergeID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *ProjectRepo) ListOthers(exceptID string) ([]model.WorkProject, error) {
	items, err := r.ListFiltered("", 0, "")
	if err != nil {
		return nil, err
	}
	var out []model.WorkProject
	for _, p := range items {
		if p.ProjectID == exceptID {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}
