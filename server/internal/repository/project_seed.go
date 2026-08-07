package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"customer-support/internal/model"
)

const projectSeedMetaKey = "__meta:seed_default_projects_v1"

// seedDefaultProjects 제시된 6개 사업을 1회 시드한다.
func seedDefaultProjects(db *sql.DB) {
	if metaDone(db, projectSeedMetaKey) {
		return
	}
	type rule struct {
		parentHint   string // 고객명 LIKE
		productKeys  string
		workKinds    string
		notes        string
	}
	type seed struct {
		id        string
		name      string
		shortName string
		year      int
		paid      int
		sort      int
		notes     string
		rules     []rule
	}
	seeds := []seed{
		{
			id: "WPSEED01", name: "2026년 충청남도교육청 도서관 통합정보시스템 SW 유지관리",
			shortName: "2026년 충청남도교육청 도서관 통합정보시스템 SW 유지관리", year: 2026, paid: 1, sort: 10,
			notes: "KLAS 정기점검·AS — 충남교육청 소속 도서관",
			rules: []rule{{
				parentHint: "충청남도교육청", productKeys: model.ProductKeyKLAS,
				workKinds: model.ScopeWorkAS + "," + model.ScopeWorkMaintenance,
				notes:     "충남교육청 소속 · KLAS",
			}},
		},
		{
			id: "WPSEED02", name: "세종시 도서관 ICT 통합정보시스템 유지관리(2026년)",
			shortName: "세종시 도서관 ICT 통합정보시스템 유지관리(2026년)", year: 2026, paid: 1, sort: 20,
			notes: "세종시 KLAS 정기점검·AS — 세종시교육청 소속 도서관",
			rules: []rule{{
				parentHint: "세종시교육청", productKeys: model.ProductKeySejongKLAS,
				workKinds: model.ScopeWorkAS + "," + model.ScopeWorkMaintenance,
				notes:     "세종시교육청 소속 · 세종 KLAS",
			}},
		},
		{
			id: "WPSEED03", name: "충남세종지역 앤로보틱스 RFID자동화 장비 유지보수 사업",
			shortName: "충남세종지역 앤로보틱스 RFID자동화 장비 유지보수 사업", year: 2026, paid: 1, sort: 30,
			notes: "앤로보틱스 정기점검·AS",
			rules: []rule{
				{
					parentHint: "충청남도교육청", productKeys: model.ProductKeyAnrobotics,
					workKinds: model.ScopeWorkAS + "," + model.ScopeWorkMaintenance,
					notes:     "충남 · 앤로보틱스",
				},
				{
					parentHint: "세종시교육청", productKeys: model.ProductKeyAnrobotics,
					workKinds: model.ScopeWorkAS + "," + model.ScopeWorkMaintenance,
					notes:     "세종 · 앤로보틱스",
				},
			},
		},
		{
			id: "WPSEED04", name: "2026년 제천시립도서관 홈페이지 유지보수 용역",
			shortName: "2026년 제천시립도서관 홈페이지 유지보수 용역", year: 2026, paid: 1, sort: 40,
			notes: "현재 미관리 — 추가 관리 예정",
			rules: nil,
		},
		{
			id: "WPSEED05", name: "2026년 세종시교육청 통합정보시스템 유지보수(무상)",
			shortName: "2026년 세종시교육청 통합정보시스템 유지보수(무상)", year: 2026, paid: 0, sort: 50,
			notes: "세종시교육청 소속 KLAS·RFID(앤로보틱스) 무상",
			rules: []rule{{
				parentHint: "세종시교육청",
				productKeys: model.ProductKeyKLAS + "," + model.ProductKeySejongKLAS + "," + model.ProductKeyAnrobotics,
				workKinds:   model.ScopeWorkAS + "," + model.ScopeWorkMaintenance,
				notes:       "세종시교육청 · KLAS/RFID 무상",
			}},
		},
		{
			id: "WPSEED06", name: "2027년 충청남도교육청 도서관 통합정보시스템 SW 유지관리",
			shortName: "2027년 충청남도교육청 도서관 통합정보시스템 SW 유지관리", year: 2027, paid: 1, sort: 60,
			notes: "행정/사업지원 업무 중심",
			rules: []rule{{
				parentHint: "충청남도교육청", productKeys: model.ProductKeyKLAS,
				workKinds: model.ScopeWorkAdmin,
				notes:     "충남교육청 · 행정/사업지원",
			}},
		},
	}

	for _, s := range seeds {
		var exists string
		_ = db.QueryRow(`SELECT project_id FROM work_projects WHERE project_id=?`, s.id).Scan(&exists)
		if exists != "" {
			continue
		}
		_, err := db.Exec(`
			INSERT INTO work_projects (
				project_id, name, short_name, plan_year, is_paid, sort_order,
				notes, color, status, start_date, end_date
			) VALUES (?,?,?,?,?,?,?,'#3B82F6','active',?,?)`,
			s.id, s.name, s.shortName, s.year, s.paid, s.sort, s.notes,
			fmt.Sprintf("%d-01-01", s.year), fmt.Sprintf("%d-12-31", s.year),
		)
		if err != nil {
			continue
		}
		for i, r := range s.rules {
			parentID := findCustomerIDByHint(db, r.parentHint)
			notes := r.notes
			if parentID == "" && r.parentHint != "" {
				notes = strings.TrimSpace(notes + " · 상위기관 미매칭:" + r.parentHint)
			}
			ruleID := fmt.Sprintf("%s-R%02d", s.id, i+1)
			_, _ = db.Exec(`
				INSERT INTO project_scope_rules (rule_id, project_id, parent_customer_id, product_keys, work_kinds, notes)
				VALUES (?,?,?,?,?,?)`,
				ruleID, s.id, nullStr(parentID), r.productKeys, r.workKinds, notes)
		}
	}
	markMetaDone(db, projectSeedMetaKey)
}

// renameSeedProjectDisplayNames 시드 사업의 짧은 표시명을 정식 사업명으로 통일한다.
func renameSeedProjectDisplayNames(db *sql.DB) {
	if metaDone(db, projectDisplayNamesV2MetaKey) {
		return
	}
	renames := []struct {
		id, name string
	}{
		{"WPSEED01", "2026년 충청남도교육청 도서관 통합정보시스템 SW 유지관리"},
		{"WPSEED02", "세종시 도서관 ICT 통합정보시스템 유지관리(2026년)"},
		{"WPSEED03", "충남세종지역 앤로보틱스 RFID자동화 장비 유지보수 사업"},
		{"WPSEED04", "2026년 제천시립도서관 홈페이지 유지보수 용역"},
		{"WPSEED05", "2026년 세종시교육청 통합정보시스템 유지보수(무상)"},
		{"WPSEED06", "2027년 충청남도교육청 도서관 통합정보시스템 SW 유지관리"},
	}
	for _, r := range renames {
		_, _ = db.Exec(`
			UPDATE work_projects
			SET name=?, short_name=?, updated_at=CURRENT_TIMESTAMP
			WHERE project_id=?`, r.name, r.name, r.id)
	}
	markMetaDone(db, projectDisplayNamesV2MetaKey)
}

// linkRFIDAssetsToAnroboticsProject 제품분류 RFID자동화 자산을
// 「충남세종지역 앤로보틱스 RFID자동화 장비 유지보수 사업」에 1회 일괄 연결한다.
func linkRFIDAssetsToAnroboticsProject(db *sql.DB) {
	if metaDone(db, assetRFIDProjectLinkMetaKey) {
		return
	}
	pid := findAnroboticsRFIDProjectID(db)
	if pid == "" {
		return
	}
	res, err := db.Exec(`
		UPDATE assets SET project_id=?, updated_at=CURRENT_TIMESTAMP
		WHERE LOWER(TRIM(COALESCE(product_category,''))) = 'rfid'`, pid)
	if err != nil {
		return
	}
	if n, _ := res.RowsAffected(); n >= 0 {
		markMetaDone(db, assetRFIDProjectLinkMetaKey)
	}
}

func findAnroboticsRFIDProjectID(db *sql.DB) string {
	return findProjectIDByHint(db, ProjectIDAnroboticsRFID, "%앤로보틱스 RFID자동화%")
}

// linkMaterialsChungnamToSWProject 제품분류 자료관리 + 상위기관 충남교육청 자산을
// 「2026년 충청남도교육청 도서관 통합정보시스템 SW 유지관리」에 1회 일괄 연결한다.
func linkMaterialsChungnamToSWProject(db *sql.DB) {
	if metaDone(db, assetMaterialsChungnamLinkMetaKey) {
		return
	}
	pid := findProjectIDByHint(db, ProjectIDChungnamSW2026, "%충청남도교육청 도서관 통합정보시스템 SW 유지관리%")
	if pid == "" {
		pid = findProjectIDByHint(db, ProjectIDChungnamSW2026, "%충남교육청%SW%2026%")
	}
	if pid == "" {
		return
	}
	res, err := db.Exec(`
		UPDATE assets SET project_id=?, updated_at=CURRENT_TIMESTAMP
		WHERE LOWER(TRIM(COALESCE(product_category,''))) = 'materials'
		  AND customer_id IN (
			SELECT c.customer_id FROM customers c
			LEFT JOIN customers p ON p.customer_id = c.parent_customer_id
			WHERE p.org_name LIKE '%충청남도교육청%' OR p.org_name LIKE '%충남교육청%'
			   OR p.official_name LIKE '%충청남도교육청%' OR p.official_name LIKE '%충남교육청%'
			   OR c.org_name LIKE '%충청남도교육청%' OR c.org_name LIKE '%충남교육청%'
		  )`, pid)
	if err != nil {
		return
	}
	if n, _ := res.RowsAffected(); n >= 0 {
		markMetaDone(db, assetMaterialsChungnamLinkMetaKey)
	}
}

// linkSejongLibraryToICTProject 상위기관(또는 본인)이 세종시립도서관인 자산에
// 「세종시 도서관 ICT 통합정보시스템 유지관리(2026년)」을 연결한다.
// RFID자동화는 앤로보틱스 사업(WPSEED03)을 유지하며, 이미 사업이 있는 자산은 덮지 않는다.
func linkSejongLibraryToICTProject(db *sql.DB) {
	if metaDone(db, assetSejongLibraryICTLinkMetaKey) {
		return
	}
	pid := findProjectIDByHint(db, ProjectIDSejongICT2026, "%세종시 도서관 ICT 통합정보시스템 유지관리%")
	if pid == "" {
		return
	}
	res, err := db.Exec(`
		UPDATE assets SET project_id=?, updated_at=CURRENT_TIMESTAMP
		WHERE LOWER(TRIM(COALESCE(product_category,''))) != 'rfid'
		  AND COALESCE(TRIM(project_id),'') = ''
		  AND customer_id IN (
			SELECT c.customer_id FROM customers c
			LEFT JOIN customers p ON p.customer_id = c.parent_customer_id
			WHERE c.org_name LIKE '%세종시립도서관%' OR c.official_name LIKE '%세종시립도서관%'
			   OR p.org_name LIKE '%세종시립도서관%' OR p.official_name LIKE '%세종시립도서관%'
		  )`, pid)
	if err != nil {
		return
	}
	if n, _ := res.RowsAffected(); n >= 0 {
		markMetaDone(db, assetSejongLibraryICTLinkMetaKey)
	}
}

func findProjectIDByHint(db *sql.DB, preferredID, nameLike string) string {
	var id string
	err := db.QueryRow(`
		SELECT project_id FROM work_projects
		WHERE project_id=? OR name LIKE ?
		ORDER BY CASE WHEN project_id=? THEN 0 ELSE 1 END
		LIMIT 1`,
		preferredID, nameLike, preferredID,
	).Scan(&id)
	if err != nil {
		return ""
	}
	return id
}

// uppercaseAssetProductTypes 제품구분 영문 소문자를 대문자로 통일한다.
func uppercaseAssetProductTypes(db *sql.DB) {
	if metaDone(db, assetProductTypeUpperMetaKey) {
		return
	}
	// 코드값: sw→SW, hw→HW …
	_, _ = db.Exec(`
		UPDATE codes SET code_value = UPPER(code_value)
		WHERE code_group = 'product_type'
		  AND code_value GLOB '*[a-z]*'`)
	// 설치자산 제품구분
	_, _ = db.Exec(`
		UPDATE assets SET product_type = UPPER(product_type), updated_at = CURRENT_TIMESTAMP
		WHERE product_type IS NOT NULL
		  AND TRIM(product_type) != ''
		  AND product_type GLOB '*[a-z]*'`)
	markMetaDone(db, assetProductTypeUpperMetaKey)
}

func findCustomerIDByHint(db *sql.DB, hint string) string {
	hint = strings.TrimSpace(hint)
	if hint == "" {
		return ""
	}
	var id string
	err := db.QueryRow(`
		SELECT customer_id FROM customers
		WHERE org_name LIKE ? OR official_name LIKE ?
		ORDER BY CASE WHEN org_name = ? OR official_name = ? THEN 0 ELSE 1 END, org_name
		LIMIT 1`,
		"%"+hint+"%", "%"+hint+"%", hint, hint).Scan(&id)
	if err != nil {
		return ""
	}
	return id
}
