package repository

import (
	"database/sql"
	"errors"
	"log"
	"sort"
	"strings"
	"time"

	"customer-support/internal/model"
)

var errKBActionRequired = errors.New("지식 본문이 비었습니다")

func applyASKbEntries(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS as_kb_entries (
			kb_id          TEXT PRIMARY KEY,
			as_id          TEXT,
			symptom_text   TEXT NOT NULL DEFAULT '',
			action_text    TEXT NOT NULL DEFAULT '',
			origin         TEXT NOT NULL DEFAULT 'added',
			rev            INTEGER NOT NULL DEFAULT 1,
			is_current     INTEGER NOT NULL DEFAULT 1,
			prev_kb_id     TEXT,
			author_id      TEXT NOT NULL DEFAULT '',
			author_name    TEXT NOT NULL DEFAULT '',
			created_at     TEXT NOT NULL DEFAULT '',
			change_note    TEXT NOT NULL DEFAULT '',
			status         TEXT NOT NULL DEFAULT 'published',
			helpful_count  INTEGER NOT NULL DEFAULT 0
		)`); err != nil {
		log.Printf("044 as_kb_entries: %v", err)
		return
	}
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_as_kb_current ON as_kb_entries(is_current, status)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_as_kb_as ON as_kb_entries(as_id)`)
}

type KBWrite struct {
	ASID       string
	ActionText string
	Symptom    string
	ChangeNote string
	AuthorID   string
	AuthorName string
}

func (r *ASRepo) ProcessWorkSnapshot(asID string) string {
	if r == nil || r.db == nil || strings.TrimSpace(asID) == "" {
		return ""
	}
	rows, err := r.db.Query(`SELECT process_id, COALESCE(work_content,'') FROM as_processes WHERE as_id=? ORDER BY process_id`, asID)
	if err != nil {
		return ""
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var id, c string
		if rows.Scan(&id, &c) == nil {
			b.WriteString(id)
			b.WriteByte('|')
			b.WriteString(c)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (r *ASRepo) LastActionByAS(asID string) (content, worker string, at time.Time) {
	asID = strings.TrimSpace(asID)
	if asID == "" || r == nil || r.db == nil {
		return "", "", time.Time{}
	}
	var dt string
	_ = r.db.QueryRow(`
		SELECT COALESCE(work_content,''), COALESCE(worker,''), COALESCE(process_datetime,'')
		FROM as_processes
		WHERE as_id=? AND TRIM(COALESCE(work_content,'')) != ''
		ORDER BY process_datetime DESC, process_id DESC LIMIT 1`, asID).Scan(&content, &worker, &dt)
	at = parseTime(dt)
	return strings.TrimSpace(content), worker, at
}

func (r *ASRepo) lastActionsByAS(ids []string) map[string]model.ASProcess {
	out := map[string]model.ASProcess{}
	clean := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			clean = append(clean, id)
		}
	}
	if len(clean) == 0 || r == nil || r.db == nil {
		return out
	}
	ph := make([]string, len(clean))
	args := make([]interface{}, len(clean))
	for i, id := range clean {
		ph[i] = "?"
		args[i] = id
	}
	rows, err := r.db.Query(`
		SELECT as_id, COALESCE(work_content,''), COALESCE(worker,''), COALESCE(process_datetime,'')
		FROM as_processes
		WHERE as_id IN (`+strings.Join(ph, ",")+`) AND TRIM(COALESCE(work_content,'')) != ''
		ORDER BY process_datetime DESC, process_id DESC`, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var p model.ASProcess
		var dt string
		if rows.Scan(&p.ASID, &p.WorkContent, &p.Worker, &dt) != nil {
			continue
		}
		if _, ok := out[p.ASID]; ok {
			continue
		}
		p.ProcessDatetime = parseTime(dt)
		out[p.ASID] = p
	}
	return out
}

func scanKB(s func(dest ...interface{}) error) (*model.ASKBEntry, error) {
	var e model.ASKBEntry
	var asID, prev sql.NullString
	var created string
	var cur, helpful int
	if err := s(&e.KBID, &asID, &e.SymptomText, &e.ActionText, &e.Origin, &e.Rev, &cur, &prev,
		&e.AuthorID, &e.AuthorName, &created, &e.ChangeNote, &e.Status, &helpful); err != nil {
		return nil, err
	}
	e.ASID = strings.TrimSpace(asID.String)
	e.PrevKBID = strings.TrimSpace(prev.String)
	e.IsCurrent = cur != 0
	e.HelpfulCount = helpful
	e.CreatedAt = parseTime(created)
	return &e, nil
}

const kbSelect = `
	SELECT kb_id, as_id, COALESCE(symptom_text,''), COALESCE(action_text,''), COALESCE(origin,''),
	       COALESCE(rev,1), COALESCE(is_current,0), prev_kb_id,
	       COALESCE(author_id,''), COALESCE(author_name,''), COALESCE(created_at,''),
	       COALESCE(change_note,''), COALESCE(status,''), COALESCE(helpful_count,0)
	FROM as_kb_entries`

func (r *ASRepo) GetKB(kbID string) (*model.ASKBEntry, error) {
	kbID = strings.TrimSpace(kbID)
	if kbID == "" {
		return nil, sql.ErrNoRows
	}
	e, err := scanKB(r.db.QueryRow(kbSelect+` WHERE kb_id=?`, kbID).Scan)
	if err != nil {
		return nil, err
	}
	return e, nil
}

func (r *ASRepo) CurrentKBByAS(asID string) *model.ASKBEntry {
	asID = strings.TrimSpace(asID)
	if asID == "" {
		return nil
	}
	e, err := scanKB(r.db.QueryRow(kbSelect+`
		WHERE as_id=? AND is_current=1 AND status=? ORDER BY rev DESC LIMIT 1`,
		asID, model.KBStatusPublished).Scan)
	if err != nil {
		return nil
	}
	return e
}

func (r *ASRepo) KBHistory(kbID string) []model.ASKBEntry {
	cur, err := r.GetKB(kbID)
	if err != nil || cur == nil {
		return nil
	}
	var out []model.ASKBEntry
	id := strings.TrimSpace(cur.PrevKBID)
	seen := map[string]bool{cur.KBID: true}
	for id != "" && !seen[id] {
		seen[id] = true
		e, err := r.GetKB(id)
		if err != nil || e == nil {
			break
		}
		out = append(out, *e)
		id = strings.TrimSpace(e.PrevKBID)
	}
	return out
}

func (r *ASRepo) PublishKB(in KBWrite) (*model.ASKBEntry, error) {
	in.ActionText = strings.TrimSpace(in.ActionText)
	if in.ActionText == "" {
		return nil, errKBActionRequired
	}
	in.ASID = strings.TrimSpace(in.ASID)
	if in.ASID != "" {
		if cur := r.CurrentKBByAS(in.ASID); cur != nil {
			return r.ReviseKB(cur.KBID, in)
		}
	}
	symptom := strings.TrimSpace(in.Symptom)
	orig := ""
	if in.ASID != "" {
		if as, err := r.GetByID(in.ASID); err == nil && as != nil {
			if symptom == "" {
				symptom = strings.TrimSpace(as.Symptom)
			}
		}
		orig, _, _ = r.LastActionByAS(in.ASID)
	}
	now := time.Now()
	e := &model.ASKBEntry{
		KBID:         newID("KB"),
		ASID:         in.ASID,
		SymptomText:  symptom,
		ActionText:   in.ActionText,
		Origin:       model.InferKBOrigin(orig, in.ActionText),
		Rev:          1,
		IsCurrent:    true,
		AuthorID:     strings.TrimSpace(in.AuthorID),
		AuthorName:   strings.TrimSpace(in.AuthorName),
		CreatedAt:    now,
		ChangeNote:   strings.TrimSpace(in.ChangeNote),
		Status:       model.KBStatusPublished,
	}
	_, err := r.db.Exec(`
		INSERT INTO as_kb_entries(
			kb_id, as_id, symptom_text, action_text, origin, rev, is_current, prev_kb_id,
			author_id, author_name, created_at, change_note, status, helpful_count)
		VALUES (?,?,?,?,?,?,1,NULL,?,?,?,?,?,0)`,
		e.KBID, nullStr(e.ASID), e.SymptomText, e.ActionText, e.Origin, e.Rev,
		e.AuthorID, e.AuthorName, now.Format("2006-01-02 15:04:05"), e.ChangeNote, e.Status)
	if err != nil {
		return nil, err
	}
	return e, nil
}

func (r *ASRepo) ReviseKB(kbID string, in KBWrite) (*model.ASKBEntry, error) {
	old, err := r.GetKB(kbID)
	if err != nil || old == nil {
		return nil, err
	}
	in.ActionText = strings.TrimSpace(in.ActionText)
	if in.ActionText == "" {
		in.ActionText = old.ActionText
	}
	orig := ""
	asID := strings.TrimSpace(old.ASID)
	if asID != "" {
		orig, _, _ = r.LastActionByAS(asID)
	}
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE as_kb_entries SET is_current=0 WHERE kb_id=?`, old.KBID); err != nil {
		return nil, err
	}
	if asID != "" {
		if _, err := tx.Exec(`UPDATE as_kb_entries SET is_current=0 WHERE as_id=? AND is_current=1`, asID); err != nil {
			return nil, err
		}
	}
	now := time.Now()
	e := &model.ASKBEntry{
		KBID:        newID("KB"),
		ASID:        asID,
		SymptomText: old.SymptomText,
		ActionText:  in.ActionText,
		Origin:      model.InferKBOrigin(orig, in.ActionText),
		Rev:         old.Rev + 1,
		IsCurrent:   true,
		PrevKBID:    old.KBID,
		AuthorID:    strings.TrimSpace(in.AuthorID),
		AuthorName:  strings.TrimSpace(in.AuthorName),
		CreatedAt:   now,
		ChangeNote:  strings.TrimSpace(in.ChangeNote),
		Status:      model.KBStatusPublished,
	}
	if _, err := tx.Exec(`
		INSERT INTO as_kb_entries(
			kb_id, as_id, symptom_text, action_text, origin, rev, is_current, prev_kb_id,
			author_id, author_name, created_at, change_note, status, helpful_count)
		VALUES (?,?,?,?,?,?,1,?,?,?,?,?,?,0)`,
		e.KBID, nullStr(e.ASID), e.SymptomText, e.ActionText, e.Origin, e.Rev, e.PrevKBID,
		e.AuthorID, e.AuthorName, now.Format("2006-01-02 15:04:05"), e.ChangeNote, e.Status); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return e, nil
}

func (r *ASRepo) ArchiveKB(kbID string) error {
	kbID = strings.TrimSpace(kbID)
	if kbID == "" {
		return sql.ErrNoRows
	}
	_, err := r.db.Exec(`UPDATE as_kb_entries SET status=?, is_current=0 WHERE kb_id=?`,
		model.KBStatusArchived, kbID)
	return err
}

func (r *ASRepo) listCurrentKB(q string) ([]model.ASKBEntry, error) {
	q = strings.TrimSpace(q)
	where := ` WHERE is_current=1 AND status=?`
	args := []interface{}{model.KBStatusPublished}
	if q != "" {
		like := "%" + q + "%"
		where += ` AND (symptom_text LIKE ? OR action_text LIKE ?)`
		args = append(args, like, like)
	}
	rows, err := r.db.Query(kbSelect+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ASKBEntry
	for rows.Next() {
		e, err := scanKB(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (r *ASRepo) currentKBForASIDs(ids []string) map[string]model.ASKBEntry {
	out := map[string]model.ASKBEntry{}
	clean := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			clean = append(clean, id)
		}
	}
	if len(clean) == 0 {
		return out
	}
	ph := make([]string, len(clean))
	args := make([]interface{}, 0, len(clean)+1)
	args = append(args, model.KBStatusPublished)
	for i, id := range clean {
		ph[i] = "?"
		args = append(args, id)
	}
	rows, err := r.db.Query(kbSelect+`
		WHERE is_current=1 AND status=? AND as_id IN (`+strings.Join(ph, ",")+`)`, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		e, err := scanKB(rows.Scan)
		if err != nil || e == nil || e.ASID == "" {
			continue
		}
		out[e.ASID] = *e
	}
	return out
}

func hitFromKB(e model.ASKBEntry, as *model.ASSearchHit) model.ASSearchHit {
	h := model.ASSearchHit{
		KBID:         e.KBID,
		IsKnowledge:  true,
		Origin:       e.Origin,
		OriginLabel:  model.KBOriginLabel(e.Origin),
		Rev:          e.Rev,
		RevLabel:     model.KBRevLabel(e.Rev),
		HistoryCount: e.Rev - 1,
		Symptom:      e.SymptomText,
		Action:       e.ActionText,
		HasAction:    strings.TrimSpace(e.ActionText) != "",
		ChangeNote:   e.ChangeNote,
		VoteCount:    e.HelpfulCount,
	}
	h.AuthorName = model.DisplayPerson(e.AuthorName)
	h.AuthorDate, h.AuthorFull = model.KnowledgeWhen(e.CreatedAt)
	if as != nil {
		h.ASID = as.ASID
		h.ASNumber = as.ASNumber
		h.OrgName = as.OrgName
		h.ReceiptDate = as.ReceiptDate
		h.CompleteDate = as.CompleteDate
		h.AssignedTo = as.AssignedTo
		h.CustomerID = as.CustomerID
		h.CauseName = as.CauseName
		if as.VoteCount > h.VoteCount {
			h.VoteCount = as.VoteCount
		}
		if strings.TrimSpace(h.Symptom) == "" {
			h.Symptom = as.Symptom
		}
	} else {
		h.ASID = e.ASID
		h.OrgName = model.KBDash
		h.LeadLabel = model.KBDash
		h.ReceiptDate = h.AuthorDate
	}
	return h
}

func applyHitAttribution(h *model.ASSearchHit, proc model.ASProcess) {
	if strings.TrimSpace(proc.WorkContent) != "" {
		h.OriginalAction = proc.WorkContent
		h.SourceName = model.DisplayPerson(proc.Worker)
		h.SourceDate, h.SourceFull = model.KnowledgeWhen(proc.ProcessDatetime)
	}
	if !h.IsKnowledge {
		h.AuthorName = model.DisplayPerson(proc.Worker)
		h.AuthorDate, h.AuthorFull = model.KnowledgeWhen(proc.ProcessDatetime)
		if h.HasAction && h.Origin == "" {
			h.Origin = model.KBOriginFromAction
			h.OriginLabel = model.KBOriginLabel(h.Origin)
		}
	}
	if h.AuthorName == "" {
		h.AuthorName = model.KBAuthorUnknown
	}
}

func (r *ASRepo) attachHitPeople(items []model.ASSearchHit) {
	ids := make([]string, 0, len(items))
	for i := range items {
		if items[i].ASID != "" {
			ids = append(ids, items[i].ASID)
		}
	}
	procs := r.lastActionsByAS(ids)
	for i := range items {
		p := procs[items[i].ASID]
		applyHitAttribution(&items[i], p)
		if items[i].IsKnowledge && items[i].Rev >= 2 && items[i].KBID != "" {
			items[i].Past = r.KBHistory(items[i].KBID)
			items[i].HistoryCount = len(items[i].Past)
			if items[i].HistoryCount < items[i].Rev-1 {
				items[i].HistoryCount = items[i].Rev - 1
			}
		}
	}
}

func kbMatchScore(h model.ASSearchHit) int {
	n := 0
	if h.MatchSymptom {
		n += 2
	}
	if h.MatchAction {
		n += 3
	}
	if h.Origin == model.KBOriginRevised {
		n += 1
	}
	return n
}

func kbRecency(h model.ASSearchHit) string {
	if h.AuthorFull != "" {
		return h.AuthorFull
	}
	return h.ReceiptDate
}

func originRank(h model.ASSearchHit) int {
	if h.Origin == model.KBOriginRevised {
		return 2
	}
	if h.IsKnowledge {
		return 1
	}
	return 0
}

func sortKnowledgeHits(items []model.ASSearchHit, newest bool) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.VoteCount != b.VoteCount {
			return a.VoteCount > b.VoteCount
		}
		if !newest {
			sa, sb := kbMatchScore(a), kbMatchScore(b)
			if sa != sb {
				return sa > sb
			}
		}
		ra, rb := originRank(a), originRank(b)
		if ra != rb {
			return ra > rb
		}
		return kbRecency(a) > kbRecency(b)
	})
}

// SearchKnowledge 원본과 현재 지식을 한 목록으로 합친다. SearchAS 결과는 그대로 둔다. §41.14
func (r *ASRepo) SearchKnowledge(f model.ASSearchFilter) ([]model.ASSearchHit, int, error) {
	q := strings.TrimSpace(f.Query)
	if q == "" {
		return nil, 0, nil
	}
	wide := f
	wide.Page = 1
	wide.PageSize = 400
	asHits, _, err := r.SearchAS(wide)
	if err != nil {
		return nil, 0, err
	}
	kbs, err := r.listCurrentKB(q)
	if err != nil {
		return nil, 0, err
	}
	asIDs := make([]string, 0, len(asHits)+len(kbs))
	for _, h := range asHits {
		asIDs = append(asIDs, h.ASID)
	}
	for _, e := range kbs {
		if e.ASID != "" {
			asIDs = append(asIDs, e.ASID)
		}
	}
	byAS := r.currentKBForASIDs(asIDs)
	asByID := map[string]model.ASSearchHit{}
	for _, h := range asHits {
		asByID[h.ASID] = h
	}

	var out []model.ASSearchHit
	seenAS := map[string]bool{}
	seenKB := map[string]bool{}

	for _, h := range asHits {
		if kb, ok := byAS[h.ASID]; ok {
			hit := hitFromKB(kb, &h)
			out = append(out, hit)
			seenAS[h.ASID] = true
			seenKB[kb.KBID] = true
			continue
		}
		h.IsKnowledge = false
		if h.HasAction {
			h.Origin = model.KBOriginFromAction
			h.OriginLabel = model.KBOriginLabel(h.Origin)
		}
		out = append(out, h)
		seenAS[h.ASID] = true
	}
	for _, e := range kbs {
		if seenKB[e.KBID] {
			continue
		}
		if e.ASID != "" && seenAS[e.ASID] {
			continue
		}
		var meta *model.ASSearchHit
		if e.ASID != "" {
			if h, ok := asByID[e.ASID]; ok {
				cp := h
				meta = &cp
			} else if as, err := r.GetByID(e.ASID); err == nil && as != nil {
				org := model.KBDash
				_ = r.db.QueryRow(`SELECT COALESCE(org_name,'') FROM customers WHERE customer_id=?`, as.CustomerID).Scan(&org)
				if org == "" {
					org = model.KBDash
				}
				meta = &model.ASSearchHit{
					ASID: as.ASID, ASNumber: as.ASNumber, OrgName: org,
					ReceiptDate: as.ReceiptDatetime.Format("2006-01-02"),
					Symptom:     as.Symptom, CustomerID: as.CustomerID,
					AssignedTo: as.AssignedTo,
				}
				if as.CompleteDatetime != nil {
					meta.CompleteDate = as.CompleteDatetime.Format("2006-01-02")
				}
			}
			if f.CustomerID != "" && (meta == nil || meta.CustomerID != f.CustomerID) {
				continue
			}
		} else if f.CustomerID != "" {
			continue
		}
		hit := hitFromKB(e, meta)
		out = append(out, hit)
		seenKB[e.KBID] = true
		if e.ASID != "" {
			seenAS[e.ASID] = true
		}
	}

	r.attachHitPeople(out)
	for i := range out {
		decorateSearchHit(&out[i], q)
		if out[i].IsKnowledge {
			out[i].ActionHTML = out[i].ActionHTML
			if out[i].Origin == "" {
				out[i].Origin = model.KBOriginAdded
				out[i].OriginLabel = model.KBOriginLabel(out[i].Origin)
			}
		}
	}
	DecorateKnowledgeHits(out, q)
	sortKnowledgeHits(out, strings.TrimSpace(f.Sort) == "newest")

	total := len(out)
	_, size, offset := searchPage(f)
	if offset > total {
		return nil, total, nil
	}
	end := offset + size
	if end > total {
		end = total
	}
	return out[offset:end], total, nil
}

func (r *ASRepo) AttachProcessAuthors(items []model.ASSearchHit) {
	r.attachHitPeople(items)
}
