package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"customer-support/internal/audit"
	"customer-support/internal/model"
)

func applyAccountLifecycleV68(db *sql.DB) {
	if db == nil {
		return
	}
	addNamedColumn(db, "users", "deleted_at", `ALTER TABLE users ADD COLUMN deleted_at DATETIME`)
	addNamedColumn(db, "users", "deleted_by", `ALTER TABLE users ADD COLUMN deleted_by TEXT`)
	addNamedColumn(db, "users", "delete_reason", `ALTER TABLE users ADD COLUMN delete_reason TEXT`)
	addNamedColumn(db, "users", "delete_snapshot", `ALTER TABLE users ADD COLUMN delete_snapshot TEXT`)
	addNamedColumn(db, "users", "must_change_password", `ALTER TABLE users ADD COLUMN must_change_password INTEGER NOT NULL DEFAULT 0`)
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS account_requests (
  request_id      TEXT PRIMARY KEY,
  org_id          TEXT NOT NULL,
  requested_by    TEXT NOT NULL,
  full_name       TEXT NOT NULL,
  username        TEXT NOT NULL,
  role            TEXT NOT NULL,
  mobile          TEXT,
  reason          TEXT,
  status          TEXT NOT NULL DEFAULT 'pending',
  restore_user_id TEXT,
  decided_by      TEXT,
  decided_at      DATETIME,
  decision_note   TEXT,
  created_user_id TEXT,
  created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
)`,
		`CREATE INDEX IF NOT EXISTS idx_account_requests_status ON account_requests(status, created_at)`,
		`CREATE TABLE IF NOT EXISTS user_lifecycle_events (
  event_id   TEXT PRIMARY KEY,
  user_id    TEXT NOT NULL,
  kind       TEXT NOT NULL,
  actor_id   TEXT,
  note       TEXT,
  detail     TEXT,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
)`,
		`CREATE INDEX IF NOT EXISTS idx_user_lifecycle_user ON user_lifecycle_events(user_id, created_at)`,
		`CREATE TABLE IF NOT EXISTS username_reservations (
  username_norm TEXT PRIMARY KEY,
  user_id       TEXT NOT NULL,
  reason        TEXT NOT NULL,
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
)`,
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			log.Printf("057 account lifecycle: %v", err)
			return
		}
	}
	seedUsernameReservations(db)
}

func seedUsernameReservations(db *sql.DB) {
	if db == nil {
		return
	}
	rows, err := db.Query(`SELECT user_id, username FROM users`)
	if err != nil {
		log.Printf("057 username seed: %v", err)
		return
	}
	defer rows.Close()
	type pair struct{ id, name, norm string }
	var all []pair
	byNorm := map[string][]pair{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		p := pair{id: id, name: name, norm: model.NormalizeUsername(name)}
		if p.norm == "" {
			continue
		}
		all = append(all, p)
		byNorm[p.norm] = append(byNorm[p.norm], p)
	}
	for n, g := range byNorm {
		if len(g) > 1 {
			var b strings.Builder
			for i, p := range g {
				if i > 0 {
					b.WriteString(" / ")
				}
				b.WriteString(p.name)
				b.WriteString("(")
				b.WriteString(p.id)
				b.WriteString(")")
			}
			log.Printf("[v68] 아이디 정규화 충돌 %s → %s — 자동 변경하지 않는다", n, b.String())
		}
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	for _, p := range all {
		_, _ = db.Exec(`INSERT OR IGNORE INTO username_reservations(username_norm,user_id,reason,created_at) VALUES(?,?,?,?)`,
			p.norm, p.id, model.ReserveCurrent, now)
	}
}

type AccountRepo struct{ db *sql.DB }

func NewAccountRepo(db *sql.DB) *AccountRepo { return &AccountRepo{db: db} }

func (r *AccountRepo) Reserve(username, userID, reason string) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("db")
	}
	norm := model.NormalizeUsername(username)
	if norm == "" || strings.TrimSpace(userID) == "" {
		return fmt.Errorf("아이디가 비었습니다")
	}
	if reason == "" {
		reason = model.ReserveCurrent
	}
	_, err := r.db.Exec(`INSERT INTO username_reservations(username_norm,user_id,reason,created_at)
		VALUES(?,?,?,?)
		ON CONFLICT(username_norm) DO UPDATE SET user_id=excluded.user_id, reason=excluded.reason`,
		norm, strings.TrimSpace(userID), reason, time.Now().Format("2006-01-02 15:04:05"))
	return err
}

func (r *AccountRepo) UsernameReserved(username, exceptUserID string) (bool, error) {
	if r == nil || r.db == nil {
		return false, fmt.Errorf("db")
	}
	norm := model.NormalizeUsername(username)
	if norm == "" {
		return false, nil
	}
	exceptUserID = strings.TrimSpace(exceptUserID)
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM username_reservations
		WHERE username_norm=? AND NOT (reason=? AND user_id=?)`,
		norm, model.ReserveCurrent, exceptUserID).Scan(&n)
	return n > 0, err
}

func (r *UserRepo) UsernameTaken(username, exceptUserID string) (bool, error) {
	username = strings.TrimSpace(username)
	if r == nil || r.db == nil || username == "" {
		return false, fmt.Errorf("아이디가 비었습니다")
	}
	acc := NewAccountRepo(r.db)
	res, err := acc.UsernameReserved(username, exceptUserID)
	if err != nil {
		return false, err
	}
	if res {
		return true, nil
	}
	var n int
	err = r.db.QueryRow(`SELECT COUNT(*) FROM users WHERE username=? COLLATE NOCASE AND user_id!=?`, username, strings.TrimSpace(exceptUserID)).Scan(&n)
	return n > 0, err
}

func (r *AccountRepo) AddLifecycle(userID, kind, actorID, note, detail string) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("db")
	}
	_, err := r.db.Exec(`INSERT INTO user_lifecycle_events(event_id,user_id,kind,actor_id,note,detail,created_at)
		VALUES(?,?,?,?,?,?,?)`,
		newID("ULE"), strings.TrimSpace(userID), kind, strings.TrimSpace(actorID), note, detail,
		time.Now().Format("2006-01-02 15:04:05"))
	return err
}

func userSnapshotJSON(u *model.User) string {
	if u == nil {
		return "{}"
	}
	b, _ := json.Marshal(map[string]any{
		"user_id": u.UserID, "username": u.Username, "full_name": u.FullName,
		"role": u.Role, "org_id": u.OrgID, "is_readonly": u.IsReadOnly, "is_test": u.IsTest,
		"mobile": u.Mobile, "tel": u.Tel, "email": u.Email,
	})
	if len(b) == 0 {
		return "{}"
	}
	return string(b)
}

func (r *UserRepo) SoftDelete(userID, by, reason string) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("계정이 없습니다")
	}
	u, err := r.GetByID(userID)
	if err != nil || u == nil {
		return fmt.Errorf("계정이 없습니다")
	}
	if u.IsDeleted() {
		return nil
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	snap := userSnapshotJSON(u)
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE users SET is_active=0, deleted_at=?, deleted_by=?, delete_reason=?, delete_snapshot=? WHERE user_id=?`,
		now, by, reason, snap, u.UserID)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	norm := model.NormalizeUsername(u.Username)
	_, err = tx.Exec(`INSERT INTO username_reservations(username_norm,user_id,reason,created_at) VALUES(?,?,?,?)
		ON CONFLICT(username_norm) DO UPDATE SET reason=excluded.reason, user_id=excluded.user_id`,
		norm, u.UserID, model.ReserveDeleted, now)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	_, err = tx.Exec(`INSERT INTO user_lifecycle_events(event_id,user_id,kind,actor_id,note,detail,created_at) VALUES(?,?,?,?,?,?,?)`,
		newID("ULE"), u.UserID, model.LifeDeleted, by, reason, snap, now)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := audit.InsertTx(tx, audit.ActionDelete, "users", "user_id", u.UserID, u.FullName, snap, "{}", reason, "소프트 삭제"); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (r *UserRepo) Delete(userID string) error {
	return r.SoftDelete(userID, "", "")
}

func (r *UserRepo) Restore(userID, actorID, role, orgID, note string) error {
	u, err := r.GetByID(userID)
	if err != nil || u == nil {
		return fmt.Errorf("계정이 없습니다")
	}
	if !u.IsDeleted() {
		return fmt.Errorf("삭제된 계정이 아닙니다")
	}
	role = model.NormalizeRole(role)
	if !model.IsKnownRole(role) || model.IsAdminGrade(role) {
		return fmt.Errorf("복구 때 업무를 고르세요")
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	detail, _ := json.Marshal(map[string]string{
		"prev_role": u.Role, "prev_org": u.OrgID, "hire_reset": "TODO 연차 원장 expire — 69·71에서 연결",
	})
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE users SET is_active=1, deleted_at='', deleted_by='', delete_reason='', role=?, org_id=?, is_readonly=0, permissions='' WHERE user_id=?`,
		role, strings.TrimSpace(orgID), u.UserID)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	_, err = tx.Exec(`INSERT INTO username_reservations(username_norm,user_id,reason,created_at) VALUES(?,?,?,?)
		ON CONFLICT(username_norm) DO UPDATE SET reason=excluded.reason, user_id=excluded.user_id`,
		model.NormalizeUsername(u.Username), u.UserID, model.ReserveCurrent, now)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	_, err = tx.Exec(`INSERT INTO user_lifecycle_events(event_id,user_id,kind,actor_id,note,detail,created_at) VALUES(?,?,?,?,?,?,?)`,
		newID("ULE"), u.UserID, model.LifeRestored, actorID, note, string(detail), now)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	_, err = tx.Exec(`INSERT INTO user_lifecycle_events(event_id,user_id,kind,actor_id,note,detail,created_at) VALUES(?,?,?,?,?,?,?)`,
		newID("ULE"), u.UserID, model.LifeLeaveExpire, actorID, "연차 근속 새로 시작(원장 expire는 69단계)", string(detail), now)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := audit.InsertTx(tx, audit.ActionUpdate, "users", "user_id", u.UserID, u.FullName, userSnapshotJSON(u), "{}", note, "계정 복구"); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (r *UserRepo) ListDeleted() ([]model.User, error) {
	rows, err := r.db.Query(userSelect + ` WHERE TRIM(COALESCE(deleted_at,''))!='' ORDER BY deleted_at DESC`)
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

func (r *UserRepo) FindDeletedByNameMobile(name, mobile string) *model.User {
	name, mobile = strings.TrimSpace(name), strings.TrimSpace(mobile)
	if name == "" || r == nil || r.db == nil {
		return nil
	}
	row := r.db.QueryRow(userSelect+` WHERE TRIM(COALESCE(deleted_at,''))!='' AND full_name=? AND TRIM(COALESCE(mobile,''))=? LIMIT 1`, name, mobile)
	u, err := scanUser(row, false)
	if err != nil {
		return nil
	}
	return &u
}

func (r *UserRepo) SetMustChangePassword(userID string, on bool) error {
	_, err := r.db.Exec(`UPDATE users SET must_change_password=? WHERE user_id=?`, boolToInt(on), userID)
	return err
}

func (r *UserRepo) TransferAssignedWork(fromID, toID string) error {
	fromID, toID = strings.TrimSpace(fromID), strings.TrimSpace(toID)
	if fromID == "" || toID == "" {
		return fmt.Errorf("이관 대상이 없습니다")
	}
	to, err := r.GetByID(toID)
	if err != nil || to == nil || to.IsDeleted() || !to.IsActive {
		return fmt.Errorf("이관 대상이 없습니다")
	}
	for _, t := range assigneeNameTargets {
		if !sqliteTableExists(r.db, t.table) || !tableHasColumn(r.db, t.table, t.idCol) {
			continue
		}
		if tableHasColumn(r.db, t.table, t.nameCol) {
			_, err = r.db.Exec(`UPDATE "`+t.table+`" SET "`+t.idCol+`"=?, "`+t.nameCol+`"=? WHERE TRIM(COALESCE("`+t.idCol+`",''))=?`,
				to.UserID, to.FullName, fromID)
		} else {
			_, err = r.db.Exec(`UPDATE "`+t.table+`" SET "`+t.idCol+`"=? WHERE TRIM(COALESCE("`+t.idCol+`",''))=?`, to.UserID, fromID)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

const accReqSelect = `SELECT request_id, org_id, requested_by, full_name, username, role, COALESCE(mobile,''), COALESCE(reason,''),
	status, COALESCE(restore_user_id,''), COALESCE(decided_by,''), COALESCE(decided_at,''), COALESCE(decision_note,''),
	COALESCE(created_user_id,''), COALESCE(created_at,'') FROM account_requests`

func scanAccountRequest(row interface{ Scan(dest ...interface{}) error }) (model.AccountRequest, error) {
	var a model.AccountRequest
	err := row.Scan(&a.RequestID, &a.OrgID, &a.RequestedBy, &a.FullName, &a.Username, &a.Role, &a.Mobile, &a.Reason,
		&a.Status, &a.RestoreUserID, &a.DecidedBy, &a.DecidedAt, &a.DecisionNote, &a.CreatedUserID, &a.CreatedAt)
	return a, err
}

func (r *AccountRepo) CreateRequest(a *model.AccountRequest) error {
	if r == nil || r.db == nil || a == nil {
		return fmt.Errorf("요청이 없습니다")
	}
	a.RequestID = newID("ARQ")
	a.Status = model.AccountReqPending
	a.CreatedAt = time.Now().Format("2006-01-02 15:04:05")
	_, err := r.db.Exec(`INSERT INTO account_requests(request_id,org_id,requested_by,full_name,username,role,mobile,reason,status,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`,
		a.RequestID, a.OrgID, a.RequestedBy, a.FullName, a.Username, a.Role, a.Mobile, a.Reason, a.Status, a.CreatedAt)
	return err
}

func (r *AccountRepo) GetRequest(id string) (*model.AccountRequest, error) {
	row := r.db.QueryRow(accReqSelect+` WHERE request_id=?`, strings.TrimSpace(id))
	a, err := scanAccountRequest(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *AccountRepo) ListRequests(status, orgID, by string) ([]model.AccountRequest, error) {
	q := accReqSelect + ` WHERE 1=1`
	var args []interface{}
	if status != "" {
		q += ` AND status=?`
		args = append(args, status)
	}
	if orgID != "" {
		q += ` AND org_id=?`
		args = append(args, orgID)
	}
	if by != "" {
		q += ` AND requested_by=?`
		args = append(args, by)
	}
	q += ` ORDER BY created_at DESC`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.AccountRequest
	for rows.Next() {
		a, err := scanAccountRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *AccountRepo) CountPending() int {
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM account_requests WHERE status=?`, model.AccountReqPending).Scan(&n)
	return n
}

func (r *AccountRepo) Decide(id, status, by, note, createdUserID, restoreUserID string) error {
	_, err := r.db.Exec(`UPDATE account_requests SET status=?, decided_by=?, decided_at=?, decision_note=?, created_user_id=?, restore_user_id=? WHERE request_id=? AND status=?`,
		status, by, time.Now().Format("2006-01-02 15:04:05"), note, createdUserID, restoreUserID, id, model.AccountReqPending)
	return err
}

func migration057RoundTrip(db *sql.DB) error {
	up, err := os.ReadFile(filepath.Join("..", "..", "migrations", "057_account_lifecycle.sql"))
	if err != nil {
		up, err = os.ReadFile(filepath.Join("migrations", "057_account_lifecycle.sql"))
	}
	if err != nil {
		return err
	}
	if _, err := db.Exec(string(up)); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate") {
		return err
	}
	return nil
}
