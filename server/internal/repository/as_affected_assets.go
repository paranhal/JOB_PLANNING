package repository

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"unicode/utf8"

	"customer-support/internal/model"
)

// 95-201: AS 조치 화면 「영향 장비」. 영향 장비는 업무·건수를 만들지 않는다.

const (
	AffectedMaxAssets = 20
	AffectedMaxLabels = 20
	AffectedLabelMax  = 80
)

var (
	ErrAffectedLimit   = fmt.Errorf("영향 장비는 자산 %d개, 직접 입력 %d줄까지입니다", AffectedMaxAssets, AffectedMaxLabels)
	ErrAffectedLabelLn = fmt.Errorf("직접 입력 항목은 %d자 이내로 적어 주세요", AffectedLabelMax)
	ErrAffectedCust    = fmt.Errorf("이 접수 고객의 자산만 영향 장비로 고를 수 있습니다")
)

func applyASAffectedAssetsV114(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := leaveLedgerSQL("114_as_affected_assets.sql")
	if err != nil {
		log.Printf("114 as_affected_assets: %v", err)
		return
	}
	if err := execWorkGroupSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("114 as_affected_assets apply: %v", err)
	}
	seedKDeviceOrgCauses(db)
}

const kdeviceLabel = "KDevice(클라이언트 프로그램)"

// seedKDeviceOrgCauses 조직별 원인 분류표를 따로 가진 조직 중 KLAS(1차)가 있는 조직에 KDevice 를 같이 넣는다. 멱등.
// 조직표는 행이 하나라도 있으면 그 조직이 자기 표만 쓰므로, 기존 표에만 넣으면 그 조직에는 보이지 않는다.
func seedKDeviceOrgCauses(db *sql.DB) {
	rows, err := db.Query(`SELECT org_id, code, label FROM as_cause_category_orgs WHERE level=1 AND upper(trim(label))='KLAS'`)
	if err != nil {
		return // 조직표가 없는 DB
	}
	type l1 struct{ org, code, label string }
	var parents []l1
	for rows.Next() {
		var p l1
		if rows.Scan(&p.org, &p.code, &p.label) == nil {
			parents = append(parents, p)
		}
	}
	rows.Close()
	type node struct {
		label string
		sort  int
		fault int
	}
	kids := []node{{"설치·재설치", 10, 0}, {"설정·자동 로그인", 20, 1}, {"장비 연동 오류", 30, 1}, {"기타", 40, 1}}
	for _, p := range parents {
		var opacSort sql.NullInt64
		_ = db.QueryRow(`SELECT sort_order FROM as_cause_category_orgs WHERE org_id=? AND parent_code=? AND label='자료검색대'`, p.org, p.code).Scan(&opacSort)
		sort2 := 35
		if opacSort.Valid {
			sort2 = int(opacSort.Int64) + 5
		} else {
			var mx sql.NullInt64
			_ = db.QueryRow(`SELECT MAX(sort_order) FROM as_cause_category_orgs WHERE org_id=? AND parent_code=?`, p.org, p.code).Scan(&mx)
			if mx.Valid {
				sort2 = int(mx.Int64) + 10
			}
		}
		c2 := model.CauseOrgPathCode(p.org, p.label, kdeviceLabel)
		if _, err := db.Exec(`INSERT OR IGNORE INTO as_cause_category_orgs (org_id, code, level, parent_code, label, sort_order, is_active, cause_type_map, is_fault, created_by)
			VALUES (?,?,2,?,?,?,1,'',1,'95-201')`, p.org, c2, p.code, kdeviceLabel, sort2); err != nil {
			log.Printf("114 KDevice org %s: %v", p.org, err)
			continue
		}
		for _, k := range kids {
			c3 := model.CauseOrgPathCode(p.org, p.label, kdeviceLabel, k.label)
			_, _ = db.Exec(`INSERT OR IGNORE INTO as_cause_category_orgs (org_id, code, level, parent_code, label, sort_order, is_active, cause_type_map, is_fault, created_by)
				VALUES (?,?,3,?,?,?,1,'',?,'95-201')`, p.org, c3, c2, k.label, k.sort, k.fault)
		}
	}
}

// ParseAffectedLabels 줄바꿈·쉼표로 나눈 직접 입력을 정리한다(빈 항목·중복 제거).
func ParseAffectedLabels(raw string) []string {
	raw = strings.ReplaceAll(raw, "\r", "\n")
	raw = strings.ReplaceAll(raw, "，", ",")
	var out []string
	seen := map[string]bool{}
	for _, line := range strings.Split(raw, "\n") {
		for _, part := range strings.Split(line, ",") {
			s := strings.TrimSpace(part)
			if s == "" || seen[s] {
				continue
			}
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// CleanAffectedInput 영향 장비 입력을 검증·정리한다. exclude(주 대상 자산·설치자산)와 같은 자산은 빼고 skipped 로 돌려준다.
func (r *ASRepo) CleanAffectedInput(customerID string, assetIDs, labels, exclude []string) (ids, outLabels, skipped []string, err error) {
	excl := map[string]bool{}
	for _, e := range exclude {
		if e = strings.TrimSpace(e); e != "" {
			excl[e] = true
		}
	}
	seen := map[string]bool{}
	for _, id := range assetIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if excl[id] {
			skipped = append(skipped, id)
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) > AffectedMaxAssets || len(labels) > AffectedMaxLabels {
		return nil, nil, nil, ErrAffectedLimit
	}
	for _, l := range labels {
		if utf8.RuneCountInString(l) > AffectedLabelMax {
			return nil, nil, nil, ErrAffectedLabelLn
		}
	}
	// 접수 고객의 자산이어야 한다(다른 고객 자산 거절)
	for _, id := range ids {
		var cid string
		e := r.db.QueryRow(`SELECT COALESCE(customer_id,'') FROM assets WHERE asset_id=?`, id).Scan(&cid)
		if e != nil || strings.TrimSpace(cid) != strings.TrimSpace(customerID) {
			return nil, nil, nil, ErrAffectedCust
		}
	}
	return ids, labels, skipped, nil
}

// ReplaceAffectedAssets 해당 접수의 영향 장비를 교체한다(같은 트랜잭션에서 지우고 다시 넣는다). 모두 비우면 행이 지워진다.
func (r *ASRepo) ReplaceAffectedAssets(asID, userID string, assetIDs, labels []string) error {
	asID = strings.TrimSpace(asID)
	if asID == "" {
		return fmt.Errorf("접수가 없습니다")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM as_receipt_affected_assets WHERE as_id=?`, asID); err != nil {
		return err
	}
	for _, id := range assetIDs {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO as_receipt_affected_assets (as_id, asset_id, label, created_by_user_id) VALUES (?,?,'',?)`,
			asID, id, nullIfEmpty(userID)); err != nil {
			return err
		}
	}
	for _, l := range labels {
		if _, err := tx.Exec(`INSERT INTO as_receipt_affected_assets (as_id, asset_id, label, created_by_user_id) VALUES (?,NULL,?,?)`,
			asID, l, nullIfEmpty(userID)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListAffectedAssets 접수의 영향 장비(자산 이름·제품군 포함). 등록 자산 먼저, 직접 입력은 뒤.
func (r *ASRepo) ListAffectedAssets(asID string) ([]model.ASAffectedAsset, error) {
	rows, err := r.db.Query(`
		SELECT af.id, af.as_id, COALESCE(af.asset_id,''), COALESCE(af.label,''),
		       COALESCE(NULLIF(TRIM(a.product_name),''), a.asset_id, ''), COALESCE(a.customer_id,''),
		       COALESCE(pg.type_name,'')
		  FROM as_receipt_affected_assets af
		  LEFT JOIN assets a ON a.asset_id = af.asset_id
		  LEFT JOIN product_groups pg ON pg.group_id = a.product_group_id
		 WHERE af.as_id=?
		 ORDER BY CASE WHEN af.asset_id IS NULL THEN 1 ELSE 0 END, af.id`, strings.TrimSpace(asID))
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.ASAffectedAsset
	for rows.Next() {
		var it model.ASAffectedAsset
		if err := rows.Scan(&it.ID, &it.ASID, &it.AssetID, &it.Label, &it.AssetName, &it.CustomerID, &it.ProductGroup); err != nil {
			return nil, err
		}
		it.Name = it.AssetName
		if it.AssetID == "" {
			it.Name = it.Label
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// AffectedLine 「영향 장비: 이름 외 N대」 한 줄. 없으면 빈 문자열.
func AffectedLine(items []model.ASAffectedAsset) string {
	if len(items) == 0 {
		return ""
	}
	first := strings.TrimSpace(items[0].Name)
	if first == "" {
		first = "(이름 없음)"
	}
	if len(items) == 1 {
		return first
	}
	return fmt.Sprintf("%s 외 %d대", first, len(items)-1)
}

// AffectedStat 영향 장비 통계.
type AffectedStat struct {
	ReceiptN int
	AssetN   int
	ByGroup  []AffectedGroupN
}

type AffectedGroupN struct {
	Group string
	N     int
}

// AffectedStats 기간·조직 안에서 영향 장비가 1건 이상인 접수 수, 영향 장비 수, 제품군별 장비 수. 접수 건수 집계는 바꾸지 않는다.
// from/to 는 접수일(YYYY-MM-DD, 비어 있으면 제한 없음). orgID 가 OrgAll 이거나 비어 있으면 전체.
func (r *ASRepo) AffectedStats(from, to, orgID string) (AffectedStat, error) {
	var st AffectedStat
	where := ` WHERE 1=1`
	var args []interface{}
	if f := strings.TrimSpace(from); f != "" {
		where += ` AND date(ar.receipt_datetime) >= date(?)`
		args = append(args, f)
	}
	if t := strings.TrimSpace(to); t != "" {
		where += ` AND date(ar.receipt_datetime) <= date(?)`
		args = append(args, t)
	}
	if o := strings.TrimSpace(orgID); o != "" && o != OrgAll {
		where += ` AND ar.org_id=?`
		args = append(args, o)
	}
	base := ` FROM as_receipt_affected_assets af JOIN as_receipts ar ON ar.as_id = af.as_id`
	if err := r.db.QueryRow(`SELECT COUNT(DISTINCT af.as_id), COUNT(*)`+base+where, args...).Scan(&st.ReceiptN, &st.AssetN); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return st, nil
		}
		return st, err
	}
	rows, err := r.db.Query(`
		SELECT CASE WHEN af.asset_id IS NULL THEN '직접 입력'
		            ELSE COALESCE(NULLIF(TRIM(pg.type_name),''), '(제품군 없음)') END AS grp, COUNT(*) n`+base+`
		  LEFT JOIN assets a ON a.asset_id = af.asset_id
		  LEFT JOIN product_groups pg ON pg.group_id = a.product_group_id`+where+`
		 GROUP BY grp ORDER BY n DESC, grp LIMIT 10`, args...)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var g AffectedGroupN
		if err := rows.Scan(&g.Group, &g.N); err != nil {
			return st, err
		}
		st.ByGroup = append(st.ByGroup, g)
	}
	return st, rows.Err()
}
