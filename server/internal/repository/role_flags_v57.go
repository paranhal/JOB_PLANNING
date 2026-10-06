package repository

import (
	"database/sql"
	"log"

	"customer-support/internal/audit"
)

const roleFlagsV57MetaKey = "role_flags_v57"

func applyRoleFlagsV57(db *sql.DB) {
	if db == nil {
		return
	}
	addNamedColumn(db, "users", "is_readonly",
		`ALTER TABLE users ADD COLUMN is_readonly INTEGER DEFAULT 0`)
	addNamedColumn(db, "users", "v57_role",
		`ALTER TABLE users ADD COLUMN v57_role TEXT`)
	addNamedColumn(db, "users", "v57_base",
		`ALTER TABLE users ADD COLUMN v57_base TEXT`)
	addNamedColumn(db, "users", "v57_perms",
		`ALTER TABLE users ADD COLUMN v57_perms TEXT`)
	if err := migrateRoleFlagsV57(db); err != nil {
		log.Printf("[v57] 전환 실패: %v", err)
	}
}

func migrateRoleFlagsV57(db *sql.DB) error {
	if db == nil || metaDone(db, roleFlagsV57MetaKey) {
		return nil
	}
	if _, err := db.Exec(`UPDATE users SET v57_role=role, v57_base=COALESCE(base_role,''),
		v57_perms=COALESCE(permissions,'') WHERE v57_role IS NULL`); err != nil {
		return err
	}
	// 1) 옛 글자값 정규화 — NormalizeRole 과 같은 표. 먼저 돌려야 한다.
	if _, err := db.Exec(`UPDATE users SET role='observer' WHERE role IN ('viewer','열람','열람사용자','옵저버')`); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE users SET role='org_admin' WHERE role IN ('admin','관리자')`); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE users SET role='support' WHERE role IN ('office','행정','receipt','접수','지원')`); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE users SET role='tech' WHERE role IN ('기술','기술담당')`); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE users SET role='sales' WHERE role IN ('영업','영업담당')`); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE users SET role='tester' WHERE role='테스터'`); err != nil {
		return err
	}

	pop := audit.Push(audit.Actor{Name: "시스템", Role: "system"})
	defer pop()

	r1, err := db.Exec(`UPDATE users SET
		role = CASE WHEN TRIM(COALESCE(base_role,'')) IN ('sales','tech','support')
			THEN base_role ELSE 'org_admin' END,
		is_readonly = 1, base_role = '', permissions = ''
		WHERE role='observer'`)
	if err != nil {
		return err
	}
	r2, err := db.Exec(`UPDATE users SET
		role = CASE WHEN TRIM(COALESCE(base_role,'')) IN ('sales','tech','support')
			THEN base_role ELSE 'support' END,
		is_test = 1, base_role = ''
		WHERE role='tester'`)
	if err != nil {
		return err
	}
	n1, _ := r1.RowsAffected()
	n2, _ := r2.RowsAffected()
	log.Printf("[v57] 옵저버 %d줄 · 테스터 %d줄 을 깃발로 옮겼다", n1, n2)
	markMetaDone(db, roleFlagsV57MetaKey)
	return nil
}
