package leaveimport

import (
	"database/sql"
	"strings"
)

type Candidate struct {
	UserID, Name, OrgID string
}

type PreviewRow struct {
	StaffRow
	UserID       string
	OrgID        string
	Status       string
	Candidates   []Candidate
	HireConflict bool
	HireExisting string
}

func Match(db *sql.DB, rows []StaffRow) ([]PreviewRow, error) {
	out := make([]PreviewRow, 0, len(rows))
	for _, r := range rows {
		pr := PreviewRow{StaffRow: r, Status: "skip"}
		if r.GrantEmpty {
			pr.Status = "unregistered"
			out = append(out, pr)
			continue
		}
		cands, err := findByName(db, r.Name)
		if err != nil {
			return nil, err
		}
		pr.Candidates = cands
		switch len(cands) {
		case 0:
			pr.Status = "unmatched"
		case 1:
			pr.UserID = cands[0].UserID
			pr.OrgID = cands[0].OrgID
			pr.Status = "matched"
		default:
			pr.Status = "duplicate"
		}
		if pr.UserID != "" && r.HireDate != "" {
			conflict, existing, err := hireConflict(db, pr.UserID, r.HireDate)
			if err != nil {
				return nil, err
			}
			pr.HireConflict = conflict
			pr.HireExisting = existing
		}
		if len(r.Errors) > 0 {
			pr.Status = "error"
		}
		out = append(out, pr)
	}
	return out, nil
}

func ApplyPicks(db *sql.DB, rows []PreviewRow, picks map[int]string) []PreviewRow {
	for i, r := range rows {
		id := strings.TrimSpace(picks[r.ExcelRow])
		if id == "" || r.GrantEmpty || len(r.Errors) > 0 {
			continue
		}
		var org string
		err := db.QueryRow(`SELECT COALESCE(org_id,'') FROM users WHERE user_id=? AND is_active=1`, id).Scan(&org)
		if err != nil {
			continue
		}
		rows[i].UserID = id
		rows[i].OrgID = org
		rows[i].Status = "matched"
	}
	return rows
}

func findByName(db *sql.DB, name string) ([]Candidate, error) {
	name = strings.TrimSpace(name)
	rows, err := db.Query(`SELECT user_id, full_name, COALESCE(org_id,'') FROM users
		WHERE is_active=1 AND TRIM(full_name)=? ORDER BY user_id`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Candidate
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.UserID, &c.Name, &c.OrgID); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func hireConflict(db *sql.DB, userID, hire string) (bool, string, error) {
	var existing sql.NullString
	err := db.QueryRow(`SELECT hire_date FROM employee_profiles WHERE user_id=?`, userID).Scan(&existing)
	if err == sql.ErrNoRows {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	got := strings.TrimSpace(existing.String)
	if got != "" && got != hire {
		return true, got, nil
	}
	return false, got, nil
}

func Ready(r PreviewRow) bool {
	return r.UserID != "" && !r.GrantEmpty && len(r.Errors) == 0
}
