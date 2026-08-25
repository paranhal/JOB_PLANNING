package audit

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	ActionCreate = "create"
	ActionUpdate = "update"
	ActionDelete = "delete"

	KindAuto       = "auto"
	KindManual     = "manual"
	KindLogArchive = "log_archive"
)

var dbRef *sql.DB

// Init 변경 이력 DB 연결.
func Init(db *sql.DB) { dbRef = db }

// Use 기록에 쓸 DB. repo·백업이 Init 순서와 무관하게 남기도록 한다.
func Use(db *sql.DB) {
	if db != nil {
		dbRef = db
	}
}

func DB() *sql.DB { return dbRef }

// ChangeLog 데이터 변경 한 건.
type ChangeLog struct {
	LogID        string
	OccurredAt   string
	UserID       string
	Username     string
	UserName     string
	Action       string
	ActionLabel  string
	TableName    string
	PKColumn     string
	EntityID     string
	EntityLabel  string
	Summary      string
	BeforeJSON   string
	AfterJSON    string
	RolledBack   bool
	RolledBackAt string
}

func actionLabel(a string) string {
	switch a {
	case ActionCreate:
		return "등록"
	case ActionUpdate:
		return "수정"
	case ActionDelete:
		return "삭제"
	default:
		return a
	}
}

// Log 변경 이력을 남긴다. 감사 테이블 자신은 기록하지 않는다.
func Log(action, table, pk, id, label, before, after string) {
	if dbRef == nil || table == "data_change_logs" || table == "data_backups" || table == "data_log_archives" {
		return
	}
	if strings.TrimSpace(id) == "" && strings.TrimSpace(before) == "" && strings.TrimSpace(after) == "" {
		return
	}
	a := Current()
	if label == "" {
		label = table
	}
	sum := fmt.Sprintf("%s %s (%s)", actionLabel(action), label, id)
	_, err := dbRef.Exec(`
		INSERT INTO data_change_logs (
			log_id, occurred_at, user_id, username, user_name,
			action, table_name, pk_column, entity_id, entity_label, summary,
			before_json, after_json, rolled_back
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,0)`,
		"L"+uuid.New().String(),
		time.Now().Format("2006-01-02 15:04:05"),
		a.UserID, a.Username, a.Name,
		action, table, pk, id, label, sum,
		nullEmpty(before), nullEmpty(after),
	)
	if err != nil {
		log.Printf("data_change_logs 기록 실패 (%s %s %s): %v", action, table, id, err)
	}
}

func nullEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return "{}"
	}
	return s
}

// ListLogs 최근 변경 이력.
func ListLogs(limit int) ([]ChangeLog, error) {
	if dbRef == nil {
		return nil, nil
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := dbRef.Query(`
		SELECT log_id, occurred_at, COALESCE(user_id,''), COALESCE(username,''), COALESCE(user_name,''),
		       action, table_name, COALESCE(pk_column,''), COALESCE(entity_id,''), COALESCE(entity_label,''),
		       COALESCE(summary,''), COALESCE(before_json,''), COALESCE(after_json,''),
		       COALESCE(rolled_back,0), COALESCE(rolled_back_at,'')
		FROM data_change_logs
		ORDER BY occurred_at DESC, log_id DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChangeLog
	for rows.Next() {
		var l ChangeLog
		var rb int
		if err := rows.Scan(&l.LogID, &l.OccurredAt, &l.UserID, &l.Username, &l.UserName,
			&l.Action, &l.TableName, &l.PKColumn, &l.EntityID, &l.EntityLabel,
			&l.Summary, &l.BeforeJSON, &l.AfterJSON, &rb, &l.RolledBackAt); err != nil {
			return nil, err
		}
		l.RolledBack = rb == 1
		l.ActionLabel = actionLabel(l.Action)
		out = append(out, l)
	}
	return out, rows.Err()
}

func getLog(id string) (*ChangeLog, error) {
	var l ChangeLog
	var rb int
	err := dbRef.QueryRow(`
		SELECT log_id, occurred_at, COALESCE(user_id,''), COALESCE(username,''), COALESCE(user_name,''),
		       action, table_name, COALESCE(pk_column,''), COALESCE(entity_id,''), COALESCE(entity_label,''),
		       COALESCE(summary,''), COALESCE(before_json,''), COALESCE(after_json,''),
		       COALESCE(rolled_back,0), COALESCE(rolled_back_at,'')
		FROM data_change_logs WHERE log_id=?`, id).Scan(
		&l.LogID, &l.OccurredAt, &l.UserID, &l.Username, &l.UserName,
		&l.Action, &l.TableName, &l.PKColumn, &l.EntityID, &l.EntityLabel,
		&l.Summary, &l.BeforeJSON, &l.AfterJSON, &rb, &l.RolledBackAt)
	if err != nil {
		return nil, err
	}
	l.RolledBack = rb == 1
	l.ActionLabel = actionLabel(l.Action)
	return &l, nil
}

// RecordBackup 백업 목록에 한 건을 남긴다. 같은 폴더면 갱신한다.
func RecordBackup(kind, folder, note string) {
	if dbRef == nil {
		log.Printf("data_backups 기록 생략: audit.Init 전 (%s)", folder)
		return
	}
	if folder == "" {
		return
	}
	a := Current()
	who := strings.TrimSpace(a.Name)
	if who == "" {
		who = a.Username
	}
	if kind == KindAuto && who == "" {
		who = "시스템"
	}
	_, err := dbRef.Exec(`
		INSERT INTO data_backups (
			backup_id, folder_name, kind, created_at, created_by_id, created_by_name, note
		) VALUES (?,?,?,?,?,?,?)
		ON CONFLICT(folder_name) DO UPDATE SET
			kind=excluded.kind,
			created_at=excluded.created_at,
			created_by_id=excluded.created_by_id,
			created_by_name=excluded.created_by_name,
			note=excluded.note`,
		"B"+uuid.New().String(), folder, kind,
		time.Now().Format("2006-01-02 15:04:05"),
		a.UserID, who, note,
	)
	if err != nil {
		log.Printf("data_backups 기록 실패 (%s): %v", folder, err)
	}
}

// HasBackupFolder data_backups에 해당 폴더가 있는지.
func HasBackupFolder(folder string) bool {
	if dbRef == nil || folder == "" {
		return false
	}
	var n int
	_ = dbRef.QueryRow(`SELECT COUNT(*) FROM data_backups WHERE folder_name=?`, folder).Scan(&n)
	return n > 0
}

// EnsureBackupRow 폴더 스캔으로 누락된 백업 목록 행을 넣는다. 이미 있으면 그대로 둔다.
func EnsureBackupRow(kind, folder, createdAt, who, note string) error {
	if dbRef == nil {
		return fmt.Errorf("audit DB가 없습니다")
	}
	if folder == "" {
		return nil
	}
	if createdAt == "" {
		createdAt = time.Now().Format("2006-01-02 15:04:05")
	}
	who = strings.TrimSpace(who)
	if kind == KindAuto && who == "" {
		who = "시스템"
	}
	_, err := dbRef.Exec(`
		INSERT INTO data_backups (
			backup_id, folder_name, kind, created_at, created_by_id, created_by_name, note
		) VALUES (?,?,?,?,?,?,?)
		ON CONFLICT(folder_name) DO NOTHING`,
		"B"+uuid.New().String(), folder, kind, createdAt, "", who, note)
	if err != nil {
		log.Printf("data_backups 보정 실패 (%s): %v", folder, err)
	}
	return err
}

// BackupRowCount 목록 테이블 건수.
func BackupRowCount() int {
	if dbRef == nil {
		return 0
	}
	var n int
	_ = dbRef.QueryRow(`SELECT COUNT(*) FROM data_backups`).Scan(&n)
	return n
}

// BackupActors 폴더명 → 저장한 사람.
func BackupActors() map[string]string {
	out := map[string]string{}
	if dbRef == nil {
		return out
	}
	rows, err := dbRef.Query(`SELECT folder_name, COALESCE(created_by_name,'') FROM data_backups`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var folder, who string
		if rows.Scan(&folder, &who) == nil && folder != "" {
			out[folder] = who
		}
	}
	return out
}

// LogStats 현재 테이블에 남은 변경 이력 건수·가장 오래된 시각.
func LogStats() (count int, oldest string) {
	if dbRef == nil {
		return 0, ""
	}
	_ = dbRef.QueryRow(`SELECT COUNT(*), COALESCE(MIN(occurred_at),'') FROM data_change_logs`).Scan(&count, &oldest)
	return count, oldest
}

// BackupRow 백업 목록 한 줄.
type BackupRow struct {
	BackupID      string
	FolderName    string
	Kind          string
	KindLabel     string
	CreatedAt     string
	CreatedByName string
	Note          string
	Exists        bool
}

func KindLabel(kind string) string {
	switch kind {
	case KindManual:
		return "사용자 백업"
	case KindLogArchive:
		return "관리 로그"
	default:
		return "정기 백업"
	}
}

func (l ChangeLog) Who() string {
	if strings.TrimSpace(l.UserName) != "" {
		return l.UserName
	}
	if strings.TrimSpace(l.Username) != "" {
		return l.Username
	}
	return "(시스템)"
}

func (l ChangeLog) TableLabel() string { return TableLabel(l.TableName) }

func (l ChangeLog) CanRollback() bool {
	if l.RolledBack {
		return false
	}
	return l.Action == ActionCreate || l.Action == ActionUpdate || l.Action == ActionDelete
}

func TableLabel(t string) string {
	switch t {
	case "customers":
		return "고객"
	case "assets":
		return "설치자산"
	case "as_receipts":
		return "AS접수"
	case "as_work_items":
		return "AS하부업무"
	case "contacts":
		return "담당자"
	case "codes":
		return "코드"
	case "users":
		return "사용자"
	case "attachments":
		return "첨부"
	case "work_projects":
		return "사업"
	case "sales_projects":
		return "영업 사업"
	case "sales_projects":
		return "영업 사업"
	case "work_tasks":
		return "일일업무"
	case "work_actions":
		return "다음 행동"
	case "work_activities":
		return "조치 이력"
	case "maintenance_visits":
		return "정기점검"
	case "holidays":
		return "휴무일"
	case "staff_leaves":
		return "연차"
	case "app_settings":
		return "설정"
	case "as_processes":
		return "AS조치"
	case "customer_buildings":
		return "건물"
	case "customer_floors":
		return "층"
	case "customer_rooms":
		return "호실"
	default:
		return t
	}
}

func prettyJSON(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "{}" {
		return ""
	}
	var v any
	if json.Unmarshal([]byte(s), &v) != nil {
		return s
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return s
	}
	return string(b)
}

// LogDetail 화면용 전·후 JSON 정리.
func (l ChangeLog) BeforePretty() string { return prettyJSON(l.BeforeJSON) }
func (l ChangeLog) AfterPretty() string  { return prettyJSON(l.AfterJSON) }

