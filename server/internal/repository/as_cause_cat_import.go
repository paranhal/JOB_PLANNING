package repository

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

const (
	causeCatImportMaxBytes = 2 << 20
	causeCatImportMaxRows  = 2000
	causeCatSheetInput     = "입력"
	causeCatSheetGuide     = "작성 안내"
	causeCatSheetTypes     = "장애유형 목록"
	causeCatLabelMax       = 40
)

var causeCatHdr = []string{"1차 분류", "2차 분류", "3차 분류", "장애유형(선택)", "장애 여부(Y/N, 선택)", "정렬순서(선택)", "사용여부(Y/N, 기본 Y)"}

var defaultCauseTypeCodes = map[string]string{
	"hw": "hw", "sw": "sw", "network": "network", "env": "env",
	"hw고장": "hw", "sw오류": "sw", "네트워크": "network", "환경문제": "env",
}

func CauseCatGuideLines() []string {
	return []string{
		"한 행은 한 경로입니다. 2차·3차가 비어 있으면 그 단계까지가 하나의 분류입니다.",
		"3차가 있으면 2차가 필요하고, 2차가 있으면 1차가 필요합니다.",
		"분류 코드는 적지 않습니다. 서버가 조직·1차>2차>3차 경로로 만듭니다.",
		"같은 파일을 다시 올리면 같은 경로의 코드가 유지됩니다. 이름을 바꾸면 새 분류가 추가되고 옛 분류는 남습니다.",
		"파일에 없는 분류는 지우지 않습니다. 끄려면 사용여부를 N으로 두세요.",
		"장애유형은 2차 행에 적으면 연결됩니다. 비워도 됩니다.",
		"글자는 각 40자까지입니다. 수식 셀은 쓸 수 없습니다.",
	}
}

func CauseCatSampleRows() [][]string {
	return [][]string{
		{"RFID", "HW", "부품 점검", "hw", "Y", "10", "Y"},
	}
}

func CauseTypeSheetRows() [][]string {
	return [][]string{
		{"코드", "이름"},
		{"hw", "HW고장"},
		{"sw", "SW오류"},
		{"network", "네트워크"},
		{"env", "환경문제"},
	}
}

func ParseCauseCatWorkbook(raw []byte) ([][]string, error) {
	if len(raw) > causeCatImportMaxBytes {
		return nil, fmt.Errorf("파일은 2MB 이하여야 합니다")
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("빈 파일입니다")
	}
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("엑셀을 열 수 없습니다")
	}
	defer f.Close()
	rows, err := sheetRows(f, causeCatSheetInput)
	if err != nil {
		return nil, err
	}
	if err := matchCatHeaders(causeCatSheetInput, rows, causeCatHdr); err != nil {
		return nil, err
	}
	n := countNonEmpty(rows, 7, []int{0, 1, 2})
	if n > causeCatImportMaxRows {
		return nil, fmt.Errorf("분류는 회당 %d행까지입니다", causeCatImportMaxRows)
	}
	if n == 0 {
		return nil, fmt.Errorf("올릴 행이 없습니다")
	}
	return rows, nil
}

func BuildCauseCatWorkbook(guide []string, input [][]string, types [][]string, sample [][]string) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	_ = f.SetSheetName(f.GetSheetName(0), causeCatSheetInput)
	_, _ = f.NewSheet(causeCatSheetGuide)
	_, _ = f.NewSheet(causeCatSheetTypes)
	hdrFill, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 11, Family: "맑은 고딕", Color: "FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"1E3A5F"}, Pattern: 1},
	})
	sampleFill, _ := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{"E5E7EB"}, Pattern: 1},
	})
	writeSheet := func(sheet string, rows [][]string) {
		for r, row := range rows {
			for c, v := range row {
				cell, _ := excelize.CoordinatesToCellName(c+1, r+1)
				_ = f.SetCellValue(sheet, cell, v)
			}
		}
		if len(rows) > 0 {
			last, _ := excelize.ColumnNumberToName(len(rows[0]))
			_ = f.SetCellStyle(sheet, "A1", last+"1", hdrFill)
		}
	}
	if len(input) == 0 {
		input = [][]string{append([]string{}, causeCatHdr...)}
		for _, s := range sample {
			input = append(input, s)
		}
	}
	writeSheet(causeCatSheetInput, input)
	if len(input) > 1 {
		last, _ := excelize.ColumnNumberToName(len(causeCatHdr))
		_ = f.SetCellStyle(causeCatSheetInput, "A2", last+"2", sampleFill)
	}
	for i, line := range guide {
		_ = f.SetCellValue(causeCatSheetGuide, fmt.Sprintf("A%d", i+1), line)
	}
	writeSheet(causeCatSheetTypes, types)
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func BuildCauseCatTemplateBytes() ([]byte, error) {
	return BuildCauseCatWorkbook(CauseCatGuideLines(), nil, CauseTypeSheetRows(), CauseCatSampleRows())
}

func (r *ASRepo) CurrentCauseCatExport(orgID string) ([][]string, error) {
	cats, err := r.listOrgCauseCategories(orgID, false)
	if err != nil {
		return nil, err
	}
	if len(cats) == 0 && orgID == legacyCauseOrgID {
		cats, err = r.listLegacyCauseCategories(false)
		if err != nil {
			return nil, err
		}
	}
	byCode := map[string]model.CauseCategory{}
	for _, c := range cats {
		byCode[c.Code] = c
	}
	out := [][]string{append([]string{}, causeCatHdr...)}
	for _, c := range cats {
		if !c.IsActive && c.Level != 1 {
			// 비활성도 내려준다
		}
		l1, l2, l3 := causePathLabels(c, byCode)
		if c.Level == 2 && l3 != "" {
			continue
		}
		if c.Level == 1 {
			hasChild := false
			for _, o := range cats {
				if o.ParentCode == c.Code {
					hasChild = true
					break
				}
			}
			if hasChild {
				continue
			}
		}
		if c.Level == 2 {
			hasChild := false
			for _, o := range cats {
				if o.Level == 3 && o.ParentCode == c.Code {
					hasChild = true
					break
				}
			}
			if hasChild {
				continue
			}
		}
		ynFault, ynUse := "N", "N"
		if c.IsFault {
			ynFault = "Y"
		}
		if c.IsActive {
			ynUse = "Y"
		}
		cmap := ""
		if c.Level == 2 {
			cmap = c.CauseTypeMap
		}
		out = append(out, []string{l1, l2, l3, cmap, ynFault, strconv.Itoa(c.SortOrder), ynUse})
	}
	return out, nil
}

func causePathLabels(c model.CauseCategory, byCode map[string]model.CauseCategory) (l1, l2, l3 string) {
	switch c.Level {
	case 1:
		return c.Label, "", ""
	case 2:
		p := byCode[c.ParentCode]
		return p.Label, c.Label, ""
	case 3:
		p2 := byCode[c.ParentCode]
		p1 := byCode[p2.ParentCode]
		return p1.Label, p2.Label, c.Label
	}
	return c.Label, "", ""
}

func (r *ASRepo) PreviewCauseCatImport(raw []byte, fileName, orgID string) (*model.CauseCatImportPreview, error) {
	rows, err := ParseCauseCatWorkbook(raw)
	if err != nil {
		return nil, err
	}
	types := r.causeTypeLookup()
	out := &model.CauseCatImportPreview{FileName: fileName, OrgID: orgID}
	seenPath := map[string]int{}
	nodes := map[string]*model.CauseCatImportRow{}
	for i, row := range rows {
		if i == 0 || catRowEmpty(row, []int{0, 1, 2}) {
			continue
		}
		expanded := r.parseCauseCatExcelRow(i+1, row, orgID, types)
		for _, ir := range expanded {
			cp := ir
			if ir.Action != "err" {
				if prev, ok := seenPath[ir.Key]; ok && ir.Level == expanded[len(expanded)-1].Level {
					cp.Action, cp.Msg = "err", fmt.Sprintf("같은 파일에 경로가 중복입니다(%d행)", prev)
				} else if ir.Level == expanded[len(expanded)-1].Level {
					seenPath[ir.Key] = ir.Line
				}
				if cp.Action != "err" {
					r.classifyCauseCatRow(&cp)
					nodes[cp.Code] = &cp
				}
			}
			out.Rows = append(out.Rows, cp)
		}
	}
	for _, ir := range out.Rows {
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
	}
	return out, nil
}

func (r *ASRepo) parseCauseCatExcelRow(line int, row []string, orgID string, types map[string]string) []model.CauseCatImportRow {
	l1 := strings.TrimSpace(cell(row, 0))
	l2 := strings.TrimSpace(cell(row, 1))
	l3 := strings.TrimSpace(cell(row, 2))
	cmapRaw := strings.TrimSpace(cell(row, 3))
	faultRaw := strings.TrimSpace(cell(row, 4))
	sortRaw := strings.TrimSpace(cell(row, 5))
	useRaw := strings.TrimSpace(cell(row, 6))
	mkErr := func(msg string) []model.CauseCatImportRow {
		return []model.CauseCatImportRow{{Sheet: causeCatSheetInput, Line: line, Action: "err", Key: l1 + ">" + l2 + ">" + l3, Msg: msg, OrgID: orgID}}
	}
	for _, v := range []string{l1, l2, l3, cmapRaw, faultRaw, sortRaw, useRaw} {
		if ExcelFormulaInjected(v) {
			return mkErr("수식 셀은 쓸 수 없습니다")
		}
	}
	if l3 != "" && l2 == "" {
		return mkErr("3차가 있으면 2차가 필요합니다")
	}
	if l2 != "" && l1 == "" {
		return mkErr("2차가 있으면 1차가 필요합니다")
	}
	if l1 == "" {
		return mkErr("1차 분류가 필요합니다")
	}
	for _, lab := range []string{l1, l2, l3} {
		if lab != "" && utf8.RuneCountInString(lab) > causeCatLabelMax {
			return mkErr("분류명은 40자까지입니다")
		}
	}
	sortN, err := parseSort(sortRaw, line*10)
	if err != nil {
		return mkErr(err.Error())
	}
	active, err := parseYN(useRaw, true)
	if err != nil {
		return mkErr("사용여부는 Y/N 입니다")
	}
	fault, err := parseYN(faultRaw, true)
	if err != nil {
		return mkErr("장애 여부는 Y/N 입니다")
	}
	cmap := ""
	if cmapRaw != "" {
		key := strings.ToLower(strings.ReplaceAll(cmapRaw, " ", ""))
		code, ok := types[key]
		if !ok {
			code, ok = types[strings.ToLower(cmapRaw)]
		}
		if !ok {
			return mkErr("알 수 없는 장애유형입니다")
		}
		cmap = code
	}
	var out []model.CauseCatImportRow
	c1 := model.CauseOrgPathCode(orgID, l1)
	out = append(out, model.CauseCatImportRow{
		Sheet: causeCatSheetInput, Line: line, OrgID: orgID, Code: c1, Level: 1, ParentCode: "",
		Label: l1, SortOrder: sortN, IsActive: active, IsFault: fault, Key: orgID + "·" + l1,
	})
	if l2 != "" {
		c2 := model.CauseOrgPathCode(orgID, l1, l2)
		out = append(out, model.CauseCatImportRow{
			Sheet: causeCatSheetInput, Line: line, OrgID: orgID, Code: c2, Level: 2, ParentCode: c1,
			Label: l2, SortOrder: sortN, IsActive: active, IsFault: fault, CauseTypeMap: cmap,
			Key: orgID + "·" + l1 + ">" + l2,
		})
	}
	if l3 != "" {
		c3 := model.CauseOrgPathCode(orgID, l1, l2, l3)
		out = append(out, model.CauseCatImportRow{
			Sheet: causeCatSheetInput, Line: line, OrgID: orgID, Code: c3, Level: 3,
			ParentCode: model.CauseOrgPathCode(orgID, l1, l2),
			Label: l3, SortOrder: sortN, IsActive: active, IsFault: fault,
			Key: orgID + "·" + l1 + ">" + l2 + ">" + l3,
		})
	}
	return out
}

func (r *ASRepo) classifyCauseCatRow(ir *model.CauseCatImportRow) {
	ex, _ := r.getCauseCategoryOrg(ir.OrgID, ir.Code)
	if ex == nil {
		ir.Action = "new"
		return
	}
	same := ex.Level == ir.Level && ex.ParentCode == ir.ParentCode && ex.Label == ir.Label &&
		ex.SortOrder == ir.SortOrder && ex.IsActive == ir.IsActive && ex.IsFault == ir.IsFault &&
		strings.TrimSpace(ex.CauseTypeMap) == strings.TrimSpace(ir.CauseTypeMap)
	if same {
		ir.Action = "same"
		return
	}
	ir.Action = "upd"
}

func (r *ASRepo) causeTypeLookup() map[string]string {
	out := map[string]string{}
	for k, v := range defaultCauseTypeCodes {
		out[k] = v
	}
	if r == nil || r.db == nil {
		return out
	}
	rows, err := r.db.Query(`SELECT code_value, code_name FROM codes WHERE group_code='cause_type'`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var code, name string
		if err := rows.Scan(&code, &name); err != nil {
			continue
		}
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		out[strings.ToLower(code)] = code
		out[strings.ToLower(strings.ReplaceAll(name, " ", ""))] = code
	}
	return out
}

func (r *ASRepo) ApplyCauseCatImport(userID, fileName, orgID string, raw []byte) (*model.CauseCatImportPreview, error) {
	prev, err := r.PreviewCauseCatImport(raw, fileName, orgID)
	if err != nil {
		return nil, err
	}
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	skip := 0
	for i := range prev.Rows {
		row := &prev.Rows[i]
		if row.Action == "err" {
			skip++
			continue
		}
		if row.Action == "same" {
			continue
		}
		if err := r.upsertCauseCategoryOrgTx(tx, orgID, userID, *row); err != nil {
			row.Action, row.Msg = "err", err.Error()
			skip++
			continue
		}
	}
	prev.ErrN = skip
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return prev, nil
}

func BuildCauseCatErrorExcel(prev *model.CauseCatImportPreview) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	_ = f.SetSheetName(f.GetSheetName(0), "오류")
	for i, h := range []string{"시트", "행", "처리", "키", "사유"} {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue("오류", cell, h)
	}
	r := 2
	if prev != nil {
		for _, row := range prev.Rows {
			if row.Action != "err" {
				continue
			}
			_ = f.SetCellValue("오류", fmt.Sprintf("A%d", r), row.Sheet)
			_ = f.SetCellValue("오류", fmt.Sprintf("B%d", r), row.Line)
			_ = f.SetCellValue("오류", fmt.Sprintf("C%d", r), row.Action)
			_ = f.SetCellValue("오류", fmt.Sprintf("D%d", r), row.Key)
			_ = f.SetCellValue("오류", fmt.Sprintf("E%d", r), row.Msg)
			r++
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
