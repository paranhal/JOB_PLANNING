package repository

import (
	"fmt"
	"strings"
	"time"

	"customer-support/internal/audit"
)

// 95-204: 자산 연결이 없는 AS·정기점검 중 「고객 자산이 정확히 1개」인 건을 그 자산에 연결한다(일괄 연결).
//
// 연결 규칙은 모두 여기 한 곳에 둔다. 화면의 값은 믿지 않고 요청 시점에 다시 판정한다.
//   - AS: as_receipts.asset_id 만 채운다(단건 저장과 같이 변경 이력·검색 색인을 남긴다. project_id 는 다시 계산하지 않는다).
//   - 정기점검: 방문 대상 행을 하나 추가한다(기존 직접입력 행은 그대로 둔다). 방문 업무가 이미 있을 때만 work_task_assets 를 채운다(업무를 새로 만들지 않는다).
//   - 이미 연결됐거나 자산이 늘어난 건은 건너뛴다(멱등).

// LinkSingleMax 한 번에 처리하는 최대 건수.
const LinkSingleMax = 100

// 건너뜀 사유 코드.
const (
	LinkSkipNotFound = "not_found" // 범위 밖·없음·이미 연결됨(요청 시점 기준 대상이 아님)
	LinkSkipNoAsset  = "no_asset"  // 자산 0개
	LinkSkipMulti    = "multi"     // 자산 2개 이상
	LinkSkipMismatch = "mismatch"  // 점검 제품과 자산 제품군이 명백히 다름
	LinkSkipClosed   = "closed"    // 완료·잠금
	LinkSkipDenied   = "denied"    // 권한 없음
	LinkSkipError    = "error"
)

// LinkSkipText 사유 코드의 화면 문구.
func LinkSkipText(code string) string {
	switch code {
	case LinkSkipNotFound:
		return "이미 연결됐거나 대상이 아닙니다"
	case LinkSkipNoAsset:
		return "고객 자산이 없습니다"
	case LinkSkipMulti:
		return "고객 자산이 2개 이상입니다"
	case LinkSkipMismatch:
		return "점검 제품과 자산 제품군이 다릅니다"
	case LinkSkipClosed:
		return "완료된 건은 잠금 해제 후 조치 화면에서 연결하세요"
	case LinkSkipDenied:
		return "연결 권한이 없습니다"
	default:
		return "처리 중 오류가 났습니다"
	}
}

// MntProductMismatch 방문 제품(KLAS·앤로보틱스)과 자산 제품군이 명백히 다르면 true.
// 명백한 조합만 막는다: 자료관리↔KLAS, RFID자동화↔앤로보틱스. 다른 제품군이나 빈 제품은 막지 않는다.
func MntProductMismatch(productType, group string) bool {
	pt, g := strings.TrimSpace(productType), strings.TrimSpace(group)
	if pt == "" {
		return false
	}
	want := map[string]string{"자료관리": "KLAS", "RFID자동화": "앤로보틱스"}
	w, ok := want[g]
	return ok && w != pt
}

// LinkEligibility 후보 자산 수·제품군으로 본 연결 가능 여부(대상이 아니면 사유 코드, 가능하면 "").
func LinkEligibility(c AssetCandidates, mntProductType string, mnt bool) string {
	switch {
	case c.N == 0:
		return LinkSkipNoAsset
	case c.N > 1:
		return LinkSkipMulti
	case c.OneAssetID == "":
		return LinkSkipNotFound
	case mnt && MntProductMismatch(mntProductType, c.OneGroup):
		return LinkSkipMismatch
	}
	return ""
}

// LinkSingleTarget 요청 시점에 다시 읽은 연결 대상 한 건.
type LinkSingleTarget struct {
	ID, Title, OrgID                                         string
	Closed                                                   bool
	Status, AssignedTo, AssignedUserID, CreatedBy, WorkGroup string
	AssetID, AssetName, AssetGroup                           string
	Skip                                                     string // 비면 연결 가능
}

func inPlaceholders(ids []string) (string, []interface{}) {
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return strings.TrimSuffix(strings.Repeat("?,", len(ids)), ","), args
}

// LinkSingleTargetsAS ids 중 지금도 「자산 연결 없음」이고 조직 범위 안인 AS 를 읽고, 후보 자산(고객당 1회 집계)으로 연결 가능 여부를 정한다.
// 반환에 없는 id 는 대상이 아니다(없음·범위 밖·이미 연결).
func (r *ASRepo) LinkSingleTargetsAS(orgID string, ids []string) (map[string]LinkSingleTarget, error) {
	out := map[string]LinkSingleTarget{}
	if len(ids) == 0 {
		return out, nil
	}
	base, args, err := asUnlinkedBase(UnlinkedFilter{OrgID: orgID}, false)
	if err != nil {
		return nil, err
	}
	ph, iargs := inPlaceholders(ids)
	rows, err := queryTimed(r.db, "as.linkSingle.targetsAS",
		`SELECT ar.as_id, ar.as_number, c.org_name, ar.customer_id, COALESCE(ar.org_id,''), COALESCE(ar.status,''),
		        COALESCE(ar.assigned_to,''), COALESCE(ar.assigned_user_id,''), COALESCE(ar.created_by_user_id,''), COALESCE(ar.work_group_id,'')`+
			base+` AND ar.as_id IN (`+ph+`)`, append(args, iargs...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	custOf := map[string]string{}
	for rows.Next() {
		var t LinkSingleTarget
		var asNo, org, cust string
		if err := rows.Scan(&t.ID, &asNo, &org, &cust, &t.OrgID, &t.Status, &t.AssignedTo, &t.AssignedUserID, &t.CreatedBy, &t.WorkGroup); err != nil {
			return nil, err
		}
		t.Title = fmt.Sprintf("%s %s", strings.TrimSpace(org), asNo)
		out[t.ID] = t
		custOf[t.ID] = cust
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return r.fillLinkCandidates(out, custOf, nil)
}

// LinkSingleTargetsMnt 정기점검 방문 판. 방문의 product_type 이 자산 제품군과 명백히 다르면 Skip=mismatch.
func (r *ASRepo) LinkSingleTargetsMnt(orgID string, ids []string) (map[string]LinkSingleTarget, error) {
	out := map[string]LinkSingleTarget{}
	if len(ids) == 0 {
		return out, nil
	}
	base, args, err := mntUnlinkedBase(UnlinkedFilter{OrgID: orgID}, false)
	if err != nil {
		return nil, err
	}
	ph, iargs := inPlaceholders(ids)
	rows, err := queryTimed(r.db, "as.linkSingle.targetsMnt",
		`SELECT v.visit_id, c.org_name, v.customer_id, COALESCE(v.product_type,''), COALESCE(v.round_no,0), COALESCE(v.completed,0),
		        COALESCE((SELECT p.org_id FROM maintenance_plans p WHERE p.plan_id = v.plan_id),'')`+
			base+` AND v.visit_id IN (`+ph+`)`, append(args, iargs...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	custOf := map[string]string{}
	pt := map[string]string{}
	for rows.Next() {
		var t LinkSingleTarget
		var org, cust, ptype string
		var round, done int
		if err := rows.Scan(&t.ID, &org, &cust, &ptype, &round, &done, &t.OrgID); err != nil {
			return nil, err
		}
		t.Closed = done != 0
		t.Title = MntVisitTitle(org, ptype, round)
		out[t.ID] = t
		custOf[t.ID] = cust
		pt[t.ID] = ptype
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return r.fillLinkCandidates(out, custOf, pt)
}

func (r *ASRepo) fillLinkCandidates(out map[string]LinkSingleTarget, custOf, productOf map[string]string) (map[string]LinkSingleTarget, error) {
	var custs []string
	for _, c := range custOf {
		custs = append(custs, c)
	}
	cands, err := r.candidateAssetsByCustomer(custs) // 고객 묶음 한 번의 집계(건마다 조회하지 않는다)
	if err != nil {
		return nil, err
	}
	for id, t := range out {
		c := cands[custOf[id]]
		t.Skip = LinkEligibility(c, productOf[id], productOf != nil)
		t.AssetID, t.AssetName, t.AssetGroup = c.OneAssetID, c.OneAssetName, c.OneGroup
		out[id] = t
	}
	return out, nil
}

// LinkSingleAS 한 건 연결. 쓰는 순간에도 조건(조직·아직 미연결·고객 자산 정확히 1개이고 그 자산)을 한 문장으로 다시 검사하므로
// 사이에 자산이 늘었거나 이미 연결된 건은 0행이 되어 건너뛴다(false). 재전송해도 같은 결과(멱등).
func (r *ASRepo) LinkSingleAS(orgID, asID, assetID, label string) (bool, error) {
	org, oargs, err := AppendOrgSQL("ar", orgID)
	if err != nil {
		return false, err
	}
	linked := false
	now := time.Now().Format("2006-01-02 15:04:05")
	err = touchUpdate(r.db, "as_receipts", "as_id", asID, label, func() error {
		args := append([]interface{}{assetID, now, asID}, oargs...)
		args = append(args, assetID)
		res, err := r.db.Exec(`UPDATE as_receipts AS ar SET asset_id=?, updated_at=?
			WHERE ar.as_id=? AND `+asUnlinkedWhere+org+`
			  AND (SELECT COUNT(*) FROM assets s WHERE s.customer_id = ar.customer_id) = 1
			  AND EXISTS (SELECT 1 FROM assets s WHERE s.asset_id=? AND s.customer_id = ar.customer_id)`, args...)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		linked = n > 0
		return nil
	})
	if err != nil || !linked {
		return false, err
	}
	reindexASSearch(r.db, asID)
	return true, nil
}

// LinkSingleMnt 한 건 연결(정기점검). 방문 대상 행을 하나 더하고, 방문 업무가 이미 있으면 work_task_assets 도 채운다.
// 요청 시점에 다시 검사하고(0행이면 건너뜀), 제품 불일치도 같은 트랜잭션에서 한 번 더 본다.
func (r *ASRepo) LinkSingleMnt(orgID, visitID, assetID string) (bool, error) {
	if err := RequireOrgID(orgID); err != nil {
		return false, err
	}
	tx, err := r.db.Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var pt, grp string
	err = tx.QueryRow(`SELECT COALESCE(v.product_type,''), COALESCE(NULLIF(TRIM(pg.type_name),''),'')
		FROM maintenance_visits v JOIN assets a ON a.asset_id=? AND a.customer_id=v.customer_id
		LEFT JOIN product_groups pg ON pg.group_id=a.product_group_id WHERE v.visit_id=?`, assetID, visitID).Scan(&pt, &grp)
	if err != nil {
		return false, nil // 없는 방문·다른 고객 자산 → 대상 아님
	}
	if MntProductMismatch(pt, grp) {
		return false, nil
	}
	cond := ""
	var cargs []interface{}
	if orgID != OrgAll {
		cond = ` AND v.plan_id IN (SELECT plan_id FROM maintenance_plans WHERE org_id = ?)`
		cargs = []interface{}{orgID}
	}
	id := randomID("mvt_")
	args := append([]interface{}{id, assetID, visitID}, cargs...)
	res, err := tx.Exec(`INSERT INTO maintenance_visit_targets(target_id, visit_id, asset_id, name, memo, sort_order, org_id)
		SELECT ?, v.visit_id, a.asset_id, COALESCE(a.product_name,''), '',
		       COALESCE((SELECT MAX(t.sort_order)+1 FROM maintenance_visit_targets t WHERE t.visit_id = v.visit_id), 0),
		       COALESCE((SELECT p.org_id FROM maintenance_plans p WHERE p.plan_id = v.plan_id),'')
		  FROM maintenance_visits v JOIN assets a ON a.asset_id=? AND a.customer_id = v.customer_id
		 WHERE v.visit_id=? AND `+mntUnlinkedWhere+cond+`
		   AND (SELECT COUNT(*) FROM assets s WHERE s.customer_id = v.customer_id) = 1`, args...)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, nil
	}
	// 방문 업무가 이미 있을 때만 업무 자산을 채운다(업무를 새로 만들지 않는다).
	if _, err := tx.Exec(`INSERT OR IGNORE INTO work_task_assets(task_id, asset_id)
		SELECT w.task_id, ? FROM work_tasks w WHERE w.source_type='maintenance' AND w.source_id=?`, assetID, visitID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	logCreate(r.db, "maintenance_visit_targets", "target_id", id, "정기점검 자산 일괄 연결 "+visitID)
	return true, nil
}

// LogLinkSingleBulk 일괄 연결 한 번의 요약 감사 기록(누가는 감사 기록이 요청 맥락에서 채운다).
func LogLinkSingleBulk(kind string, linked, skipped int, ids []string) {
	if linked+skipped == 0 {
		return
	}
	kindLabel := "AS"
	if kind == "mnt" {
		kindLabel = "정기점검"
	}
	first := ""
	if len(ids) > 0 {
		first = ids[0]
	}
	label := fmt.Sprintf("자산 미연결 일괄 연결 · %s %d건 연결 · %d건 건너뜀", kindLabel, linked, skipped)
	_ = audit.InsertStrict(audit.ActionUpdate, "as_unlinked_bulk", "kind", randomID("ulb_")+first, label, "", "", "자산 1개 고객 일괄 연결", "")
}
