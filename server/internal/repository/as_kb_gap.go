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

const gapNoPublishedKBSQL = `NOT EXISTS (SELECT 1 FROM as_kb_entries k
	WHERE k.as_id=ar.as_id AND k.is_current=1 AND k.status='published')`

const gapLatestActionSQL = `COALESCE((SELECT p.work_content FROM as_processes p
	WHERE p.as_id=ar.as_id ORDER BY p.process_datetime DESC LIMIT 1), '')`

const gapHasProcessSQL = `EXISTS (SELECT 1 FROM as_processes p WHERE p.as_id=ar.as_id)`

func classifyGapKind(action string, hasProcess bool) (kind, short string) {
	action = strings.TrimSpace(action)
	if !hasProcess || action == "" {
		return "none", ""
	}
	return "short", action
}

func (r *ASRepo) ListMissingActionSymptoms() ([]model.ASMissingAction, error) {
	rows, err := r.db.Query(`
		SELECT ar.as_id, COALESCE(ar.symptom,''), ` + gapLatestActionSQL + `,
		       CASE WHEN ` + gapHasProcessSQL + ` THEN 1 ELSE 0 END
		  FROM as_receipts ar
		 WHERE NOT (` + knowledgeWorkSQL + `)
		   AND ` + gapNoPublishedKBSQL + `
		 ORDER BY ar.receipt_datetime DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ASMissingAction
	for rows.Next() {
		var row model.ASMissingAction
		var has int
		if err := rows.Scan(&row.ASID, &row.Symptom, &row.Action, &has); err != nil {
			return nil, err
		}
		row.Kind, row.Action = classifyGapKind(row.Action, has == 1)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *ASRepo) MissingActionCount() int {
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM as_receipts ar WHERE NOT (` + knowledgeWorkSQL + `) AND ` + gapNoPublishedKBSQL).Scan(&n)
	return n
}

func (r *ASRepo) KeywordLinkCount() int {
	var n int
	if r == nil || r.db == nil {
		return 0
	}
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM as_keyword_links`).Scan(&n)
	return n
}

func (r *ASRepo) ReceiptCount() int {
	var n int
	if r == nil || r.db == nil {
		return 0
	}
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM as_receipts`).Scan(&n)
	return n
}

const gapEmptyActionSQL = `(NOT (` + gapHasProcessSQL + `) OR TRIM(` + gapLatestActionSQL + `)='')`

// ListGapKeywordGroups 묶음 건수를 목록과 같은 SQL 로 센다. §41.17 · §38
func (r *ASRepo) ListGapKeywordGroups() ([]model.ASGapSymptomGroup, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	where := `NOT (` + knowledgeWorkSQL + `) AND ` + gapNoPublishedKBSQL
	q := `
		SELECT k.keyword_id, k.keyword,
		       COUNT(DISTINCT ar.as_id) AS total,
		       COUNT(DISTINCT CASE WHEN ` + gapEmptyActionSQL + ` THEN ar.as_id END) AS none_count
		  FROM as_receipts ar
		  JOIN as_keyword_links kl ON kl.as_id = ar.as_id
		  JOIN as_keywords k ON k.keyword_id = kl.keyword_id AND k.is_active = 1
		 WHERE ` + where + `
		 GROUP BY k.keyword_id, k.keyword
		 ORDER BY total DESC, k.keyword`
	rows, err := r.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ASGapSymptomGroup
	for rows.Next() {
		var g model.ASGapSymptomGroup
		if err := rows.Scan(&g.KeywordID, &g.Keyword, &g.Count, &g.NoneCount); err != nil {
			return nil, err
		}
		g.ShortCount = g.Count - g.NoneCount
		if g.ShortCount < 0 {
			g.ShortCount = 0
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var other model.ASGapSymptomGroup
	other.Keyword = "그 외"
	other.Other = true
	err = r.db.QueryRow(`
		SELECT COUNT(DISTINCT ar.as_id),
		       COUNT(DISTINCT CASE WHEN `+gapEmptyActionSQL+` THEN ar.as_id END)
		  FROM as_receipts ar
		 WHERE `+where+`
		   AND NOT EXISTS (SELECT 1 FROM as_keyword_links kl WHERE kl.as_id=ar.as_id)`).
		Scan(&other.Count, &other.NoneCount)
	if err != nil {
		return nil, err
	}
	other.ShortCount = other.Count - other.NoneCount
	if other.ShortCount < 0 {
		other.ShortCount = 0
	}
	if other.Count > 0 {
		out = append(out, other)
	}
	return out, nil
}

// GapReceiptRow 지식 보완 묶음을 펼친 접수 한 줄. §41.16.3
type GapReceiptRow struct {
	ASID, ASNumber, ReceiptDate string
	CustomerName                string
	Symptom                     string
	ShortAction                 string
	Kind                        string // "none" | "short"
}

type GapReceiptSite struct {
	CustomerID, Name string
}

// ListGapReceipts 키워드(또는 그 외) 묶음의 접수 목록. LIMIT 없이 부르지 않는다. §41.16.6
func (r *ASRepo) ListGapReceipts(keywordID string, other bool, site string,
	sort, dir string, offset, limit int) ([]GapReceiptRow, int, error) {
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	where, args := gapReceiptsWhere(keywordID, other, site)
	from := `
		FROM as_receipts ar
		LEFT JOIN customers cu ON cu.customer_id = ar.customer_id
		WHERE ` + where
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) `+from, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := gapReceiptsOrder(sort, dir)
	q := `SELECT ar.as_id, COALESCE(ar.as_number,''), SUBSTR(ar.receipt_datetime,1,10),
		COALESCE(cu.org_name,''), COALESCE(ar.symptom,''), ` + gapLatestActionSQL + `,
		CASE WHEN ` + gapHasProcessSQL + ` THEN 1 ELSE 0 END
		` + from + ` ORDER BY ` + order + ` LIMIT ? OFFSET ?`
	args = append(append([]any{}, args...), limit, offset)
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []GapReceiptRow
	for rows.Next() {
		var row GapReceiptRow
		var action string
		var has int
		if err := rows.Scan(&row.ASID, &row.ASNumber, &row.ReceiptDate, &row.CustomerName, &row.Symptom, &action, &has); err != nil {
			return nil, 0, err
		}
		row.Kind, row.ShortAction = classifyGapKind(action, has == 1)
		out = append(out, row)
	}
	return out, total, rows.Err()
}

func (r *ASRepo) ListGapReceiptSites(keywordID string, other bool) ([]GapReceiptSite, error) {
	where, args := gapReceiptsWhere(keywordID, other, "")
	rows, err := r.db.Query(`
		SELECT ar.customer_id, COALESCE(cu.org_name,'')
		  FROM as_receipts ar
		  LEFT JOIN customers cu ON cu.customer_id = ar.customer_id
		 WHERE `+where+`
		   AND TRIM(COALESCE(ar.customer_id,'')) != ''
		 GROUP BY ar.customer_id
		 ORDER BY cu.org_name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GapReceiptSite
	for rows.Next() {
		var s GapReceiptSite
		if err := rows.Scan(&s.CustomerID, &s.Name); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func gapReceiptsWhere(keywordID string, other bool, site string) (string, []any) {
	where := `NOT (` + knowledgeWorkSQL + `) AND ` + gapNoPublishedKBSQL
	var args []any
	if other {
		where += ` AND NOT EXISTS (SELECT 1 FROM as_keyword_links kl WHERE kl.as_id=ar.as_id)`
	} else {
		where += ` AND EXISTS (SELECT 1 FROM as_keyword_links kl WHERE kl.as_id=ar.as_id AND kl.keyword_id=?)`
		args = append(args, strings.TrimSpace(keywordID))
	}
	if site = strings.TrimSpace(site); site != "" {
		where += ` AND ar.customer_id=?`
		args = append(args, site)
	}
	return where, args
}

func gapReceiptsOrder(sort, dir string) string {
	col := "ar.receipt_datetime"
	switch sort {
	case "as_number":
		col = "ar.as_number"
	case "customer":
		col = "cu.org_name"
	case "symptom":
		col = "ar.symptom"
	}
	if dir != "asc" {
		dir = "desc"
	}
	return col + " " + dir + ", ar.as_id " + dir
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
			if row.Kind == "short" {
				other.ShortCount++
			} else {
				other.NoneCount++
			}
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
		if row.Kind == "short" {
			a.g.ShortCount++
		} else {
			a.g.NoneCount++
		}
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
