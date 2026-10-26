package repository

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"customer-support/internal/model"
)

const (
	pgSeqKey                 = "product_group"
	assetProductGroupMetaKey = "__meta:asset_product_group_backfill_v1"
)

var (
	ErrProductGroupInUse      = fmt.Errorf("사용 중인 제품군은 삭제할 수 없습니다. 사용중지하십시오")
	ErrProductGroupMissing    = fmt.Errorf("제품군을 찾을 수 없습니다")
	ErrProductGroupDup        = fmt.Errorf("같은 제품분류·제품구분·세부제품군이 있습니다")
	ErrProductTypeClassLocked = fmt.Errorf("같은 제품구분은 하나의 제품분류에만 속합니다")
)

type productGroupSeed struct {
	ID, Class, Type, Detail, SyncType, SyncCat, Updated string
	Sort                                                int
}

// 시안 16건 글자 그대로. PG-0003 은 전선스위치.
var productGroupSeeds95_64 = []productGroupSeed{
	{"PG-0001", "hw", "네트워크장비", "L2스위치", "NETWORK", "l2_switch", "2025-05-26 00:00:00", 1},
	{"PG-0002", "hw", "네트워크장비", "L3스위치", "NETWORK", "", "2025-05-26 00:00:00", 2},
	{"PG-0003", "hw", "네트워크장비", "전선스위치", "NETWORK", "aggregation_switch", "2025-05-26 00:00:00", 3},
	{"PG-0004", "hw", "네트워크장비", "PoE스위치", "NETWORK", "poe_switch", "2025-05-26 00:00:00", 4},
	{"PG-0005", "hw", "네트워크장비", "Indoor", "NETWORK", "indoor", "2025-05-26 00:00:00", 5},
	{"PG-0006", "hw", "주변장비", "", "PERIPHERAL", "", "2025-05-23 00:00:00", 6},
	{"PG-0007", "hw", "키오스크", "", "KIOSK", "", "2025-05-23 00:00:00", 7},
	{"PG-0008", "hw", "RFID자동화", "", "HW", "rfid", "2025-05-23 00:00:00", 8},
	{"PG-0009", "sw", "홈페이지", "", "SW", "homepage", "2025-05-23 00:00:00", 9},
	{"PG-0010", "sw", "전자도서관", "", "SW", "elibrary", "2025-05-23 00:00:00", 10},
	{"PG-0011", "sw", "모바일", "", "SW", "mobile", "2025-05-23 00:00:00", 11},
	{"PG-0012", "sw", "자료관리", "", "SW", "materials", "2025-05-23 00:00:00", 12},
	{"PG-0013", "security", "보안L2스위치", "", "HW", "sec_l2_switch", "2025-05-23 00:00:00", 13},
	{"PG-0014", "security", "방화벽", "", "HW", "firewall", "2025-05-23 00:00:00", 14},
	{"PG-0015", "security", "DDOS", "", "HW", "", "2025-05-23 00:00:00", 15},
	{"PG-0016", "security", "웹방화벽", "", "HW", "", "2025-05-23 00:00:00", 16},
}

func applyProductGroups95_64(db *sql.DB) {
	if db == nil {
		return
	}
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS product_classes (
		class_id TEXT PRIMARY KEY,
		class_name TEXT NOT NULL,
		sort_order INTEGER DEFAULT 0
	)`)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS product_groups (
		group_id TEXT PRIMARY KEY,
		class_id TEXT NOT NULL,
		type_name TEXT NOT NULL,
		detail_name TEXT NOT NULL DEFAULT '',
		sync_type TEXT NOT NULL DEFAULT '',
		sync_category TEXT NOT NULL DEFAULT '',
		is_active INTEGER NOT NULL DEFAULT 1,
		sort_order INTEGER DEFAULT 0,
		created_at TEXT,
		updated_at TEXT
	)`)
	if _, err := db.Exec(`ALTER TABLE assets ADD COLUMN product_group_id TEXT`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		// 신규 CREATE 에는 컬럼이 없을 수 있어 ALTER 가 정상 경로다.
	}
	_, _ = db.Exec(`INSERT OR IGNORE INTO product_classes(class_id,class_name,sort_order) VALUES
		('hw','HW',1),('sw','SW',2),('security','보안장비',3)`)
	for _, s := range productGroupSeeds95_64 {
		_, _ = db.Exec(`INSERT OR IGNORE INTO product_groups(
			group_id,class_id,type_name,detail_name,sync_type,sync_category,is_active,sort_order,created_at,updated_at)
			VALUES (?,?,?,?,?,?,1,?,?,?)`,
			s.ID, s.Class, s.Type, s.Detail, s.SyncType, s.SyncCat, s.Sort, s.Updated, s.Updated)
	}
	ensurePGSeqFloor(db)
	backfillAssetProductGroups(db)
}

func ensurePGSeqFloor(db *sql.DB) {
	if db == nil {
		return
	}
	floor := 0
	rows, err := db.Query(`SELECT group_id FROM product_groups WHERE group_id LIKE 'PG-%'`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			if rows.Scan(&id) != nil {
				continue
			}
			if n := parsePGNumber(id); n > floor {
				floor = n
			}
		}
	}
	var last int
	err = db.QueryRow(`SELECT last_no FROM id_sequences WHERE seq_key=?`, pgSeqKey).Scan(&last)
	if err == sql.ErrNoRows || last < floor {
		_, _ = db.Exec(`INSERT OR REPLACE INTO id_sequences(seq_key,last_no) VALUES (?,?)`, pgSeqKey, floor)
	}
}

func parsePGNumber(id string) int {
	id = strings.ToUpper(strings.TrimSpace(id))
	if !strings.HasPrefix(id, "PG-") {
		return 0
	}
	n, err := strconv.Atoi(id[3:])
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func FormatProductGroupID(n int) string {
	return fmt.Sprintf("PG-%04d", n)
}

func backfillAssetProductGroups(db *sql.DB) {
	if db == nil || metaDone(db, assetProductGroupMetaKey) {
		return
	}
	if !tableHasColumn(db, "assets", "product_group_id") {
		return
	}
	rows, err := db.Query(`SELECT asset_id, COALESCE(product_type,''), COALESCE(product_category,'')
		FROM assets WHERE TRIM(COALESCE(product_group_id,''))=''`)
	if err != nil {
		return
	}
	defer rows.Close()
	type row struct{ id, typ, cat string }
	var list []row
	for rows.Next() {
		var r row
		if rows.Scan(&r.id, &r.typ, &r.cat) != nil {
			continue
		}
		list = append(list, r)
	}
	r := &ProductGroupRepo{db: db}
	for _, a := range list {
		gid := r.MatchGroup(a.cat, a.typ)
		if gid == "" {
			continue
		}
		_, _ = db.Exec(`UPDATE assets SET product_group_id=? WHERE asset_id=? AND TRIM(COALESCE(product_group_id,''))=''`, gid, a.id)
	}
	markMetaDone(db, assetProductGroupMetaKey)
}

type ProductGroupRepo struct{ db *sql.DB }

func NewProductGroupRepo(db *sql.DB) *ProductGroupRepo { return &ProductGroupRepo{db: db} }

func (r *ProductGroupRepo) Classes() ([]model.ProductClass, error) {
	rows, err := r.db.Query(`SELECT class_id, class_name, sort_order FROM product_classes ORDER BY sort_order, class_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ProductClass
	for rows.Next() {
		var c model.ProductClass
		if err := rows.Scan(&c.ClassID, &c.ClassName, &c.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func (r *ProductGroupRepo) List(classID, q string) ([]model.ProductGroup, error) {
	sqlStr := `
		SELECT g.group_id, g.class_id, c.class_name, g.type_name, g.detail_name,
		       g.sync_type, g.sync_category, g.is_active, g.sort_order, COALESCE(g.updated_at,''),
		       (SELECT COUNT(*) FROM assets a WHERE a.product_group_id=g.group_id) AS n
		FROM product_groups g
		JOIN product_classes c ON c.class_id=g.class_id
		WHERE 1=1`
	args := []interface{}{}
	if classID != "" {
		sqlStr += ` AND g.class_id=?`
		args = append(args, classID)
	}
	if q != "" {
		like := "%" + q + "%"
		sqlStr += ` AND (g.group_id LIKE ? OR g.type_name LIKE ? OR g.detail_name LIKE ? OR c.class_name LIKE ?)`
		args = append(args, like, like, like, like)
	}
	sqlStr += ` ORDER BY g.sort_order, g.group_id`
	rows, err := r.db.Query(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ProductGroup
	for rows.Next() {
		var g model.ProductGroup
		var active int
		if err := rows.Scan(&g.GroupID, &g.ClassID, &g.ClassName, &g.TypeName, &g.DetailName,
			&g.SyncType, &g.SyncCategory, &active, &g.SortOrder, &g.UpdatedAt, &g.AssetCount); err != nil {
			return nil, err
		}
		g.IsActive = active == 1
		out = append(out, g)
	}
	return out, nil
}

func (r *ProductGroupRepo) KPI(items []model.ProductGroup) model.ProductGroupKPI {
	k := model.ProductGroupKPI{Total: len(items)}
	classes := map[string]struct{}{}
	types := map[string]struct{}{}
	for _, g := range items {
		classes[g.ClassID] = struct{}{}
		types[g.ClassID+"\x00"+g.TypeName] = struct{}{}
		if strings.TrimSpace(g.DetailName) == "" {
			k.NoDetail++
		}
	}
	k.Classes = len(classes)
	k.Types = len(types)
	return k
}

func (r *ProductGroupRepo) Get(id string) (*model.ProductGroup, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	var g model.ProductGroup
	var active int
	err := r.db.QueryRow(`
		SELECT g.group_id, g.class_id, c.class_name, g.type_name, g.detail_name,
		       g.sync_type, g.sync_category, g.is_active, g.sort_order, COALESCE(g.updated_at,''),
		       (SELECT COUNT(*) FROM assets a WHERE a.product_group_id=g.group_id)
		FROM product_groups g
		JOIN product_classes c ON c.class_id=g.class_id
		WHERE g.group_id=?`, id).Scan(
		&g.GroupID, &g.ClassID, &g.ClassName, &g.TypeName, &g.DetailName,
		&g.SyncType, &g.SyncCategory, &active, &g.SortOrder, &g.UpdatedAt, &g.AssetCount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	g.IsActive = active == 1
	return &g, nil
}

func (r *ProductGroupRepo) Create(g *model.ProductGroup) error {
	if err := r.prepareWrite(g); err != nil {
		return err
	}
	if strings.TrimSpace(g.GroupID) == "" && g.DetailName != "" {
		if empty := r.emptyDetailRow(g.TypeName); empty != nil {
			g.GroupID = empty.GroupID
			g.ClassID = empty.ClassID
			if g.SyncType == "" {
				g.SyncType = empty.SyncType
			}
			if g.SyncCategory == "" {
				g.SyncCategory = empty.SyncCategory
			}
			g.SortOrder = empty.SortOrder
			g.IsActive = true
			return r.updateRow(g)
		}
	}
	if strings.TrimSpace(g.GroupID) == "" {
		id, err := r.nextID()
		if err != nil {
			return err
		}
		g.GroupID = id
	} else {
		if err := r.bumpSeqTo(parsePGNumber(g.GroupID)); err != nil {
			return err
		}
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	if g.UpdatedAt == "" {
		g.UpdatedAt = now
	}
	_, err := r.db.Exec(`INSERT INTO product_groups(
		group_id,class_id,type_name,detail_name,sync_type,sync_category,is_active,sort_order,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		g.GroupID, g.ClassID, g.TypeName, g.DetailName, g.SyncType, g.SyncCategory,
		boolToInt(g.IsActive), g.SortOrder, now, g.UpdatedAt)
	return err
}

func (r *ProductGroupRepo) Update(g *model.ProductGroup) error {
	if err := r.prepareWrite(g); err != nil {
		return err
	}
	cur, err := r.Get(g.GroupID)
	if err != nil {
		return err
	}
	if cur == nil {
		return ErrProductGroupMissing
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	if cur.TypeName != g.TypeName || cur.ClassID != g.ClassID {
		_, err = r.db.Exec(`UPDATE product_groups SET class_id=?, type_name=?, updated_at=? WHERE type_name=? AND class_id=?`,
			g.ClassID, g.TypeName, now, cur.TypeName, cur.ClassID)
		if err != nil {
			return err
		}
	}
	return r.updateRow(g)
}

func (r *ProductGroupRepo) updateRow(g *model.ProductGroup) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	res, err := r.db.Exec(`UPDATE product_groups SET
		class_id=?, type_name=?, detail_name=?, sync_type=?, sync_category=?,
		is_active=?, sort_order=?, updated_at=?
		WHERE group_id=?`,
		g.ClassID, g.TypeName, g.DetailName, g.SyncType, g.SyncCategory,
		boolToInt(g.IsActive), g.SortOrder, now, g.GroupID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrProductGroupMissing
	}
	g.UpdatedAt = now
	return nil
}

func (r *ProductGroupRepo) Deactivate(id string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	res, err := r.db.Exec(`UPDATE product_groups SET is_active=0, updated_at=? WHERE group_id=?`, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrProductGroupMissing
	}
	return nil
}

func (r *ProductGroupRepo) Delete(id string) error {
	g, err := r.Get(id)
	if err != nil {
		return err
	}
	if g == nil {
		return ErrProductGroupMissing
	}
	if g.AssetCount > 0 {
		return ErrProductGroupInUse
	}
	_, err = r.db.Exec(`DELETE FROM product_groups WHERE group_id=?`, id)
	return err
}

func (r *ProductGroupRepo) prepareWrite(g *model.ProductGroup) error {
	g.ClassID = strings.TrimSpace(g.ClassID)
	g.TypeName = strings.TrimSpace(g.TypeName)
	g.DetailName = strings.TrimSpace(g.DetailName)
	if strings.EqualFold(g.DetailName, "미등록") {
		g.DetailName = ""
	}
	g.SyncType = strings.ToUpper(strings.TrimSpace(g.SyncType))
	g.SyncCategory = strings.TrimSpace(g.SyncCategory)
	if g.ClassID == "" || g.TypeName == "" {
		return fmt.Errorf("제품분류와 제품구분을 입력하세요")
	}
	var other string
	qClass := `SELECT class_id FROM product_groups WHERE type_name=?`
	argsClass := []interface{}{g.TypeName}
	if g.GroupID != "" {
		qClass += ` AND type_name != (SELECT type_name FROM product_groups WHERE group_id=?)`
		argsClass = append(argsClass, g.GroupID)
	}
	_ = r.db.QueryRow(qClass+` LIMIT 1`, argsClass...).Scan(&other)
	if other != "" && other != g.ClassID {
		return ErrProductTypeClassLocked
	}
	var n int
	q := `SELECT COUNT(*) FROM product_groups WHERE class_id=? AND type_name=? AND detail_name=?`
	args := []interface{}{g.ClassID, g.TypeName, g.DetailName}
	if g.GroupID != "" {
		q += ` AND group_id!=?`
		args = append(args, g.GroupID)
	}
	if err := r.db.QueryRow(q, args...).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrProductGroupDup
	}
	if g.SortOrder == 0 {
		_ = r.db.QueryRow(`SELECT COALESCE(MAX(sort_order),0)+1 FROM product_groups`).Scan(&g.SortOrder)
	}
	if !g.IsActive && g.GroupID == "" {
		g.IsActive = true
	}
	return nil
}

func (r *ProductGroupRepo) nextID() (string, error) {
	n, err := NextSeq(r.db, pgSeqKey)
	if err != nil {
		return "", err
	}
	return FormatProductGroupID(n), nil
}

func (r *ProductGroupRepo) bumpSeqTo(n int) error {
	if n <= 0 {
		return nil
	}
	var last int
	err := r.db.QueryRow(`SELECT last_no FROM id_sequences WHERE seq_key=?`, pgSeqKey).Scan(&last)
	if err == sql.ErrNoRows || last < n {
		_, err = r.db.Exec(`INSERT OR REPLACE INTO id_sequences(seq_key,last_no) VALUES (?,?)`, pgSeqKey, n)
		return err
	}
	return err
}

func (r *ProductGroupRepo) emptyDetailRow(typeName string) *model.ProductGroup {
	var id string
	err := r.db.QueryRow(`SELECT group_id FROM product_groups WHERE type_name=? AND TRIM(detail_name)='' LIMIT 1`,
		strings.TrimSpace(typeName)).Scan(&id)
	if err != nil || id == "" {
		return nil
	}
	g, _ := r.Get(id)
	return g
}

func (r *ProductGroupRepo) CountUnmappedAssets() int {
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM assets WHERE TRIM(COALESCE(product_group_id,''))=''`).Scan(&n)
	return n
}

func (r *ProductGroupRepo) BulkDelete(ids []string) (deleted, skipped int, err error) {
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		e := r.Delete(id)
		if e == ErrProductGroupInUse {
			skipped++
			continue
		}
		if e != nil {
			return deleted, skipped, e
		}
		deleted++
	}
	return deleted, skipped, nil
}

// SyncAssetLegacyFromGroup 제품군 → 기존 product_type/product_category. 통계·주간보고·긴급도가 이 값을 쓴다.
func SyncAssetLegacyFromGroup(a *model.Asset, g *model.ProductGroup) {
	if a == nil || g == nil {
		return
	}
	a.ProductGroupID = g.GroupID
	if strings.TrimSpace(g.SyncType) != "" {
		a.ProductType = g.SyncType
	}
	a.ProductCategory = g.SyncCategory
}

// MatchGroup 이름 추론 없이 category 우선, 그다음 type+세부미등록.
func (r *ProductGroupRepo) MatchGroup(category, productType string) string {
	cat := strings.ToLower(strings.TrimSpace(category))
	typ := strings.ToUpper(strings.TrimSpace(productType))
	if cat != "" && cat != "other" {
		ids := r.idsBy(`SELECT group_id FROM product_groups WHERE LOWER(TRIM(sync_category))=? AND is_active=1`, cat)
		if len(ids) == 1 {
			return ids[0]
		}
	}
	if typ == "" {
		return ""
	}
	ids := r.idsBy(`SELECT group_id FROM product_groups WHERE UPPER(TRIM(sync_type))=? AND TRIM(detail_name)='' AND is_active=1`, typ)
	if len(ids) == 1 {
		return ids[0]
	}
	return ""
}

func (r *ProductGroupRepo) idsBy(q string, arg interface{}) []string {
	rows, err := r.db.Query(q, arg)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil && id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func (r *ProductGroupRepo) ApplyToAsset(a *model.Asset) {
	if a == nil {
		return
	}
	if gid := strings.TrimSpace(a.ProductGroupID); gid != "" {
		g, err := r.Get(gid)
		if err != nil || g == nil {
			return
		}
		SyncAssetLegacyFromGroup(a, g)
		return
	}
	a.ProductGroupID = r.MatchGroup(a.ProductCategory, a.ProductType)
	if gid := strings.TrimSpace(a.ProductGroupID); gid != "" {
		g, _ := r.Get(gid)
		if g != nil {
			SyncAssetLegacyFromGroup(a, g)
		}
	}
}

func (r *ProductGroupRepo) ResolveImportCell(cell, productType, productCategory string) string {
	cell = strings.TrimSpace(cell)
	if strings.HasPrefix(strings.ToUpper(cell), "PG-") {
		if g, _ := r.Get(cell); g != nil {
			return g.GroupID
		}
	}
	if cell != "" {
		if id := r.MatchGroup(cell, ""); id != "" {
			return id
		}
		if id := r.findUniqueDetail(cell); id != "" {
			return id
		}
		if empty := r.emptyDetailRow(cell); empty != nil {
			return empty.GroupID
		}
	}
	return r.MatchGroup(productCategory, productType)
}

func (r *ProductGroupRepo) findUniqueDetail(name string) string {
	ids := r.idsBy(`SELECT group_id FROM product_groups WHERE detail_name=? AND is_active=1`, strings.TrimSpace(name))
	if len(ids) == 1 {
		return ids[0]
	}
	return ""
}

func (r *ProductGroupRepo) FindClass(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	var id string
	_ = r.db.QueryRow(`SELECT class_id FROM product_classes WHERE class_id=? OR class_name=? COLLATE NOCASE LIMIT 1`,
		strings.ToLower(name), name).Scan(&id)
	if id != "" {
		return id
	}
	switch strings.ToUpper(name) {
	case "HW":
		return "hw"
	case "SW":
		return "sw"
	case "보안장비", "SECURITY":
		return "security"
	}
	return ""
}

func (r *ProductGroupRepo) ImportRow(groupID, className, typeName, detail string) (string, error) {
	g := &model.ProductGroup{
		GroupID:    strings.TrimSpace(groupID),
		ClassID:    r.FindClass(className),
		TypeName:   strings.TrimSpace(typeName),
		DetailName: strings.TrimSpace(detail),
		IsActive:   true,
	}
	if g.ClassID == "" {
		return "", fmt.Errorf("제품분류를 알 수 없습니다")
	}
	if existing, _ := r.findDup(g.ClassID, g.TypeName, g.DetailName, g.GroupID); existing != nil && g.GroupID == "" {
		return existing.GroupID, nil
	}
	if g.GroupID != "" {
		cur, _ := r.Get(g.GroupID)
		if cur != nil {
			g.SyncType = cur.SyncType
			g.SyncCategory = cur.SyncCategory
			g.SortOrder = cur.SortOrder
			g.IsActive = true
			return g.GroupID, r.Update(g)
		}
	}
	if err := r.Create(g); err != nil {
		return "", err
	}
	return g.GroupID, nil
}

func (r *ProductGroupRepo) findDup(classID, typeName, detail, exceptID string) (*model.ProductGroup, error) {
	if strings.EqualFold(strings.TrimSpace(detail), "미등록") {
		detail = ""
	}
	detail = strings.TrimSpace(detail)
	var id string
	q := `SELECT group_id FROM product_groups WHERE class_id=? AND type_name=? AND detail_name=?`
	args := []interface{}{classID, strings.TrimSpace(typeName), detail}
	if exceptID != "" {
		q += ` AND group_id!=?`
		args = append(args, exceptID)
	}
	err := r.db.QueryRow(q+" LIMIT 1", args...).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r.Get(id)
}
