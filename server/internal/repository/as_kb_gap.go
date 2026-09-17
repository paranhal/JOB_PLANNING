package repository

import (
	"database/sql"
	"log"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"customer-support/internal/model"
)

func applyASKbGaps(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS as_kb_gaps (
			gap_id          TEXT PRIMARY KEY,
			query           TEXT NOT NULL DEFAULT '',
			query_norm      TEXT NOT NULL DEFAULT '',
			searched_by     TEXT NOT NULL DEFAULT '',
			searched_at     TEXT NOT NULL DEFAULT '',
			hit_count       INTEGER NOT NULL DEFAULT 0,
			as_id           TEXT,
			search_count    INTEGER NOT NULL DEFAULT 1,
			resolved_kb_id  TEXT,
			resolved_at     TEXT
		)`); err != nil {
		log.Printf("045 as_kb_gaps: %v", err)
		return
	}
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_as_kb_gaps_open ON as_kb_gaps(query_norm, hit_count)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_as_kb_gaps_resolved ON as_kb_gaps(resolved_kb_id)`)
}

func (r *ASRepo) RecordKBGap(query, searchedBy, asID string) (*model.ASKBGap, error) {
	display := strings.TrimSpace(query)
	norm := model.NormalizeKBQuery(display)
	if norm == "" || r == nil || r.db == nil {
		return nil, nil
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	asID = strings.TrimSpace(asID)
	searchedBy = strings.TrimSpace(searchedBy)

	cur, err := r.openGapByNorm(norm)
	if err != nil {
		return nil, err
	}
	if cur != nil {
		if asID != "" && strings.TrimSpace(cur.ASID) == "" {
			cur.ASID = asID
		}
		cur.Query = display
		cur.SearchedBy = searchedBy
		cur.SearchedAt = now
		cur.SearchCount++
		_, err = r.db.Exec(`
			UPDATE as_kb_gaps
			   SET query=?, searched_by=?, searched_at=?, as_id=?, search_count=?
			 WHERE gap_id=?`,
			cur.Query, cur.SearchedBy, cur.SearchedAt, nullStr(cur.ASID), cur.SearchCount, cur.GapID)
		if err != nil {
			return nil, err
		}
		cur.RecentLabel = model.GapRecentLabel(cur.SearchedAt)
		return cur, nil
	}

	g := &model.ASKBGap{
		GapID:       newID("GAP"),
		Query:       display,
		QueryNorm:   norm,
		SearchedBy:  searchedBy,
		SearchedAt:  now,
		HitCount:    0,
		ASID:        asID,
		SearchCount: 1,
	}
	_, err = r.db.Exec(`
		INSERT INTO as_kb_gaps(
			gap_id, query, query_norm, searched_by, searched_at, hit_count, as_id, search_count, resolved_kb_id, resolved_at)
		VALUES (?,?,?,?,?,0,?,1,NULL,NULL)`,
		g.GapID, g.Query, g.QueryNorm, g.SearchedBy, g.SearchedAt, nullStr(g.ASID))
	if err != nil {
		return nil, err
	}
	g.RecentLabel = model.GapRecentLabel(g.SearchedAt)
	return g, nil
}

func (r *ASRepo) openGapByNorm(norm string) (*model.ASKBGap, error) {
	row := r.db.QueryRow(`
		SELECT gap_id, query, query_norm, searched_by, searched_at, hit_count, COALESCE(as_id,''),
		       search_count, COALESCE(resolved_kb_id,''), COALESCE(resolved_at,'')
		  FROM as_kb_gaps
		 WHERE query_norm=? AND hit_count=0 AND TRIM(COALESCE(resolved_kb_id,''))=''
		 ORDER BY search_count DESC, searched_at DESC
		 LIMIT 1`, norm)
	g, err := scanKBGap(row.Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return g, err
}

func scanKBGap(s func(dest ...interface{}) error) (*model.ASKBGap, error) {
	var g model.ASKBGap
	if err := s(&g.GapID, &g.Query, &g.QueryNorm, &g.SearchedBy, &g.SearchedAt, &g.HitCount,
		&g.ASID, &g.SearchCount, &g.ResolvedKBID, &g.ResolvedAt); err != nil {
		return nil, err
	}
	g.RecentLabel = model.GapRecentLabel(g.SearchedAt)
	return &g, nil
}

func (r *ASRepo) GetKBGap(id string) (*model.ASKBGap, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, sql.ErrNoRows
	}
	g, err := scanKBGap(r.db.QueryRow(`
		SELECT gap_id, query, query_norm, searched_by, searched_at, hit_count, COALESCE(as_id,''),
		       search_count, COALESCE(resolved_kb_id,''), COALESCE(resolved_at,'')
		  FROM as_kb_gaps WHERE gap_id=?`, id).Scan)
	if err != nil {
		return nil, err
	}
	return g, nil
}

func (r *ASRepo) ListOpenKBGaps() ([]model.ASKBGap, error) {
	rows, err := r.db.Query(`
		SELECT gap_id, query, query_norm, searched_by, searched_at, hit_count, COALESCE(as_id,''),
		       search_count, COALESCE(resolved_kb_id,''), COALESCE(resolved_at,'')
		  FROM as_kb_gaps
		 WHERE hit_count=0 AND TRIM(COALESCE(resolved_kb_id,''))=''
		 ORDER BY search_count DESC, searched_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ASKBGap
	for rows.Next() {
		g, err := scanKBGap(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}

func (r *ASRepo) ResolveKBGap(gapID, kbID string) error {
	gapID = strings.TrimSpace(gapID)
	kbID = strings.TrimSpace(kbID)
	if gapID == "" || kbID == "" {
		return nil
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	g, err := r.GetKBGap(gapID)
	if err != nil || g == nil {
		return err
	}
	_, err = r.db.Exec(`
		UPDATE as_kb_gaps SET resolved_kb_id=?, resolved_at=?
		 WHERE query_norm=? AND hit_count=0 AND TRIM(COALESCE(resolved_kb_id,''))=''`,
		kbID, now, g.QueryNorm)
	return err
}

func (r *ASRepo) KBGapProgress() (filledMonth, openQueries int) {
	month := time.Now().Format("2006-01")
	_ = r.db.QueryRow(`
		SELECT COUNT(*) FROM as_kb_entries
		 WHERE is_current=1 AND status=? AND created_at LIKE ?`,
		model.KBStatusPublished, month+"%").Scan(&filledMonth)
	_ = r.db.QueryRow(`
		SELECT COUNT(*) FROM as_kb_gaps
		 WHERE hit_count=0 AND TRIM(COALESCE(resolved_kb_id,''))=''`).Scan(&openQueries)
	return
}

func (r *ASRepo) ListMissingActionSymptoms() ([]model.ASMissingAction, error) {
	rows, err := r.db.Query(`
		SELECT ar.as_id, COALESCE(ar.symptom,'')
		  FROM as_receipts ar
		 WHERE NOT (` + knowledgeWorkSQL + `)
		 ORDER BY ar.receipt_datetime DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ASMissingAction
	for rows.Next() {
		var row model.ASMissingAction
		if err := rows.Scan(&row.ASID, &row.Symptom); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *ASRepo) MissingActionCount() int {
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM as_receipts ar WHERE NOT (` + knowledgeWorkSQL + `)`).Scan(&n)
	return n
}

func GroupMissingActionByKeyword(dict []model.ASKeyword, rows []model.ASMissingAction) []model.ASGapSymptomGroup {
	type acc struct {
		g model.ASGapSymptomGroup
	}
	byID := map[string]*acc{}
	var other model.ASGapSymptomGroup
	other.Keyword = "그 외"
	other.Other = true
	for _, row := range rows {
		kw := pickKeywordForSymptom(dict, row.Symptom)
		if kw == nil {
			other.Count++
			if other.SampleASID == "" {
				other.SampleASID = row.ASID
				other.SampleSymptom = row.Symptom
			}
			continue
		}
		a, ok := byID[kw.KeywordID]
		if !ok {
			a = &acc{g: model.ASGapSymptomGroup{
				Keyword: kw.Keyword, KeywordID: kw.KeywordID,
			}}
			byID[kw.KeywordID] = a
		}
		a.g.Count++
		if a.g.SampleASID == "" {
			a.g.SampleASID = row.ASID
			a.g.SampleSymptom = row.Symptom
		}
	}
	out := make([]model.ASGapSymptomGroup, 0, len(byID)+1)
	for _, a := range byID {
		out = append(out, a.g)
	}
	sortGapGroups(out)
	if other.Count > 0 {
		out = append(out, other)
	}
	return out
}

func sortGapGroups(items []model.ASGapSymptomGroup) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		return items[i].Keyword < items[j].Keyword
	})
}

func pickKeywordForSymptom(dict []model.ASKeyword, symptom string) *model.ASKeyword {
	if k := bestKeyword(dict, symptom, true); k != nil {
		return k
	}
	return bestKeyword(dict, symptom, false)
}

func bestKeyword(dict []model.ASKeyword, text string, primary bool) *model.ASKeyword {
	bestScore := -1
	var best *model.ASKeyword
	for i := range dict {
		k := &dict[i]
		if primary && k.Group != model.KWGroupTarget && k.Group != model.KWGroupSymptom {
			continue
		}
		if !keywordMatchesText(text, *k) {
			continue
		}
		score := keywordGroupWeight(k.Group)
		for _, term := range k.MatchTerms() {
			term = strings.TrimSpace(term)
			if term == "" || !matchKeywordTerm(text, term) {
				continue
			}
			n := utf8.RuneCountInString(term)*1000 + keywordGroupWeight(k.Group)
			if n > score {
				score = n
			}
		}
		if score > bestScore {
			bestScore = score
			best = k
		}
	}
	return best
}

func keywordGroupWeight(g string) int {
	switch g {
	case model.KWGroupTarget:
		return 400
	case model.KWGroupSymptom:
		return 300
	case model.KWGroupCause:
		return 200
	case model.KWGroupAction:
		return 100
	default:
		return 0
	}
}
