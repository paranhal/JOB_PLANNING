package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"customer-support/internal/audit"
	"customer-support/internal/model"
)

// 95-226: 하부 작업 부하량(%) · 진행률(자동 3모드 + 수동) · 일자별 부분 완료 · 연기(대기 업무).

var (
	ErrLoadValue   = errors.New("부하량은 0 이상의 숫자로 입력하세요")
	ErrLoadSum     = errors.New("부하량 합계는 100%를 넘을 수 없습니다")
	ErrLoadSites   = errors.New("사이트가 있는 대상은 부하량을 쓰지 않습니다(사이트 수로 계산)")
	ErrDayRange    = errors.New("업무 기간 안의 날짜를 고르세요")
	ErrDonePct     = errors.New("완료 비율은 0~100 사이로 입력하세요")
	ErrManualPct   = errors.New("진행률은 0~100 사이로 입력하세요")
	ErrNotDefer    = errors.New("연기할 수 없는 업무입니다(이미 완료·취소했거나 대기 중입니다)")
	ErrNotDeferred = errors.New("대기 중인 업무가 아닙니다")
)

const DeferNoDateReason = "연기(대기)"

func applyLoadProgressDeferV119(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := leaveLedgerSQL("119_load_progress_defer.sql")
	if err != nil {
		log.Printf("119 load_progress_defer: %v", err)
		return
	}
	if err := execWorkGroupSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("119 load_progress_defer apply: %v", err)
	}
}

// ParseLoad 입력 문자열을 부하량으로. 빈 값은 nil(자동 균등 분할), 음수·비숫자는 오류.
func ParseLoad(s string) (*float64, error) {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%"))
	if s == "" {
		return nil, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 100 {
		return nil, ErrLoadValue
	}
	return &v, nil
}

// ValidateLoads 합계가 100 을 넘으면 거부한다(100 미만은 통과: 안내만).
func ValidateLoads(loads []*float64) error {
	sum := 0.0
	for _, l := range loads {
		if l != nil {
			if *l < 0 {
				return ErrLoadValue
			}
			sum += *l
		}
	}
	if sum > 100.0001 {
		return ErrLoadSum
	}
	return nil
}

// EffectiveLoads 입력이 없는 칸은 남은 비율을 균등 분할한다. 모두 비어 있으면 100/n 씩.
func EffectiveLoads(loads []*float64) []float64 {
	out := make([]float64, len(loads))
	sum, unset := 0.0, 0
	for _, l := range loads {
		if l != nil {
			sum += *l
		} else {
			unset++
		}
	}
	rest := 100 - sum
	if rest < 0 {
		rest = 0
	}
	for i, l := range loads {
		if l != nil {
			out[i] = *l
		} else if unset > 0 {
			out[i] = rest / float64(unset)
		}
	}
	return out
}

// LoadRow 부하량 입력 화면 한 줄.
type LoadRow struct {
	ID        string
	Name      string
	Given     *float64
	Effective float64
	HasSites  bool
}

func (r *WBRepo) packageHasSites(pkgID string) bool {
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM package_sites WHERE package_id=?`, pkgID).Scan(&n)
	return n > 0
}

func (r *WBRepo) taskHasSites(taskID string) bool {
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM work_task_sites WHERE task_id=?`, taskID).Scan(&n)
	return n > 0
}

// PhaseLoadView 한 단계의 하부 업무 부하량 현황(사이트가 있는 하부 업무는 부하량 없음).
func (r *WBRepo) PhaseLoadView(phaseID string) []LoadRow {
	rows, err := r.db.Query(`SELECT package_id, name, load_pct FROM phase_packages WHERE phase_id=? AND COALESCE(is_active,1)=1 ORDER BY seq, package_id`, phaseID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []LoadRow
	for rows.Next() {
		var lr LoadRow
		var l sql.NullFloat64
		if rows.Scan(&lr.ID, &lr.Name, &l) != nil {
			continue
		}
		if l.Valid {
			v := l.Float64
			lr.Given = &v
		}
		out = append(out, lr)
	}
	for i := range out {
		out[i].HasSites = r.packageHasSites(out[i].ID)
	}
	fillEffective(out)
	return out
}

// SubtaskLoadView 한 업무의 하위 업무 부하량 현황.
func (r *WBRepo) SubtaskLoadView(parentID string) []LoadRow {
	rows, err := r.db.Query(`SELECT task_id, title, load_pct FROM work_tasks WHERE parent_task_id=? AND status!='cancelled' ORDER BY COALESCE(NULLIF(work_date,''),'9999'), task_id`, parentID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []LoadRow
	for rows.Next() {
		var lr LoadRow
		var l sql.NullFloat64
		if rows.Scan(&lr.ID, &lr.Name, &l) != nil {
			continue
		}
		if l.Valid {
			v := l.Float64
			lr.Given = &v
		}
		out = append(out, lr)
	}
	for i := range out {
		out[i].HasSites = r.taskHasSites(out[i].ID)
	}
	fillEffective(out)
	return out
}

func fillEffective(rows []LoadRow) {
	var idx []int
	var loads []*float64
	for i, lr := range rows {
		if lr.HasSites {
			continue
		}
		idx = append(idx, i)
		loads = append(loads, lr.Given)
	}
	eff := EffectiveLoads(loads)
	for k, i := range idx {
		rows[i].Effective = eff[k]
	}
}

// SavePackageLoads 한 단계의 하부 업무 부하량을 저장한다. loads 에 없는 하부 업무는 현재 값을 유지한다.
// 합계 100 초과·음수·비숫자·사이트가 있는 대상은 거부하고 아무것도 저장하지 않는다.
func (r *WBRepo) SavePackageLoads(phaseID string, loads map[string]*float64, actor string) error {
	view := r.PhaseLoadView(phaseID)
	return r.saveLoads(view, loads, actor, "phase_packages", "package_id", "하부 업무 부하량")
}

// SaveSubtaskLoads 한 업무의 하위 업무 부하량을 저장한다.
func (r *WBRepo) SaveSubtaskLoads(parentID string, loads map[string]*float64, actor string) error {
	view := r.SubtaskLoadView(parentID)
	return r.saveLoads(view, loads, actor, "work_tasks", "task_id", "하위 업무 부하량")
}

func (r *WBRepo) saveLoads(view []LoadRow, in map[string]*float64, actor, table, pk, label string) error {
	next := map[string]*float64{}
	var all []*float64
	for _, lr := range view {
		v := lr.Given
		if nv, ok := in[lr.ID]; ok {
			if nv != nil && lr.HasSites {
				return ErrLoadSites
			}
			v = nv
		}
		next[lr.ID] = v
		if !lr.HasSites {
			all = append(all, v)
		}
	}
	for id := range in {
		if _, ok := next[id]; !ok {
			return ErrLoadValue
		}
	}
	if err := ValidateLoads(all); err != nil {
		return err
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, lr := range view {
		v := next[lr.ID]
		if equalLoad(v, lr.Given) {
			continue
		}
		var arg interface{}
		if v != nil {
			arg = *v
		}
		if _, err := tx.Exec(`UPDATE `+table+` SET load_pct=? WHERE `+pk+`=?`, arg, lr.ID); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	audit.Use(r.db)
	for _, lr := range view {
		if !equalLoad(next[lr.ID], lr.Given) {
			audit.LogWithReason(audit.ActionUpdate, table, pk, lr.ID, lr.Name, fmtLoad(lr.Given), fmtLoad(next[lr.ID]), label+" 변경 by "+actor)
		}
	}
	return nil
}

func equalLoad(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return math.Abs(*a-*b) < 0.0001
}

func fmtLoad(v *float64) string {
	if v == nil {
		return "자동"
	}
	return strconv.FormatFloat(*v, 'f', -1, 64) + "%"
}

// ───────── 진행률 ─────────

// ProgressInfo 진행률 표시값. 자동값과 수동값을 같이 주고, 수동이 있으면 수동을 먼저 보인다.
type ProgressInfo struct {
	Mode      string   // sites | subtasks | days | single
	Auto      float64  // 자동 계산값(0~100)
	Manual    *float64 // 수동 입력값
	Shown     float64
	Source    string // auto | manual
	Done      int
	Total     int
	Remaining int
}

func clampPct(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return math.Round(v*10) / 10
}

// DayProgress 한 날의 부하량·완료 비율.
type DayProgress struct {
	Date      string
	Load      *float64
	Effective float64
	Done      float64
	Note      string
}

func (r *WBRepo) taskRange(taskID string) (work, due, status string, manual sql.NullFloat64, err error) {
	err = r.db.QueryRow(`SELECT COALESCE(work_date,''), COALESCE(due_date,''), COALESCE(status,''), progress_manual FROM work_tasks WHERE task_id=?`, taskID).
		Scan(&work, &due, &status, &manual)
	return
}

// ListDays 업무 기간의 날짜마다 부하량·완료 비율(없는 날은 자동 균등·0%). 기간이 하루이거나 날짜가 없으면 비어 있다.
func (r *WBRepo) ListDays(taskID string) []DayProgress {
	work, due, _, _, err := r.taskRange(taskID)
	if err != nil || work == "" || due == "" || due <= work {
		return nil
	}
	dates := model.DatesInclusive(work, due)
	if len(dates) < 2 {
		return nil
	}
	saved := map[string]DayProgress{}
	rows, err := r.db.Query(`SELECT work_date, load_pct, done_pct, note FROM work_task_day_progress WHERE task_id=?`, taskID)
	if err == nil {
		for rows.Next() {
			var d DayProgress
			var l sql.NullFloat64
			if rows.Scan(&d.Date, &l, &d.Done, &d.Note) == nil {
				if l.Valid {
					v := l.Float64
					d.Load = &v
				}
				saved[d.Date] = d
			}
		}
		rows.Close()
	}
	out := make([]DayProgress, 0, len(dates))
	loads := make([]*float64, 0, len(dates))
	for _, dt := range dates {
		d := saved[dt]
		d.Date = dt
		out = append(out, d)
		loads = append(loads, d.Load)
	}
	eff := EffectiveLoads(loads)
	for i := range out {
		out[i].Effective = eff[i]
	}
	return out
}

// ComputeProgress 업무의 진행률(자동 3모드 + 수동). 남은 건수는 업무에 저장해 다음 날 이어서 보인다.
//   - 사이트가 있는 업무: 완료 사이트 / 전체 사이트
//   - 하위 업무가 있는 업무: Σ(완료한 하위 업무의 부하량)
//   - 여러 날 단일 업무: Σ(일자 부하량 × 완료 비율)
//   - 그 밖: 완료 100 · 미완료 0(기존 진행 입력값이 있으면 그 값)
func (r *WBRepo) ComputeProgress(taskID string) ProgressInfo {
	var p ProgressInfo
	work, due, status, manual, err := r.taskRange(taskID)
	if err != nil {
		return p
	}
	_ = work
	_ = due
	switch {
	case r.taskHasSites(taskID):
		p.Mode = "sites"
		_ = r.db.QueryRow(`SELECT COUNT(*) FROM work_task_sites WHERE task_id=?`, taskID).Scan(&p.Total)
		_ = r.db.QueryRow(`SELECT COUNT(DISTINCT customer_id) FROM work_task_site_progress WHERE task_id=? AND status='done'`, taskID).Scan(&p.Done)
		if p.Total > 0 {
			p.Auto = clampPct(float64(p.Done) * 100 / float64(p.Total))
		}
		p.Remaining = p.Total - p.Done
	default:
		kids := r.SubtaskLoadView(taskID)
		if len(kids) > 0 {
			p.Mode = "subtasks"
			p.Total = len(kids)
			sum := 0.0
			for _, k := range kids {
				var st string
				_ = r.db.QueryRow(`SELECT COALESCE(status,'') FROM work_tasks WHERE task_id=?`, k.ID).Scan(&st)
				if st == model.WBTaskComplete {
					p.Done++
					sum += k.Effective
				}
			}
			p.Auto = clampPct(sum)
			p.Remaining = p.Total - p.Done
			break
		}
		if days := r.ListDays(taskID); len(days) > 0 {
			p.Mode = "days"
			p.Total = len(days)
			sum := 0.0
			for _, d := range days {
				sum += d.Effective * d.Done / 100
				if d.Done >= 100 {
					p.Done++
				}
			}
			p.Auto = clampPct(sum)
			p.Remaining = p.Total - p.Done
			break
		}
		p.Mode = "single"
		p.Total = 1
		if status == model.WBTaskComplete {
			p.Auto, p.Done = 100, 1
		} else {
			p.Remaining = 1
		}
	}
	if status == model.WBTaskComplete {
		p.Auto = 100
	}
	if manual.Valid {
		v := clampPct(manual.Float64)
		p.Manual = &v
		p.Shown, p.Source = v, "manual"
	} else {
		p.Shown, p.Source = p.Auto, "auto"
	}
	_, _ = r.db.Exec(`UPDATE work_tasks SET remain_count=? WHERE task_id=? AND COALESCE(remain_count,-1)<>?`, p.Remaining, taskID, p.Remaining)
	return p
}

// SetManualProgress 수동 진행률을 정하거나(nil 이면 해제) 바꾼다. 변경 이력을 남긴다.
func (r *WBRepo) SetManualProgress(taskID string, pct *float64, actor string) error {
	if pct != nil && (*pct < 0 || *pct > 100 || math.IsNaN(*pct)) {
		return ErrManualPct
	}
	var before sql.NullFloat64
	if err := r.db.QueryRow(`SELECT progress_manual FROM work_tasks WHERE task_id=?`, taskID).Scan(&before); err != nil {
		return fmt.Errorf("업무가 없습니다")
	}
	var arg interface{}
	if pct != nil {
		arg = *pct
	}
	if _, err := r.db.Exec(`UPDATE work_tasks SET progress_manual=?, updated_at=datetime('now','localtime') WHERE task_id=?`, arg, taskID); err != nil {
		return err
	}
	b := "자동"
	if before.Valid {
		b = strconv.FormatFloat(before.Float64, 'f', -1, 64) + "%"
	}
	a := "자동"
	if pct != nil {
		a = strconv.FormatFloat(*pct, 'f', -1, 64) + "%"
	}
	audit.Use(r.db)
	audit.LogWithReason(audit.ActionUpdate, "work_tasks", "task_id", taskID, "수동 진행률", b, a, "수동 진행률 변경 by "+actor+": "+b+" → "+a)
	return nil
}

// ───────── 일자별 부분 완료 ─────────

// SaveDayProgress 한 날의 부하량·완료 비율을 저장한다. 모든 날이 100% 가 되면 업무를 완료하고 complete_date 를 그 날로 둔다.
func (r *WBRepo) SaveDayProgress(taskID, workDate string, load *float64, donePct float64, note, actor string) (completed bool, err error) {
	if donePct < 0 || donePct > 100 || math.IsNaN(donePct) {
		return false, ErrDonePct
	}
	days := r.ListDays(taskID)
	if len(days) == 0 {
		return false, ErrDayRange
	}
	found := false
	loads := make([]*float64, 0, len(days))
	for _, d := range days {
		l := d.Load
		if d.Date == workDate {
			found = true
			l = load
		}
		loads = append(loads, l)
	}
	if !found {
		return false, ErrDayRange
	}
	if err := ValidateLoads(loads); err != nil {
		return false, err
	}
	var larg interface{}
	if load != nil {
		larg = *load
	}
	if _, err := r.db.Exec(`INSERT INTO work_task_day_progress(task_id,work_date,load_pct,done_pct,note,done_by,updated_at) VALUES (?,?,?,?,?,?,?)
		ON CONFLICT(task_id,work_date) DO UPDATE SET load_pct=excluded.load_pct, done_pct=excluded.done_pct, note=excluded.note, done_by=excluded.done_by, updated_at=excluded.updated_at`,
		taskID, workDate, larg, donePct, strings.TrimSpace(note), actor, nowStr()); err != nil {
		return false, err
	}
	all := true
	for _, d := range r.ListDays(taskID) {
		if d.Done < 100 {
			all = false
		}
	}
	if all {
		return true, r.completeFromDays(taskID, workDate, actor)
	}
	_, _ = r.db.Exec(`UPDATE work_tasks SET status=CASE WHEN status='waiting' THEN 'in_progress' ELSE status END, updated_at=datetime('now','localtime') WHERE task_id=?`, taskID)
	return false, nil
}

func (r *WBRepo) completeFromDays(taskID, date, actor string) error {
	if _, err := r.db.Exec(`UPDATE work_tasks SET status='complete', progress=100, complete_date=?, updated_at=datetime('now','localtime') WHERE task_id=?`, date, taskID); err != nil {
		return err
	}
	audit.Use(r.db)
	audit.LogWithReason(audit.ActionUpdate, "work_tasks", "task_id", taskID, "일자별 부분 완료", `{"status":"open"}`, `{"status":"complete"}`, "마지막 부분 완료로 업무 완료(완료일 "+date+") by "+actor)
	return nil
}

// CompleteMultiDay 여러 날 업무에서 완료를 누를 때의 선택(95-223 #3).
//   - "today": date 까지의 일정만 완료로 하고 남은 날짜는 이어서 진행한다(전부 끝났으면 업무 완료).
//   - "all"  : 이후 일정을 제외하고 업무를 완료한다(종료일을 date 로 줄이고 complete_date=date).
func (r *WBRepo) CompleteMultiDay(taskID, mode, date, actor string) (completed bool, err error) {
	days := r.ListDays(taskID)
	if len(days) == 0 {
		return false, ErrDayRange
	}
	in := false
	for _, d := range days {
		if d.Date == date {
			in = true
		}
	}
	if !in {
		return false, ErrDayRange
	}
	switch mode {
	case "today":
		for _, d := range days {
			if d.Date <= date && d.Done < 100 {
				if _, e := r.SaveDayProgress(taskID, d.Date, d.Load, 100, d.Note, actor); e != nil {
					return false, e
				}
			}
		}
		var st string
		_ = r.db.QueryRow(`SELECT status FROM work_tasks WHERE task_id=?`, taskID).Scan(&st)
		return st == model.WBTaskComplete, nil
	case "all":
		if _, err := r.db.Exec(`DELETE FROM work_task_day_progress WHERE task_id=? AND work_date>?`, taskID, date); err != nil {
			return false, err
		}
		if _, err := r.db.Exec(`UPDATE work_tasks SET due_date=?, status='complete', progress=100, complete_date=?, updated_at=datetime('now','localtime') WHERE task_id=?`, date, date, taskID); err != nil {
			return false, err
		}
		audit.Use(r.db)
		audit.LogWithReason(audit.ActionUpdate, "work_tasks", "task_id", taskID, "다일 업무 전체 완료", `{"status":"open"}`, `{"status":"complete"}`, "전체 완료(이후 일정 제외, 종료일 "+date+") by "+actor)
		return true, nil
	}
	return false, fmt.Errorf("완료 방식을 선택하세요")
}

// ───────── 연기(대기 업무) ─────────

// DeferTask 업무를 연기한다: 날짜를 비우고 waiting 상태를 유지하며 미정 사유를 「연기(대기)」로 기록한다.
// 지연 사유를 받지 않고, 지연일수 통계(work_task_delays)에도 넣지 않는다(기존 이월과 별개).
func (r *WBRepo) DeferTask(taskID, actor string) error {
	var st, work, due string
	if err := r.db.QueryRow(`SELECT COALESCE(status,''), COALESCE(work_date,''), COALESCE(due_date,'') FROM work_tasks WHERE task_id=?`, taskID).Scan(&st, &work, &due); err != nil {
		return fmt.Errorf("업무가 없습니다")
	}
	if st == model.WBTaskComplete || st == "cancelled" {
		return ErrNotDefer
	}
	var already string
	_ = r.db.QueryRow(`SELECT COALESCE(deferred_at,'') FROM work_tasks WHERE task_id=?`, taskID).Scan(&already)
	if already != "" {
		return ErrNotDefer
	}
	from := work
	if from == "" {
		from = due
	}
	if _, err := r.db.Exec(`UPDATE work_tasks SET work_date='', due_date='', start_time='', end_time='', status='waiting',
		no_date_reason=?, deferred_at=?, defer_from_date=?, from_deferred=0, updated_at=datetime('now','localtime') WHERE task_id=?`,
		DeferNoDateReason, nowStr(), from, taskID); err != nil {
		return err
	}
	audit.Use(r.db)
	audit.LogWithReason(audit.ActionUpdate, "work_tasks", "task_id", taskID, "연기(대기)", fmt.Sprintf(`{"work_date":%q}`, work), `{"work_date":""}`, "연기: 대기 업무로 이동 by "+actor)
	return nil
}

// UndeferTask 대기 업무를 지정한 날의 업무로 등록한다. 그날 끝나도 미완료면 ReturnUnfinishedDeferred 가 다시 대기로 돌린다.
func (r *WBRepo) UndeferTask(taskID, date, start, end, actor string) error {
	date = strings.TrimSpace(date)
	if err := model.RequireAppDateYear(date); err != nil {
		return err
	}
	var deferred string
	if err := r.db.QueryRow(`SELECT COALESCE(deferred_at,'') FROM work_tasks WHERE task_id=?`, taskID).Scan(&deferred); err != nil || deferred == "" {
		return ErrNotDeferred
	}
	if _, err := r.db.Exec(`UPDATE work_tasks SET work_date=?, due_date=?, start_time=?, end_time=?, status='waiting',
		no_date_reason='', deferred_at='', from_deferred=1, updated_at=datetime('now','localtime') WHERE task_id=?`, date, date, start, end, taskID); err != nil {
		return err
	}
	audit.Use(r.db)
	audit.LogWithReason(audit.ActionUpdate, "work_tasks", "task_id", taskID, "대기 → 오늘 업무", `{"deferred":true}`, fmt.Sprintf(`{"work_date":%q}`, date), "대기 업무를 "+date+" 업무로 등록 by "+actor)
	return nil
}

// ReturnUnfinishedDeferred 대기에서 가져왔지만 그 날짜가 지나도록 끝나지 않은 업무를 다시 대기로 돌린다. 돌린 건수.
func (r *WBRepo) ReturnUnfinishedDeferred(today string) int {
	rows, err := r.db.Query(`SELECT task_id FROM work_tasks WHERE from_deferred=1 AND COALESCE(deferred_at,'')='' AND status NOT IN ('complete','cancelled')
		AND COALESCE(NULLIF(due_date,''), work_date)<>'' AND COALESCE(NULLIF(due_date,''), work_date)<?`, today)
	if err != nil {
		return 0
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	n := 0
	for _, id := range ids {
		if err := r.DeferTask(id, "system"); err == nil {
			n++
		}
	}
	return n
}

// DeferredItem 대기 업무 한 건(대기 그룹 카드용).
type DeferredItem struct {
	TaskID      string
	Title       string
	ProjectID   string
	ProjectName string
	CategoryKey string
	FromDate    string
	DeferredAt  string
	Remaining   int
	Assignee    string
}

// Heading 사업명 또는 업무 구분 이름(그룹 제목).
func (d DeferredItem) Heading() string {
	if d.ProjectName != "" {
		return d.ProjectName
	}
	if l := model.WeeklyActivityLabel(d.CategoryKey); l != "" {
		return l
	}
	return "(사업·구분 없음)"
}

// ListDeferred 대기 업무. 보이는 범위는 userID 기준: 등록자·담당·참여·공동 작업 범위·수락한 배정만(관리자급은 orgID 전체).
func (r *WBRepo) ListDeferred(orgID, userID string, wide bool) []DeferredItem {
	q := `SELECT t.task_id, t.title, COALESCE(t.project_id,''), COALESCE(p.name,''), COALESCE(t.activity_kind,''),
	             COALESCE(t.defer_from_date,''), COALESCE(t.deferred_at,''), COALESCE(t.remain_count,0), COALESCE(t.assignee,'')
	        FROM work_tasks t LEFT JOIN work_projects p ON p.project_id=t.project_id
	       WHERE COALESCE(t.deferred_at,'')<>'' AND t.status NOT IN ('complete','cancelled')`
	var args []interface{}
	if orgID = strings.TrimSpace(orgID); orgID != "" && orgID != OrgAll {
		q += ` AND t.org_id=?`
		args = append(args, orgID)
	}
	if !wide {
		q += ` AND (t.created_by_user_id=? OR t.assignee_user_id=?
		        OR EXISTS (SELECT 1 FROM work_task_members m WHERE m.task_id=t.task_id AND m.user_id=?)
		        OR EXISTS (SELECT 1 FROM work_collab_members cm WHERE cm.task_id=t.task_id AND cm.user_id=?)
		        OR t.collab_scope='org'
		        OR EXISTS (SELECT 1 FROM work_assignments a WHERE a.task_id=t.task_id AND a.to_user_id=? AND a.status='accepted'))`
		args = append(args, userID, userID, userID, userID, userID)
	}
	q += ` ORDER BY COALESCE(NULLIF(p.name,''), t.activity_kind), t.deferred_at DESC`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []DeferredItem
	for rows.Next() {
		var d DeferredItem
		if rows.Scan(&d.TaskID, &d.Title, &d.ProjectID, &d.ProjectName, &d.CategoryKey, &d.FromDate, &d.DeferredAt, &d.Remaining, &d.Assignee) == nil {
			out = append(out, d)
		}
	}
	return out
}

var _ = time.Now

// PackagePhase 하부 업무가 속한 단계 ID.
func (r *WBRepo) PackagePhase(pkgID string) string {
	var ph string
	_ = r.db.QueryRow(`SELECT phase_id FROM phase_packages WHERE package_id=?`, pkgID).Scan(&ph)
	return ph
}
