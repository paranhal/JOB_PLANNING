package repository

import (
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"customer-support/internal/model"
)

const (
	ProductCategoryGroup = "stats_product_category"
	spcSeqKey            = "spc_product_category"
)

var (
	ErrProductCategoryDupName   = fmt.Errorf("같은 분류명이 있습니다")
	ErrProductCategoryDupKey    = fmt.Errorf("같은 키가 있습니다")
	ErrProductCategoryKeyLocked = fmt.Errorf("키는 바꿀 수 없습니다")
	ErrProductCategoryInUse     = fmt.Errorf("사용 중인 분류는 삭제할 수 없습니다. 사용중지하십시오")
	ErrProductCategoryMissing   = fmt.Errorf("분류를 찾을 수 없습니다")
)

var productCategorySeeds95_63 = []model.Code{
	{CodeID: "SPC007", CodeValue: "firewall", CodeName: "방화벽", SortOrder: 7},
	{CodeID: "SPC008", CodeValue: "sec_l2_switch", CodeName: "보안L2스위치", SortOrder: 8},
	{CodeID: "SPC009", CodeValue: "l2_switch", CodeName: "L2스위치", SortOrder: 9},
	{CodeID: "SPC010", CodeValue: "aggregation_switch", CodeName: "집선스위치", SortOrder: 10},
	{CodeID: "SPC011", CodeValue: "poe_switch", CodeName: "PoE스위치", SortOrder: 11},
	{CodeID: "SPC012", CodeValue: "indoor", CodeName: "indoor", SortOrder: 12},
}

func applyProductCategory95_63(db *sql.DB) {
	if db == nil {
		return
	}
	for _, c := range productCategorySeeds95_63 {
		_, _ = db.Exec(
			`INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active)
			 VALUES (?,?,?,?,?,1)`,
			c.CodeID, ProductCategoryGroup, c.CodeValue, c.CodeName, c.SortOrder)
	}
	ensureSPCSeqFloor(db)
	reloadCodes(db)
}

func ensureSPCSeqFloor(db *sql.DB) {
	if db == nil {
		return
	}
	floor := 0
	rows, err := db.Query(`SELECT code_id FROM codes WHERE code_group=? AND code_id LIKE 'SPC%'`, ProductCategoryGroup)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			if rows.Scan(&id) != nil {
				continue
			}
			if n := parseSPCNumber(id); n > floor {
				floor = n
			}
		}
	}
	var last int
	err = db.QueryRow(`SELECT last_no FROM id_sequences WHERE seq_key=?`, spcSeqKey).Scan(&last)
	if err == sql.ErrNoRows || last < floor {
		_, _ = db.Exec(`INSERT OR REPLACE INTO id_sequences(seq_key, last_no) VALUES (?,?)`, spcSeqKey, floor)
	}
}

func parseSPCNumber(id string) int {
	id = strings.ToUpper(strings.TrimSpace(id))
	if !strings.HasPrefix(id, "SPC") {
		return 0
	}
	n, err := strconv.Atoi(id[3:])
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func isProductCategoryGroup(g string) bool {
	return strings.TrimSpace(g) == ProductCategoryGroup
}

func normProductCatName(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(s)), " "))
}

func productCategoryList() []model.Code {
	items, ok := codesSnapshot()
	if !ok {
		return nil
	}
	var out []model.Code
	for _, c := range items {
		if c.CodeGroup == ProductCategoryGroup {
			out = append(out, c)
		}
	}
	return out
}

// ProductCategoryLabel 저장 키(또는 분류명) → 표시명. 빈 값은 —.
func ProductCategoryLabel(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "—"
	}
	if c := findProductCategory(s); c != nil {
		return c.CodeName
	}
	return s
}

func findProductCategory(s string) *model.Code {
	key := normProductCatName(s)
	if key == "" {
		return nil
	}
	for _, c := range productCategoryList() {
		if normProductCatName(c.CodeValue) == key || normProductCatName(c.CodeName) == key {
			cp := c
			return &cp
		}
	}
	return nil
}

// ResolveProductCategoryValue 엑셀·폼 입력 → 저장 키.
func ResolveProductCategoryValue(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if c := findProductCategory(s); c != nil {
		return c.CodeValue
	}
	return s
}

func otherProductCategoryName() string {
	if c := findProductCategory("other"); c != nil {
		return c.CodeName
	}
	return "기타"
}

// MapStatsProductCategory 통계 행 표시명. codes 조회 + 제품구분·제품명 보조.
func MapStatsProductCategory(productCategory, productType, productName string) string {
	if c := findProductCategory(productCategory); c != nil {
		return c.CodeName
	}
	if c := findProductCategory(productType); c != nil {
		return c.CodeName
	}
	combined := strings.ToLower(strings.TrimSpace(productType) + " " + strings.TrimSpace(productName) + " " + strings.TrimSpace(productCategory))
	type cand struct {
		name string
		n    int
	}
	var cs []cand
	for _, c := range productCategoryList() {
		if normProductCatName(c.CodeValue) == "other" || c.CodeName == "기타" {
			continue
		}
		cs = append(cs, cand{c.CodeName, len([]rune(c.CodeName))})
		if v := c.CodeValue; v != "" {
			cs = append(cs, cand{v, len([]rune(v))})
		}
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].n > cs[j].n })
	for _, x := range cs {
		needle := strings.ToLower(x.name)
		if needle != "" && strings.Contains(combined, needle) {
			if hit := findProductCategory(x.name); hit != nil {
				return hit.CodeName
			}
		}
	}
	return otherProductCategoryName()
}

func (r *CodeRepo) Get(id string) (*model.Code, error) {
	if r == nil || r.db == nil {
		return nil, ErrProductCategoryMissing
	}
	items, err := scanCodesQuery(r.db, `SELECT code_id, code_group, code_value, code_name, sort_order, is_active
		 FROM codes WHERE code_id=?`, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrProductCategoryMissing
	}
	c := items[0]
	return &c, nil
}

func (r *CodeRepo) ProductCategoryUseCount(value, name string) int {
	if r == nil || r.db == nil {
		return 0
	}
	var n int
	_ = r.db.QueryRow(
		`SELECT COUNT(*) FROM assets WHERE product_category IN (?,?)`,
		strings.TrimSpace(value), strings.TrimSpace(name),
	).Scan(&n)
	return n
}

func (r *CodeRepo) ListProductCategories() ([]model.Code, []int, error) {
	items, err := r.ListByGroup(ProductCategoryGroup)
	if err != nil {
		return nil, nil, err
	}
	uses := make([]int, len(items))
	for i, c := range items {
		uses[i] = r.ProductCategoryUseCount(c.CodeValue, c.CodeName)
	}
	return items, uses, nil
}

func (r *CodeRepo) productCatTaken(name, value, exceptID string) error {
	items, err := r.ListByGroup(ProductCategoryGroup)
	if err != nil {
		return err
	}
	nn, nv := normProductCatName(name), normProductCatName(value)
	for _, it := range items {
		if it.CodeID == exceptID {
			continue
		}
		if nn != "" && normProductCatName(it.CodeName) == nn {
			return ErrProductCategoryDupName
		}
		if nv != "" && normProductCatName(it.CodeValue) == nv {
			return ErrProductCategoryDupKey
		}
	}
	return nil
}

func (r *CodeRepo) CreateProductCategory(c *model.Code) error {
	if r == nil || r.db == nil || c == nil {
		return fmt.Errorf("저장할 수 없습니다")
	}
	c.CodeGroup = ProductCategoryGroup
	c.CodeName = strings.TrimSpace(c.CodeName)
	c.CodeValue = strings.TrimSpace(c.CodeValue)
	if c.CodeName == "" || c.CodeValue == "" {
		return fmt.Errorf("키와 분류명을 입력하세요")
	}
	if err := r.productCatTaken(c.CodeName, c.CodeValue, ""); err != nil {
		return err
	}
	ensureSPCSeqFloor(r.db)
	n, err := NextSeq(r.db, spcSeqKey)
	if err != nil {
		return err
	}
	c.CodeID = fmt.Sprintf("SPC%03d", n)
	if c.SortOrder == 0 {
		_ = r.db.QueryRow(`SELECT COALESCE(MAX(sort_order),0)+1 FROM codes WHERE code_group=?`, ProductCategoryGroup).Scan(&c.SortOrder)
	}
	if _, err := r.db.Exec(
		`INSERT INTO codes (code_id,code_group,code_value,code_name,sort_order,is_active)
		 VALUES (?,?,?,?,?,?)`,
		c.CodeID, c.CodeGroup, c.CodeValue, c.CodeName, c.SortOrder, boolToInt(c.IsActive)); err != nil {
		return err
	}
	logCreate(r.db, "codes", "code_id", c.CodeID, c.CodeName)
	reloadCodes(r.db)
	return nil
}

func (r *CodeRepo) UpdateProductCategory(c *model.Code) error {
	cur, err := r.Get(c.CodeID)
	if err != nil {
		return err
	}
	c.CodeName = strings.TrimSpace(c.CodeName)
	if c.CodeName == "" {
		return fmt.Errorf("분류명을 입력하세요")
	}
	if strings.TrimSpace(c.CodeValue) != "" && normProductCatName(c.CodeValue) != normProductCatName(cur.CodeValue) {
		return ErrProductCategoryKeyLocked
	}
	if err := r.productCatTaken(c.CodeName, cur.CodeValue, cur.CodeID); err != nil {
		return err
	}
	c.CodeGroup = ProductCategoryGroup
	c.CodeValue = cur.CodeValue
	return r.updateCodeRow(c)
}

func (r *CodeRepo) DeactivateProductCategory(id string) error {
	cur, err := r.Get(id)
	if err != nil {
		return err
	}
	cur.IsActive = false
	return r.updateCodeRow(cur)
}

func (r *CodeRepo) DeleteProductCategory(id string) error {
	cur, err := r.Get(id)
	if err != nil {
		return err
	}
	if r.ProductCategoryUseCount(cur.CodeValue, cur.CodeName) > 0 {
		return ErrProductCategoryInUse
	}
	return r.deleteCodeRow(id)
}

func (r *CodeRepo) updateCodeRow(c *model.Code) error {
	err := touchUpdate(r.db, "codes", "code_id", c.CodeID, c.CodeName, func() error {
		_, err := r.db.Exec(
			`UPDATE codes SET code_group=?,code_value=?,code_name=?,sort_order=?,is_active=?
			 WHERE code_id=?`,
			c.CodeGroup, c.CodeValue, c.CodeName, c.SortOrder, boolToInt(c.IsActive), c.CodeID)
		return err
	})
	if err == nil {
		reloadCodes(r.db)
	}
	return err
}
