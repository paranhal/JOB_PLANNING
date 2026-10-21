package repository

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

const schoolSheetData = "기관"

var schoolHdr = []string{
	"상위기관명 *", "기관명 *", "기관코드", "지역(시·군)", "주소", "대표전화", "비고",
}

func NormalizeOrgName(s string) string {
	s = strings.Join(strings.Fields(strings.ReplaceAll(s, "\n", " ")), " ")
	return strings.TrimSpace(s)
}

func ParseSchoolMasterWorkbook(raw []byte) ([][]string, error) {
	if len(raw) > model.SchoolImportMaxBytes {
		return nil, fmt.Errorf("파일은 10MB 이하여야 합니다")
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("빈 파일입니다")
	}
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("엑셀을 열 수 없습니다")
	}
	defer f.Close()
	rows, err := sheetRows(f, schoolSheetData)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("시트 %s 헤더가 없습니다", schoolSheetData)
	}
	got := rows[0]
	if len(got) < len(schoolHdr) {
		return nil, fmt.Errorf("시트 %s 헤더가 다릅니다", schoolSheetData)
	}
	for i, w := range schoolHdr {
		if strings.TrimSpace(cell(got, i)) != w {
			return nil, fmt.Errorf("시트 %s 헤더가 다릅니다(기대 %s)", schoolSheetData, strings.Join(schoolHdr, ","))
		}
	}
	n := 0
	for i := 1; i < len(rows); i++ {
		if NormalizeOrgName(cell(rows[i], 1)) != "" {
			n++
		}
	}
	if n == 0 {
		return nil, fmt.Errorf("올릴 행이 없습니다")
	}
	if n > model.SchoolImportMaxRows {
		return nil, fmt.Errorf("기관은 회당 %d행까지입니다", model.SchoolImportMaxRows)
	}
	return rows, nil
}

func (r *CustomerRepo) PreviewSchoolMaster(raw []byte, orgID string, createParent bool) (*model.SchoolImportPreview, error) {
	rows, err := ParseSchoolMasterWorkbook(raw)
	if err != nil {
		return nil, err
	}
	out := &model.SchoolImportPreview{CreateParent: createParent}
	seen := map[string]bool{}
	for i := 1; i < len(rows); i++ {
		ir := r.parseSchoolRow(i+1, rows[i], orgID)
		if ir.Name == "" && ir.Action != "err" {
			continue
		}
		key := orgID + "|" + ir.ParentName + "|" + ir.OrgCode + "|" + ir.Name
		if ir.Action != "err" && seen[key] {
			ir.Action, ir.Msg = "err", "같은 파일 안 중복입니다"
		}
		if ir.Action != "err" {
			seen[key] = true
			r.classifySchoolDup(&ir, orgID)
		}
		if ir.Action == "err" && strings.Contains(ir.Msg, "상위 기관이 없습니다") {
			out.NeedParent = true
			if createParent {
				ir.Action, ir.Msg = "new", "상위 기관을 만들고 등록합니다"
			}
		}
		out.Rows = append(out.Rows, ir)
	}
	for _, ir := range out.Rows {
		switch ir.Action {
		case "new":
			out.NewN++
		case "skip":
			out.SkipN++
		default:
			out.ErrN++
		}
	}
	return out, nil
}

func (r *CustomerRepo) parseSchoolRow(line int, row []string, orgID string) model.SchoolImportRow {
	ir := model.SchoolImportRow{Line: line, Action: "new", OrgID: orgID}
	for i := 0; i < len(schoolHdr); i++ {
		if ExcelFormulaInjected(cell(row, i)) {
			ir.Action, ir.Msg = "err", "수식은 넣을 수 없습니다"
			return ir
		}
	}
	ir.ParentName = NormalizeOrgName(cell(row, 0))
	if ir.ParentName == "" {
		ir.ParentName = model.SchoolParentOfficialName
	}
	ir.Name = NormalizeOrgName(cell(row, 1))
	if ir.Name == "" {
		ir.Action, ir.Msg = "err", "기관명이 없습니다"
		return ir
	}
	ir.OrgCode = strings.TrimSpace(cell(row, 2))
	ir.Region = strings.TrimSpace(cell(row, 3))
	ir.Address = strings.TrimSpace(cell(row, 4))
	ir.Phone = strings.TrimSpace(cell(row, 5))
	ir.Notes = strings.TrimSpace(cell(row, 6))
	pid, err := r.lookupParentInOrg(ir.ParentName, orgID)
	if err != nil {
		ir.Action, ir.Msg = "err", err.Error()
		return ir
	}
	ir.ParentID = pid
	return ir
}

func (r *CustomerRepo) lookupParentInOrg(name, orgID string) (string, error) {
	name = NormalizeOrgName(name)
	rows, err := r.db.Query(`SELECT customer_id FROM customers
		WHERE org_id=? AND (has_parent=0 OR parent_customer_id IS NULL OR TRIM(COALESCE(parent_customer_id,''))='')
		  AND (TRIM(COALESCE(org_name,''))=? OR TRIM(COALESCE(official_name,''))=?)`, orgID, name, name)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", err
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return "", fmt.Errorf("상위 기관이 없습니다")
	}
	if len(ids) > 1 {
		return "", fmt.Errorf("같은 이름의 상위 기관이 여러 곳입니다")
	}
	return ids[0], nil
}

func (r *CustomerRepo) classifySchoolDup(ir *model.SchoolImportRow, orgID string) {
	var n int
	if ir.OrgCode != "" {
		_ = r.db.QueryRow(`SELECT COUNT(*) FROM customers WHERE org_id=? AND COALESCE(parent_customer_id,'')=? AND TRIM(COALESCE(org_code,''))=?`,
			orgID, ir.ParentID, ir.OrgCode).Scan(&n)
		if n > 0 {
			ir.Action, ir.Msg = "skip", "이미 같은 기관코드가 있습니다"
			return
		}
	}
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM customers WHERE org_id=? AND COALESCE(parent_customer_id,'')=? AND TRIM(org_name)=?`,
		orgID, ir.ParentID, ir.Name).Scan(&n)
	if n > 0 {
		ir.Action, ir.Msg = "skip", "이미 같은 기관이 있습니다"
	}
}

func (r *CustomerRepo) ApplySchoolMaster(userID, orgID string, raw []byte, createParent bool) (*model.SchoolImportPreview, error) {
	prev, err := r.PreviewSchoolMaster(raw, orgID, createParent)
	if err != nil {
		return nil, err
	}
	_ = userID
	parents := map[string]string{}
	for i := range prev.Rows {
		ir := &prev.Rows[i]
		if ir.Action != "new" {
			continue
		}
		pid := ir.ParentID
		if pid == "" {
			if !createParent {
				ir.Action, ir.Msg = "err", "상위 기관이 없습니다"
				continue
			}
			if id, ok := parents[ir.ParentName]; ok {
				pid = id
			} else {
				p := &model.Customer{
					OrgName: ir.ParentName, OfficialName: ir.ParentName,
					IsActive: true, PartyKind: "customer", OrgID: orgID,
				}
				if err := r.Create(p); err != nil {
					ir.Action, ir.Msg = "err", err.Error()
					continue
				}
				pid = p.CustomerID
				parents[ir.ParentName] = pid
			}
		}
		c := &model.Customer{
			OrgName: ir.Name, OfficialName: ir.Name, OrgCode: ir.OrgCode,
			HasParent: true, ParentCustomerID: pid,
			AddrSigungu: ir.Region, Address: ir.Address, MainPhone: ir.Phone,
			Notes: ir.Notes, IsActive: true, PartyKind: "customer", OrgID: orgID,
		}
		if err := r.Create(c); err != nil {
			ir.Action, ir.Msg = "err", err.Error()
			continue
		}
		if ir.OrgCode != "" {
			if _, err := r.db.Exec(`UPDATE customers SET org_code=? WHERE customer_id=?`, ir.OrgCode, c.CustomerID); err != nil {
				ir.Action, ir.Msg = "err", err.Error()
				continue
			}
		}
		ir.ParentID = pid
	}
	prev.NewN, prev.SkipN, prev.ErrN = 0, 0, 0
	for _, ir := range prev.Rows {
		switch ir.Action {
		case "new":
			prev.NewN++
		case "skip":
			prev.SkipN++
		default:
			prev.ErrN++
		}
	}
	return prev, nil
}

func BuildSchoolMasterWorkbook() ([]byte, error) {
	f := excelize.NewFile()
	_ = f.SetSheetName(f.GetSheetName(0), schoolSheetData)
	for i, h := range schoolHdr {
		axis, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(schoolSheetData, axis, h)
	}
	style, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"1E3A5F"}, Pattern: 1},
		Alignment: &excelize.Alignment{WrapText: true},
	})
	_ = f.SetCellStyle(schoolSheetData, "A1", "G1", style)
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func BuildSchoolDataWorkbook(rows [][]string) ([]byte, error) {
	f := excelize.NewFile()
	_ = f.SetSheetName(f.GetSheetName(0), schoolSheetData)
	for r, row := range rows {
		for c, v := range row {
			axis, _ := excelize.CoordinatesToCellName(c+1, r+1)
			_ = f.SetCellValue(schoolSheetData, axis, v)
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
