package repository

import (
	"database/sql"
	"time"

	"customer-support/internal/model"
)

type UserRepo struct{ db *sql.DB }

func NewUserRepo(db *sql.DB) *UserRepo { return &UserRepo{db: db} }

func (r *UserRepo) ListAll() ([]model.User, error) {
	rows, err := r.db.Query(`
		SELECT user_id, username, full_name, role, is_active, created_at
		FROM users ORDER BY full_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.User
	for rows.Next() {
		var u model.User
		var active int
		var createdStr string
		if err := rows.Scan(&u.UserID, &u.Username, &u.FullName, &u.Role, &active, &createdStr); err != nil {
			return nil, err
		}
		u.IsActive = active == 1
		u.CreatedAt = parseTime(createdStr)
		items = append(items, u)
	}
	return items, rows.Err()
}

func (r *UserRepo) GetByUsername(username string) (*model.User, error) {
	var u model.User
	var active int
	var createdStr string
	err := r.db.QueryRow(`
		SELECT user_id, username, password_hash, full_name, role, is_active, created_at
		FROM users WHERE username=?`, username).
		Scan(&u.UserID, &u.Username, &u.PasswordHash, &u.FullName, &u.Role, &active, &createdStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.IsActive = active == 1
	u.CreatedAt = parseTime(createdStr)
	return &u, nil
}

func (r *UserRepo) GetByID(id string) (*model.User, error) {
	var u model.User
	var active int
	var createdStr string
	err := r.db.QueryRow(`
		SELECT user_id, username, password_hash, full_name, role, is_active, created_at
		FROM users WHERE user_id=?`, id).
		Scan(&u.UserID, &u.Username, &u.PasswordHash, &u.FullName, &u.Role, &active, &createdStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.IsActive = active == 1
	u.CreatedAt = parseTime(createdStr)
	return &u, nil
}

func (r *UserRepo) Create(u *model.User) error {
	u.UserID = newID("USR")
	_, err := r.db.Exec(`
		INSERT INTO users (user_id,username,password_hash,full_name,role,is_active,created_at)
		VALUES (?,?,?,?,?,?,?)`,
		u.UserID, u.Username, u.PasswordHash, u.FullName, u.Role,
		boolToInt(u.IsActive), time.Now().Format("2006-01-02 15:04:05"))
	return err
}

func (r *UserRepo) Update(u *model.User) error {
	_, err := r.db.Exec(`
		UPDATE users SET full_name=?,role=?,is_active=? WHERE user_id=?`,
		u.FullName, u.Role, boolToInt(u.IsActive), u.UserID)
	return err
}

func (r *UserRepo) UpdatePassword(id, hash string) error {
	_, err := r.db.Exec(`UPDATE users SET password_hash=? WHERE user_id=?`, hash, id)
	return err
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
		Role:         "admin",
		IsActive:     true,
	}
	return r.Create(admin)
}
