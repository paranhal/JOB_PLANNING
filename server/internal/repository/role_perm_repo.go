package repository

import (
	"database/sql"
	"log"
	"strings"
	"time"

	"customer-support/internal/model"
)

type RolePermRepo struct{ db *sql.DB }

func NewRolePermRepo(db *sql.DB) *RolePermRepo { return &RolePermRepo{db: db} }

func applyRolePermissionsV58(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS role_permissions (
		role       TEXT NOT NULL,
		perm_key   TEXT NOT NULL,
		access     INTEGER NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_by TEXT DEFAULT '',
		PRIMARY KEY (role, perm_key)
	)`); err != nil {
		log.Printf("role_permissions: %v", err)
	}
}

func applyUsernameNocaseV58(db *sql.DB) {
	if db == nil || metaDone(db, "users_login_nocase_v58") {
		return
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM (
		SELECT 1 FROM users GROUP BY LOWER(username) HAVING COUNT(*)>1)`).Scan(&n)
	if n > 0 {
		log.Printf("[v58] username nocase 충돌 %d묶음 — 인덱스를 만들지 않는다", n)
		return
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS ux_users_login_nocase ON users(username COLLATE NOCASE)`); err != nil {
		log.Printf("[v58] ux_users_login_nocase: %v", err)
		return
	}
	markMetaDone(db, "users_login_nocase_v58")
}

func (r *RolePermRepo) Load() (map[string]map[string]model.Access, error) {
	out := map[string]map[string]model.Access{}
	if r == nil || r.db == nil {
		return out, nil
	}
	rows, err := r.db.Query(`SELECT role, perm_key, access FROM role_permissions`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var role, key string
		var acc int
		if err := rows.Scan(&role, &key, &acc); err != nil {
			return out, err
		}
		role = model.NormalizeRole(role)
		key = model.CanonicalPerm(key)
		if role == "" || key == "" {
			continue
		}
		if out[role] == nil {
			out[role] = map[string]model.Access{}
		}
		out[role][key] = model.Access(acc)
	}
	return out, rows.Err()
}

func (r *RolePermRepo) Save(role, key string, acc model.Access, by string) error {
	if r == nil || r.db == nil {
		return sql.ErrConnDone
	}
	role = model.NormalizeRole(role)
	key = model.CanonicalPerm(key)
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := r.db.Exec(`INSERT INTO role_permissions(role,perm_key,access,updated_at,updated_by)
		VALUES(?,?,?,?,?)
		ON CONFLICT(role,perm_key) DO UPDATE SET access=excluded.access, updated_at=excluded.updated_at, updated_by=excluded.updated_by`,
		role, key, int(acc), now, strings.TrimSpace(by))
	return err
}

func (r *RolePermRepo) Delete(role, key string) error {
	if r == nil || r.db == nil {
		return sql.ErrConnDone
	}
	_, err := r.db.Exec(`DELETE FROM role_permissions WHERE role=? AND perm_key=?`,
		model.NormalizeRole(role), model.CanonicalPerm(key))
	return err
}
