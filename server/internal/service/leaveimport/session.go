package leaveimport

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

type Session struct {
	Year  int
	Rows  []PreviewRow
	Until time.Time
}

var (
	sessMu   sync.Mutex
	sessions = map[string]*Session{}
)

func PutPreview(rows []PreviewRow, year int) string {
	tok := uuid.NewString()
	sessMu.Lock()
	defer sessMu.Unlock()
	for k, v := range sessions {
		if time.Now().After(v.Until) {
			delete(sessions, k)
		}
	}
	sessions[tok] = &Session{Year: year, Rows: rows, Until: time.Now().Add(30 * time.Minute)}
	return tok
}

func GetPreview(tok string) (*Session, bool) {
	sessMu.Lock()
	defer sessMu.Unlock()
	s, ok := sessions[tok]
	if !ok || time.Now().After(s.Until) {
		delete(sessions, tok)
		return nil, false
	}
	return s, true
}

func DropPreview(tok string) {
	sessMu.Lock()
	defer sessMu.Unlock()
	delete(sessions, tok)
}
