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
// sort: ""=기본(카테고리), customer_id|org_name|parent|industry|phone|assets|as|status
func (r *CustomerRepo) List(search, category, industry, sort, dir string, page, pageSize int, reviewOnly bool, partyKind string) ([]model.CustomerListItem, int, error) {
	offset := (page - 1) * pageSize

	baseQuery := `
		SELECT c.customer_id, c.org_name, c.official_name,
		       COALESCE(c.industry,''), COALESCE(c.main_phone,''), c.is_active,
		       COUNT(DISTINCT a.asset_id) AS asset_count,
		       COUNT(DISTINCT ar.as_id) AS as_count,
		       COALESCE(c.parent_customer_id,''),
		       COALESCE(p.org_name,''),
		       CASE WHEN c.has_parent=1 AND COALESCE(c.parent_customer_id,'')!='' THEN 1 ELSE 0 END,
		       COALESCE(c.needs_review,0), COALESCE(c.review_reason,''),
		       COALESCE(NULLIF(TRIM(c.party_kind),''),'customer')
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

	if reviewOnly {
		cond := ` AND COALESCE(c.needs_review,0)=1`
		baseQuery += cond
		countQuery += cond
	}

	if pk := strings.TrimSpace(partyKind); pk != "" {
		cond := ` AND COALESCE(NULLIF(TRIM(c.party_kind),''),'customer')=?`
		baseQuery += cond
		countQuery += cond
		args = append(args, pk)
		countArgs = append(countArgs, pk)
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
	case "customer_id":
		baseQuery += `ORDER BY c.customer_id ` + orderDir
	case "org_name":
		baseQuery += `ORDER BY c.org_name COLLATE NOCASE ` + orderDir
	case "parent":
		baseQuery += `ORDER BY COALESCE(p.org_name,'') COLLATE NOCASE ` + orderDir + `, c.org_name ASC`
	case "industry":
		baseQuery += `ORDER BY c.industry COLLATE NOCASE ` + orderDir + `, c.org_name ASC`
	case "phone":
		baseQuery += `ORDER BY COALESCE(c.main_phone,'') ` + orderDir + `, c.org_name ASC`
	case "assets":
		baseQuery += `ORDER BY asset_count ` + orderDir + `, c.org_name ASC`
	case "as":
		baseQuery += `ORDER BY as_count ` + orderDir + `, c.org_name ASC`
	case "status":
		baseQuery += `ORDER BY c.is_active ` + orderDir + `, c.org_name ASC`
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
		var isActive, hasParent, needsReview int
		if err := rows.Scan(
			&item.CustomerID, &item.OrgName, &item.OfficialName,
			&item.Industry, &item.MainPhone, &isActive,
			&item.AssetCount, &item.AsCount,
			&item.ParentCustomerID, &item.ParentOrgName, &hasParent,
			&needsReview, &item.ReviewReason, &item.PartyKind,
		); err != nil {
			return nil, 0, err
		}
		item.IsActive = isActive == 1
		item.HasParent = hasParent == 1
		item.NeedsReview = needsReview == 1
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
	c, err := scanCustomerAPI(r.db.QueryRow(customerAPISelect+` WHERE customer_id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return c, nil
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
			is_active, notes, party_kind, created_at, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.CustomerID, c.OrgName, c.OfficialName, c.OrgEmail, c.MainPhone,
		c.Website, nullStr(strings.TrimSpace(c.BusinessNumber)), c.Representative, c.Industry,
		boolToInt(c.HasParent), nullStr(c.ParentCustomerID),
		c.PostalCode, c.AddrSido, c.AddrSigungu, c.AddrDong,
		c.Address, c.AddressDetail,
		boolToInt(c.IsActive), c.Notes, model.NormalizePartyKind(c.PartyKind), now, now,
	)
	if err != nil {
		return err
	}
	logCreate(r.db, "customers", "customer_id", c.CustomerID, c.OrgName)
	rememberCustomerName(c.CustomerID, c.OrgName)
	return nil
}

// Update 고객 수정
func (r *CustomerRepo) Update(c *model.Customer) error {
	err := touchUpdate(r.db, "customers", "customer_id", c.CustomerID, c.OrgName, func() error {
		c.SyncCombinedAddress()
		now := time.Now().Format("2006-01-02 15:04:05")
		_, err := r.db.Exec(`
		UPDATE customers SET
			org_name=?, official_name=?, org_email=?, main_phone=?,
			website=?, business_number=?, representative=?, industry=?,
			has_parent=?, parent_customer_id=?,
			postal_code=?, addr_sido=?, addr_sigungu=?, addr_dong=?,
			address=?, address_detail=?,
			is_active=?, notes=?, party_kind=?, updated_at=?
		WHERE customer_id=?`,
			c.OrgName, c.OfficialName, c.OrgEmail, c.MainPhone,
			c.Website, nullStr(strings.TrimSpace(c.BusinessNumber)), c.Representative, c.Industry,
			boolToInt(c.HasParent), nullStr(c.ParentCustomerID),
			c.PostalCode, c.AddrSido, c.AddrSigungu, c.AddrDong,
			c.Address, c.AddressDetail,
			boolToInt(c.IsActive), c.Notes, model.NormalizePartyKind(c.PartyKind), now, c.CustomerID,
		)
		return err
	})
	if err == nil {
		rememberCustomerName(c.CustomerID, c.OrgName)
	}
	return err
}

// Delete 고객 비활성화 (실제 삭제 안 함)
func (r *CustomerRepo) Delete(id string) error {
	return touchUpdate(r.db, "customers", "customer_id", id, "고객", func() error {
		_, err := r.db.Exec(
			`UPDATE customers SET is_active=0, updated_at=? WHERE customer_id=?`,
			time.Now().Format("2006-01-02 15:04:05"), id,
		)
		return err
	})
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

// ListForReceipt AS 접수·정기점검용. party_kind=customer 만. §34.2.5
func (r *CustomerRepo) ListForReceipt() ([]model.Customer, error) {
	return r.listForReceipt("")
}

func (r *CustomerRepo) ListForReceiptIncluding(id string) ([]model.Customer, error) {
	return r.listForReceipt(id)
}

func (r *CustomerRepo) listForReceipt(includeID string) ([]model.Customer, error) {
	q := `
		SELECT c.customer_id, c.org_name,
		       COALESCE(cfg.short_name,''), COALESCE(cfg.region,''),
		       COALESCE(NULLIF(TRIM(c.party_kind),''),'customer')
		FROM customers c
		LEFT JOIN maintenance_site_config cfg ON cfg.customer_id = c.customer_id
		WHERE c.is_active=1
		  AND (COALESCE(NULLIF(TRIM(c.party_kind),''),'customer')='customer'`
	args := []interface{}{}
	if includeID = strings.TrimSpace(includeID); includeID != "" {
		q += ` OR c.customer_id=?`
		args = append(args, includeID)
	}
	q += `) ORDER BY c.org_name`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Customer
	for rows.Next() {
		var c model.Customer
		if err := rows.Scan(&c.CustomerID, &c.OrgName, &c.ShortName, &c.Region, &c.PartyKind); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

const customerAPISelect = `
		SELECT customer_id, org_name, official_name,
		       COALESCE(org_email,''), COALESCE(main_phone,''),
		       COALESCE(website,''), COALESCE(business_number,''),
		       COALESCE(representative,''), COALESCE(industry,''),
		       has_parent, COALESCE(parent_customer_id,''),
		       COALESCE(postal_code,''), COALESCE(addr_sido,''),
		       COALESCE(addr_sigungu,''), COALESCE(addr_dong,''),
		       COALESCE(address,''), COALESCE(address_detail,''),
		       is_active, COALESCE(notes,''),
		       COALESCE(needs_review,0), COALESCE(review_reason,''),
		       COALESCE(NULLIF(TRIM(party_kind),''),'customer'),
		       created_at, updated_at
		FROM customers`

func scanCustomerAPI(sc interface{ Scan(...interface{}) error }) (*model.Customer, error) {
	var c model.Customer
	var hasParent, isActive, needsReview int
	var createdAt, updatedAt string
	if err := sc.Scan(
		&c.CustomerID, &c.OrgName, &c.OfficialName, &c.OrgEmail,
		&c.MainPhone, &c.Website, &c.BusinessNumber, &c.Representative,
		&c.Industry, &hasParent, &c.ParentCustomerID,
		&c.PostalCode, &c.AddrSido, &c.AddrSigungu, &c.AddrDong,
		&c.Address, &c.AddressDetail,
		&isActive, &c.Notes, &needsReview, &c.ReviewReason, &c.PartyKind,
		&createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}
	c.HasParent = hasParent == 1
	c.IsActive = isActive == 1
	c.NeedsReview = needsReview == 1
	c.PartyKind = model.NormalizePartyKind(c.PartyKind)
	c.CreatedAt = parseTime(createdAt)
	c.UpdatedAt = parseTime(updatedAt)
	return &c, nil
}

// ListForAPI 영업관리 연동용 고객 목록. search 는 기관명·공식명칭·ID·사업자번호.
func (r *CustomerRepo) ListForAPI(search string, active *bool, limit, offset int) ([]model.Customer, int, error) {
	if limit < 1 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	where := ` WHERE 1=1`
	args := []interface{}{}
	if search = strings.TrimSpace(search); search != "" {
		like := "%" + search + "%"
		where += ` AND (org_name LIKE ? OR official_name LIKE ? OR customer_id LIKE ? OR COALESCE(business_number,'') LIKE ?)`
		args = append(args, like, like, like, like)
	}
	if active != nil {
		where += ` AND is_active=?`
		args = append(args, boolToInt(*active))
	}
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM customers`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	qargs := append(append([]interface{}{}, args...), limit, offset)
	rows, err := r.db.Query(customerAPISelect+where+` ORDER BY org_name LIMIT ? OFFSET ?`, qargs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var items []model.Customer
	for rows.Next() {
		c, err := scanCustomerAPI(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *c)
	}
	if items == nil {
		items = []model.Customer{}
	}
	return items, total, rows.Err()
}

// ListExport 엑셀용 전체 목록 (주소·점검사이트 지역 포함, 페이징 없음)
func (r *CustomerRepo) ListExport(search, category, industry, sort, dir, region, siteID string) ([]model.CustomerListItem, error) {
	items, _, err := r.List(search, category, industry, sort, dir, 1, 0, false, "")
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

// ReviewReasonImportedCustomer AS 엑셀 적재 중 기관명이 매칭되지 않아 새로 만든 고객에 붙는 사유
const ReviewReasonImportedCustomer = "AS 완료내역 엑셀 적재 중 기관명이 기존 고객과 매칭되지 않아 자동 생성됨 — 기관 정보 확인 필요"

// SetNeedsReview 확인 필요 표식을 켜거나 끈다. 끌 때는 사유도 함께 지운다.
func (r *CustomerRepo) SetNeedsReview(id string, on bool, reason string) error {
	if !on {
		reason = ""
	}
	_, err := r.db.Exec(
		`UPDATE customers SET needs_review=?, review_reason=?, updated_at=? WHERE customer_id=?`,
		boolToInt(on), reason, time.Now().Format("2006-01-02 15:04:05"), id,
	)
	return err
}

// CountNeedsReview 확인이 필요한 고객 수
func (r *CustomerRepo) CountNeedsReview() int {
	var n int
	r.db.QueryRow(`SELECT COUNT(*) FROM customers WHERE COALESCE(needs_review,0)=1`).Scan(&n)
	return n
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
