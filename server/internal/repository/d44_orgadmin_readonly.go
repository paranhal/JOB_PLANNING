package repository

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	"customer-support/internal/audit"
	"customer-support/internal/model"
)

const d44ReadonlyOrgAdminMetaKey = "d44_readonly_orgadmin_to_tech"

// D44ProtectedUsername 읽기전용이 아닌 O01 팀장. 대상에 있으면 이관을 중단한다.
const D44ProtectedUsername = "hjlee"

type D44Candidate struct {
	UserID, Username, FullName, Role, OrgID string
	IsReadOnly, IsActive                    bool
}

func ListD44ReadonlyOrgAdmins(db *sql.DB) ([]D44Candidate, error) {
	if db == nil {
		return nil, fmt.Errorf("db 없음")
	}
	rows, err := db.Query(`
		SELECT user_id, username, full_name, role, COALESCE(org_id,''), COALESCE(is_readonly,0), COALESCE(is_active,0)
		FROM users
		WHERE role='org_admin' AND COALESCE(is_readonly,0)=1
		ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []D44Candidate
	for rows.Next() {
		var c D44Candidate
		var ro, act int
		if err := rows.Scan(&c.UserID, &c.Username, &c.FullName, &c.Role, &c.OrgID, &ro, &act); err != nil {
			return nil, err
		}
		c.IsReadOnly = ro == 1
		c.IsActive = act == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

func d44ContainsProtected(list []D44Candidate) bool {
	for _, c := range list {
		if strings.EqualFold(strings.TrimSpace(c.Username), D44ProtectedUsername) {
			return true
		}
	}
	return false
}

// ApplyD44ReadonlyOrgAdminToTech 읽기전용 조직관리자를 기술+읽기전용으로 옮긴다. metaDone 가드. InitDB 에는 아직 물리지 않는다.
func ApplyD44ReadonlyOrgAdminToTech(db *sql.DB) error {
	if db == nil || metaDone(db, d44ReadonlyOrgAdminMetaKey) {
		return nil
	}
	list, err := ListD44ReadonlyOrgAdmins(db)
	if err != nil {
		return err
	}
	if d44ContainsProtected(list) {
		return fmt.Errorf("D-44 중단: %s 가 읽기전용 조직관리자 대상에 있다", D44ProtectedUsername)
	}
	pop := audit.Push(audit.Actor{Name: "시스템", Role: "system"})
	defer pop()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	n := 0
	for _, c := range list {
		if _, err := tx.Exec(`UPDATE users SET role=? WHERE user_id=? AND role='org_admin' AND COALESCE(is_readonly,0)=1`,
			model.RoleTech, c.UserID); err != nil {
			_ = tx.Rollback()
			return err
		}
		sum := fmt.Sprintf("D-44 등급·업무 조직관리자 → 기술 (%s)", c.FullName)
		before := fmt.Sprintf(`{"role":"org_admin","is_readonly":true,"username":%q}`, c.Username)
		after := fmt.Sprintf(`{"role":"tech","is_readonly":true,"username":%q}`, c.Username)
		if err := audit.InsertTx(tx, audit.ActionUpdate, "users", "user_id", c.UserID, c.FullName,
			before, after, "D-44 옵저버 계열 조직관리자 → 기술", sum); err != nil {
			_ = tx.Rollback()
			return err
		}
		n++
	}
	sum := fmt.Sprintf("시스템 D-44 읽기전용 조직관리자 %d줄을 기술로 옮겼다", n)
	if err := audit.InsertTx(tx, audit.ActionUpdate, "users", "user_id", "d44", "시스템 이관",
		"{}", "{}", "D-44", sum); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO id_sequences(seq_key,last_no) VALUES(?,1)`, d44ReadonlyOrgAdminMetaKey); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	log.Printf("[d44] 읽기전용 조직관리자 %d줄을 기술로 옮겼다", n)
	return nil
}
