package repository

import (
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"customer-support/internal/model"
)

// 시드 SQL 은 migrations/021_as_keywords.sql 과 같다. 목록은 표(SQL)가 출처다.
const asKeywordSeedSQL = `
CREATE TABLE IF NOT EXISTS as_keywords (
  keyword_id TEXT PRIMARY KEY,
  keyword    TEXT NOT NULL,
  kw_group   TEXT NOT NULL,
  synonyms   TEXT NOT NULL DEFAULT '',
  is_active  INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  note       TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS as_keyword_links (
  as_id      TEXT NOT NULL,
  keyword_id TEXT NOT NULL,
  source     TEXT NOT NULL,
  field      TEXT NOT NULL,
  PRIMARY KEY (as_id, keyword_id, field)
);
CREATE TABLE IF NOT EXISTS as_keyword_stopwords (
  word TEXT PRIMARY KEY,
  note TEXT NOT NULL DEFAULT ''
);
`

const asKeywordInsertSQL = `
INSERT OR IGNORE INTO as_keywords(keyword_id, keyword, kw_group, synonyms, is_active, sort_order, note) VALUES
('KW001','홈페이지','대상','누리집',1,10,''),
('KW002','전자도서관','대상','',1,20,''),
('KW003','자료관리시스템','대상','KLAS',1,30,''),
('KW004','상호대차','대상','책이음',1,40,''),
('KW005','무인예약','대상','',1,50,''),
('KW006','자가대출반납기','대상','',1,60,''),
('KW007','키오스크','대상','',1,70,''),
('KW008','서버','대상','',1,80,''),
('KW009','WAS','대상','',1,90,''),
('KW010','DB','대상','',1,100,''),
('KW011','방화벽','대상','',1,110,''),
('KW012','오류','증상','',1,10,''),
('KW013','장애','증상','',1,20,''),
('KW014','접속불가','증상','',1,30,''),
('KW015','팝업','증상','',1,40,''),
('KW016','검색안됨','증상','',1,50,''),
('KW017','로그인','증상','',1,60,''),
('KW018','속도지연','증상','',1,70,''),
('KW019','출력','증상','',1,80,''),
('KW020','인식불가','증상','',1,90,''),
('KW021','패치누락','원인','',1,10,''),
('KW022','설정오류','원인','',1,20,''),
('KW023','네트워크','원인','',1,30,''),
('KW024','전원','원인','',1,40,''),
('KW025','소모품','원인','',1,50,''),
('KW026','사용자오류','원인','',1,60,''),
('KW027','데이터오류','원인','',1,70,''),
('KW028','재기동','조치','재시작',1,10,''),
('KW029','패치','조치','',1,20,''),
('KW030','설정변경','조치','',1,30,''),
('KW031','교체','조치','',1,40,''),
('KW032','원격지원','조치','',1,50,''),
('KW033','방문','조치','',1,60,''),
('KW034','자료송부','조치','',1,70,''),
('KW035','안내','조치','',1,80,''),
('KW036','이관','조치','',1,90,'');
`

const asKeywordStopwordSQL = `
INSERT OR IGNORE INTO as_keyword_stopwords(word, note) VALUES
('안녕하세요','인사말'),
('안녕하십니까','인사말'),
('감사합니다','인사말'),
('고맙습니다','인사말'),
('부탁드립니다','인사말'),
('부탁드리겠습니다','인사말'),
('수고하세요','인사말'),
('수고하셨습니다','인사말'),
('확인부탁드립니다','인사말'),
('드립니다','인사말'),
('입니다','조사·어미'),
('습니다','조사·어미'),
('합니다','조사·어미'),
('됩니다','조사·어미'),
('해주세요','조사·어미'),
('그리고','접속'),
('또는','접속'),
('kr','URL 조각'),
('go','URL 조각'),
('com','URL 조각'),
('www','URL 조각'),
('http','URL 조각'),
('https','URL 조각'),
('co','URL 조각'),
('ne','URL 조각'),
('ac','URL 조각'),
('041','전화번호 조각'),
('044','전화번호 조각'),
('02','전화번호 조각'),
('010','전화번호 조각');
`

func applyASKeywords(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(asKeywordSeedSQL); err != nil {
		log.Printf("021 as_keywords schema: %v", err)
		return
	}
	if _, err := db.Exec(asKeywordInsertSQL); err != nil {
		log.Printf("021 as_keywords seed: %v", err)
	}
	if _, err := db.Exec(asKeywordStopwordSQL); err != nil {
		log.Printf("021 as_keyword_stopwords: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO id_sequences(seq_key, last_no) VALUES ('as_keyword', 36)
		ON CONFLICT(seq_key) DO UPDATE SET last_no=excluded.last_no
		WHERE id_sequences.last_no < excluded.last_no`); err != nil {
		log.Printf("021 as_keyword seq: %v", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_as_keyword_links_kw ON as_keyword_links(keyword_id, as_id)`); err != nil {
		log.Printf("051 idx_as_keyword_links_kw: %v", err)
	}
}

// seedKeywordLinksIfNeeded 덮은 비율이 절반 미만이면 전체를 한 번 채운다. 기동마다 다시 만들지 않는다. §41.17.1.1
func seedKeywordLinksIfNeeded(db *sql.DB) {
	if db == nil {
		return
	}
	var links, receipts, kws, covered int
	_ = db.QueryRow(`SELECT COUNT(*) FROM as_keyword_links`).Scan(&links)
	_ = db.QueryRow(`SELECT COUNT(*) FROM as_receipts`).Scan(&receipts)
	_ = db.QueryRow(`SELECT COUNT(*) FROM as_keywords WHERE is_active=1`).Scan(&kws)
	_ = db.QueryRow(`SELECT COUNT(DISTINCT as_id) FROM as_keyword_links`).Scan(&covered)

	if receipts == 0 || kws == 0 {
		log.Printf("as_keyword_links 시드 건너뜀: 접수 %d건 · 활성 키워드 %d개", receipts, kws)
		return
	}
	if covered*2 >= receipts {
		return
	}
	log.Printf("as_keyword_links 색인 부족: 접수 %d건 중 %d건만 덮음(링크 %d). 다시 만든다", receipts, covered, links)

	n, err := NewASRepo(db).RebuildKeywordLinks(nil)
	if err != nil {
		log.Printf("as_keyword_links 시드 실패: %v", err)
		return
	}
	if n == 0 {
		log.Printf("as_keyword_links 시드 경고: 접수 %d건 · 키워드 %d개인데 링크가 0건 만들어졌다. 매칭을 확인하라", receipts, kws)
		return
	}
	log.Printf("as_keyword_links 시드 완료: %d건", n)
}

type ASKeywordRepo struct{ db *sql.DB }

func NewASKeywordRepo(db *sql.DB) *ASKeywordRepo { return &ASKeywordRepo{db: db} }

func (r *ASKeywordRepo) List(activeOnly bool) ([]model.ASKeyword, error) {
	q := `SELECT k.keyword_id, k.keyword, k.kw_group, k.synonyms, k.is_active, k.sort_order, k.note,
	             (SELECT COUNT(*) FROM as_keyword_links l WHERE l.keyword_id=k.keyword_id) AS n
	      FROM as_keywords k`
	if activeOnly {
		q += ` WHERE k.is_active=1`
	}
	q += ` ORDER BY k.kw_group, k.sort_order, k.keyword`
	rows, err := r.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanASKeywords(rows)
}

func scanASKeywords(rows *sql.Rows) ([]model.ASKeyword, error) {
	var out []model.ASKeyword
	for rows.Next() {
		var k model.ASKeyword
		var active int
		if err := rows.Scan(&k.KeywordID, &k.Keyword, &k.Group, &k.Synonyms, &active, &k.SortOrder, &k.Note, &k.LinkCount); err != nil {
			return nil, err
		}
		k.IsActive = active == 1
		out = append(out, k)
	}
	return out, rows.Err()
}

func (r *ASKeywordRepo) Get(id string) (*model.ASKeyword, error) {
	var k model.ASKeyword
	var active int
	err := r.db.QueryRow(`SELECT keyword_id, keyword, kw_group, synonyms, is_active, sort_order, note,
		(SELECT COUNT(*) FROM as_keyword_links l WHERE l.keyword_id=as_keywords.keyword_id)
		FROM as_keywords WHERE keyword_id=?`, id).
		Scan(&k.KeywordID, &k.Keyword, &k.Group, &k.Synonyms, &active, &k.SortOrder, &k.Note, &k.LinkCount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	k.IsActive = active == 1
	return &k, nil
}

func (r *ASKeywordRepo) Create(k *model.ASKeyword) error {
	k.Keyword = strings.TrimSpace(k.Keyword)
	k.Group = strings.TrimSpace(k.Group)
	if k.Keyword == "" || !validKWGroup(k.Group) {
		return fmt.Errorf("keyword")
	}
	if r.isStopword(k.Keyword) {
		return fmt.Errorf("stopword")
	}
	n, err := NextSeq(r.db, "as_keyword")
	if err != nil {
		return err
	}
	k.KeywordID = fmt.Sprintf("KW%03d", n)
	_, err = r.db.Exec(`INSERT INTO as_keywords(keyword_id, keyword, kw_group, synonyms, is_active, sort_order, note)
		VALUES (?,?,?,?,1,?,?)`, k.KeywordID, k.Keyword, k.Group, strings.TrimSpace(k.Synonyms), k.SortOrder, strings.TrimSpace(k.Note))
	return err
}

func (r *ASKeywordRepo) Update(k *model.ASKeyword) error {
	k.Keyword = strings.TrimSpace(k.Keyword)
	k.Group = strings.TrimSpace(k.Group)
	if k.Keyword == "" || !validKWGroup(k.Group) {
		return fmt.Errorf("keyword")
	}
	active := 0
	if k.IsActive {
		active = 1
	}
	_, err := r.db.Exec(`UPDATE as_keywords SET keyword=?, kw_group=?, synonyms=?, is_active=?, sort_order=?, note=?
		WHERE keyword_id=?`, k.Keyword, k.Group, strings.TrimSpace(k.Synonyms), active, k.SortOrder, strings.TrimSpace(k.Note), k.KeywordID)
	return err
}

func (r *ASKeywordRepo) Delete(id string) error {
	if _, err := r.db.Exec(`DELETE FROM as_keyword_links WHERE keyword_id=?`, id); err != nil {
		return err
	}
	_, err := r.db.Exec(`DELETE FROM as_keywords WHERE keyword_id=?`, id)
	return err
}

func validKWGroup(g string) bool {
	for _, x := range model.KWGroups() {
		if x == g {
			return true
		}
	}
	return false
}

func (r *ASKeywordRepo) isStopword(word string) bool {
	word = strings.ToLower(strings.TrimSpace(word))
	if word == "" {
		return true
	}
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM as_keyword_stopwords WHERE lower(word)=?`, word).Scan(&n)
	return n > 0
}

func (r *ASKeywordRepo) Stopwords() (map[string]bool, error) {
	rows, err := r.db.Query(`SELECT word FROM as_keyword_stopwords`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var w string
		if err := rows.Scan(&w); err != nil {
			return nil, err
		}
		out[strings.ToLower(strings.TrimSpace(w))] = true
	}
	return out, rows.Err()
}

// Suggest 사전과 본문을 대조해 후보만 돌려 준다. 저장하지 않는다. §12.11.6
func (r *ASKeywordRepo) Suggest(text string) ([]model.ASKeyword, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	all, err := r.List(true)
	if err != nil {
		return nil, err
	}
	var out []model.ASKeyword
	for _, k := range all {
		if keywordMatchesText(text, k) {
			k.Suggested = true
			out = append(out, k)
		}
	}
	return out, nil
}

const maxSimilarQueryTerms = 8

// QueryFromSymptom 증상에서 사전 낱말·토큰을 뽑아 FTS 질의로 만든다. §41.2
func (r *ASKeywordRepo) QueryFromSymptom(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	seen := map[string]bool{}
	var parts []string
	add := func(term string) {
		term = strings.TrimSpace(term)
		if term == "" {
			return
		}
		low := strings.ToLower(term)
		if seen[low] || len(parts) >= maxSimilarQueryTerms {
			return
		}
		seen[low] = true
		parts = append(parts, term)
	}
	if sug, err := r.Suggest(text); err == nil {
		for _, k := range sug {
			add(k.Keyword)
		}
	}
	stop, _ := r.Stopwords()
	if stop == nil {
		stop = map[string]bool{}
	}
	for _, orig := range tokenizeASKeywordText(text) {
		tok := stripTrailingJosa(orig)
		lowOrig := strings.ToLower(orig)
		low := strings.ToLower(tok)
		if stop[lowOrig] || stop[low] || isNoiseToken(tok) {
			continue
		}
		add(tok)
	}
	return strings.Join(parts, " ")
}

// CountInTexts 사전 낱말이 본문에 몇 건 나오는지. 새 사전을 만들지 않는다. §41.3.2
func (r *ASKeywordRepo) CountInTexts(texts []string, limit int) []model.ASKeywordFreq {
	if limit < 1 {
		limit = 8
	}
	all, err := r.List(true)
	if err != nil || len(all) == 0 {
		return nil
	}
	type acc struct {
		word string
		n    int
	}
	var hits []acc
	for _, k := range all {
		n := 0
		for _, t := range texts {
			if keywordMatchesText(t, k) {
				n++
			}
		}
		if n > 0 {
			hits = append(hits, acc{k.Keyword, n})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].n == hits[j].n {
			return hits[i].word < hits[j].word
		}
		return hits[i].n > hits[j].n
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]model.ASKeywordFreq, len(hits))
	for i, h := range hits {
		out[i] = model.ASKeywordFreq{Keyword: h.word, Count: h.n}
	}
	return out
}

func keywordMatchesText(text string, k model.ASKeyword) bool {
	for _, term := range k.MatchTerms() {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		if matchKeywordTerm(text, term) {
			return true
		}
	}
	return false
}

func matchKeywordTerm(text, term string) bool {
	if utf8.RuneCountInString(term) <= 3 && isASCIIWord(term) {
		return containsWordFold(text, term)
	}
	return strings.Contains(strings.ToLower(text), strings.ToLower(term))
}

func isASCIIWord(s string) bool {
	for _, r := range s {
		if r > 127 || (!unicode.IsLetter(r) && !unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}

func containsWordFold(text, word string) bool {
	tl, wl := strings.ToLower(text), strings.ToLower(word)
	idx := 0
	for {
		i := strings.Index(tl[idx:], wl)
		if i < 0 {
			return false
		}
		i += idx
		leftOK := i == 0 || !isWordChar(rune(tl[i-1]))
		end := i + len(wl)
		rightOK := end >= len(tl) || !isWordChar(rune(tl[end]))
		if leftOK && rightOK {
			return true
		}
		idx = i + 1
	}
}

func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func (r *ASKeywordRepo) LinksByAS(asID string) ([]model.ASKeywordLink, error) {
	rows, err := r.db.Query(`
		SELECT l.as_id, l.keyword_id, l.source, l.field, k.keyword, k.kw_group
		FROM as_keyword_links l
		JOIN as_keywords k ON k.keyword_id=l.keyword_id
		WHERE l.as_id=?
		ORDER BY k.kw_group, k.sort_order, k.keyword`, asID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ASKeywordLink
	for rows.Next() {
		var l model.ASKeywordLink
		if err := rows.Scan(&l.ASID, &l.KeywordID, &l.Source, &l.Field, &l.Keyword, &l.Group); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ReplaceLinks 체크된 것만 붙인다. 체크 안 한 후보는 저장하지 않는다. §12.11.6
func (r *ASKeywordRepo) ReplaceLinks(asID, field string, checked, suggested []string) error {
	asID = strings.TrimSpace(asID)
	if asID == "" || (field != model.KWFieldSymptom && field != model.KWFieldAction) {
		return nil
	}
	sug := map[string]bool{}
	for _, id := range suggested {
		id = strings.TrimSpace(id)
		if id != "" {
			sug[id] = true
		}
	}
	seen := map[string]bool{}
	var ids []string
	for _, id := range checked {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if _, err := r.db.Exec(`DELETE FROM as_keyword_links WHERE as_id=? AND field=? AND source IN ('manual','auto')`, asID, field); err != nil {
		return err
	}
	for _, id := range ids {
		var active int
		err := r.db.QueryRow(`SELECT is_active FROM as_keywords WHERE keyword_id=?`, id).Scan(&active)
		if err != nil || active != 1 {
			continue
		}
		src := model.KWSourceManual
		if sug[id] {
			src = model.KWSourceAuto
		}
		if _, err := r.db.Exec(`INSERT INTO as_keyword_links(as_id, keyword_id, source, field) VALUES (?,?,?,?)`,
			asID, id, src, field); err != nil {
			return err
		}
	}
	return nil
}

// FrequencyCandidates 본문 빈도 상위. 불용어·숫자·URL 조각·기존 사전 단어는 뺀다. 자동 확정하지 않는다.
func (r *ASKeywordRepo) FrequencyCandidates(limit int) ([]model.ASKeywordCandidate, error) {
	if limit < 1 || limit > 80 {
		limit = 40
	}
	stop, err := r.Stopwords()
	if err != nil {
		return nil, err
	}
	known, err := r.knownTerms()
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	fields := map[string]string{}
	add := func(text, field string) {
		for _, orig := range tokenizeASKeywordText(text) {
			tok := stripTrailingJosa(orig)
			lowOrig := strings.ToLower(orig)
			low := strings.ToLower(tok)
			if stop[lowOrig] || stop[low] || known[low] || isNoiseToken(tok) {
				continue
			}
			counts[tok]++
			if _, ok := fields[tok]; !ok {
				fields[tok] = field
			}
		}
	}
	rows, err := r.db.Query(`SELECT COALESCE(symptom,''), COALESCE(action_taken,'') FROM as_receipts`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var symptom, action string
		if err := rows.Scan(&symptom, &action); err != nil {
			rows.Close()
			return nil, err
		}
		add(symptom, model.KWFieldSymptom)
		add(action, model.KWFieldAction)
	}
	rows.Close()
	prows, err := r.db.Query(`SELECT COALESCE(work_content,''), COALESCE(notes,'') FROM as_processes`)
	if err != nil {
		return nil, err
	}
	for prows.Next() {
		var w, n string
		if err := prows.Scan(&w, &n); err != nil {
			prows.Close()
			return nil, err
		}
		add(w+" "+n, model.KWFieldAction)
	}
	prows.Close()

	var out []model.ASKeywordCandidate
	for w, n := range counts {
		out = append(out, model.ASKeywordCandidate{Word: w, Count: n, Field: fields[w]})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Word < out[j].Word
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *ASKeywordRepo) knownTerms() (map[string]bool, error) {
	all, err := r.List(true)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, k := range all {
		for _, t := range k.MatchTerms() {
			out[strings.ToLower(strings.TrimSpace(t))] = true
		}
	}
	return out, nil
}

func isNoiseToken(s string) bool {
	if s == "" {
		return true
	}
	digits, letters := 0, 0
	for _, r := range s {
		if unicode.IsDigit(r) {
			digits++
		} else if unicode.IsLetter(r) {
			letters++
		}
	}
	if letters == 0 {
		return true
	}
	if utf8.RuneCountInString(s) < 2 {
		return true
	}
	if isASCIIWord(s) && utf8.RuneCountInString(s) < 3 {
		return true
	}
	return false
}

func tokenizeASKeywordText(s string) []string {
	var tokens []string
	var cur []rune
	flush := func() {
		if len(cur) == 0 {
			return
		}
		tok := string(cur)
		if tok != "" {
			tokens = append(tokens, tok)
		}
		cur = cur[:0]
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur = append(cur, r)
			continue
		}
		flush()
	}
	flush()
	return tokens
}

var hangulJosa = []string{"으로", "에서", "부터", "까지", "이나", "이나", "이", "가", "을", "를", "은", "는", "에", "의", "와", "과", "도", "로", "만"}

func stripTrailingJosa(s string) string {
	r := []rune(s)
	if len(r) < 3 {
		return s
	}
	for _, j := range hangulJosa {
		jr := []rune(j)
		if len(r) <= len(jr) {
			continue
		}
		ok := true
		for i := 0; i < len(jr); i++ {
			if r[len(r)-len(jr)+i] != jr[i] {
				ok = false
				break
			}
		}
		if ok {
			return string(r[:len(r)-len(jr)])
		}
	}
	return s
}

// RebuildKeywordLinks 증상·조치 글을 키워드 사전으로 훑어 as_keyword_links 를 다시 만든다.
// asIDs 가 비면 전체를 다시 만든다. source='dict' 만 지운다. §41.16.6 · §41.17
func (r *ASRepo) RebuildKeywordLinks(asIDs []string) (int, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	ids := uniqueASKeywordIDs(asIDs)
	dict, err := NewASKeywordRepo(r.db).List(true)
	if err != nil {
		return 0, err
	}

	q := `SELECT ar.as_id, COALESCE(ar.symptom,''),
		TRIM(COALESCE(ar.action_taken,'') || char(10) || COALESCE((
			SELECT GROUP_CONCAT(TRIM(COALESCE(p.work_content,'') || char(10) || COALESCE(p.notes,'')), char(10))
			FROM as_processes p WHERE p.as_id = ar.as_id
		), ''))
		FROM as_receipts ar`
	var args []any
	if len(ids) > 0 {
		ph := strings.Repeat("?,", len(ids))
		q += ` WHERE ar.as_id IN (` + ph[:len(ph)-1] + `)`
		args = make([]any, len(ids))
		for i, id := range ids {
			args[i] = id
		}
	}
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return 0, err
	}
	type srcRow struct{ asID, symptom, action string }
	var src []srcRow
	for rows.Next() {
		var s srcRow
		if err := rows.Scan(&s.asID, &s.symptom, &s.action); err != nil {
			rows.Close()
			return 0, err
		}
		src = append(src, s)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if len(ids) == 0 {
		if _, err := tx.Exec(`DELETE FROM as_keyword_links WHERE source='dict'`); err != nil {
			return 0, err
		}
	} else {
		ph := strings.Repeat("?,", len(ids))
		delArgs := make([]any, len(ids))
		for i, id := range ids {
			delArgs[i] = id
		}
		if _, err := tx.Exec(`DELETE FROM as_keyword_links WHERE source='dict' AND as_id IN (`+ph[:len(ph)-1]+`)`, delArgs...); err != nil {
			return 0, err
		}
	}

	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO as_keyword_links(as_id, keyword_id, source, field) VALUES (?,?,?,?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	n := 0
	unmatched := 0
	for _, s := range src {
		hits := 0
		for _, k := range dictKeywordHits(dict, s.symptom) {
			if _, err := stmt.Exec(s.asID, k.KeywordID, model.KWSourceDict, model.KWFieldSymptom); err != nil {
				return 0, err
			}
			n++
			hits++
		}
		for _, k := range dictKeywordHits(dict, s.action) {
			if _, err := stmt.Exec(s.asID, k.KeywordID, model.KWSourceDict, model.KWFieldAction); err != nil {
				return 0, err
			}
			n++
			hits++
		}
		if hits == 0 {
			unmatched++
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	log.Printf("as_keyword_links: 링크 %d건 · 접수 %d건 중 %d건은 키워드 없음", n, len(src), unmatched)
	return n, nil
}

func uniqueASKeywordIDs(asIDs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range asIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// dictKeywordHits 사전 매칭. 대표 키워드(pickKeywordForSymptom)를 포함하고, 같은 글에 걸린 나머지도 모두 남긴다.
func dictKeywordHits(dict []model.ASKeyword, text string) []model.ASKeyword {
	text = strings.TrimSpace(text)
	if text == "" || len(dict) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []model.ASKeyword
	add := func(k *model.ASKeyword) {
		if k == nil || k.KeywordID == "" || seen[k.KeywordID] {
			return
		}
		seen[k.KeywordID] = true
		out = append(out, *k)
	}
	add(pickKeywordForSymptom(dict, text))
	for i := range dict {
		if keywordMatchesText(text, dict[i]) {
			add(&dict[i])
		}
	}
	return out
}
