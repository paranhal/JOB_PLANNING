package repository

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"customer-support/internal/model"
)

// 95-202: 「AS 접수·조치」 하단의 「자산 연결이 없는 작업」(AS·정기점검) 조회. 조회 전용이다.
//
// 「자산 연결 없음」 판정 SQL 은 아래 두 상수 한 곳에만 있다. 건수·목록·시험이 모두 같은 상수를 써서 어긋나지 않는다.
// 자산을 연결하면(조치 화면 설치자산 저장 등) 다음 조회부터 자동으로 빠진다.

// asUnlinkedWhere AS: 접수의 주 자산이 비어 있고(공백 포함), 해당 AS 업무(source_type='as')에도 work_task_assets 가 없다. 취소·테스트 제외.
const asUnlinkedWhere = `COALESCE(ar.is_test,0)=0 AND ar.status != 'cancelled'
	AND TRIM(COALESCE(ar.asset_id,''))=''
	AND NOT EXISTS (SELECT 1 FROM work_tasks w JOIN work_task_assets x ON x.task_id = w.task_id
	                 WHERE w.source_type='as' AND w.source_id = ar.as_id)`

// mntUnlinkedWhere 정기점검: maintenance_visit_targets 에 asset_id 가 있는 행이 없고, 해당 방문 업무(source_type='maintenance')에도 work_task_assets 가 없다. 테스트 제외. 완료된 방문도 포함한다.
const mntUnlinkedWhere = `COALESCE(v.is_test,0)=0
	AND NOT EXISTS (SELECT 1 FROM maintenance_visit_targets t WHERE t.visit_id = v.visit_id AND TRIM(COALESCE(t.asset_id,''))!='')
	AND NOT EXISTS (SELECT 1 FROM work_tasks w JOIN work_task_assets x ON x.task_id = w.task_id
	                 WHERE w.source_type='maintenance' AND w.source_id = v.visit_id)`

// UnlinkedFilter 목록·건수 공통 조건. Scope(조직·내 배정)는 건수와 목록 모두에 적용하고, 나머지는 목록에만 적용한다.
type UnlinkedFilter struct {
	OrgID      string
	MineUserID string
	MineKeys   []string
	From, To   string // YYYY-MM-DD (AS 는 접수일, 정기점검은 방문일)
	Status     string // all | open | done
	Q          string
	Product    string // 정기점검 제품(product_type) 일치
	// ExcludeGroup 「내 배정」이고 업무그룹 포함이 꺼져 있으면(/as 목록과 같은 규칙) 그룹 업무를 뺀다.
	ExcludeGroup bool
	Page         int
	PageSize     int
}

// UnlinkedAS AS 탭 한 줄.
type UnlinkedAS struct {
	ASID, ASNumber, CustomerID, Customer string
	ReceiptDate                          string
	Symptom, ActionTaken                 string
	AssignedTo, Status                   string
	CauseCat1, CauseCat1Custom           string
	ProjectName                          string
	DaysElapsed                          int
	Open                                 bool
	Candidates                           AssetCandidates
	ProductGuess                         string
	Title                                string
}

// UnlinkedMnt 정기점검 탭 한 줄.
type UnlinkedMnt struct {
	VisitID, CustomerID, Customer string
	VisitDate, CompletedDate      string
	ProductType, ProductLabel     string
	RoundNo, DurationMin          int
	Assignee, ProjectName, Notes  string
	Completed                     bool
	State                         string // 예정 | 지연 | 완료
	Candidates                    AssetCandidates
	Title                         string
}

// AssetCandidates 같은 고객의 자산 수와 제품군 요약(후보 자산 표시용).
type AssetCandidates struct {
	N       int
	Summary string // "3대 · 자료관리 1 · RFID자동화 2", 없으면 "자산 없음"
	Groups  []string
}

func normalizeUnlinkedFilter(f UnlinkedFilter) UnlinkedFilter {
	if f.PageSize <= 0 {
		f.PageSize = 20
	}
	if f.Page < 1 {
		f.Page = 1
	}
	switch f.Status {
	case "open", "done":
	default:
		f.Status = "all"
	}
	f.Q = strings.TrimSpace(f.Q)
	return f
}

// asUnlinkedBase 건수·목록이 같이 쓰는 FROM/WHERE(범위만). withFilter 이면 기간·상태·검색도 더한다.
func asUnlinkedBase(f UnlinkedFilter, withFilter bool) (string, []interface{}, error) {
	q := ` FROM as_receipts ar JOIN customers c ON c.customer_id = ar.customer_id WHERE ` + asUnlinkedWhere
	var args []interface{}
	org, oargs, err := AppendOrgSQL("ar", f.OrgID)
	if err != nil {
		return "", nil, err
	}
	q += org
	args = append(args, oargs...)
	if cond, cargs := mineAssigneeCond("ar.", f.MineUserID, f.MineKeys); cond != "" {
		q += cond
		args = append(args, cargs...)
	}
	if f.ExcludeGroup {
		q += ` AND TRIM(COALESCE(ar.work_group_id,''))=''`
	}
	if !withFilter {
		return q, args, nil
	}
	if f.From != "" {
		q += ` AND date(ar.receipt_datetime) >= date(?)`
		args = append(args, f.From)
	}
	if f.To != "" {
		q += ` AND date(ar.receipt_datetime) <= date(?)`
		args = append(args, f.To)
	}
	switch f.Status {
	case "open":
		q += ` AND ar.status IN ` + model.SQLStatusOpenIncomplete
	case "done":
		q += ` AND ar.status IN ` + model.SQLStatusFullyClosed
	}
	if f.Q != "" {
		like := "%" + f.Q + "%"
		q += ` AND (c.org_name LIKE ? OR ar.as_number LIKE ? OR ar.symptom LIKE ?)`
		args = append(args, like, like, like)
	}
	return q, args, nil
}

func mntUnlinkedBase(f UnlinkedFilter, withFilter bool) (string, []interface{}, error) {
	q := ` FROM maintenance_visits v JOIN customers c ON c.customer_id = v.customer_id WHERE ` + mntUnlinkedWhere
	var args []interface{}
	if err := RequireOrgID(f.OrgID); err != nil {
		return "", nil, err
	}
	if f.OrgID != OrgAll {
		// 방문에는 org_id 가 없다. 계획(plan_id)의 조직으로 구분한다.
		q += ` AND v.plan_id IN (SELECT plan_id FROM maintenance_plans WHERE org_id = ?)`
		args = append(args, f.OrgID)
	}
	if cond, cargs := mineOrGroupCond(AssigneeKindMaintenance, "v.", f.MineUserID, f.MineKeys); cond != "" {
		q += cond
		args = append(args, cargs...)
	}
	if f.ExcludeGroup {
		q += ` AND TRIM(COALESCE(v.work_group_id,''))=''`
	}
	if !withFilter {
		return q, args, nil
	}
	if f.From != "" {
		q += ` AND v.visit_date >= ?`
		args = append(args, f.From)
	}
	if f.To != "" {
		q += ` AND v.visit_date <= ?`
		args = append(args, f.To)
	}
	switch f.Status {
	case "open":
		q += ` AND COALESCE(v.completed,0)=0`
	case "done":
		q += ` AND COALESCE(v.completed,0)=1`
	}
	if p := strings.TrimSpace(f.Product); p != "" {
		q += ` AND COALESCE(v.product_type,'') = ?`
		args = append(args, p)
	}
	if f.Q != "" {
		like := "%" + f.Q + "%"
		q += ` AND (c.org_name LIKE ? OR COALESCE(v.notes,'') LIKE ? OR COALESCE(v.product_type,'') LIKE ?)`
		args = append(args, like, like, like)
	}
	return q, args, nil
}

// CountUnlinkedAS 탭 건수(범위만, 기간·상태·검색 필터는 적용하지 않는다).
func (r *ASRepo) CountUnlinkedAS(f UnlinkedFilter) (int, error) {
	f = normalizeUnlinkedFilter(f)
	base, args, err := asUnlinkedBase(f, false)
	if err != nil {
		return 0, err
	}
	var n int
	err = queryRowTimed(r.db, "as.unlinked.count", `SELECT COUNT(*)`+base, args...).Scan(&n)
	return n, err
}

func (r *ASRepo) CountUnlinkedMnt(f UnlinkedFilter) (int, error) {
	f = normalizeUnlinkedFilter(f)
	base, args, err := mntUnlinkedBase(f, false)
	if err != nil {
		return 0, err
	}
	var n int
	err = queryRowTimed(r.db, "as.unlinkedMnt.count", `SELECT COUNT(*)`+base, args...).Scan(&n)
	return n, err
}

// ListUnlinkedAS 최근 접수일 순 한 페이지와 필터 적용 총건수. 후보 자산은 고객 ID 묶음 한 번의 집계로 채운다(N+1 없음).
func (r *ASRepo) ListUnlinkedAS(f UnlinkedFilter) ([]UnlinkedAS, int, error) {
	f = normalizeUnlinkedFilter(f)
	base, args, err := asUnlinkedBase(f, true)
	if err != nil {
		return nil, 0, err
	}
	var total int
	if err := queryRowTimed(r.db, "as.unlinked.listCount", `SELECT COUNT(*)`+base, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	q := `SELECT ar.as_id, ar.as_number, ar.customer_id, c.org_name, COALESCE(ar.receipt_datetime,''),
	             COALESCE(ar.symptom,''), COALESCE(` + asActionTakenSQL("ar") + `,''), COALESCE(ar.assigned_to,''), COALESCE(ar.status,''),
	             COALESCE(ar.cause_cat1,''), COALESCE(ar.cause_cat1_custom,''),
	             COALESCE((SELECT p.name FROM work_projects p WHERE p.project_id = ar.project_id),''),
	             COALESCE(` + daysElapsedSQL + `, 0)` + base +
		` ORDER BY ar.receipt_datetime DESC, ar.as_number DESC LIMIT ? OFFSET ?`
	qargs := append(append([]interface{}{}, args...), f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := queryTimed(r.db, "as.unlinked.list", q, qargs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []UnlinkedAS
	var custIDs []string
	for rows.Next() {
		var it UnlinkedAS
		var recv string
		if err := rows.Scan(&it.ASID, &it.ASNumber, &it.CustomerID, &it.Customer, &recv, &it.Symptom, &it.ActionTaken,
			&it.AssignedTo, &it.Status, &it.CauseCat1, &it.CauseCat1Custom, &it.ProjectName, &it.DaysElapsed); err != nil {
			return nil, 0, err
		}
		if t := parseTime(recv); !t.IsZero() {
			it.ReceiptDate = t.Format("2006-01-02")
		} else if len(recv) >= 10 {
			it.ReceiptDate = recv[:10]
		}
		it.Open = strings.Contains(model.SQLStatusOpenIncomplete, "'"+it.Status+"'")
		it.Title = model.FormatASWorkTitle(it.Customer, it.ASNumber)
		out = append(out, it)
		custIDs = append(custIDs, it.CustomerID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	cands, err := r.candidateAssetsByCustomer(custIDs)
	if err != nil {
		return nil, 0, err
	}
	for i := range out {
		out[i].Candidates = cands[out[i].CustomerID]
		out[i].ProductGuess = GuessProductLabel(out[i].Candidates.Groups, out[i].Symptom+" "+out[i].ActionTaken)
	}
	return out, total, nil
}

// ListUnlinkedMnt 최근 방문일 순 한 페이지.
func (r *ASRepo) ListUnlinkedMnt(f UnlinkedFilter, today string) ([]UnlinkedMnt, int, error) {
	f = normalizeUnlinkedFilter(f)
	if today == "" {
		today = time.Now().Format("2006-01-02")
	}
	base, args, err := mntUnlinkedBase(f, true)
	if err != nil {
		return nil, 0, err
	}
	var total int
	if err := queryRowTimed(r.db, "as.unlinkedMnt.listCount", `SELECT COUNT(*)`+base, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	q := `SELECT v.visit_id, v.customer_id, c.org_name, COALESCE(v.visit_date,''), COALESCE(v.completed_date,''),
	             COALESCE(v.product_type,''), COALESCE(v.round_no,0), COALESCE(v.duration_min,0),
	             COALESCE(v.assignee,''), COALESCE((SELECT p.name FROM work_projects p WHERE p.project_id = v.project_id),''),
	             COALESCE(v.notes,''), COALESCE(v.completed,0)` + base +
		` ORDER BY v.visit_date DESC, v.visit_id DESC LIMIT ? OFFSET ?`
	qargs := append(append([]interface{}{}, args...), f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := queryTimed(r.db, "as.unlinkedMnt.list", q, qargs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []UnlinkedMnt
	var custIDs []string
	for rows.Next() {
		var it UnlinkedMnt
		var done int
		if err := rows.Scan(&it.VisitID, &it.CustomerID, &it.Customer, &it.VisitDate, &it.CompletedDate, &it.ProductType,
			&it.RoundNo, &it.DurationMin, &it.Assignee, &it.ProjectName, &it.Notes, &done); err != nil {
			return nil, 0, err
		}
		it.Completed = done != 0
		switch {
		case it.Completed:
			it.State = "완료"
		case it.VisitDate != "" && it.VisitDate < today:
			it.State = "지연"
		default:
			it.State = "예정"
		}
		it.ProductLabel = MntProductLabel(it.ProductType)
		it.Title = MntVisitTitle(it.Customer, it.ProductType, it.RoundNo)
		out = append(out, it)
		custIDs = append(custIDs, it.CustomerID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	cands, err := r.candidateAssetsByCustomer(custIDs)
	if err != nil {
		return nil, 0, err
	}
	for i := range out {
		out[i].Candidates = cands[out[i].CustomerID]
	}
	return out, total, nil
}

// candidateAssetsByCustomer 고객 ID 묶음의 자산 수·제품군 요약을 쿼리 한 번으로 구한다.
func (r *ASRepo) candidateAssetsByCustomer(ids []string) (map[string]AssetCandidates, error) {
	atomic.AddInt64(&unlinkedCandidateQueries, 1)
	out := map[string]AssetCandidates{}
	seen := map[string]bool{}
	var uniq []interface{}
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" && !seen[id] {
			seen[id] = true
			uniq = append(uniq, id)
		}
	}
	for id := range seen {
		out[id] = AssetCandidates{Summary: "자산 없음"}
	}
	if len(uniq) == 0 {
		return out, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(uniq)), ",")
	rows, err := queryTimed(r.db, "as.unlinked.candidates", `
		SELECT a.customer_id, COALESCE(NULLIF(TRIM(pg.type_name),''), '(제품군 없음)') AS grp, COUNT(*)
		  FROM assets a LEFT JOIN product_groups pg ON pg.group_id = a.product_group_id
		 WHERE a.customer_id IN (`+ph+`)
		 GROUP BY a.customer_id, grp ORDER BY a.customer_id, COUNT(*) DESC, grp`, uniq...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type part struct {
		g string
		n int
	}
	parts := map[string][]part{}
	for rows.Next() {
		var cid, g string
		var n int
		if err := rows.Scan(&cid, &g, &n); err != nil {
			return nil, err
		}
		parts[cid] = append(parts[cid], part{g, n})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for cid, ps := range parts {
		total := 0
		var segs, groups []string
		for _, p := range ps {
			total += p.n
			segs = append(segs, fmt.Sprintf("%s %d", p.g, p.n))
			if p.g != "(제품군 없음)" {
				groups = append(groups, p.g)
			}
		}
		out[cid] = AssetCandidates{N: total, Summary: fmt.Sprintf("%d대 · %s", total, strings.Join(segs, " · ")), Groups: groups}
	}
	return out, rows.Err()
}

var unlinkedCandidateQueries int64

// UnlinkedCandidateQueries 후보 자산 집계 쿼리가 실행된 횟수(시험용). 목록 한 번에 1회여야 한다(N+1 없음).
func UnlinkedCandidateQueries() int64 { return atomic.LoadInt64(&unlinkedCandidateQueries) }

// MntProductLabel 정기점검 product_type 을 화면 라벨로.
func MntProductLabel(pt string) string {
	switch strings.TrimSpace(pt) {
	case "":
		return "—"
	case "앤로보틱스":
		return "RFID(앤로보틱스)"
	default:
		return strings.TrimSpace(pt)
	}
}

// MntVisitTitle 방문 작업명: {사이트} {제품} 정기점검 {회차}회.
func MntVisitTitle(customer, productType string, round int) string {
	if round < 1 {
		round = 1
	}
	prod := ""
	if p := strings.TrimSpace(productType); p != "" {
		prod = MntProductLabel(p) + " "
	}
	return fmt.Sprintf("%s %s정기점검 %d회", strings.TrimSpace(customer), prod, round)
}

// GuessProductLabel 표시 전용 제품 추정(DB 저장·통계·집계에 쓰지 않는다).
// ① 고객 자산의 제품군이 한 종류뿐이면 그 제품군 ② 증상·조치 문구의 키워드 ③ 모르면 「—」. 두 제품 키워드가 모두 있으면 「KLAS·RFID」.
func GuessProductLabel(customerGroups []string, text string) string {
	distinct := map[string]bool{}
	var only string
	for _, g := range customerGroups {
		if g = strings.TrimSpace(g); g != "" && !distinct[g] {
			distinct[g] = true
			only = g
		}
	}
	if len(distinct) == 1 {
		return only
	}
	t := strings.ToLower(strings.Join(strings.Fields(text), " "))
	klas, rfid := false, false
	for _, k := range guessKLASKeywords {
		if strings.Contains(t, k) {
			klas = true
			break
		}
	}
	for _, k := range guessRFIDKeywords {
		if strings.Contains(t, k) {
			rfid = true
			break
		}
	}
	switch {
	case klas && rfid:
		return "KLAS·RFID"
	case klas:
		return "KLAS"
	case rfid:
		return "RFID"
	}
	return "—"
}

// 제품 추정 키워드 사전(소문자). 시험으로 고정한다. 보완이 필요하면 여기만 고친다.
var (
	guessKLASKeywords = []string{"klas", "kdevice", "케이디바이스", "자료관리"}
	guessRFIDKeywords = []string{"rfid", "무인반납", "자가대출", "스마트도서관", "앤로보틱스"}
)
