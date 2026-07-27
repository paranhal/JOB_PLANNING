package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

type CustomerRepo struct {
	db *sql.DB
}

func NewCustomerRepo(db *sql.DB) *CustomerRepo {
	return &CustomerRepo{db: db}
}

// List 고객 목록 조회 (검색, 상위기관·업종 필터, 정렬, 페이징)
// category: ""=전체, "none"=상위기관 없음, 그 외=상위기관 customer_id
// sort: ""=기본(카테고리), org_name|industry|assets|as
func (r *CustomerRepo) List(search, category, industry, sort, dir string, page, pageSize int) ([]model.CustomerListItem, int, error) {
	offset := (page - 1) * pageSize

	baseQuery := `
		SELECT c.customer_id, c.org_name, c.official_name,
		       COALESCE(c.industry,''), COALESCE(c.main_phone,''), c.is_active,
		       COUNT(DISTINCT a.asset_id) AS asset_count,
		       COUNT(DISTINCT ar.as_id) AS as_count,
		       COALESCE(c.parent_customer_id,''),
		       COALESCE(p.org_name,''),
		       CASE WHEN c.has_parent=1 AND COALESCE(c.parent_customer_id,'')!='' THEN 1 ELSE 0 END
		FROM customers c
		LEFT JOIN customers p ON p.customer_id = c.parent_customer_id
		LEFT JOIN assets a ON a.customer_id = c.customer_id
		LEFT JOIN as_receipts ar ON ar.customer_id = c.customer_id
		WHERE 1=1`

	countQuery := `SELECT COUNT(*) FROM customers c WHERE 1=1`
	args := []interface{}{}
	countArgs := []interface{}{}

	switch category {
	case "":
	case "none":
		cond := ` AND (c.has_parent=0 OR c.parent_customer_id IS NULL OR TRIM(c.parent_customer_id)='')`
		baseQuery += cond
		countQuery += cond
	default:
		cond := ` AND (c.customer_id=? OR c.parent_customer_id=?)`
		baseQuery += cond
		countQuery += cond
		args = append(args, category, category)
		countArgs = append(countArgs, category, category)
	}

	if industry != "" {
		if industry == "none" {
			cond := ` AND (c.industry IS NULL OR TRIM(c.industry)='')`
			baseQuery += cond
			countQuery += cond
		} else {
			baseQuery += ` AND c.industry=?`
			countQuery += ` AND c.industry=?`
			args = append(args, industry)
			countArgs = append(countArgs, industry)
		}
	}

	if search != "" {
		like := "%" + search + "%"
		baseQuery += ` AND (c.org_name LIKE ? OR c.official_name LIKE ? OR c.main_phone LIKE ? OR COALESCE(p.org_name,'') LIKE ? OR COALESCE(c.industry,'') LIKE ?)`
		countQuery += ` AND (c.org_name LIKE ? OR c.official_name LIKE ? OR c.main_phone LIKE ? OR COALESCE(c.industry,'') LIKE ?)`
		args = append(args, like, like, like, like, like)
		countArgs = append(countArgs, like, like, like, like)
	}

	baseQuery += ` GROUP BY c.customer_id `
	orderDir := "ASC"
	if strings.EqualFold(dir, "desc") {
		orderDir = "DESC"
	}
	switch sort {
	case "industry":
		baseQuery += `ORDER BY c.industry ` + orderDir + `, c.org_name ASC`
	case "org_name":
		baseQuery += `ORDER BY c.org_name ` + orderDir
	case "assets":
		baseQuery += `ORDER BY asset_count ` + orderDir + `, c.org_name ASC`
	case "as":
		baseQuery += `ORDER BY as_count ` + orderDir + `, c.org_name ASC`
	default:
		if category == "" && industry == "" {
			baseQuery += `ORDER BY
				CASE WHEN c.has_parent=1 AND COALESCE(c.parent_customer_id,'')!='' THEN 0 ELSE 1 END,
				COALESCE((
					SELECT COUNT(*) FROM customers ch
					WHERE ch.parent_customer_id = COALESCE(NULLIF(c.parent_customer_id,''), '__none__')
					  AND ch.is_active=1
				), 0) DESC,
				COALESCE(p.org_name, ''),
				c.org_name`
		} else if industry != "" {
			baseQuery += `ORDER BY c.industry ASC, c.org_name ASC`
		} else {
			baseQuery += `ORDER BY c.org_name ASC`
		}
	}
	if pageSize > 0 {
		baseQuery += ` LIMIT ? OFFSET ?`
		args = append(args, pageSize, offset)
	}

	var total int
	if err := r.db.QueryRow(countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Query(baseQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []model.CustomerListItem
	for rows.Next() {
		var item model.CustomerListItem
		var isActive, hasParent int
		if err := rows.Scan(
			&item.CustomerID, &item.OrgName, &item.OfficialName,
			&item.Industry, &item.MainPhone, &isActive,
			&item.AssetCount, &item.AsCount,
			&item.ParentCustomerID, &item.ParentOrgName, &hasParent,
		); err != nil {
			return nil, 0, err
		}
		item.IsActive = isActive == 1
		item.HasParent = hasParent == 1
		items = append(items, item)
	}
	return items, total, rows.Err()
}

// ListIndustries 업종별 건수 (많은 순)
func (r *CustomerRepo) ListIndustries() ([]model.CustomerIndustryStat, error) {
	rows, err := r.db.Query(`
		SELECT COALESCE(NULLIF(TRIM(industry),''), '') AS ind, COUNT(*) AS cnt
		FROM customers
		WHERE is_active=1
		GROUP BY ind
		ORDER BY CASE WHEN ind='' THEN 1 ELSE 0 END, cnt DESC, ind`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.CustomerIndustryStat
	for rows.Next() {
		var s model.CustomerIndustryStat
		if err := rows.Scan(&s.Industry, &s.Count); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ListCategories 상위기관 카테고리(하위기관 많은 순). 맨앞 전체·맨끝 상위기관없음은 핸들러에서 붙임.
func (r *CustomerRepo) ListCategories() ([]model.CustomerCategory, int, error) {
	rows, err := r.db.Query(`
		SELECT p.customer_id, p.org_name, COUNT(c.customer_id) AS cnt
		FROM customers p
		JOIN customers c ON c.parent_customer_id = p.customer_id AND c.is_active=1
		WHERE p.is_active=1
		GROUP BY p.customer_id
		ORDER BY cnt DESC, p.org_name COLLATE NOCASE`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var cats []model.CustomerCategory
	for rows.Next() {
		var cat model.CustomerCategory
		if err := rows.Scan(&cat.ParentID, &cat.Name, &cat.ChildCount); err != nil {
			return nil, 0, err
		}
		cats = append(cats, cat)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var noneCount int
	_ = r.db.QueryRow(`
		SELECT COUNT(*) FROM customers
		WHERE is_active=1 AND (has_parent=0 OR parent_customer_id IS NULL OR TRIM(parent_customer_id)='')`).Scan(&noneCount)

	return cats, noneCount, nil
}

// GetByID 고객 단건 조회
func (r *CustomerRepo) GetByID(id string) (*model.Customer, error) {
	query := `
		SELECT customer_id, org_name, official_name,
		       COALESCE(org_email,''), COALESCE(main_phone,''),
		       COALESCE(website,''), COALESCE(business_number,''),
		       COALESCE(representative,''), COALESCE(industry,''),
		       has_parent, COALESCE(parent_customer_id,''),
		       COALESCE(postal_code,''), COALESCE(addr_sido,''),
		       COALESCE(addr_sigungu,''), COALESCE(addr_dong,''),
		       COALESCE(address,''), COALESCE(address_detail,''),
		       is_active, COALESCE(notes,''), created_at, updated_at
		FROM customers WHERE customer_id = ?`

	var c model.Customer
	var hasParent, isActive int
	var createdAt, updatedAt string

	err := r.db.QueryRow(query, id).Scan(
		&c.CustomerID, &c.OrgName, &c.OfficialName, &c.OrgEmail,
		&c.MainPhone, &c.Website, &c.BusinessNumber, &c.Representative,
		&c.Industry, &hasParent, &c.ParentCustomerID,
		&c.PostalCode, &c.AddrSido, &c.AddrSigungu, &c.AddrDong,
		&c.Address, &c.AddressDetail,
		&isActive, &c.Notes, &createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.HasParent = hasParent == 1
	c.IsActive = isActive == 1
	c.CreatedAt = parseTime(createdAt)
	c.UpdatedAt = parseTime(updatedAt)
	return &c, nil
}

// Create 고객 등록
func (r *CustomerRepo) Create(c *model.Customer) error {
	id, err := NextCustomerID(r.db, c.MainPhone)
	if err != nil {
		return err
	}
	c.CustomerID = id
	c.SyncCombinedAddress()
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err = r.db.Exec(`
		INSERT INTO customers (
			customer_id, org_name, official_name, org_email, main_phone,
			website, business_number, representative, industry,
			has_parent, parent_customer_id,
			postal_code, addr_sido, addr_sigungu, addr_dong,
			address, address_detail,
			is_active, notes, created_at, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.CustomerID, c.OrgName, c.OfficialName, c.OrgEmail, c.MainPhone,
		c.Website, nullStr(strings.TrimSpace(c.BusinessNumber)), c.Representative, c.Industry,
		boolToInt(c.HasParent), nullStr(c.ParentCustomerID),
		c.PostalCode, c.AddrSido, c.AddrSigungu, c.AddrDong,
		c.Address, c.AddressDetail,
		boolToInt(c.IsActive), c.Notes, now, now,
	)
	return err
}

// Update 고객 수정
func (r *CustomerRepo) Update(c *model.Customer) error {
	c.SyncCombinedAddress()
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := r.db.Exec(`
		UPDATE customers SET
			org_name=?, official_name=?, org_email=?, main_phone=?,
			website=?, business_number=?, representative=?, industry=?,
			has_parent=?, parent_customer_id=?,
			postal_code=?, addr_sido=?, addr_sigungu=?, addr_dong=?,
			address=?, address_detail=?,
			is_active=?, notes=?, updated_at=?
		WHERE customer_id=?`,
		c.OrgName, c.OfficialName, c.OrgEmail, c.MainPhone,
		c.Website, nullStr(strings.TrimSpace(c.BusinessNumber)), c.Representative, c.Industry,
		boolToInt(c.HasParent), nullStr(c.ParentCustomerID),
		c.PostalCode, c.AddrSido, c.AddrSigungu, c.AddrDong,
		c.Address, c.AddressDetail,
		boolToInt(c.IsActive), c.Notes, now, c.CustomerID,
	)
	return err
}

// Delete 고객 비활성화 (실제 삭제 안 함)
func (r *CustomerRepo) Delete(id string) error {
	_, err := r.db.Exec(
		`UPDATE customers SET is_active=0, updated_at=? WHERE customer_id=?`,
		time.Now().Format("2006-01-02 15:04:05"), id,
	)
	return err
}

// ListAll 전체 고객 목록 (드롭다운용)
func (r *CustomerRepo) ListAll() ([]model.Customer, error) {
	rows, err := r.db.Query(
		`SELECT customer_id, org_name FROM customers WHERE is_active=1 ORDER BY org_name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var customers []model.Customer
	for rows.Next() {
		var c model.Customer
		if err := rows.Scan(&c.CustomerID, &c.OrgName); err != nil {
			return nil, err
		}
		customers = append(customers, c)
	}
	return customers, rows.Err()
}

// ListExport 엑셀용 전체 목록 (주소·점검사이트 지역 포함, 페이징 없음)
func (r *CustomerRepo) ListExport(search, category, industry, sort, dir, region, siteID string) ([]model.CustomerListItem, error) {
	items, _, err := r.List(search, category, industry, sort, dir, 1, 0)
	if err != nil {
		return nil, err
	}
	// 주소·사이트지역 보강
	addrMap := map[string]string{}
	sidoMap := map[string]string{}
	siteRegMap := map[string]string{}
	rows, err := r.db.Query(`
		SELECT c.customer_id, COALESCE(c.address,''), COALESCE(c.addr_sido,''), COALESCE(s.region,'')
		FROM customers c
		LEFT JOIN maintenance_site_config s ON s.customer_id = c.customer_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, addr, sido, reg string
		if err := rows.Scan(&id, &addr, &sido, &reg); err != nil {
			return nil, err
		}
		addrMap[id] = addr
		sidoMap[id] = sido
		siteRegMap[id] = reg
	}
	out := make([]model.CustomerListItem, 0, len(items))
	for _, it := range items {
		it.Address = addrMap[it.CustomerID]
		it.AddrSido = sidoMap[it.CustomerID]
		it.SiteRegion = siteRegMap[it.CustomerID]
		if siteID != "" && it.CustomerID != siteID {
			continue
		}
		if region != "" {
			lab := ExtractKoreaRegion(it.AddrSido)
			if lab == "" {
				lab = ExtractKoreaRegion(it.Address)
			}
			if lab == "" {
				lab = ExtractKoreaRegion(it.SiteRegion)
			}
			if lab == "" && strings.TrimSpace(it.SiteRegion) != "" {
				lab = strings.TrimSpace(it.SiteRegion)
			}
			if lab != region {
				continue
			}
		}
		out = append(out, it)
	}
	return out, nil
}

// ListExportRegions 엑셀 필터용 지역 목록
func (r *CustomerRepo) ListExportRegions() ([]string, error) {
	rows, err := r.db.Query(`
		SELECT COALESCE(c.address,''), COALESCE(c.addr_sido,''), COALESCE(s.region,'')
		FROM customers c
		LEFT JOIN maintenance_site_config s ON s.customer_id = c.customer_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var addrs, regs []string
	for rows.Next() {
		var a, sido, reg string
		if err := rows.Scan(&a, &sido, &reg); err != nil {
			return nil, err
		}
		addrs = append(addrs, a, sido)
		if strings.TrimSpace(reg) != "" {
			regs = append(regs, reg)
		}
	}
	return CollectDistinctRegions(addrs, regs), rows.Err()
}

// CountActive 활성 고객 수
func (r *CustomerRepo) CountActive(count *int) {
	r.db.QueryRow(`SELECT COUNT(*) FROM customers WHERE is_active=1`).Scan(count)
}

// ── 유틸 ──────────────────────────────────────────────────────────

func newID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05.999999",
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04",
		"2006-01-02T15:04",
		"2006-01-02",
		time.RFC3339,
		time.RFC3339Nano,
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
		if t, err := time.ParseInLocation(f, s, time.Local); err == nil {
			return t
		}
	}
	return time.Time{}
}
