package repository

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

const siteSchoolSheet = "사이트"

var siteSchoolHdr = []string{"상위기관명", "기관코드", "기관명 *", "상태", "설치일", "검수일", "비고"}

func ParseSiteSchoolWorkbook(raw []byte) ([][]string, error) {
	if len(raw) > model.SchoolImportMaxBytes {
		return nil, fmt.Errorf("파일은 10MB 이하여야 합니다")
	}
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("엑셀을 열 수 없습니다")
	}
	defer f.Close()
	rows, err := sheetRows(f, siteSchoolSheet)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("시트 %s 헤더가 없습니다", siteSchoolSheet)
	}
	for i, w := range siteSchoolHdr {
		if strings.TrimSpace(cell(rows[0], i)) != w {
			return nil, fmt.Errorf("시트 %s 헤더가 다릅니다", siteSchoolSheet)
		}
	}
	n := 0
	for i := 1; i < len(rows); i++ {
		if NormalizeOrgName(cell(rows[i], 2)) != "" || strings.TrimSpace(cell(rows[i], 1)) != "" {
			n++
		}
	}
	if n == 0 {
		return nil, fmt.Errorf("올릴 행이 없습니다")
	}
	if n > model.SchoolImportMaxRows {
		return nil, fmt.Errorf("사이트는 회당 %d행까지입니다", model.SchoolImportMaxRows)
	}
	return rows, nil
}

func (r *ProjectEquipRepo) matchSchoolInOrg(orgID, parentName, code, name string) (id string, err error) {
	code = strings.TrimSpace(code)
	name = NormalizeOrgName(name)
	parentName = NormalizeOrgName(parentName)
	if parentName == "" {
		parentName = model.SchoolParentOfficialName
	}
	if code != "" {
		err = r.db.QueryRow(`SELECT customer_id FROM customers WHERE org_id=? AND TRIM(COALESCE(org_code,''))=? AND is_active=1 LIMIT 2`, orgID, code).Scan(&id)
		if err == nil && id != "" {
			return id, nil
		}
	}
	if name == "" {
		return "", fmt.Errorf("마스터에 없습니다")
	}
	q := `SELECT c.customer_id FROM customers c
		LEFT JOIN customers p ON p.customer_id=c.parent_customer_id
		WHERE c.org_id=? AND TRIM(c.org_name)=? AND c.is_active=1`
	args := []interface{}{orgID, name}
	if parentName != "" {
		q += ` AND (TRIM(COALESCE(p.org_name,''))=? OR TRIM(COALESCE(p.official_name,''))=?)`
		args = append(args, parentName, parentName)
	}
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var cid string
		if err := rows.Scan(&cid); err != nil {
			return "", err
		}
		ids = append(ids, cid)
	}
	if len(ids) == 0 {
		return "", fmt.Errorf("마스터에 없습니다")
	}
	if len(ids) > 1 {
		return "", fmt.Errorf("동명 학교가 여러 곳입니다")
	}
	return ids[0], nil
}

func (r *ProjectEquipRepo) PreviewSiteSchool(projectID, orgID string, raw []byte, mode string, createMissing bool) (*model.EquipImportPreview, error) {
	rows, err := ParseSiteSchoolWorkbook(raw)
	if err != nil {
		return nil, err
	}
	if mode != "status" {
		mode = "add"
	}
	out := &model.EquipImportPreview{}
	for i := 1; i < len(rows); i++ {
		ir := r.parseSiteSchoolRow(i+1, rows[i], projectID, orgID, mode, createMissing)
		out.Rows = append(out.Rows, ir)
		switch ir.Action {
		case "new":
			out.SitesNew++
		case "upd", "skip":
			out.SitesUpd++
		default:
			out.ErrN++
		}
	}
	return out, nil
}

func (r *ProjectEquipRepo) parseSiteSchoolRow(line int, row []string, projectID, orgID, mode string, createMissing bool) model.EquipImportRow {
	ir := model.EquipImportRow{Sheet: siteSchoolSheet, Line: line, Kind: "site"}
	for i := 0; i < len(siteSchoolHdr); i++ {
		if ExcelFormulaInjected(cell(row, i)) {
			ir.Action, ir.Msg = "err", "수식은 넣을 수 없습니다"
			return ir
		}
	}
	parent := NormalizeOrgName(cell(row, 0))
	code := strings.TrimSpace(cell(row, 1))
	name := NormalizeOrgName(cell(row, 2))
	stRaw := strings.TrimSpace(cell(row, 3))
	ir.Key = name
	if name == "" && code == "" {
		ir.Action, ir.Msg = "err", "기관명이 없습니다"
		return ir
	}
	cid, err := r.matchSchoolInOrg(orgID, parent, code, name)
	if err != nil {
		if createMissing && mode == "add" && name != "" {
			ir.Action, ir.Msg = "new", "마스터에 신규 등록 후 대상으로 넣습니다"
			s := &model.ProjectSite{ProjectID: projectID, SiteName: name, Status: model.SitePlanned, OrgID: orgID, Note: ExcelSafeText(cell(row, 6))}
			if st := model.ParseSiteStatus(stRaw); st != "" {
				s.Status = st
			}
			ir.Site = s
			return ir
		}
		ir.Action, ir.Msg = "err", err.Error()
		return ir
	}
	ex, _ := r.findSiteByCustomer(projectID, cid)
	st := model.ParseSiteStatus(stRaw)
	if mode == "status" {
		if ex == nil {
			ir.Action, ir.Msg = "err", "대상에 없는 학교입니다"
			return ir
		}
		if stRaw == "" {
			ir.Action, ir.Msg = "skip", "상태 없음"
			return ir
		}
		if st == "" {
			ir.Action, ir.Msg = "err", "상태 오류"
			return ir
		}
		ex.Status = st
		ex.InstalledAt = strings.TrimSpace(cell(row, 4))
		ex.InspectedAt = strings.TrimSpace(cell(row, 5))
		ex.Note = ExcelSafeText(cell(row, 6))
		ir.Action, ir.Site = "upd", ex
		return ir
	}
	if ex != nil {
		ir.Action, ir.Msg = "skip", "이미 대상입니다"
		return ir
	}
	if st == "" {
		st = model.SitePlanned
	}
	s := &model.ProjectSite{
		ProjectID: projectID, CustomerID: cid, SiteName: name, Status: st, OrgID: orgID,
		InstalledAt: strings.TrimSpace(cell(row, 4)), InspectedAt: strings.TrimSpace(cell(row, 5)),
		Note: ExcelSafeText(cell(row, 6)),
	}
	ir.Action, ir.Site = "new", s
	return ir
}

func (r *ProjectEquipRepo) findSiteByCustomer(projectID, customerID string) (*model.ProjectSite, error) {
	var s model.ProjectSite
	var active int
	err := r.db.QueryRow(`SELECT id, project_id, COALESCE(customer_id,''), site_name, COALESCE(region,''), COALESCE(address,''), COALESCE(contact_note,''), status, sort_order, is_active, COALESCE(note,''), org_id
		FROM project_sites WHERE project_id=? AND customer_id=?`, projectID, customerID).Scan(
		&s.ID, &s.ProjectID, &s.CustomerID, &s.SiteName, &s.Region, &s.Address, &s.ContactNote, &s.Status, &s.SortOrder, &active, &s.Note, &s.OrgID)
	if err != nil {
		return nil, err
	}
	s.IsActive = active == 1
	return &s, nil
}

func (r *ProjectEquipRepo) ApplySiteSchool(projectID, orgID, userID, fileName, mode string, raw []byte, createMissing bool) (*model.EquipImportPreview, error) {
	prev, err := r.PreviewSiteSchool(projectID, orgID, raw, mode, createMissing)
	if err != nil {
		return nil, err
	}
	now := time.Now().Format("2006-01-02")
	okN := 0
	for i := range prev.Rows {
		ir := &prev.Rows[i]
		if ir.Action != "new" && ir.Action != "upd" {
			continue
		}
		if ir.Site != nil && ir.Site.CustomerID == "" && createMissing && ir.Action == "new" {
			parentName := model.SchoolParentOfficialName
			cr := NewCustomerRepo(r.db)
			pid, _ := cr.lookupParentInOrg(parentName, orgID)
			if pid == "" {
				p := &model.Customer{OrgName: parentName, OfficialName: parentName, IsActive: true, PartyKind: "customer", OrgID: orgID}
				if err := cr.Create(p); err != nil {
					ir.Action, ir.Msg = "err", err.Error()
					continue
				}
				pid = p.CustomerID
			}
			c := &model.Customer{OrgName: ir.Site.SiteName, OfficialName: ir.Site.SiteName, HasParent: true, ParentCustomerID: pid, IsActive: true, PartyKind: "customer", OrgID: orgID}
			if err := cr.Create(c); err != nil {
				ir.Action, ir.Msg = "err", err.Error()
				continue
			}
			ir.Site.CustomerID = c.CustomerID
		}
		if ir.Action == "new" {
			if err := r.SaveSite(ir.Site); err != nil {
				ir.Action, ir.Msg = "err", err.Error()
				continue
			}
			_, _ = r.db.Exec(`INSERT INTO project_site_events(site_id,project_id,from_status,to_status,event_date,note,created_by,source)
				VALUES(?,?,?,?,?,?,?,?)`, ir.Site.ID, projectID, "", ir.Site.Status, now, ir.Site.Note, userID, "excel")
			okN++
			continue
		}
		ids := []string{ir.Site.ID}
		if err := r.BulkSiteStatus(projectID, userID, ir.Site.Status, firstNonEmptyDate(ir.Site.InstalledAt, ir.Site.InspectedAt, now), ir.Site.Note, ids); err != nil {
			ir.Action, ir.Msg = "err", err.Error()
			continue
		}
		okN++
	}
	_, _ = r.db.Exec(`INSERT INTO site_import_batches(id,project_id,file_name,mode,rows_total,rows_ok,rows_error,status,created_by)
		VALUES(?,?,?,?,?,?,?,'applied',?)`, fmt.Sprintf("SIB%s", time.Now().Format("150405.000")), projectID, fileName, mode, len(prev.Rows), okN, prev.ErrN, userID)
	return prev, nil
}

func firstNonEmptyDate(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func BuildSiteSchoolWorkbook() ([]byte, error) {
	f := excelize.NewFile()
	_ = f.SetSheetName(f.GetSheetName(0), siteSchoolSheet)
	for i, h := range siteSchoolHdr {
		axis, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(siteSchoolSheet, axis, h)
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func BuildSiteSchoolDataWorkbook(rows [][]string) ([]byte, error) {
	f := excelize.NewFile()
	_ = f.SetSheetName(f.GetSheetName(0), siteSchoolSheet)
	for r, row := range rows {
		for c, v := range row {
			axis, _ := excelize.CoordinatesToCellName(c+1, r+1)
			_ = f.SetCellValue(siteSchoolSheet, axis, v)
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
