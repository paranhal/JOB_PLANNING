package repository

import (
	"database/sql"
	"strings"
	"time"

	"customer-support/internal/model"
)

type UserRepo struct{ db *sql.DB }

func NewUserRepo(db *sql.DB) *UserRepo { return &UserRepo{db: db} }

const userSelect = `SELECT user_id, username, full_name, role,
	COALESCE(permissions,''), is_active, created_at FROM users`
const userSelectAuth = `SELECT user_id, username, password_hash, full_name, role,
	COALESCE(permissions,''), is_active, created_at FROM users`

func scanUser(rows interface {
	Scan(dest ...interface{}) error
}, withPassword bool) (model.User, error) {
	var u model.User
	var active int
	var createdStr string
	var err error
	if withPassword {
		err = rows.Scan(&u.UserID, &u.Username, &u.PasswordHash, &u.FullName, &u.Role,
			&u.Permissions, &active, &createdStr)
	} else {
		err = rows.Scan(&u.UserID, &u.Username, &u.FullName, &u.Role,
			&u.Permissions, &active, &createdStr)
	}
	if err != nil {
		return u, err
	}
	u.Role = model.NormalizeRole(u.Role)
	u.IsActive = active == 1
	u.CreatedAt = parseTime(createdStr)
	return u, nil
}

func (r *UserRepo) ListAll() ([]model.User, error) {
	rows, err := r.db.Query(userSelect + ` ORDER BY full_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.User
	for rows.Next() {
		u, err := scanUser(rows, false)
		if err != nil {
			return nil, err
		}
		items = append(items, u)
	}
	return items, rows.Err()
}

func (r *UserRepo) ListTechActive() ([]model.User, error) {
	rows, err := r.db.Query(userSelect + `
		WHERE is_active=1 AND role IN ('tech','admin')
		ORDER BY full_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.User
	for rows.Next() {
		u, err := scanUser(rows, false)
		if err != nil {
			return nil, err
		}
		items = append(items, u)
	}
	return items, rows.Err()
}

func (r *UserRepo) ListAssignable() ([]model.User, error) {
	rows, err := r.db.Query(userSelect + `
		WHERE is_active=1 AND role IN ('admin','tech','office','receipt','sales')
		ORDER BY full_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.User
	for rows.Next() {
		u, err := scanUser(rows, false)
		if err != nil {
			return nil, err
		}
		items = append(items, u)
	}
	return items, rows.Err()
}

func (r *UserRepo) GetByUsername(username string) (*model.User, error) {
	row := r.db.QueryRow(userSelectAuth+` WHERE username=?`, username)
	u, err := scanUser(row, true)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// FindAssignable 사용자 ID·로그인명·표시명으로 배정 대상 사용자를 찾는다. §42.5
func (r *UserRepo) FindAssignable(idOrName string) *model.User {
	idOrName = strings.TrimSpace(idOrName)
	if r == nil || idOrName == "" {
		return nil
	}
	users, err := r.ListAssignable()
	if err != nil {
		return nil
	}
	for i := range users {
		u := users[i]
		if u.UserID == idOrName || u.Username == idOrName || strings.TrimSpace(u.FullName) == idOrName {
			return &u
		}
	}
	return nil
}

func (r *UserRepo) GetByID(id string) (*model.User, error) {
	row := r.db.QueryRow(userSelectAuth+` WHERE user_id=?`, id)
	u, err := scanUser(row, true)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepo) Create(u *model.User) error {
	u.UserID = newID("USR")
	u.Role = model.NormalizeRole(u.Role)
	if strings.TrimSpace(u.Permissions) == "" {
		u.Permissions = model.FormatPermissions(model.DefaultPermissions(u.Role))
	}
	_, err := r.db.Exec(`
		INSERT INTO users (user_id,username,password_hash,full_name,role,permissions,is_active,created_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		u.UserID, u.Username, u.PasswordHash, u.FullName, u.Role, u.Permissions,
		boolToInt(u.IsActive), time.Now().Format("2006-01-02 15:04:05"))
	if err != nil {
		return err
	}
	logCreate(r.db, "users", "user_id", u.UserID, u.FullName)
	rememberUserName(u.UserID, u.FullName)
	return nil
}

func (r *UserRepo) Update(u *model.User) error {
	u.Role = model.NormalizeRole(u.Role)
	err := touchUpdate(r.db, "users", "user_id", u.UserID, u.FullName, func() error {
		_, err := r.db.Exec(`
		UPDATE users SET full_name=?,role=?,permissions=?,is_active=? WHERE user_id=?`,
			u.FullName, u.Role, u.Permissions, boolToInt(u.IsActive), u.UserID)
		return err
	})
	if err == nil {
		rememberUserName(u.UserID, u.FullName)
	}
	return err
}

func (r *UserRepo) UpdatePassword(id, hash string) error {
	return touchUpdate(r.db, "users", "user_id", id, "비밀번호", func() error {
		_, err := r.db.Exec(`UPDATE users SET password_hash=? WHERE user_id=?`, hash, id)
		return err
	})
}

// PasswordHashStats 사용자 비밀번호 해시 전환 현황 (건수만, 해시 값은 반환하지 않음).
func (r *UserRepo) PasswordHashStats() (total, bcryptN int, err error) {
	err = r.db.QueryRow(`
		SELECT COUNT(*),
		       COALESCE(SUM(CASE
		         WHEN password_hash LIKE '$2a$%'
		           OR password_hash LIKE '$2b$%'
		           OR password_hash LIKE '$2y$%' THEN 1 ELSE 0 END), 0)
		FROM users`).Scan(&total, &bcryptN)
	return
}

// EnsureAdmin 최초 관리자 계정이 없으면 생성
func (r *UserRepo) EnsureAdmin(hash string) error {
	var count int
	r.db.QueryRow(`SELECT COUNT(*) FROM users WHERE role='admin'`).Scan(&count)
	if count > 0 {
		return nil
	}
	admin := &model.User{
		Username:     "admin",
		PasswordHash: hash,
		FullName:     "관리자",
		Role:         model.RoleAdmin,
		Permissions:  model.FormatPermissions(model.DefaultPermissions(model.RoleAdmin)),
		IsActive:     true,
	}
	return r.Create(admin)
}

// EnsureObserver 옵저버 계정 obs (없으면 생성, 있으면 조회 권한 보강)
func (r *UserRepo) EnsureObserver(hash string) error {
	viewPerms := model.FormatPermissions(model.ObserverViewPermissions())
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM users WHERE username='obs'`).Scan(&n)
	if n > 0 {
		// 예전 기본값(통계만)이면 전체 조회 권한으로 승격
		_, _ = r.db.Exec(`
			UPDATE users SET role=?, permissions=?
			WHERE username='obs' AND role IN ('observer','viewer')
			  AND (TRIM(COALESCE(permissions,''))='' OR permissions='stats')`,
			model.RoleObserver, viewPerms)
		return nil
	}
	u := &model.User{
		Username:     "obs",
		PasswordHash: hash,
		FullName:     "옵저버",
		Role:         model.RoleObserver,
		Permissions:  viewPerms,
		IsActive:     true,
	}
	return r.Create(u)
}
