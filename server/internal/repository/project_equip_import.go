package repository

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

const (
	equipImportMaxBytes = 10 << 20
	equipImportMaxSites = 3000
	equipImportMaxEquip = 30000
)

func ExcelSafeText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@':
		return "'" + s
	}
	return s
}

func ParseEquipWorkbook(raw []byte) (sites [][]string, equips [][]string, err error) {
	if len(raw) > equipImportMaxBytes {
		return nil, nil, fmt.Errorf("파일은 10MB 이하여야 합니다")
	}
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		return nil, nil, fmt.Errorf("엑셀을 열 수 없습니다")
	}
	defer f.Close()
	sites, err = sheetRows(f, "사이트")
	if err != nil {
		return nil, nil, err
	}
	equips, err = sheetRows(f, "장비")
	if err != nil {
		return nil, nil, err
	}
	if len(sites) > equipImportMaxSites+1 {
		return nil, nil, fmt.Errorf("사이트는 회당 %d행까지입니다", equipImportMaxSites)
	}
	if len(equips) > equipImportMaxEquip+1 {
		return nil, nil, fmt.Errorf("장비는 회당 %d행까지입니다", equipImportMaxEquip)
	}
	return sites, equips, nil
}

func sheetRows(f *excelize.File, name string) ([][]string, error) {
	if idx, _ := f.GetSheetIndex(name); idx < 0 {
		return nil, fmt.Errorf("시트 %s 가 없습니다", name)
	}
	return f.GetRows(name)
}

func (r *ProjectEquipRepo) PreviewEquipImport(projectID string, raw []byte, fileName string) (*model.EquipImportPreview, error) {
	siteRows, equipRows, err := ParseEquipWorkbook(raw)
	if err != nil {
		return nil, err
	}
	org, err := r.ProjectOrg(projectID)
	if err != nil {
		return nil, err
	}
	out := &model.EquipImportPreview{FileName: fileName}
	siteSeen := map[string]bool{}
	for i, row := range siteRows {
		if i == 0 {
			continue
		}
		ir := r.parseSiteRow(projectID, org, i+1, row, siteSeen)
		out.Rows = append(out.Rows, ir)
		tallyPreview(out, ir)
	}
	var equipHdr []string
	if len(equipRows) > 0 {
		equipHdr = equipRows[0]
	}
	for i, row := range equipRows {
		if i == 0 {
			continue
		}
		ir := r.parseEquipRow(projectID, org, i+1, row, equipHdr, siteSeen)
		out.Rows = append(out.Rows, ir)
		tallyPreview(out, ir)
	}
	return out, nil
}

func tallyPreview(out *model.EquipImportPreview, ir model.EquipImportRow) {
	switch ir.Action {
	case "new":
		if ir.Kind == "site" {
			out.SitesNew++
		} else {
			out.EquipNew++
		}
	case "upd":
		if ir.Kind == "site" {
			out.SitesUpd++
		} else {
			out.EquipUpd++
		}
	default:
		out.ErrN++
	}
}

func (r *ProjectEquipRepo) parseSiteRow(projectID, org string, line int, row []string, seen map[string]bool) model.EquipImportRow {
	ir := model.EquipImportRow{Sheet: "사이트", Line: line, Kind: "site"}
	name := ExcelSafeText(cell(row, 0))
	if name == "" {
		ir.Action, ir.Msg = "err", "사이트명 없음"
		return ir
	}
	st := model.ParseSiteStatus(cell(row, 3))
	if cell(row, 3) == "" {
		st = model.SitePlanned
	}
	if st == "" {
		ir.Action, ir.Msg = "err", "상태 오류"
		return ir
	}
	s := &model.ProjectSite{
		ProjectID: projectID, SiteName: name, Region: ExcelSafeText(cell(row, 1)),
		Address: ExcelSafeText(cell(row, 2)), Status: st, Note: ExcelSafeText(cell(row, 4)), OrgID: org,
	}
	ir.Site, ir.Key = s, name
	seen[name] = true
	ex, err := r.FindSiteByName(projectID, name)
	if err != nil {
		ir.Action, ir.Msg = "err", err.Error()
		return ir
	}
	if ex != nil {
		s.ID = ex.ID
		ir.Action = "upd"
	} else {
		ir.Action = "new"
	}
	return ir
}

func (r *ProjectEquipRepo) parseEquipRow(projectID, org string, line int, row, header []string, siteSeen map[string]bool) model.EquipImportRow {
	ir := model.EquipImportRow{Sheet: "장비", Line: line, Kind: "equip"}
	siteName := ExcelSafeText(cell(row, 0))
	prod := ExcelSafeText(cell(row, 2))
	if siteName == "" || prod == "" {
		ir.Action, ir.Msg = "err", "사이트명·제품명 필수"
		return ir
	}
	st := model.ParseEquipStatus(cell(row, 9))
	if cell(row, 9) == "" {
		st = model.EquipPlanned
	}
	if st == "" {
		ir.Action, ir.Msg = "err", "상태 오류"
		return ir
	}
	qty := 1
	if raw := strings.TrimSpace(cell(row, 5)); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			ir.Action, ir.Msg = "err", "수량 오류"
			return ir
		}
		qty = n
	}
	e := &model.ProjectEquipment{
		ProjectID: projectID, ProductName: prod, Model: ExcelSafeText(cell(row, 3)),
		Manufacturer: ExcelSafeText(cell(row, 4)), Qty: qty, SerialNo: ExcelSafeText(cell(row, 6)),
		InstallLocation: ExcelSafeText(cell(row, 7)), InstallDate: strings.TrimSpace(cell(row, 8)),
		Status: st, Note: ExcelSafeText(cell(row, 11)), OrgID: org, Values: map[string]string{},
	}
	code := strings.TrimSpace(cell(row, 1))
	if code != "" {
		cat, err := r.CategoryByCode(org, code)
		if err != nil || cat == nil {
			ir.Action, ir.Msg = "err", "모르는 분류코드"
			return ir
		}
		e.CategoryID, e.CategoryCode = cat.ID, cat.Code
	}
	if yn := strings.TrimSpace(cell(row, 10)); yn != "" {
		n, err := strconv.Atoi(yn)
		if err != nil {
			ir.Action, ir.Msg = "err", "연차번호 오류"
			return ir
		}
		ys, _ := r.ListYears(projectID)
		for _, y := range ys {
			if y.YearNo == n {
				e.YearID, e.YearNo = y.ID, y.YearNo
			}
		}
		if e.YearID == "" {
			ir.Action, ir.Msg = "err", "없는 연차번호"
			return ir
		}
	}
	site, _ := r.FindSiteByName(projectID, siteName)
	if site == nil && !siteSeen[siteName] {
		ir.Action, ir.Msg = "err", "사이트 없음"
		return ir
	}
	if site != nil {
		e.SiteID, e.SiteName = site.ID, site.SiteName
	} else {
		e.SiteName = siteName
	}
	if e.SerialNo != "" {
		if ex, _ := r.FindEquipBySerial(projectID, e.SerialNo); ex != nil {
			e.ID = ex.ID
			ir.Action = "upd"
		}
	}
	if ir.Action == "" {
		ir.Action = "new"
	}
	for i := 12; i < len(header); i++ {
		key := strings.TrimSpace(header[i])
		if key == "" {
			continue
		}
		e.Values[key] = ExcelSafeText(cell(row, i))
	}
	if err := r.validateValues(e); err != nil {
		ir.Action, ir.Msg = "err", err.Error()
		ir.Equip = e
		return ir
	}
	e.SerialWarn = r.SerialDup(projectID, e.SerialNo, e.ID)
	if e.SerialWarn {
		ir.Msg = "시리얼 중복 경고"
	}
	ir.Equip, ir.Key = e, e.SerialNo
	if ir.Key == "" {
		ir.Key = siteName + "/" + prod
	}
	return ir
}

func cell(row []string, i int) string {
	if i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func (r *ProjectEquipRepo) ApplyEquipImport(projectID, userID, fileName string, raw []byte) (*model.EquipImportPreview, error) {
	prev, err := r.PreviewEquipImport(projectID, raw, fileName)
	if err != nil {
		return nil, err
	}
	ok, skip := 0, 0
	for i := range prev.Rows {
		row := &prev.Rows[i]
		if row.Action == "err" {
			skip++
			continue
		}
		if row.Kind == "site" && row.Site != nil {
			if err := r.SaveSite(row.Site); err != nil {
				row.Action, row.Msg = "err", err.Error()
				skip++
				continue
			}
			ok++
		}
		if row.Kind == "equip" && row.Equip != nil {
			if row.Equip.SiteID == "" {
				s, _ := r.FindSiteByName(projectID, row.Equip.SiteName)
				if s == nil {
					skip++
					continue
				}
				row.Equip.SiteID = s.ID
			}
			if err := r.SaveEquipment(row.Equip); err != nil {
				skip++
				continue
			}
			ok++
		}
	}
	prev.ErrN = skip
	_ = r.RecordBatch(&model.EquipImportBatch{
		ProjectID: projectID, FileName: fileName, RowsTotal: len(prev.Rows), RowsOK: ok, RowsError: skip, Status: "applied", CreatedBy: userID,
	})
	return prev, nil
}
