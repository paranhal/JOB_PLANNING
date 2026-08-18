package auditlog

import (
	"database/sql"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// 업무 서버는 이 패키지로 INSERT 만 한다. 조회·수정·삭제 API는 두지 않는다(§25.3.1).
// 이전 행 해시는 DB를 읽지 않고 chain.head 파일로만 이어 간다.

var errWriteOnly = errors.New("접속기록 DB는 추가만 허용한다")

var (
	defaultMu     sync.Mutex
	defaultWriter *Writer
)

// Writer access.db 쓰기 전용 연결.
type Writer struct {
	mu       sync.Mutex
	db       *sql.DB
	headPath string
	prevHash string
}

// Init 접속기록 DB를 연다. 실패해도 업무를 막지 않도록 호출 쪽에서 로그만 남긴다.
func Init(dbPath string) error {
	w, err := Open(dbPath)
	if err != nil {
		return err
	}
	defaultMu.Lock()
	if defaultWriter != nil {
		_ = defaultWriter.Close()
	}
	defaultWriter = w
	defaultMu.Unlock()
	return nil
}

// Close 기본 Writer를 닫는다.
func Close() {
	defaultMu.Lock()
	w := defaultWriter
	defaultWriter = nil
	defaultMu.Unlock()
	if w != nil {
		_ = w.Close()
	}
}

// Append 한 줄을 추가한다. 미초기화·실패는 로그만 남기고 업무는 계속한다.
func Append(rec Record) {
	defaultMu.Lock()
	w := defaultWriter
	defaultMu.Unlock()
	if w == nil {
		return
	}
	w.Append(rec)
}

// Open 지정 경로의 access.db 를 연다. 테스트와 Init 가 사용한다.
func Open(dbPath string) (*Writer, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	w := &Writer{
		db:       db,
		headPath: filepath.Join(filepath.Dir(dbPath), "chain.head"),
	}
	if err := w.exec(`PRAGMA journal_mode=WAL`); err != nil {
		db.Close()
		return nil, err
	}
	if err := w.exec(`PRAGMA busy_timeout=5000`); err != nil {
		db.Close()
		return nil, err
	}
	for _, q := range schemaStmts {
		if err := w.exec(q); err != nil {
			db.Close()
			return nil, err
		}
	}
	w.prevHash = loadHead(w.headPath)
	return w, nil
}

// Append 한 줄을 추가한다. 실패해도 호출자에게 에러를 올리지 않는다.
func (w *Writer) Append(rec Record) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.db == nil {
		return
	}
	if rec.LogID == "" {
		rec.LogID = uuid.New().String()
	}
	if rec.AccessAt == "" {
		rec.AccessAt = nowSeoul()
	}
	if rec.Result == "" {
		rec.Result = ResultOK
	}
	rec.PrevHash = w.prevHash
	rec.RowHash = rowHash(rec)
	if err := w.exec(insertSQL,
		rec.LogID, rec.UserID, rec.Username, rec.FullName, rec.Role, rec.AccessAt,
		rec.ClientIP, rec.ForwardedFor, rec.UserAgent, rec.SessionID,
		rec.SubjectType, rec.SubjectID, rec.SubjectName,
		rec.Action, rec.TargetTable, rec.TargetID, rec.Detail,
		rec.BeforeJSON, rec.AfterJSON, rec.Reason, rec.Result,
		rec.PrevHash, rec.RowHash,
	); err != nil {
		log.Printf("접속기록 추가 실패: %v", err)
		return
	}
	w.prevHash = rec.RowHash
	if err := persistHead(w.headPath, rec.RowHash); err != nil {
		log.Printf("접속기록 연쇄 해시 저장 실패: %v", err)
	}
}

// Close 연결을 닫는다.
func (w *Writer) Close() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.db == nil {
		return nil
	}
	err := w.db.Close()
	w.db = nil
	return err
}

func (w *Writer) exec(query string, args ...any) error {
	if err := writeOnly(query); err != nil {
		return err
	}
	_, err := w.db.Exec(query, args...)
	return err
}

func writeOnly(query string) error {
	fields := strings.Fields(strings.TrimSpace(query))
	if len(fields) == 0 {
		return errWriteOnly
	}
	first := strings.ToUpper(strings.TrimRight(fields[0], ";"))
	switch first {
	case "INSERT", "CREATE":
		return nil
	case "PRAGMA":
		key := ""
		if len(fields) >= 2 {
			key = strings.ToUpper(strings.TrimRight(fields[1], "=;"))
		}
		if strings.HasPrefix(key, "JOURNAL_MODE") || strings.HasPrefix(key, "BUSY_TIMEOUT") || strings.HasPrefix(key, "SYNCHRONOUS") {
			return nil
		}
		return errWriteOnly
	default:
		return errWriteOnly
	}
}

func loadHead(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func persistHead(path, hash string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(hash+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
