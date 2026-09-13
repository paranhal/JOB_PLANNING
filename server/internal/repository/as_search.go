package repository

import (
	"database/sql"
	"html"
	"html/template"
	"log"
	"sort"
	"strings"
	"unicode/utf8"

	"customer-support/internal/model"
)

const asSearchCreateSQL = `CREATE VIRTUAL TABLE as_search USING fts5(
  as_id UNINDEXED, symptom, action, cause_detail, conclusion,
  tokenize='trigram'
)`

const asSearchSelectSQL = `
	SELECT ar.as_id,
	       COALESCE(ar.symptom,''),
	       TRIM(COALESCE(ar.action_taken,'') || char(10) || COALESCE((
	         SELECT GROUP_CONCAT(TRIM(COALESCE(p.work_content,'') || ' ' || COALESCE(p.notes,'')), char(10))
	         FROM as_processes p WHERE p.as_id = ar.as_id
	       ), '')),
	       COALESCE(ar.cause_detail,''),
	       COALESCE(ar.conclusion,'')
	FROM as_receipts ar`

// applyASSearch FTS5+trigram 색인을 만든다. 안 되면 LIKE 로 간다. §12.11.3
func applyASSearch(db *sql.DB) {
	if db == nil {
		return
	}
	var ddl string
	_ = db.QueryRow(`SELECT sql FROM sqlite_master WHERE name='as_search'`).Scan(&ddl)
	if ddl != "" && !strings.Contains(strings.ToLower(ddl), "trigram") {
		if _, err := db.Exec(`DROP TABLE as_search`); err != nil {
			log.Printf("as_search drop(구 토크나이저): %v", err)
		}
		ddl = ""
	}
	if ddl == "" {
		if _, err := db.Exec(asSearchCreateSQL); err != nil {
			log.Printf("as_search: FTS5+trigram 없음, LIKE 로 검색한다 (§12.11.3): %v", err)
			return
		}
	}
	if err := RebuildASSearch(db); err != nil {
		log.Printf("as_search rebuild: %v", err)
	}
}

func asSearchHasFTS(db *sql.DB) bool {
	if db == nil {
		return false
	}
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='as_search'`).Scan(&n)
	return err == nil && n > 0
}

// RebuildASSearch 접수·조치 본문을 FTS 색인에 다시 넣는다.
func RebuildASSearch(db *sql.DB) error {
	if !asSearchHasFTS(db) {
		return nil
	}
	if _, err := db.Exec(`DELETE FROM as_search`); err != nil {
		return err
	}
	_, err := db.Exec(`INSERT INTO as_search(as_id, symptom, action, cause_detail, conclusion)` + asSearchSelectSQL)
	return err
}

func reindexASSearch(db *sql.DB, asID string) {
	asID = strings.TrimSpace(asID)
	if db == nil || asID == "" || !asSearchHasFTS(db) {
		return
	}
	_, _ = db.Exec(`DELETE FROM as_search WHERE as_id=?`, asID)
	_, _ = db.Exec(`INSERT INTO as_search(as_id, symptom, action, cause_detail, conclusion)`+
		asSearchSelectSQL+` WHERE ar.as_id=?`, asID)
}

func deleteASSearch(db *sql.DB, asID string) {
	if db == nil || asID == "" || !asSearchHasFTS(db) {
		return
	}
	_, _ = db.Exec(`DELETE FROM as_search WHERE as_id=?`, asID)
}

func (r *ASRepo) SearchUsesFTS() bool {
	if r == nil {
		return false
	}
	return asSearchHasFTS(r.db)
}

func searchPage(f model.ASSearchFilter) (page, size, offset int) {
	page = f.Page
	if page < 1 {
		page = 1
	}
	size = f.PageSize
	if size < 1 {
		size = 20
	}
	if size > 2000 {
		size = 2000
	}
	return page, size, (page - 1) * size
}

func appendASSearchFilters(where string, args []interface{}, f model.ASSearchFilter) (string, []interface{}) {
	// §4.5 기준일·data_origin 필터를 넣지 않는다. 검색은 옛 자료를 찾는다.
	if s := strings.TrimSpace(f.DateFrom); s != "" {
		where += ` AND date(ar.receipt_datetime) >= date(?)`
		args = append(args, s)
	}
	if s := strings.TrimSpace(f.DateTo); s != "" {
		where += ` AND date(ar.receipt_datetime) <= date(?)`
		args = append(args, s)
	}
	if s := strings.TrimSpace(f.CustomerID); s != "" {
		where += ` AND ar.customer_id = ?`
		args = append(args, s)
	}
	if s := strings.TrimSpace(f.Product); s != "" {
		where += ` AND COALESCE(a.product_name,'') = ?`
		args = append(args, s)
	}
	if s := strings.TrimSpace(f.CauseType); s != "" {
		where += ` AND ar.cause_type = ?`
		args = append(args, s)
	}
	if s := strings.TrimSpace(f.ProcessType); s != "" {
		where += ` AND ar.process_type = ?`
		args = append(args, s)
	}
	if s := strings.TrimSpace(f.Assigned); s != "" {
		frag, a := assigneeMatchSQL(AssigneeKindAS, "ar", s)
		where += frag
		args = append(args, a...)
	}
	if s := strings.TrimSpace(f.KeywordID); s != "" {
		where += ` AND EXISTS (SELECT 1 FROM as_keyword_links l WHERE l.as_id=ar.as_id AND l.keyword_id=?)`
		args = append(args, s)
	}
	if s := strings.TrimSpace(f.CauseCat); s != "" {
		where += ` AND (ar.cause_cat1 = ? OR ar.cause_cat2 = ? OR ar.cause_cat3 = ?)`
		args = append(args, s, s, s)
	}
	if f.RequireAction {
		where += ` AND` + similarHasWorkSQL()
	}
	return where, args
}

const asSearchHitSQL = `
		SELECT ar.as_id, ar.as_number, c.org_name, date(ar.receipt_datetime),
		       COALESCE(ar.symptom,''),
		       COALESCE(ar.action_taken,''),
		       COALESCE((
		         SELECT p.work_content FROM as_processes p
		          WHERE p.as_id = ar.as_id AND TRIM(COALESCE(p.work_content,'')) != ''
		          ORDER BY p.process_datetime DESC LIMIT 1
		       ), ''),
		       COALESCE(ct.code_name, ar.cause_type, ''),
		       COALESCE(ar.assigned_to,''),
		       COALESCE(date(ar.complete_datetime), ''),
		       COALESCE(ar.customer_id,''),
		       COALESCE(kv.n, 0)
		FROM as_receipts ar
		JOIN customers c ON c.customer_id = ar.customer_id
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		LEFT JOIN codes ct ON ct.code_group='cause_type' AND ct.code_value = ar.cause_type
		LEFT JOIN (SELECT as_id, COUNT(*) AS n FROM as_case_votes GROUP BY as_id) kv ON kv.as_id = ar.as_id`

// SearchAS 증상·조치·원인·결론·처리이력을 찾는다. 기준일 없음. §12.11.4
func (r *ASRepo) SearchAS(f model.ASSearchFilter) ([]model.ASSearchHit, int, error) {
	q := strings.TrimSpace(f.Query)
	kw := strings.TrimSpace(f.KeywordID)
	if q == "" && kw == "" {
		return nil, 0, nil
	}
	_, size, offset := searchPage(f)
	useFTS := q != "" && asSearchHasFTS(r.db) && utf8.RuneCountInString(q) >= 3
	var items []model.ASSearchHit
	var total int
	var err error
	if useFTS {
		items, total, err = r.searchASFTS(f, q, size, offset)
		if err != nil {
			log.Printf("as_search FTS: %v — LIKE 로 재시도", err)
			useFTS = false
		}
	}
	if !useFTS {
		items, total, err = r.searchASLike(f, q, size, offset)
	}
	if err != nil {
		return nil, 0, err
	}
	for i := range items {
		decorateSearchHit(&items[i], q)
	}
	r.attachHitPeople(items)
	return items, total, nil
}

// MatchIDs 제목·기관·본문 검색에 쓰는 접수 ID. §12.11 FTS5+trigram, 없으면 LIKE.
func (r *ASRepo) MatchIDs(q string) map[string]bool {
	out := map[string]bool{}
	q = strings.TrimSpace(q)
	if q == "" || r == nil || r.db == nil {
		return out
	}
	var rows *sql.Rows
	var err error
	if asSearchHasFTS(r.db) && utf8.RuneCountInString(q) >= 3 {
		rows, err = r.db.Query(`SELECT as_id FROM as_search WHERE as_search MATCH ?`, fts5Query(q))
		if err != nil {
			log.Printf("as_search MatchIDs FTS: %v — LIKE", err)
			rows = nil
		}
	}
	if rows == nil {
		like := "%" + q + "%"
		rows, err = r.db.Query(`
			SELECT ar.as_id FROM as_receipts ar
			JOIN customers c ON c.customer_id = ar.customer_id
			WHERE ar.as_number LIKE ? OR c.org_name LIKE ? OR ar.symptom LIKE ? OR ar.action_taken LIKE ?
			   OR ar.cause_detail LIKE ? OR ar.conclusion LIKE ?
			   OR EXISTS (SELECT 1 FROM as_processes p WHERE p.as_id = ar.as_id
			              AND (p.work_content LIKE ? OR p.notes LIKE ?))`,
			like, like, like, like, like, like, like, like)
		if err != nil {
			return out
		}
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil && id != "" {
			out[id] = true
		}
	}
	return out
}

func decorateSearchHit(h *model.ASSearchHit, q string) {
	actionSrc := strings.TrimSpace(h.Action)
	if actionSrc == "" {
		h.HasAction = false
		h.Action = model.ASActionMissing
		h.ActionHTML = template.HTML(html.EscapeString(model.ASActionMissing))
	} else {
		h.HasAction = true
		h.ActionHTML = template.HTML(highlightExcerpt(actionSrc, q, 120))
	}
	h.SymptomHTML = template.HTML(highlightExcerpt(h.Symptom, q, 120))
	h.MatchSymptom = textHasQuery(h.Symptom, q)
	h.MatchAction = h.HasAction && textHasQuery(actionSrc, q)
	h.SymptomLong = utf8.RuneCountInString(compactSpace(h.Symptom)) > 90
}

func (r *ASRepo) searchASFTS(f model.ASSearchFilter, q string, size, offset int) ([]model.ASSearchHit, int, error) {
	match := fts5Query(q)
	like := "%" + q + "%"
	where := ` WHERE (as_search MATCH ? OR ar.as_number LIKE ? OR c.org_name LIKE ?)`
	args := []interface{}{match, like, like}
	where, args = appendASSearchFilters(where, args, f)

	countQ := `SELECT COUNT(*) FROM as_search
		JOIN as_receipts ar ON ar.as_id = as_search.as_id
		JOIN customers c ON c.customer_id = ar.customer_id
		LEFT JOIN assets a ON a.asset_id = ar.asset_id` + where
	var total int
	if err := r.db.QueryRow(countQ, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	order := ` ORDER BY COALESCE(kv.n,0) DESC, bm25(as_search), ar.receipt_datetime DESC`
	if strings.TrimSpace(f.Sort) == "newest" {
		order = ` ORDER BY COALESCE(kv.n,0) DESC, ar.receipt_datetime DESC`
	}
	listQ := asSearchHitSQL + `
		JOIN as_search ON as_search.as_id = ar.as_id` + where + order + ` LIMIT ? OFFSET ?`
	listArgs := append(append([]interface{}{}, args...), size, offset)
	rows, err := r.db.Query(listQ, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items, err := scanASSearchHits(rows)
	return items, total, err
}

func (r *ASRepo) searchASLike(f model.ASSearchFilter, q string, size, offset int) ([]model.ASSearchHit, int, error) {
	where := ` WHERE 1=1`
	args := []interface{}{}
	if q != "" {
		like := "%" + q + "%"
		where += ` AND (
		ar.as_number LIKE ? OR c.org_name LIKE ? OR ar.symptom LIKE ? OR ar.action_taken LIKE ?
		OR ar.cause_detail LIKE ? OR ar.conclusion LIKE ?
		OR EXISTS (SELECT 1 FROM as_processes p WHERE p.as_id = ar.as_id
		           AND (p.work_content LIKE ? OR p.notes LIKE ?))
	)`
		args = append(args, like, like, like, like, like, like, like, like)
	}
	where, args = appendASSearchFilters(where, args, f)

	countQ := `SELECT COUNT(*) FROM as_receipts ar
		JOIN customers c ON c.customer_id = ar.customer_id
		LEFT JOIN assets a ON a.asset_id = ar.asset_id` + where
	var total int
	if err := r.db.QueryRow(countQ, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := ` ORDER BY COALESCE(kv.n,0) DESC, ar.receipt_datetime DESC`
	listQ := asSearchHitSQL + where + order + ` LIMIT ? OFFSET ?`
	listArgs := append(append([]interface{}{}, args...), size, offset)
	rows, err := r.db.Query(listQ, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items, err := scanASSearchHits(rows)
	return items, total, err
}

func scanASSearchHits(rows *sql.Rows) ([]model.ASSearchHit, error) {
	var items []model.ASSearchHit
	for rows.Next() {
		var h model.ASSearchHit
		var actionTaken, lastWork string
		if err := rows.Scan(&h.ASID, &h.ASNumber, &h.OrgName, &h.ReceiptDate,
			&h.Symptom, &actionTaken, &lastWork, &h.CauseName,
			&h.AssignedTo, &h.CompleteDate, &h.CustomerID, &h.VoteCount); err != nil {
			return nil, err
		}
		h.Action = strings.TrimSpace(actionTaken)
		if h.Action == "" {
			h.Action = strings.TrimSpace(lastWork)
		}
		items = append(items, h)
	}
	return items, rows.Err()
}

func fts5Query(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return `""`
	}
	parts := strings.Fields(q)
	if len(parts) == 0 {
		parts = []string{q}
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, `"`+strings.ReplaceAll(p, `"`, `""`)+`"`)
	}
	if len(out) == 0 {
		return `""`
	}
	return strings.Join(out, " ")
}

// SimilarCases 접수 증상과 비슷한 과거 건. 같은 사이트 → 관련도 → 최신순. §41.2
func (r *ASRepo) SimilarCases(f model.ASSimilarFilter) ([]model.ASSimilarCase, error) {
	q := strings.TrimSpace(f.Query)
	if utf8.RuneCountInString(q) < 2 {
		return nil, nil
	}
	limit := f.Limit
	if limit < 1 {
		limit = 5
	}
	if limit > 5 {
		limit = 5
	}
	fetch := limit * 2
	if fetch > 16 {
		fetch = 16
	}
	key := similarQueryKey(q)
	product := ""
	if aid := strings.TrimSpace(f.AssetID); aid != "" {
		_ = r.db.QueryRow(`SELECT COALESCE(product_name,'') FROM assets WHERE asset_id=?`, aid).Scan(&product)
	}

	var items []model.ASSimilarCase
	var err error
	if asSearchHasFTS(r.db) && utf8.RuneCountInString(key) >= 2 {
		items, err = r.similarFTS(f, key, product, fetch)
		if err != nil {
			log.Printf("as similar FTS: %v — LIKE", err)
			items, err = r.similarLike(f, key, product, fetch)
		}
	} else {
		items, err = r.similarLike(f, key, product, fetch)
	}
	if err != nil {
		return nil, err
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func similarQueryKey(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return q
	}
	if strings.Contains(q, " ") {
		r := []rune(q)
		if len(r) > 80 {
			return string(r[:80])
		}
		return q
	}
	r := []rune(q)
	if len(r) > 12 {
		return string(r[:12])
	}
	return string(r)
}

const similarWorkMinSQL = `LENGTH(TRIM(COALESCE(p.work_content,''))) >= 10`

func similarHasWorkSQL() string {
	return ` EXISTS (SELECT 1 FROM as_processes p WHERE p.as_id=ar.as_id AND ` + similarWorkMinSQL + `)`
}

func similarFTSMatch(key string) string {
	parts := strings.Fields(strings.TrimSpace(key))
	if len(parts) == 0 {
		return fts5Query(key)
	}
	quoted := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if utf8.RuneCountInString(p) < 2 {
			continue
		}
		quoted = append(quoted, `"`+strings.ReplaceAll(p, `"`, `""`)+`"`)
	}
	if len(quoted) == 0 {
		return fts5Query(key)
	}
	return strings.Join(quoted, " OR ")
}

func (r *ASRepo) similarFTS(f model.ASSimilarFilter, key, product string, limit int) ([]model.ASSimilarCase, error) {
	match := similarFTSMatch(key)
	where := ` WHERE as_search MATCH ? AND ` + similarHasWorkSQL()
	args := []interface{}{match}
	if x := strings.TrimSpace(f.ExcludeID); x != "" {
		where += ` AND ar.as_id <> ?`
		args = append(args, x)
	}
	q := similarSelectSQL() + `
		JOIN as_search ON as_search.as_id = ar.as_id` + where + similarOrderSQL(true) + ` LIMIT ?`
	args = append(similarWeightArgs(f), args...)
	args = append(args, limit)
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSimilarCases(rows, f, product)
}

func (r *ASRepo) similarLike(f model.ASSimilarFilter, key, product string, limit int) ([]model.ASSimilarCase, error) {
	parts := strings.Fields(strings.TrimSpace(key))
	if len(parts) == 0 {
		parts = []string{key}
	}
	var ors []string
	var args []interface{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if utf8.RuneCountInString(p) < 2 {
			continue
		}
		like := "%" + p + "%"
		ors = append(ors, `(ar.symptom LIKE ? OR ar.action_taken LIKE ?
			OR EXISTS (SELECT 1 FROM as_processes p WHERE p.as_id=ar.as_id AND p.work_content LIKE ?))`)
		args = append(args, like, like, like)
	}
	if len(ors) == 0 {
		like := "%" + strings.TrimSpace(key) + "%"
		ors = []string{`(ar.symptom LIKE ? OR ar.action_taken LIKE ?
			OR EXISTS (SELECT 1 FROM as_processes p WHERE p.as_id=ar.as_id AND p.work_content LIKE ?))`}
		args = []interface{}{like, like, like}
	}
	where := ` WHERE ` + similarHasWorkSQL() + ` AND (` + strings.Join(ors, " OR ") + `)`
	if x := strings.TrimSpace(f.ExcludeID); x != "" {
		where += ` AND ar.as_id <> ?`
		args = append(args, x)
	}
	q := similarSelectSQL() + where + similarOrderSQL(false) + ` LIMIT ?`
	args = append(similarWeightArgs(f), args...)
	args = append(args, limit)
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSimilarCases(rows, f, product)
}

func similarWeightArgs(f model.ASSimilarFilter) []interface{} {
	cust := strings.TrimSpace(f.CustomerID)
	return []interface{}{cust, cust}
}

func similarSelectSQL() string {
	return `
		SELECT ar.as_id, ar.as_number, c.org_name,
		       date(ar.receipt_datetime),
		       COALESCE(date(ar.complete_datetime), ''),
		       COALESCE(ar.symptom,''),
		       COALESCE((
		         SELECT p.work_content FROM as_processes p
		          WHERE p.as_id = ar.as_id AND ` + similarWorkMinSQL + `
		          ORDER BY p.process_datetime DESC LIMIT 1
		       ), ''),
		       ar.status, COALESCE(ar.asset_id,''), ar.customer_id, COALESCE(a.product_name,''),
		       CASE
		         WHEN ? != '' AND ar.customer_id = ? THEN 0
		         ELSE 1
		       END AS w,
		       (SELECT COUNT(*) FROM as_case_votes v WHERE v.as_id = ar.as_id) AS votes,
		       COALESCE((
		         SELECT p.worker FROM as_processes p
		          WHERE p.as_id = ar.as_id AND ` + similarWorkMinSQL + `
		          ORDER BY p.process_datetime DESC LIMIT 1
		       ), ''),
		       COALESCE((
		         SELECT p.process_datetime FROM as_processes p
		          WHERE p.as_id = ar.as_id AND ` + similarWorkMinSQL + `
		          ORDER BY p.process_datetime DESC LIMIT 1
		       ), '')
		FROM as_receipts ar
		JOIN customers c ON c.customer_id = ar.customer_id
		LEFT JOIN assets a ON a.asset_id = ar.asset_id`
}

func similarOrderSQL(fts bool) string {
	if fts {
		return ` ORDER BY votes DESC, w ASC, bm25(as_search), ar.receipt_datetime DESC`
	}
	return ` ORDER BY votes DESC, w ASC, ar.receipt_datetime DESC`
}

func scanSimilarCases(rows *sql.Rows, f model.ASSimilarFilter, product string) ([]model.ASSimilarCase, error) {
	var items []model.ASSimilarCase
	for rows.Next() {
		var it model.ASSimilarCase
		var symptom, lastWork, assetID, customerID, prodName string
		var w, votes int
		var worker, procAt string
		if err := rows.Scan(&it.ASID, &it.ASNumber, &it.OrgName,
			&it.ReceiptDate, &it.CompleteDate, &symptom, &lastWork,
			&it.Status, &assetID, &customerID, &prodName, &w, &votes, &worker, &procAt); err != nil {
			return nil, err
		}
		it.VoteCount = votes
		it.AuthorName = model.DisplayPerson(worker)
		it.AuthorDate, it.AuthorFull = model.KnowledgeWhenString(procAt)
		it.ProcessDate = it.CompleteDate
		if it.ProcessDate == "" {
			it.ProcessDate = it.ReceiptDate
		}
		it.SymptomSummary = truncateRunes(compactSpace(symptom), 80)
		action := strings.TrimSpace(lastWork)
		if utf8.RuneCountInString(action) < 10 {
			continue
		}
		it.HasAction = true
		it.ActionSummary = truncateRunes(compactSpace(action), 80)
		it.SameAsset = strings.TrimSpace(f.AssetID) != "" && assetID == f.AssetID
		it.SameCustomer = strings.TrimSpace(f.CustomerID) != "" && customerID == f.CustomerID
		it.SameProduct = strings.TrimSpace(product) != "" && prodName == product
		it.CanReopen = it.SameAsset && model.CanReopenAS(it.Status)
		if it.SameCustomer {
			it.WeightLabel = "같은 사이트"
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func (r *ASRepo) SearchProductNames() ([]string, error) {
	rows, err := r.db.Query(`
		SELECT DISTINCT a.product_name FROM assets a
		JOIN as_receipts ar ON ar.asset_id = a.asset_id
		WHERE TRIM(COALESCE(a.product_name,'')) != ''
		ORDER BY a.product_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

const knowledgeWorkSQL = `EXISTS (SELECT 1 FROM as_processes p WHERE p.as_id=ar.as_id AND LENGTH(TRIM(COALESCE(p.work_content,''))) >= 10)`

// KnowledgeCorpus 조치 있는 건 / 전체 / 원인분류된 건. 숫자는 화면이 한계를 말할 때 쓴다. §41.1.2
func (r *ASRepo) KnowledgeCorpus() (withAction, total, classified int, err error) {
	err = r.db.QueryRow(`SELECT COUNT(*) FROM as_receipts ar`).Scan(&total)
	if err != nil {
		return
	}
	err = r.db.QueryRow(`SELECT COUNT(*) FROM as_receipts ar WHERE ` + knowledgeWorkSQL).Scan(&withAction)
	if err != nil {
		return
	}
	err = r.db.QueryRow(`
		SELECT COUNT(*) FROM as_receipts ar
		 WHERE TRIM(COALESCE(ar.cause_cat1,'')) != ''
		    OR TRIM(COALESCE(ar.cause_cat2,'')) != ''
		    OR TRIM(COALESCE(ar.cause_cat3,'')) != ''`).Scan(&classified)
	return
}

// KnowledgeSites 사이트 목록. assets 를 거치지 않는다. §41.1.1
func (r *ASRepo) KnowledgeSites() ([]model.ASKnowledgeSite, error) {
	rows, err := r.db.Query(`
		SELECT ar.customer_id, c.org_name, COUNT(*)
		  FROM as_receipts ar
		  JOIN customers c ON c.customer_id = ar.customer_id
		 WHERE TRIM(COALESCE(ar.customer_id,'')) != ''
		 GROUP BY ar.customer_id
		 ORDER BY c.org_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ASKnowledgeSite
	for rows.Next() {
		var s model.ASKnowledgeSite
		if err := rows.Scan(&s.CustomerID, &s.OrgName, &s.Count); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// KnowledgeSiteCases 한 사이트의 접수. 최신순. ar.customer_id.
func (r *ASRepo) KnowledgeSiteCases(customerID string, limit int) ([]model.ASSearchHit, error) {
	customerID = strings.TrimSpace(customerID)
	if customerID == "" {
		return nil, nil
	}
	if limit < 1 || limit > 2000 {
		limit = 200
	}
	q := asSearchHitSQL + `
		WHERE ar.customer_id = ?
		ORDER BY ar.receipt_datetime DESC
		LIMIT ?`
	rows, err := r.db.Query(q, customerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanASSearchHits(rows)
	if err != nil {
		return nil, err
	}
	for i := range items {
		decorateSearchHit(&items[i], "")
	}
	r.attachHitPeople(items)
	return items, nil
}

func DecorateKnowledgeHits(items []model.ASSearchHit, q string) {
	for i := range items {
		items[i].SymptomHTML = template.HTML(highlightFull(items[i].Symptom, q))
		if items[i].HasAction {
			items[i].ActionHTML = template.HTML(highlightFull(items[i].Action, q))
		}
		items[i].MatchSymptom = textHasQuery(items[i].Symptom, q)
		items[i].MatchAction = items[i].HasAction && textHasQuery(items[i].Action, q)
		items[i].SymptomLong = utf8.RuneCountInString(compactSpace(items[i].Symptom)) > 90
	}
}

func highlightFull(text, query string) string {
	text = compactSpace(text)
	if text == "" {
		return ""
	}
	return highlightTokens(html.EscapeString(text), query)
}

func textHasQuery(text, query string) bool {
	text = strings.ToLower(compactSpace(text))
	if text == "" {
		return false
	}
	for _, t := range searchTokens(query) {
		if t == "" {
			continue
		}
		if strings.Contains(text, strings.ToLower(t)) {
			return true
		}
	}
	return false
}

func highlightExcerpt(text, query string, maxRunes int) string {
	text = compactSpace(text)
	if text == "" {
		return ""
	}
	ex := excerptAround(text, query, maxRunes)
	escaped := html.EscapeString(ex)
	return highlightTokens(escaped, query)
}

func highlightTokens(escaped, query string) string {
	toks := searchTokens(query)
	sort.Slice(toks, func(i, j int) bool {
		return utf8.RuneCountInString(toks[i]) > utf8.RuneCountInString(toks[j])
	})
	for _, t := range toks {
		et := html.EscapeString(t)
		if et == "" {
			continue
		}
		escaped = strings.ReplaceAll(escaped, et, "<mark>"+et+"</mark>")
	}
	return escaped
}

func searchTokens(query string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	add(query)
	for _, p := range strings.Fields(query) {
		add(p)
	}
	return out
}

func excerptAround(text, query string, maxRunes int) string {
	if maxRunes < 24 {
		maxRunes = 80
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	idx := -1
	for _, t := range searchTokens(query) {
		idx = strings.Index(text, t)
		if idx >= 0 {
			break
		}
	}
	if idx < 0 {
		return string(runes[:maxRunes]) + "…"
	}
	start := utf8.RuneCountInString(text[:idx])
	half := maxRunes / 2
	from := start - half
	if from < 0 {
		from = 0
	}
	to := from + maxRunes
	if to > len(runes) {
		to = len(runes)
		from = to - maxRunes
		if from < 0 {
			from = 0
		}
	}
	s := string(runes[from:to])
	if from > 0 {
		s = "…" + s
	}
	if to < len(runes) {
		s += "…"
	}
	return s
}

func compactSpace(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\n", " ")), " ")
}
