package repository

import (
	"database/sql"
	"log"
	"strings"
	"time"
)

func applyASCaseVotes(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS as_case_votes (
			as_id      TEXT NOT NULL,
			user_id    TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (as_id, user_id)
		)`); err != nil {
		log.Printf("043 as_case_votes: %v", err)
		return
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_as_case_votes_as ON as_case_votes(as_id)`); err != nil {
		log.Printf("043 as_case_votes idx: %v", err)
	}
}

func (r *ASRepo) ToggleCaseVote(asID, userID string) (voted bool, err error) {
	asID = strings.TrimSpace(asID)
	userID = strings.TrimSpace(userID)
	if asID == "" || userID == "" {
		return false, nil
	}
	var n int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM as_case_votes WHERE as_id=? AND user_id=?`, asID, userID).Scan(&n); err != nil {
		return false, err
	}
	if n > 0 {
		_, err = r.db.Exec(`DELETE FROM as_case_votes WHERE as_id=? AND user_id=?`, asID, userID)
		return false, err
	}
	_, err = r.db.Exec(`INSERT INTO as_case_votes(as_id, user_id, created_at) VALUES (?,?,?)`,
		asID, userID, time.Now().Format("2006-01-02 15:04:05"))
	return err == nil, err
}

func (r *ASRepo) CaseVoteCount(asID string) int {
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM as_case_votes WHERE as_id=?`, asID).Scan(&n)
	return n
}

func (r *ASRepo) CaseVotedBy(asID, userID string) bool {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return false
	}
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM as_case_votes WHERE as_id=? AND user_id=?`, asID, userID).Scan(&n)
	return n > 0
}

func (r *ASRepo) CaseVotedSet(userID string, ids []string) map[string]bool {
	out := map[string]bool{}
	userID = strings.TrimSpace(userID)
	if userID == "" || len(ids) == 0 {
		return out
	}
	args := make([]interface{}, 0, len(ids)+1)
	args = append(args, userID)
	ph := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		ph = append(ph, "?")
		args = append(args, id)
	}
	if len(ph) == 0 {
		return out
	}
	rows, err := r.db.Query(`SELECT as_id FROM as_case_votes WHERE user_id=? AND as_id IN (`+strings.Join(ph, ",")+`)`, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			out[id] = true
		}
	}
	return out
}
