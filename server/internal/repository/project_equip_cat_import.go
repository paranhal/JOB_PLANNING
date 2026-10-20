package repository

import (
	"bytes"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

const (
	equipCatImportMaxBytes  = 2 << 20
	equipCatImportMaxCats   = 500
	equipCatImportMaxFields = 2000
	catSheetCats            = "분류"
	catSheetFields          = "추가항목"
	catSheetGuide           = "안내"
	catSheetSample          = "예시"
)

var (
	catCodeRe   = regexp.MustCompile(`^[A-Z0-9_]{1,30}$`)
	catFieldRe  = regexp.MustCompile(`^[a-z0-9_]{1,30}$`)
	catHdrCats  = []string{"조직코드", "분류코드", "분류명", "상위분류코드", "정렬", "사용"}
	catHdrField = []string{"조직코드", "분류코드", "필드키", "항목명", "형식", "선택값", "필수", "정렬"}
)

func ExcelFormulaInjected(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	switch s[0] {
	case '=', '+', '@':
		return true
	case '-':
		if _, err := strconv.Atoi(s); err == nil {
			return false
		}
		return true
	}
	return false
}

func ParseSelectOptions(s string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range strings.Split(s, "|") {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func JoinSelectOptions(parts []string) string {
	return strings.Join(parts, "|")
}

func ParseEquipCatWorkbook(raw []byte) (cats [][]string, fields [][]string, err error) {
	if len(raw) > equipCatImportMaxBytes {
		return nil, nil, fmt.Errorf("파일은 2MB 이하여야 합니다")
	}
	if len(raw) == 0 {
		return nil, nil, fmt.Errorf("빈 파일입니다")
	}
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		return nil, nil, fmt.Errorf("엑셀을 열 수 없습니다")
	}
	defer f.Close()
	cats, err = sheetRows(f, catSheetCats)
	if err != nil {
		return nil, nil, err
	}
	fields, err = sheetRows(f, catSheetFields)
	if err != nil {
		return nil, nil, err
	}
	if err := matchCatHeaders(catSheetCats, cats, catHdrCats); err != nil {
		return nil, nil, err
	}
	if err := matchCatHeaders(catSheetFields, fields, catHdrField); err != nil {
		return nil, nil, err
	}
	catN, fieldN := countNonEmpty(cats, 6, []int{0, 1, 2}), countNonEmpty(fields, 8, []int{0, 1, 2})
	if catN > equipCatImportMaxCats {
		return nil, nil, fmt.Errorf("분류는 회당 %d행까지입니다", equipCatImportMaxCats)
	}
	if fieldN > equipCatImportMaxFields {
		return nil, nil, fmt.Errorf("추가항목은 회당 %d행까지입니다", equipCatImportMaxFields)
	}
	if catN == 0 && fieldN == 0 {
		return nil, nil, fmt.Errorf("올릴 행이 없습니다")
	}
	return cats, fields, nil
}

func countNonEmpty(rows [][]string, minCols int, keys []int) int {
	n := 0
	for i, row := range rows {
		if i == 0 {
			continue
		}
		if catRowEmpty(row, keys) {
			continue
		}
		n++
		_ = minCols
	}
	return n
}

func catRowEmpty(row []string, keys []int) bool {
	for _, i := range keys {
		if strings.TrimSpace(cell(row, i)) != "" {
			return false
		}
	}
	return true
}

func matchCatHeaders(sheet string, rows [][]string, want []string) error {
	if len(rows) == 0 {
		return fmt.Errorf("시트 %s 헤더가 없습니다", sheet)
	}
	got := rows[0]
	var gnorm, wnorm []string
	for _, w := range want {
		wnorm = append(wnorm, strings.ToLower(strings.TrimSpace(w)))
	}
	for i := 0; i < len(want); i++ {
		gnorm = append(gnorm, strings.ToLower(strings.TrimSpace(cell(got, i))))
	}
	if len(got) < len(want) {
		return fmt.Errorf("시트 %s 헤더가 다릅니다(기대 %s / 실제 %s)", sheet, strings.Join(want, ","), strings.Join(trimRow(got), ","))
	}
	for i := range want {
		if gnorm[i] != wnorm[i] {
			return fmt.Errorf("시트 %s 헤더가 다릅니다(기대 %s / 실제 %s)", sheet, strings.Join(want, ","), strings.Join(trimRow(got), ","))
		}
	}
	return nil
}

func trimRow(row []string) []string {
	out := make([]string, 0, len(row))
	for _, v := range row {
		out = append(out, strings.TrimSpace(v))
	}
	return out
}

type CatImportScope struct {
	Allowed map[string]bool
}

func (r *ProjectEquipRepo) PreviewCategoryImport(raw []byte, fileName string, scope CatImportScope) (*model.EquipCatImportPreview, error) {
	catRows, fieldRows, err := ParseEquipCatWorkbook(raw)
	if err != nil {
		return nil, err
	}
	out := &model.EquipCatImportPreview{FileName: fileName}
	fileCats := map[string]*model.EquipCatImportRow{}
	seenCat := map[string]int{}
	for i, row := range catRows {
		if i == 0 || catRowEmpty(row, []int{0, 1, 2}) {
			continue
		}
		ir := r.parseCatSheetRow(i+1, row, scope)
		key := ir.OrgID + "/" + ir.Code
		if ir.Action != "err" {
			if prev, ok := seenCat[key]; ok {
				ir.Action, ir.Msg = "err", fmt.Sprintf("같은 파일에 분류가 중복입니다(%d행)", prev)
			} else {
				seenCat[key] = ir.Line
				cp := ir
				fileCats[key] = &cp
			}
		}
		out.Rows = append(out.Rows, ir)
	}
	for i := range out.Rows {
		row := &out.Rows[i]
		if row.Kind != "cat" || row.Action == "err" {
			continue
		}
		r.resolveCatParent(row, fileCats)
		if row.Action != "err" {
			r.classifyCatRow(row)
		}
	}
	seenField := map[string]int{}
	for i, row := range fieldRows {
		if i == 0 || catRowEmpty(row, []int{0, 1, 2}) {
			continue
		}
		ir := r.parseFieldSheetRow(i+1, row, scope, fileCats)
		if ir.Action != "err" {
			key := ir.OrgID + "/" + ir.Code + "/" + ir.FieldKey
			if prev, ok := seenField[key]; ok {
				ir.Action, ir.Msg = "err", fmt.Sprintf("같은 파일에 추가항목이 중복입니다(%d행)", prev)
			} else {
				seenField[key] = ir.Line
			}
		}
		if ir.Action != "err" {
			r.classifyFieldRow(&ir, fileCats)
		}
		out.Rows = append(out.Rows, ir)
	}
	for _, ir := range out.Rows {
		tallyCatPreview(out, ir)
	}
	orgs := map[string]bool{}
	for _, ir := range out.Rows {
		if ir.OrgID != "" && ir.OrgID != OrgAll {
			orgs[ir.OrgID] = true
		}
	}
	var scopeIDs []string
	for id := range orgs {
		scopeIDs = append(scopeIDs, id)
	}
	out.OrgScope = strings.Join(scopeIDs, ",")
	return out, nil
}

func tallyCatPreview(out *model.EquipCatImportPreview, ir model.EquipCatImportRow) {
	switch ir.Action {
	case "new":
		out.NewN++
	case "upd":
		out.UpdN++
	case "same":
		out.SameN++
	default:
		out.ErrN++
	}
	if strings.Contains(ir.Msg, "경고") {
		out.WarnN++
	}
}

func (r *ProjectEquipRepo) parseCatSheetRow(line int, row []string, scope CatImportScope) model.EquipCatImportRow {
	ir := model.EquipCatImportRow{Sheet: catSheetCats, Line: line, Kind: "cat"}
	vals := []string{cell(row, 0), cell(row, 1), cell(row, 2), cell(row, 3), cell(row, 4), cell(row, 5)}
	for _, v := range vals {
		if ExcelFormulaInjected(v) {
			ir.Action, ir.Msg = "err", "엑셀 수식 주입이 의심되는 값입니다"
			return ir
		}
	}
	org := strings.TrimSpace(vals[0])
	code := strings.TrimSpace(vals[1])
	name := strings.TrimSpace(vals[2])
	parent := strings.TrimSpace(vals[3])
	sortRaw := strings.TrimSpace(vals[4])
	useRaw := strings.TrimSpace(vals[5])
	ir.OrgID, ir.Code, ir.Name, ir.ParentCode = org, code, name, parent
	ir.Key = org + "/" + code
	if org == "" || org == OrgAll {
		ir.Action, ir.Msg = "err", "조직코드가 없습니다"
		return ir
	}
	if !r.orgActive(org) {
		ir.Action, ir.Msg = "err", "없는 조직입니다"
		return ir
	}
	if scope.Allowed == nil || !scope.Allowed[org] {
		ir.Action, ir.Msg = "err", "쓸 수 없는 조직입니다"
		return ir
	}
	if name == "" {
		ir.Action, ir.Msg = "err", "분류명은 필수입니다"
		return ir
	}
	if !catCodeRe.MatchString(code) {
		ir.Action, ir.Msg = "err", "분류코드 형식이 아닙니다"
		return ir
	}
	active, err := parseYN(useRaw, true)
	if err != nil {
		ir.Action, ir.Msg = "err", err.Error()
		return ir
	}
	sort, err := parseSort(sortRaw, line)
	if err != nil {
		ir.Action, ir.Msg = "err", err.Error()
		return ir
	}
	ir.IsActive, ir.SortOrder = active, sort
	if parent == code {
		ir.Action, ir.Msg = "err", "자기 자신을 상위로 둘 수 없습니다"
		return ir
	}
	ir.Cat = &model.EquipmentCategory{
		OrgID: org, Code: code, Name: name, SortOrder: sort, IsActive: active,
	}
	return ir
}

func (r *ProjectEquipRepo) resolveCatParent(ir *model.EquipCatImportRow, fileCats map[string]*model.EquipCatImportRow) {
	if ir.ParentCode == "" {
		return
	}
	if !catCodeRe.MatchString(ir.ParentCode) {
		ir.Action, ir.Msg = "err", "상위분류코드 형식이 아닙니다"
		return
	}
	pkey := ir.OrgID + "/" + ir.ParentCode
	if fr, ok := fileCats[pkey]; ok {
		if fr.Action == "err" {
			ir.Action, ir.Msg = "err", "상위 분류 행이 오류입니다"
			return
		}
		if fr.ParentCode != "" {
			ir.Action, ir.Msg = "err", "분류는 2단계까지입니다"
			return
		}
		return
	}
	p, err := r.CategoryByCode(ir.OrgID, ir.ParentCode)
	if err != nil || p == nil {
		ir.Action, ir.Msg = "err", "상위 분류가 없습니다"
		return
	}
	if p.OrgID != ir.OrgID {
		ir.Action, ir.Msg = "err", "상위 분류 조직이 다릅니다"
		return
	}
	if p.ParentID != "" {
		ir.Action, ir.Msg = "err", "분류는 2단계까지입니다"
		return
	}
}

func (r *ProjectEquipRepo) classifyCatRow(ir *model.EquipCatImportRow) {
	ex, err := r.CategoryByCode(ir.OrgID, ir.Code)
	if err != nil {
		ir.Action, ir.Msg = "err", err.Error()
		return
	}
	if ex == nil {
		ir.Action = "new"
		return
	}
	ir.Cat.ID = ex.ID
	parentID := ""
	if ir.ParentCode != "" {
		p, _ := r.CategoryByCode(ir.OrgID, ir.ParentCode)
		if p != nil {
			parentID = p.ID
		}
	}
	same := ex.Name == ir.Name && ex.ParentID == parentID && ex.SortOrder == ir.SortOrder && ex.IsActive == ir.IsActive
	if same {
		ir.Action = "same"
		return
	}
	ir.Action = "upd"
}

func (r *ProjectEquipRepo) parseFieldSheetRow(line int, row []string, scope CatImportScope, fileCats map[string]*model.EquipCatImportRow) model.EquipCatImportRow {
	ir := model.EquipCatImportRow{Sheet: catSheetFields, Line: line, Kind: "field"}
	vals := []string{cell(row, 0), cell(row, 1), cell(row, 2), cell(row, 3), cell(row, 4), cell(row, 5), cell(row, 6), cell(row, 7)}
	for i, v := range vals {
		if i == 5 {
			for _, p := range strings.Split(v, "|") {
				if ExcelFormulaInjected(p) {
					ir.Action, ir.Msg = "err", "엑셀 수식 주입이 의심되는 값입니다"
					return ir
				}
			}
			continue
		}
		if ExcelFormulaInjected(v) {
			ir.Action, ir.Msg = "err", "엑셀 수식 주입이 의심되는 값입니다"
			return ir
		}
	}
	org := strings.TrimSpace(vals[0])
	code := strings.TrimSpace(vals[1])
	fkey := strings.TrimSpace(vals[2])
	label := strings.TrimSpace(vals[3])
	typ := strings.ToLower(strings.TrimSpace(vals[4]))
	optRaw := strings.TrimSpace(vals[5])
	reqRaw := strings.TrimSpace(vals[6])
	sortRaw := strings.TrimSpace(vals[7])
	ir.OrgID, ir.Code, ir.FieldKey, ir.Label, ir.FieldType = org, code, fkey, label, typ
	ir.Key = org + "/" + code + "/" + fkey
	if org == "" || org == OrgAll {
		ir.Action, ir.Msg = "err", "조직코드가 없습니다"
		return ir
	}
	if !r.orgActive(org) {
		ir.Action, ir.Msg = "err", "없는 조직입니다"
		return ir
	}
	if scope.Allowed == nil || !scope.Allowed[org] {
		ir.Action, ir.Msg = "err", "쓸 수 없는 조직입니다"
		return ir
	}
	if !catCodeRe.MatchString(code) {
		ir.Action, ir.Msg = "err", "분류코드 형식이 아닙니다"
		return ir
	}
	if !catFieldRe.MatchString(fkey) {
		ir.Action, ir.Msg = "err", "필드키 형식이 아닙니다"
		return ir
	}
	if label == "" {
		ir.Action, ir.Msg = "err", "항목명은 필수입니다"
		return ir
	}
	switch typ {
	case model.EquipFieldText, model.EquipFieldNumber, model.EquipFieldDate, model.EquipFieldSelect:
	default:
		ir.Action, ir.Msg = "err", "형식을 확인하세요"
		return ir
	}
	opts := ParseSelectOptions(optRaw)
	if typ == model.EquipFieldSelect {
		if len(opts) == 0 {
			ir.Action, ir.Msg = "err", "select 는 선택값이 필요합니다"
			return ir
		}
		ir.Options = JoinSelectOptions(opts)
	} else if optRaw != "" {
		ir.Msg = "경고: 선택값은 select가 아니면 무시합니다"
		ir.Options = ""
	}
	req, err := parseYN(reqRaw, false)
	if err != nil {
		ir.Action, ir.Msg = "err", err.Error()
		return ir
	}
	sort, err := parseSort(sortRaw, line)
	if err != nil {
		ir.Action, ir.Msg = "err", err.Error()
		return ir
	}
	ir.Required, ir.SortOrder = req, sort
	ckey := org + "/" + code
	if fr, ok := fileCats[ckey]; ok && fr.Action == "err" {
		ir.Action, ir.Msg = "err", "분류 행이 오류입니다"
		return ir
	}
	ir.Field = &model.EquipmentField{
		FieldKey: fkey, Label: label, FieldType: typ, Options: ir.Options,
		Required: req, SortOrder: sort, IsActive: true,
	}
	return ir
}

func (r *ProjectEquipRepo) classifyFieldRow(ir *model.EquipCatImportRow, fileCats map[string]*model.EquipCatImportRow) {
	cat, _ := r.CategoryByCode(ir.OrgID, ir.Code)
	if cat == nil {
		if _, ok := fileCats[ir.OrgID+"/"+ir.Code]; ok {
			ir.Action = "new"
			return
		}
		ir.Action, ir.Msg = "err", "분류가 없습니다"
		return
	}
	f, err := r.fieldByKey(cat.ID, ir.FieldKey)
	if err != nil {
		ir.Action, ir.Msg = "err", err.Error()
		return
	}
	if f == nil {
		ir.Action = "new"
		ir.Field.CategoryID = cat.ID
		return
	}
	ir.Field.ID = f.ID
	ir.Field.CategoryID = cat.ID
	if f.InUse && f.FieldType != ir.FieldType {
		ir.Action, ir.Msg = "err", "사용 중인 항목의 형식은 바꿀 수 없습니다"
		return
	}
	oldOpts := ParseSelectOptions(strings.ReplaceAll(f.Options, "\n", "|"))
	newOpts := ParseSelectOptions(ir.Options)
	if f.InUse && ir.FieldType == model.EquipFieldSelect && optionsRemoved(oldOpts, newOpts) {
		if ir.Msg != "" {
			ir.Msg += "; "
		}
		ir.Msg += "경고: 선택값을 빼도 기존 장비 값은 유지됩니다"
	}
	same := f.Label == ir.Label && f.FieldType == ir.FieldType && JoinSelectOptions(oldOpts) == JoinSelectOptions(newOpts) &&
		f.Required == ir.Required && f.SortOrder == ir.SortOrder && f.IsActive
	if same {
		if strings.Contains(ir.Msg, "선택값은 select가 아니면") {
			ir.Msg = strings.TrimSpace(strings.ReplaceAll(ir.Msg, "경고: 선택값은 select가 아니면 무시합니다", ""))
		}
		if ir.Msg == "" {
			ir.Action = "same"
			return
		}
	}
	if ir.Action != "err" {
		ir.Action = "upd"
	}
}

func optionsRemoved(old, neu []string) bool {
	have := map[string]bool{}
	for _, n := range neu {
		have[n] = true
	}
	for _, o := range old {
		if !have[o] {
			return true
		}
	}
	return false
}

func (r *ProjectEquipRepo) fieldByKey(categoryID, key string) (*model.EquipmentField, error) {
	fs, err := r.ListFields(categoryID, true)
	if err != nil {
		return nil, err
	}
	for i := range fs {
		if fs[i].FieldKey == key {
			return &fs[i], nil
		}
	}
	return nil, nil
}

func (r *ProjectEquipRepo) orgActive(id string) bool {
	if id == "" || id == OrgAll {
		return false
	}
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM orgs WHERE org_id=? AND is_active=1`, id).Scan(&n)
	return n == 1
}

func (r *ProjectEquipRepo) ListActiveOrgIDs() ([]string, error) {
	rows, err := r.db.Query(`SELECT org_id FROM orgs WHERE is_active=1 AND org_id!='' AND org_id!='*' ORDER BY sort_order, org_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func parseYN(s string, defTrue bool) (bool, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return defTrue, nil
	}
	switch s {
	case "Y", "YES", "1", "예":
		return true, nil
	case "N", "NO", "0", "아니오", "아니":
		return false, nil
	default:
		return false, fmt.Errorf("Y/N 만 쓸 수 있습니다")
	}
}

func parseSort(s string, line int) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return line, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("정렬은 정수여야 합니다")
	}
	return n, nil
}

func (r *ProjectEquipRepo) ApplyCategoryImport(userID, fileName string, raw []byte, scope CatImportScope) (*model.EquipCatImportPreview, error) {
	prev, err := r.PreviewCategoryImport(raw, fileName, scope)
	if err != nil {
		return nil, err
	}
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	ids := map[string]string{}
	ok, skip := 0, 0
	applyOne := func(row *model.EquipCatImportRow) {
		if row.Action == "err" {
			skip++
			return
		}
		if row.Action == "same" {
			if row.Kind == "cat" {
				if ex, _ := r.CategoryByCode(row.OrgID, row.Code); ex != nil {
					ids[row.OrgID+"/"+row.Code] = ex.ID
				}
			}
			ok++
			return
		}
		var err error
		if row.Kind == "cat" {
			err = r.applyCatRowTx(tx, row, ids)
		} else if row.Kind == "field" {
			err = r.applyFieldRowTx(tx, row, ids)
		}
		if err != nil {
			row.Action, row.Msg = "err", err.Error()
			skip++
			return
		}
		ok++
	}
	for i := range prev.Rows {
		row := &prev.Rows[i]
		if row.Kind == "cat" && row.ParentCode == "" {
			applyOne(row)
		}
	}
	for i := range prev.Rows {
		row := &prev.Rows[i]
		if row.Kind == "cat" && row.ParentCode != "" {
			applyOne(row)
		}
	}
	for i := range prev.Rows {
		row := &prev.Rows[i]
		if row.Kind == "field" {
			applyOne(row)
		}
	}
	prev.ErrN = skip
	batch := &model.EquipCatImportBatch{
		OrgScope: prev.OrgScope, FileName: fileName, RowsTotal: len(prev.Rows),
		RowsOK: ok, RowsError: skip, Status: "applied", CreatedBy: userID,
	}
	if err := r.recordCatBatchTx(tx, batch); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	logCreateWithReason(r.db, "equipment_category_import_batches", "id", batch.ID, fileName,
		fmt.Sprintf("장비분류표 엑셀 적용 %d건(오류 %d)", ok, skip))
	return prev, nil
}

func (r *ProjectEquipRepo) applyCatRowTx(tx *sql.Tx, ir *model.EquipCatImportRow, ids map[string]string) error {
	parentID := ""
	if ir.ParentCode != "" {
		if id, ok := ids[ir.OrgID+"/"+ir.ParentCode]; ok {
			parentID = id
		} else if p, _ := r.CategoryByCode(ir.OrgID, ir.ParentCode); p != nil {
			parentID = p.ID
		} else {
			return fmt.Errorf("상위 분류가 없습니다")
		}
	}
	active := 1
	if !ir.IsActive {
		active = 0
	}
	ex, err := r.CategoryByCode(ir.OrgID, ir.Code)
	if err != nil {
		return err
	}
	if ex == nil {
		n, err := nextSeqTx(tx, "equip_cat")
		if err != nil {
			return err
		}
		id := fmt.Sprintf("EC-%04d", n)
		_, err = tx.Exec(`INSERT INTO equipment_categories(id,org_id,parent_id,code,name,sort_order,is_active) VALUES(?,?,?,?,?,?,?)`,
			id, ir.OrgID, nullIfEmpty(parentID), ir.Code, ir.Name, ir.SortOrder, active)
		if err != nil {
			return err
		}
		ids[ir.OrgID+"/"+ir.Code] = id
		if ir.Cat != nil {
			ir.Cat.ID = id
		}
		return nil
	}
	ids[ir.OrgID+"/"+ir.Code] = ex.ID
	_, err = tx.Exec(`UPDATE equipment_categories SET parent_id=?, name=?, sort_order=?, is_active=? WHERE id=? AND org_id=?`,
		nullIfEmpty(parentID), ir.Name, ir.SortOrder, active, ex.ID, ir.OrgID)
	if ir.Cat != nil {
		ir.Cat.ID = ex.ID
	}
	return err
}

func (r *ProjectEquipRepo) applyFieldRowTx(tx *sql.Tx, ir *model.EquipCatImportRow, ids map[string]string) error {
	catID := ids[ir.OrgID+"/"+ir.Code]
	if catID == "" {
		cat, err := r.CategoryByCode(ir.OrgID, ir.Code)
		if err != nil {
			return err
		}
		if cat == nil {
			return fmt.Errorf("분류가 없습니다")
		}
		catID = cat.ID
	}
	cat := &model.EquipmentCategory{ID: catID}
	req, active := 0, 1
	if ir.Required {
		req = 1
	}
	f, err := r.fieldByKey(cat.ID, ir.FieldKey)
	if err != nil {
		return err
	}
	if f == nil {
		n, err := nextSeqTx(tx, "equip_field")
		if err != nil {
			return err
		}
		id := fmt.Sprintf("EF-%04d", n)
		_, err = tx.Exec(`INSERT INTO equipment_category_fields(id,category_id,field_key,label,field_type,options,required,sort_order,is_active)
			VALUES(?,?,?,?,?,?,?,?,?)`, id, cat.ID, ir.FieldKey, ir.Label, ir.FieldType, ir.Options, req, ir.SortOrder, active)
		return err
	}
	if f.InUse && f.FieldType != ir.FieldType {
		return fmt.Errorf("사용 중인 항목의 형식은 바꿀 수 없습니다")
	}
	_, err = tx.Exec(`UPDATE equipment_category_fields SET label=?, field_type=?, options=?, required=?, sort_order=?, is_active=? WHERE id=?`,
		ir.Label, ir.FieldType, ir.Options, req, ir.SortOrder, active, f.ID)
	return err
}

func (r *ProjectEquipRepo) recordCatBatchTx(tx *sql.Tx, b *model.EquipCatImportBatch) error {
	n, err := nextSeqTx(tx, "equip_cat_import")
	if err != nil {
		return err
	}
	b.ID = fmt.Sprintf("CB-%04d", n)
	_, err = tx.Exec(`INSERT INTO equipment_category_import_batches(id,org_scope,file_name,rows_total,rows_ok,rows_error,status,created_by)
		VALUES(?,?,?,?,?,?,?,?)`, b.ID, b.OrgScope, b.FileName, b.RowsTotal, b.RowsOK, b.RowsError, b.Status, b.CreatedBy)
	return err
}

func ynText(v bool) string {
	if v {
		return "Y"
	}
	return "N"
}

func BuildEquipCatWorkbook(guide []string, cats, fields, sample [][]string) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	_ = f.SetSheetName(f.GetSheetName(0), catSheetGuide)
	_, _ = f.NewSheet(catSheetCats)
	_, _ = f.NewSheet(catSheetFields)
	_, _ = f.NewSheet(catSheetSample)
	hdrFill, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 11, Family: "맑은 고딕", Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"1E3A5F"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	putCol := func(sheet string, rows [][]string) {
		for i, row := range rows {
			for j, v := range row {
				cell, _ := excelize.CoordinatesToCellName(j+1, i+1)
				_ = f.SetCellValue(sheet, cell, v)
			}
		}
		if len(rows) > 0 && len(rows[0]) > 0 {
			last, _ := excelize.ColumnNumberToName(len(rows[0]))
			_ = f.SetCellStyle(sheet, "A1", last+"1", hdrFill)
			_ = f.SetRowHeight(sheet, 1, 22)
		}
	}
	for i, line := range guide {
		_ = f.SetCellValue(catSheetGuide, fmt.Sprintf("A%d", i+1), line)
	}
	putCol(catSheetCats, cats)
	putCol(catSheetFields, fields)
	putCol(catSheetSample, sample)
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func EquipCatGuideLines() []string {
	return []string{
		"장비 분류표 업로드 양식",
		"",
		"1. 「분류」 시트: 장비 종류(대분류·소분류)를 한 줄에 하나씩 적습니다.",
		"2. 「추가항목」 시트: 분류별로 따로 받을 항목이 있을 때만 적습니다. 없으면 비워 두세요.",
		"3. 「예시」 시트는 작성 형식을 보여 주는 참고용입니다. 업로드되지 않습니다.",
		"4. 헤더(첫 줄)는 바꾸지 마세요. 시트 이름도 바꾸지 마세요.",
	}
}

func EquipCatSampleRows() [][]string {
	return [][]string{
		{"※ 작성 형식 참고용 예시입니다. 업로드되지 않습니다."},
		{},
		{"조직코드", "분류코드", "분류명", "상위분류코드", "정렬", "사용"},
		{"O02", "EX_MAIN", "(예시) 대분류", "", "1", "Y"},
		{"O02", "EX_SUB", "(예시) 소분류", "EX_MAIN", "1", "Y"},
		{},
		{"조직코드", "분류코드", "필드키", "항목명", "형식", "선택값", "필수", "정렬"},
		{"O02", "EX_SUB", "resolution", "(예시) 해상도", "select", "4K|FHD|HD", "Y", "1"},
	}
}

func (r *ProjectEquipRepo) CurrentCategoryExport(orgIDs []string) (cats [][]string, fields [][]string, err error) {
	cats = [][]string{append([]string{}, catHdrCats...)}
	fields = [][]string{append([]string{}, catHdrField...)}
	for _, org := range orgIDs {
		if org == "" || org == OrgAll {
			continue
		}
		list, err := r.ListCategories(org, true)
		if err != nil {
			return nil, nil, err
		}
		codeOf := map[string]string{}
		for _, c := range list {
			codeOf[c.ID] = c.Code
		}
		for _, c := range list {
			parent := ""
			if c.ParentID != "" {
				parent = codeOf[c.ParentID]
			}
			cats = append(cats, []string{
				ExcelSafeText(c.OrgID), ExcelSafeText(c.Code), ExcelSafeText(c.Name),
				ExcelSafeText(parent), strconv.Itoa(c.SortOrder), ynText(c.IsActive),
			})
			for _, f := range c.Fields {
				opt := strings.ReplaceAll(f.Options, "\n", "|")
				fields = append(fields, []string{
					ExcelSafeText(c.OrgID), ExcelSafeText(c.Code), ExcelSafeText(f.FieldKey),
					ExcelSafeText(f.Label), ExcelSafeText(f.FieldType), ExcelSafeText(opt),
					ynText(f.Required), strconv.Itoa(f.SortOrder),
				})
			}
		}
	}
	return cats, fields, nil
}
