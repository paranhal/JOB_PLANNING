package handler

import (
	"archive/zip"
	"bytes"
	"fmt"
	"html"
	"io"
	"regexp"
	"strings"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

const (
	companyWeeklyModeSheetAdd    = "sheet_add"
	companyWeeklyModeSingleSheet = "single_sheet"
	companyWeeklyBizSheet        = "26.8.7(사업)"
	companyWeeklyTabColor        = "FF0000FF"
	companyWeeklyMaxUpload       = 20 << 20
)

type companyWeeklyBuildInput struct {
	Source    []byte
	SheetName string
	Period    model.CompanyWeeklyPeriod
	Rows      []model.CompanyWeeklyRow
}

type companyWeeklyBuildResult struct {
	Data           []byte
	Mode           string
	OrigSheets     int
	KeptSheets     int
	OrigFormulas   int
	KeptFormulas   int
	FallbackReason string
	ASCIIName      string
	UTF8Name       string
}

// buildCompanyWeeklyWorkbook 업로드 사본에 (업무) 시트를 추가한다. §16.6.4 · §16.6.11
// 원본 수식·숨김이 보존되지 않으면 한 장짜리 파일로 후퇴한다.
func buildCompanyWeeklyWorkbook(in companyWeeklyBuildInput) (companyWeeklyBuildResult, error) {
	in.SheetName = strings.TrimSpace(in.SheetName)
	if in.SheetName == "" {
		in.SheetName = in.Period.SheetNameDefault
	}
	if err := checkCompanyWeeklySheetName(in.SheetName); err != nil {
		return companyWeeklyBuildResult{}, err
	}
	ascii, utf8 := companyWeeklyFilenames(in.Period)
	out := companyWeeklyBuildResult{
		Mode:      companyWeeklyModeSingleSheet,
		ASCIIName: ascii,
		UTF8Name:  utf8,
	}

	if len(in.Source) == 0 {
		data, err := buildCompanyWeeklySingleSheet(in)
		if err != nil {
			return out, err
		}
		out.Data = data
		out.FallbackReason = "업로드 파일이 없습니다"
		return out, nil
	}

	hints, err := snapshotCompanyWeeklyPreserve(in.Source)
	if err != nil {
		return out, fmt.Errorf("회사 엑셀을 열 수 없습니다: %w", err)
	}
	out.OrigSheets = hints.sheetCount
	out.OrigFormulas = hints.formulaCount
	if containsSheetName(hints.names, in.SheetName) {
		return out, fmt.Errorf("시트 이름 %q 이(가) 이미 있습니다. 다른 이름을 쓰세요", in.SheetName)
	}

	added, err := addCompanyWeeklySheet(in.Source, in)
	if err != nil {
		data, ferr := buildCompanyWeeklySingleSheet(in)
		if ferr != nil {
			return out, err
		}
		out.Data = data
		out.FallbackReason = "시트 추가 실패: " + err.Error()
		return out, nil
	}

	check := inspectCompanyWeeklyPreserve(in.Source, added, in.SheetName, hints)
	out.KeptSheets = check.origSheetsKept
	out.KeptFormulas = check.formulasKept
	if check.ok {
		out.Data = added
		out.Mode = companyWeeklyModeSheetAdd
		return out, nil
	}

	data, err := buildCompanyWeeklySingleSheet(in)
	if err != nil {
		return out, err
	}
	out.Data = data
	out.FallbackReason = check.reason
	return out, nil
}

func checkCompanyWeeklySheetName(name string) error {
	if name == "" {
		return fmt.Errorf("시트 이름을 입력하세요")
	}
	if len([]rune(name)) > 31 {
		return fmt.Errorf("시트 이름은 31자를 넘을 수 없습니다")
	}
	if strings.ContainsAny(name, `:\/?*[]`) {
		return fmt.Errorf("시트 이름에 : \\ / ? * [ ] 는 쓸 수 없습니다")
	}
	return nil
}

func companyWeeklyFilenames(p model.CompanyWeeklyPeriod) (ascii, utf8 string) {
	day := strings.ReplaceAll(p.Anchor, "-", "")
	if len(day) == 8 {
		day = day[2:]
	}
	if day == "" {
		day = "report"
	}
	ascii = day + "_company_weekly.xlsx"
	utf8 = fmt.Sprintf("%s_주간업무보고_사업관리_%d월%d주차_도서관사업팀.xlsx", day, p.Month, p.WeekN)
	return ascii, utf8
}

func addCompanyWeeklySheet(src []byte, in companyWeeklyBuildInput) ([]byte, error) {
	f, err := excelize.OpenReader(bytes.NewReader(src), excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, err
	}
	defer f.Close()

	names := f.GetSheetList()
	before := firstWorkSheetName(names)
	if _, err := f.NewSheet(in.SheetName); err != nil {
		return nil, err
	}
	if err := fillCompanyWeeklySheet(f, in.SheetName, in.Period, in.Rows); err != nil {
		return nil, err
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	if before == "" {
		return buf.Bytes(), nil
	}
	return moveWorkbookSheetBefore(buf.Bytes(), in.SheetName, before)
}

func firstWorkSheetName(names []string) string {
	for _, n := range names {
		if strings.Contains(n, "(업무)") {
			return n
		}
	}
	return ""
}

func containsSheetName(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

func fillCompanyWeeklySheet(f *excelize.File, sheet string, p model.CompanyWeeklyPeriod, rows []model.CompanyWeeklyRow) error {
	tab := companyWeeklyTabColor
	if err := f.SetSheetProps(sheet, &excelize.SheetPropsOptions{TabColorRGB: &tab}); err != nil {
		return err
	}
	widths := []struct {
		col   string
		width float64
	}{
		{"A", 12.88}, {"B", 15.63}, {"C", 14.75}, {"D", 4.13},
		{"E", 43.88}, {"F", 50.63}, {"G", 50.63}, {"H", 30.63},
		{"I", 30.63}, {"J", 9.0},
	}
	for _, w := range widths {
		if err := f.SetColWidth(sheet, w.col, w.col, w.width); err != nil {
			return err
		}
	}
	if err := f.SetRowHeight(sheet, 1, 20.1); err != nil {
		return err
	}
	if err := f.SetPanes(sheet, &excelize.Panes{
		Freeze: true, Split: false, XSplit: 5, YSplit: 19,
		TopLeftCell: "F20", ActivePane: "bottomRight",
	}); err != nil {
		return err
	}

	styles, err := newCompanyWeeklyStyles(f)
	if err != nil {
		return err
	}

	for _, col := range []string{"A", "B", "C", "D", "E", "F", "G", "H", "I"} {
		if err := f.SetCellStyle(sheet, col+"1", col+"1", styles.row1); err != nil {
			return err
		}
		if err := f.SetCellStyle(sheet, col+"2", col+"2", styles.row2); err != nil {
			return err
		}
	}
	if err := f.SetCellStr(sheet, "E1", p.TitleE1); err != nil {
		return err
	}
	if err := f.SetCellStr(sheet, "F1", p.PrevRangeLabel); err != nil {
		return err
	}
	if err := f.SetCellStr(sheet, "G1", p.ThisRangeLabel); err != nil {
		return err
	}
	headers := []struct{ cell, text string }{
		{"A2", "부문/실"}, {"B2", "팀"}, {"C2", "고객(사용자)"}, {"D2", "No"},
		{"E2", "프로젝트 이름 or 업무활동 (선택 or 기입)"},
		{"F2", "전주 추진내역"}, {"G2", "금주 추진계획"},
		{"H2", "지원 필요사항"}, {"I2", "주요 의사결정"},
	}
	for _, h := range headers {
		if err := f.SetCellStr(sheet, h.cell, h.text); err != nil {
			return err
		}
	}

	for _, row := range rows {
		r := row.SheetRow
		if r < 20 || r > 25 {
			continue
		}
		if err := f.SetRowHeight(sheet, r, 175.5); err != nil {
			return err
		}
		bodyA := styles.bodyName
		bodyB := styles.bodyName
		body := styles.body
		bodyWrap := styles.bodyWrap
		if row.Highlight {
			bodyWrap = styles.bodyWrapYellow
		}
		cells := []struct {
			col   string
			style int
			text  string
		}{
			{"A", bodyA, row.Division},
			{"B", bodyB, row.Team},
			{"C", body, ""},
			{"D", body, row.NoLabel},
			{"E", bodyWrap, row.DisplayName},
			{"F", bodyWrap, row.PrevText},
			{"G", bodyWrap, row.PlanText},
			{"H", bodyWrap, row.HelpText},
			{"I", bodyWrap, row.DecisionText},
		}
		for _, c := range cells {
			cell := fmt.Sprintf("%s%d", c.col, r)
			if err := f.SetCellStr(sheet, cell, c.text); err != nil {
				return err
			}
			if err := f.SetCellStyle(sheet, cell, cell, c.style); err != nil {
				return err
			}
		}
	}
	return nil
}

type companyWeeklyStyles struct {
	row1, row2, body, bodyName, bodyWrap, bodyWrapYellow int
}

func newCompanyWeeklyStyles(f *excelize.File) (companyWeeklyStyles, error) {
	var s companyWeeklyStyles
	var err error
	dotted := []excelize.Border{
		{Type: "left", Color: "FF000000", Style: 4},
		{Type: "right", Color: "FF000000", Style: 4},
		{Type: "top", Color: "FF000000", Style: 4},
		{Type: "bottom", Color: "FF000000", Style: 4},
	}
	thinLR := []excelize.Border{
		{Type: "left", Color: "FF000000", Style: 1},
		{Type: "right", Color: "FF000000", Style: 1},
	}
	s.row1, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "Century Gothic", Size: 10, Bold: true, Color: "FFFFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"FF1E3A5F"}, Pattern: 1},
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
	})
	if err != nil {
		return s, err
	}
	s.row2, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "Century Gothic", Size: 10, Bold: true},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"FFD6DCE4"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border:    thinLR,
	})
	if err != nil {
		return s, err
	}
	s.body, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "Century Gothic", Size: 10},
		Alignment: &excelize.Alignment{Vertical: "center"},
		Border:    dotted,
	})
	if err != nil {
		return s, err
	}
	s.bodyName, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "맑은 고딕", Size: 10},
		Alignment: &excelize.Alignment{Vertical: "center"},
		Border:    dotted,
	})
	if err != nil {
		return s, err
	}
	s.bodyWrap, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "Century Gothic", Size: 10},
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
		Border:    dotted,
	})
	if err != nil {
		return s, err
	}
	s.bodyWrapYellow, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "Century Gothic", Size: 10},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"FFFFFF00"}, Pattern: 1},
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
		Border:    dotted,
	})
	return s, err
}

func buildCompanyWeeklySingleSheet(in companyWeeklyBuildInput) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetName("Sheet1", in.SheetName); err != nil {
		return nil, err
	}
	if err := fillCompanyWeeklySheet(f, in.SheetName, in.Period, in.Rows); err != nil {
		return nil, err
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type companyWeeklyPreserveHints struct {
	names         []string
	sheetCount    int
	formulaCount  int
	bizSheet      string
	n3, w3, y3    string
	abHidden      bool
	hasBizChecks  bool
}

type companyWeeklyPreserveCheck struct {
	ok             bool
	reason         string
	origSheetsKept int
	formulasKept   int
}

func snapshotCompanyWeeklyPreserve(src []byte) (companyWeeklyPreserveHints, error) {
	var h companyWeeklyPreserveHints
	f, err := excelize.OpenReader(bytes.NewReader(src), excelize.Options{RawCellValue: true})
	if err != nil {
		return h, err
	}
	defer f.Close()
	h.names = f.GetSheetList()
	h.sheetCount = len(h.names)
	h.formulaCount = countWorksheetFormulas(src)
	h.bizSheet = findBizSheetName(h.names)
	if h.bizSheet == "" {
		return h, nil
	}
	h.hasBizChecks = true
	h.n3, _ = f.GetCellFormula(h.bizSheet, "N3")
	h.w3, _ = f.GetCellFormula(h.bizSheet, "W3")
	h.y3, _ = f.GetCellFormula(h.bizSheet, "Y3")
	vis, err := f.GetColVisible(h.bizSheet, "AB")
	if err == nil {
		h.abHidden = !vis
	}
	return h, nil
}

func findBizSheetName(names []string) string {
	for _, n := range names {
		if n == companyWeeklyBizSheet {
			return n
		}
	}
	for _, n := range names {
		if strings.Contains(n, "(사업)") {
			return n
		}
	}
	return ""
}

func inspectCompanyWeeklyPreserve(orig, out []byte, newSheet string, hints companyWeeklyPreserveHints) companyWeeklyPreserveCheck {
	f, err := excelize.OpenReader(bytes.NewReader(out), excelize.Options{RawCellValue: true})
	if err != nil {
		return companyWeeklyPreserveCheck{reason: "저장 파일을 다시 열 수 없습니다: " + err.Error()}
	}
	defer f.Close()

	names := f.GetSheetList()
	if !containsSheetName(names, newSheet) {
		return companyWeeklyPreserveCheck{reason: "새 (업무) 시트가 없습니다"}
	}
	kept := 0
	for _, n := range hints.names {
		if containsSheetName(names, n) {
			kept++
		}
	}
	check := companyWeeklyPreserveCheck{
		origSheetsKept: kept,
		formulasKept:   countWorksheetFormulas(out),
	}
	if len(names) != hints.sheetCount+1 {
		check.reason = fmt.Sprintf("시트 수가 %d → %d 가 아닙니다 (결과 %d)", hints.sheetCount, hints.sheetCount+1, len(names))
		return check
	}
	if kept != hints.sheetCount {
		check.reason = "원본 시트 이름이 빠졌습니다"
		return check
	}
	if check.formulasKept < hints.formulaCount {
		check.reason = fmt.Sprintf("수식이 %d개에서 %d개로 줄었습니다", hints.formulaCount, check.formulasKept)
		return check
	}
	if hints.hasBizChecks {
		if !companyWeeklyCellIsFormula(f, hints.bizSheet, "N3") {
			check.reason = hints.bizSheet + " N3 수식이 값으로 굳었습니다"
			return check
		}
		if !companyWeeklyCellIsFormula(f, hints.bizSheet, "W3") {
			check.reason = hints.bizSheet + " W3 수식이 값으로 굳었습니다"
			return check
		}
		if !companyWeeklyCellIsFormula(f, hints.bizSheet, "Y3") {
			check.reason = hints.bizSheet + " Y3 수식이 값으로 굳었습니다"
			return check
		}
		vis, err := f.GetColVisible(hints.bizSheet, "AB")
		if err != nil || vis {
			check.reason = hints.bizSheet + " AB 열이 숨김이 아닙니다"
			return check
		}
	}
	check.ok = true
	return check
}

func companyWeeklyCellIsFormula(f *excelize.File, sheet, cell string) bool {
	fm, err := f.GetCellFormula(sheet, cell)
	if err == nil && strings.TrimSpace(fm) != "" {
		return true
	}
	v, err := f.GetCellValue(sheet, cell, excelize.Options{RawCellValue: true})
	if err != nil {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(v), "=")
}

func countWorksheetFormulas(xlsx []byte) int {
	xmls, err := zipWorksheetXMLs(xlsx)
	if err != nil {
		return 0
	}
	n := 0
	for _, raw := range xmls {
		n += countFormulaTags(raw)
	}
	return n
}

func countFormulaTags(raw []byte) int {
	if len(raw) == 0 {
		return 0
	}
	return len(formulaTagRe.FindAll(raw, -1))
}

var formulaTagRe = regexp.MustCompile(`<f[\s>/]`)

func zipWorksheetXMLs(xlsx []byte) (map[string][]byte, error) {
	r, err := zip.NewReader(bytes.NewReader(xlsx), int64(len(xlsx)))
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for _, f := range r.File {
		if !strings.HasPrefix(f.Name, "xl/worksheets/sheet") || !strings.HasSuffix(f.Name, ".xml") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		out[f.Name] = b
	}
	return out, nil
}

var (
	sheetsBlockRe = regexp.MustCompile(`(?s)(<sheets[^>]*>)(.*?)(</sheets>)`)
	sheetTagRe    = regexp.MustCompile(`(?s)<sheet\b[^>]*/>|<sheet\b[^>]*>\s*</sheet>`)
	sheetNameRe   = regexp.MustCompile(`\bname="([^"]*)"`)
)

func moveWorkbookSheetBefore(xlsx []byte, source, before string) ([]byte, error) {
	r, err := zip.NewReader(bytes.NewReader(xlsx), int64(len(xlsx)))
	if err != nil {
		return nil, err
	}
	var wb []byte
	for _, f := range r.File {
		if f.Name == "xl/workbook.xml" {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			wb, err = io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				return nil, err
			}
			break
		}
	}
	if len(wb) == 0 {
		return nil, fmt.Errorf("workbook.xml 이 없습니다")
	}
	moved, err := reorderWorkbookSheetTags(wb, source, before)
	if err != nil {
		return nil, err
	}
	return replaceZipFile(xlsx, "xl/workbook.xml", moved)
}

func reorderWorkbookSheetTags(wb []byte, source, before string) ([]byte, error) {
	loc := sheetsBlockRe.FindSubmatchIndex(wb)
	if loc == nil {
		return nil, fmt.Errorf("workbook.xml 시트 목록을 찾지 못했습니다")
	}
	inner := wb[loc[4]:loc[5]]
	tags := sheetTagRe.FindAll(inner, -1)
	srcIdx, beforeIdx := -1, -1
	var srcTag []byte
	for i, tag := range tags {
		m := sheetNameRe.FindSubmatch(tag)
		if m == nil {
			continue
		}
		name := html.UnescapeString(string(m[1]))
		if name == source {
			srcIdx = i
			srcTag = tag
		}
		if name == before {
			beforeIdx = i
		}
	}
	if srcIdx < 0 {
		return nil, fmt.Errorf("시트 %s 를 찾지 못했습니다", source)
	}
	if beforeIdx < 0 || srcIdx == beforeIdx {
		return wb, nil
	}
	rest := append([][]byte{}, tags[:srcIdx]...)
	rest = append(rest, tags[srcIdx+1:]...)
	insertAt := beforeIdx
	if srcIdx < beforeIdx {
		insertAt--
	}
	if insertAt < 0 {
		insertAt = 0
	}
	if insertAt > len(rest) {
		insertAt = len(rest)
	}
	ordered := append([][]byte{}, rest[:insertAt]...)
	ordered = append(ordered, srcTag)
	ordered = append(ordered, rest[insertAt:]...)
	var innerOut bytes.Buffer
	innerOut.WriteString("\n")
	for _, t := range ordered {
		innerOut.Write(t)
		innerOut.WriteByte('\n')
	}
	var out bytes.Buffer
	out.Write(wb[:loc[4]])
	out.Write(innerOut.Bytes())
	out.Write(wb[loc[5]:])
	return out.Bytes(), nil
}

func replaceZipFile(xlsx []byte, name string, content []byte) ([]byte, error) {
	r, err := zip.NewReader(bytes.NewReader(xlsx), int64(len(xlsx)))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, f := range r.File {
		ow, err := w.CreateHeader(&zip.FileHeader{Name: f.Name, Method: zip.Deflate})
		if err != nil {
			_ = w.Close()
			return nil, err
		}
		if f.Name == name {
			if _, err := ow.Write(content); err != nil {
				_ = w.Close()
				return nil, err
			}
			continue
		}
		rc, err := f.Open()
		if err != nil {
			_ = w.Close()
			return nil, err
		}
		_, err = io.Copy(ow, rc)
		_ = rc.Close()
		if err != nil {
			_ = w.Close()
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func applyCompanyWeeklyFormEdits(rows []model.CompanyWeeklyRow, form func(string) string) []model.CompanyWeeklyRow {
	out := make([]model.CompanyWeeklyRow, len(rows))
	copy(out, rows)
	for i := range out {
		k := out[i].RowKey
		out[i].PrevText = form("prev_" + k)
		out[i].PlanText = form("plan_" + k)
		out[i].HelpText = form("help_" + k)
		out[i].DecisionText = form("decision_" + k)
	}
	return out
}
