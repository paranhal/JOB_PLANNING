package repository

import (
	"bytes"
	"database/sql"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

const (
	contractSheetGuide = "안내"
	contractSheetData  = "계약"
)

var contractHdr = []string{
	"사업명 *", "부서 *", "발주처 *", "최종수요처", "사업유형 *", "계약방식", "거래구분",
	"계약시작일 *", "계약종료일 *", "계약금액(VAT포함) *", "계약번호(선택)", "계약담당", "업무담당", "비고",
}

func ParseContractWorkbook(raw []byte) ([][]string, error) {
	if len(raw) > model.ContractImportMaxBytes {
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
	rows, err := sheetRows(f, contractSheetData)
	if err != nil {
		return nil, err
	}
	if err := matchContractHeaders(rows); err != nil {
		return nil, err
	}
	n := countNonEmpty(rows, 14, []int{0, 1, 2, 7, 8, 9})
	if n == 0 {
		return nil, fmt.Errorf("올릴 행이 없습니다")
	}
	if n > model.ContractImportMaxRows {
		return nil, fmt.Errorf("계약은 회당 %d행까지입니다", model.ContractImportMaxRows)
	}
	return rows, nil
}

func matchContractHeaders(rows [][]string) error {
	if len(rows) == 0 {
		return fmt.Errorf("시트 %s 헤더가 없습니다", contractSheetData)
	}
	got := rows[0]
	if len(got) < len(contractHdr) {
		return fmt.Errorf("시트 %s 헤더가 다릅니다(기대 %s / 실제 %s)", contractSheetData, strings.Join(contractHdr, ","), strings.Join(trimRow(got), ","))
	}
	for i, w := range contractHdr {
		if strings.TrimSpace(cell(got, i)) != w {
			return fmt.Errorf("시트 %s 헤더가 다릅니다(기대 %s / 실제 %s)", contractSheetData, strings.Join(contractHdr, ","), strings.Join(trimRow(got[:len(contractHdr)]), ","))
		}
	}
	return nil
}

func parseContractDate(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("날짜가 없습니다")
	}
	if i := strings.IndexAny(s, "T "); i > 0 {
		s = s[:i]
	}
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.ReplaceAll(s, "/", "-")
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Format("2006-01-02"), nil
	}
	if t, err := time.Parse("2006-1-2", s); err == nil {
		return t.Format("2006-01-02"), nil
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	if err != nil || f < 20000 || f > 80000 {
		return "", fmt.Errorf("날짜 형식을 확인하세요")
	}
	sec := int64(math.Round(f * 86400))
	t := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC).Add(time.Duration(sec) * time.Second)
	return t.Format("2006-01-02"), nil
}

func parseContractAmount(s string) (int, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "원", "")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("계약금액이 없습니다")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("계약금액은 숫자여야 합니다")
	}
	if f < 0 {
		return 0, fmt.Errorf("계약금액은 0 이상이어야 합니다")
	}
	return int(math.Round(f)), nil
}

func contractRowEmpty(row []string) bool {
	return catRowEmpty(row, []int{0, 1, 2, 7, 8, 9})
}

func (r *ProjectRepo) PreviewContractImport(raw []byte, fileName string, scope CatImportScope, today time.Time) (*model.ContractImportPreview, error) {
	rows, err := ParseContractWorkbook(raw)
	if err != nil {
		return nil, err
	}
	if today.IsZero() {
		today = time.Now()
	}
	out := &model.ContractImportPreview{FileName: fileName}
	seenFile := map[string]int{}
	for i, row := range rows {
		if i == 0 || contractRowEmpty(row) {
			continue
		}
		ir := r.parseContractRow(i+1, row, scope, today)
		if ir.Action != "err" {
			key := contractDupKey(ir)
			if prev, ok := seenFile[key]; ok {
				ir.Action, ir.Msg = "err", fmt.Sprintf("같은 파일에 중복입니다(%d행)", prev)
			} else {
				seenFile[key] = ir.Line
			}
		}
		if ir.Action != "err" {
			r.classifyContractDup(&ir)
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

func contractDupKey(ir model.ContractImportRow) string {
	if strings.TrimSpace(ir.ContractNo) != "" {
		return "no:" + strings.TrimSpace(ir.ContractNo)
	}
	return fmt.Sprintf("n:%s|p:%s|s:%s|a:%d", strings.TrimSpace(ir.Name), strings.TrimSpace(ir.PartyName), ir.StartDate, ir.Amount)
}

func (r *ProjectRepo) classifyContractDup(ir *model.ContractImportRow) {
	if r == nil || r.db == nil {
		return
	}
	var n int
	if strings.TrimSpace(ir.ContractNo) != "" {
		_ = r.db.QueryRow(`SELECT COUNT(*) FROM work_projects WHERE TRIM(COALESCE(contract_no,''))=?`, strings.TrimSpace(ir.ContractNo)).Scan(&n)
		if n > 0 {
			ir.Action, ir.Msg = "skip", "이미 같은 계약번호가 있습니다"
			return
		}
	}
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM work_projects
		WHERE name=? AND TRIM(COALESCE(ordering_party,''))= ? AND TRIM(COALESCE(start_date,''))=? AND COALESCE(contract_amount,0)=?`,
		ir.Name, ir.PartyName, ir.StartDate, ir.Amount).Scan(&n)
	if n > 0 {
		ir.Action, ir.Msg = "skip", "이미 같은 계약이 있습니다"
	}
}

func (r *ProjectRepo) parseContractRow(line int, row []string, scope CatImportScope, today time.Time) model.ContractImportRow {
	ir := model.ContractImportRow{Line: line, Action: "new"}
	vals := make([]string, len(contractHdr))
	for i := range contractHdr {
		vals[i] = strings.TrimSpace(cell(row, i))
		if i == 9 {
			continue
		}
		if ExcelFormulaInjected(vals[i]) {
			ir.Action, ir.Msg = "err", "수식은 넣을 수 없습니다"
			return ir
		}
	}
	if ExcelFormulaInjected(vals[9]) {
		ir.Action, ir.Msg = "err", "수식은 넣을 수 없습니다"
		return ir
	}
	ir.Name, ir.OrgLabel, ir.PartyName = vals[0], vals[1], vals[2]
	if ir.Name == "" {
		ir.Action, ir.Msg = "err", "사업명이 없습니다"
		return ir
	}
	org, err := r.resolveContractOrg(ir.OrgLabel)
	if err != nil {
		ir.Action, ir.Msg = "err", err.Error()
		return ir
	}
	ir.OrgID = org
	if scope.Allowed != nil && !scope.Allowed[org] {
		ir.Action, ir.Msg = "err", "쓸 수 없는 조직입니다"
		return ir
	}
	kind, err := model.InferContractKind(vals[4], ir.Name)
	if err != nil {
		ir.Action, ir.Msg = "err", err.Error()
		return ir
	}
	ir.Kind, ir.KindLabel = kind, model.ProjectKindLabel(kind)
	ir.ContractType = vals[5]
	dir := model.NormalizeContractDirection(vals[6])
	if dir == "" {
		ir.Action, ir.Msg = "err", "거래구분을 확인하세요"
		return ir
	}
	ir.Direction = dir
	st, err := parseContractDate(vals[7])
	if err != nil {
		ir.Action, ir.Msg = "err", "계약시작일 "+err.Error()
		return ir
	}
	en, err := parseContractDate(vals[8])
	if err != nil {
		ir.Action, ir.Msg = "err", "계약종료일 "+err.Error()
		return ir
	}
	stT, _ := time.Parse("2006-01-02", st)
	enT, _ := time.Parse("2006-01-02", en)
	if enT.Before(stT) {
		ir.Action, ir.Msg = "err", "종료일이 시작일보다 앞입니다"
		return ir
	}
	ir.StartDate, ir.EndDate = st, en
	amt, err := parseContractAmount(vals[9])
	if err != nil {
		ir.Action, ir.Msg = "err", err.Error()
		return ir
	}
	ir.Amount = amt
	ir.ContractNo, ir.SalesUser, ir.ExecUser, ir.Notes = vals[10], vals[11], vals[12], vals[13]
	partyID, err := r.lookupCustomerName(ir.PartyName)
	if err != nil {
		ir.Action, ir.Msg = "err", err.Error()
		return ir
	}
	ir.PartyID = partyID
	for _, name := range model.SplitEndCustomers(vals[3]) {
		id, err := r.lookupCustomerName(name)
		if err != nil {
			ir.Action, ir.Msg = "err", "최종수요처 "+err.Error()
			return ir
		}
		ir.EndNames = append(ir.EndNames, name)
		ir.EndIDs = append(ir.EndIDs, id)
	}
	if ir.SalesUser != "" {
		u, err := r.lookupUserName(ir.SalesUser)
		if err != nil {
			ir.Action, ir.Msg = "err", "계약담당 "+err.Error()
			return ir
		}
		ir.SalesID = u
	}
	if ir.ExecUser != "" {
		u, err := r.lookupUserName(ir.ExecUser)
		if err != nil {
			ir.Action, ir.Msg = "err", "업무담당 "+err.Error()
			return ir
		}
		ir.ExecID = u
	}
	ir.Years = model.SplitContractYears(stT, enT, ir.Amount, today)
	return ir
}

func (r *ProjectRepo) resolveContractOrg(label string) (string, error) {
	s := strings.TrimSpace(label)
	if s == "" {
		return "", fmt.Errorf("부서가 없습니다")
	}
	if s == "*" || s == OrgAll {
		return "", fmt.Errorf("부서에 * 를 넣을 수 없습니다")
	}
	switch s {
	case "도서관", "도서관사업팀", "도서관사업부", model.OrgIDLibrary:
		return model.OrgIDLibrary, nil
	case "SI", "SI사업팀", "SI사업부":
		return r.lookupOrgLabel("SI")
	case "미디어", "미디어사업팀", "미디어사업부":
		return r.lookupOrgLabel("미디어")
	}
	return r.lookupOrgLabel(s)
}

func (r *ProjectRepo) lookupOrgLabel(s string) (string, error) {
	rows, err := r.db.Query(`SELECT org_id FROM orgs
		WHERE is_active=1 AND (org_id=? OR org_no=? OR TRIM(org_name)=? OR TRIM(COALESCE(short_name,''))=?
			OR org_name LIKE ? OR COALESCE(short_name,'') LIKE ?)
		ORDER BY org_id`, s, s, s, s, "%"+s+"%", "%"+s+"%")
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
	if len(ids) == 1 {
		return ids[0], nil
	}
	if len(ids) > 1 {
		exact := []string{}
		_ = r.db.QueryRow(`SELECT COUNT(*) FROM orgs WHERE is_active=1 AND (TRIM(org_name)=? OR TRIM(COALESCE(short_name,''))=? OR org_id=? OR org_no=?)`, s, s, s, s)
		rows2, err := r.db.Query(`SELECT org_id FROM orgs WHERE is_active=1 AND (TRIM(org_name)=? OR TRIM(COALESCE(short_name,''))=? OR org_id=? OR org_no=?)`, s, s, s, s)
		if err == nil {
			defer rows2.Close()
			for rows2.Next() {
				var id string
				if rows2.Scan(&id) == nil {
					exact = append(exact, id)
				}
			}
			if len(exact) == 1 {
				return exact[0], nil
			}
		}
		return "", fmt.Errorf("부서가 여러 개 맞습니다")
	}
	return "", fmt.Errorf("부서가 없습니다")
}

func (r *ProjectRepo) lookupCustomerName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("이름이 없습니다")
	}
	rows, err := r.db.Query(`SELECT customer_id FROM customers
		WHERE TRIM(COALESCE(org_name,''))=? OR TRIM(COALESCE(official_name,''))=?`, name, name)
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
		return "", fmt.Errorf("고객 마스터에 없습니다")
	}
	if len(ids) > 1 {
		return "", fmt.Errorf("이름이 같은 고객이 여러 명입니다")
	}
	return ids[0], nil
}

func (r *ProjectRepo) lookupUserName(name string) (string, error) {
	name = strings.TrimSpace(name)
	rows, err := r.db.Query(`SELECT user_id FROM users
		WHERE is_active=1 AND TRIM(COALESCE(deleted_at,''))='' AND TRIM(full_name)=?`, name)
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
		return "", fmt.Errorf("사용자를 찾을 수 없습니다")
	}
	if len(ids) > 1 {
		return "", fmt.Errorf("이름이 같은 사용자가 여러 명입니다")
	}
	return ids[0], nil
}

func (r *ProjectRepo) ApplyContractImport(userID, fileName string, raw []byte, scope CatImportScope, today time.Time) (*model.ContractImportPreview, error) {
	prev, err := r.PreviewContractImport(raw, fileName, scope, today)
	if err != nil {
		return nil, err
	}
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	ok, skip := 0, 0
	for i := range prev.Rows {
		row := &prev.Rows[i]
		if row.Action != "new" {
			if row.Action == "err" {
				skip++
			} else {
				ok++
			}
			continue
		}
		if err := r.insertImportedContractTx(tx, userID, row); err != nil {
			row.Action, row.Msg = "err", err.Error()
			skip++
			continue
		}
		ok++
	}
	prev.ErrN, prev.NewN, prev.SkipN = 0, 0, 0
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
	n, err := nextSeqTx(tx, "contract_import")
	if err != nil {
		return nil, err
	}
	id := fmt.Sprintf("CIB-%04d", n)
	if _, err := tx.Exec(`INSERT INTO contract_import_batches(id,file_name,rows_total,rows_ok,rows_error,status,created_by)
		VALUES(?,?,?,?,?,?,?)`, id, fileName, len(prev.Rows), ok, skip, "applied", userID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return prev, nil
}

func (r *ProjectRepo) insertImportedContractTx(tx *sql.Tx, userID string, row *model.ContractImportRow) error {
	seq, err := nextSeqTx(tx, "work_project")
	if err != nil {
		return err
	}
	pid := fmt.Sprintf("WP-%03d", seq)
	cust := row.PartyID
	if len(row.EndIDs) > 0 {
		cust = row.EndIDs[0]
	}
	planYear := 0
	if len(row.StartDate) >= 4 {
		planYear, _ = strconv.Atoi(row.StartDate[:4])
	}
	_, err = tx.Exec(`INSERT INTO work_projects (
			project_id, name, short_name, plan_year, is_paid, sort_order,
			ordering_party_id, ordering_party, customer_id,
			contract_type, billing_type, start_date, end_date, notes, contact_id,
			color, status, sales_project_id, contract_amount, contract_no,
			project_kind, project_kind_set, org_id, amount_vat_included, direction, contract_date
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		pid, row.Name, "", planYear, 1, 0,
		nullStr(row.PartyID), row.PartyName, nullStr(cust),
		row.ContractType, "", row.StartDate, row.EndDate, row.Notes, nil,
		"#3B82F6", model.WBProjectActive, nil, row.Amount, row.ContractNo,
		row.Kind, 1, row.OrgID, 1, row.Direction, row.StartDate)
	if err != nil {
		return err
	}
	if row.SalesID != "" {
		if _, err := tx.Exec(`INSERT INTO project_assignments (project_id, user_id, duty, created_by) VALUES (?,?,?,?)`,
			pid, row.SalesID, model.ProjectDutySales, nullStr(userID)); err != nil {
			return err
		}
	}
	if row.ExecID != "" {
		if _, err := tx.Exec(`INSERT INTO project_assignments (project_id, user_id, duty, created_by) VALUES (?,?,?,?)`,
			pid, row.ExecID, model.ProjectDutyExecution, nullStr(userID)); err != nil {
			return err
		}
	}
	for i, cid := range row.EndIDs {
		if _, err := tx.Exec(`INSERT INTO contract_end_customers (contract_id, customer_id, sort_order) VALUES (?,?,?)`,
			pid, cid, i); err != nil {
			return err
		}
	}
	for _, y := range row.Years {
		yn, err := nextSeqTx(tx, "contract_year")
		if err != nil {
			return err
		}
		yid := fmt.Sprintf("CY-%04d", yn)
		if _, err := tx.Exec(`INSERT INTO contract_years(id,project_id,year_no,label,start_date,end_date,amount,status,note,org_id,created_by)
			VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
			yid, pid, y.YearNo, y.Label, y.StartDate, y.EndDate, y.Amount, y.Status, "", row.OrgID, userID); err != nil {
			return mapYearErr(err)
		}
	}
	row.Msg = pid
	return nil
}

func ContractGuideLines() []string {
	return []string{
		"사업(계약) 등록 업로드 양식",
		"",
		"· 「계약」 시트만 업로드됩니다. 「안내」 시트는 무시됩니다. 시트 이름과 헤더(첫 줄)는 바꾸지 마세요.",
		"· 빨간 헤더(*) 열은 필수입니다. 부서·사업유형·거래구분은 안내된 값으로 적습니다.",
		"· 부서: 도서관사업팀 / SI사업팀 / 미디어사업팀.",
		"· 사업유형: 공사 / 용역 / 구매 / 유지보수 / 구축 / 기타. 용역은 개발, 구매는 기타로 저장합니다.",
		"· 계약시작일·계약종료일은 날짜(예: 2026-01-01)로 두 칸에 나눠 적습니다.",
		"· 계약금액은 부가세(VAT) 포함 금액입니다.",
		"· 거래구분: 매출 또는 매입. 비우면 매출. 대시보드 합계는 매출만 집계합니다.",
		"· 계약담당·업무담당은 비워 두고 나중에 입력해도 됩니다.",
		"· 진행상태는 적지 않습니다. 계약기간으로 계산합니다.",
		"· 연차는 적지 않습니다. 1년을 넘으면 시작일 기준 12개월로 나눕니다(마지막 31일 이하면 앞 연차에 합침).",
		"· 최종수요처는 세미콜론(;)으로 구분합니다.",
	}
}

func BuildContractWorkbook(guide []string, rows [][]string) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	_ = f.SetSheetName(f.GetSheetName(0), contractSheetGuide)
	_, _ = f.NewSheet(contractSheetData)
	hdrFill, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 11, Family: "맑은 고딕", Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"1E3A5F"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", WrapText: true},
	})
	for i, line := range guide {
		_ = f.SetCellValue(contractSheetGuide, fmt.Sprintf("A%d", i+1), line)
	}
	if len(rows) == 0 {
		rows = [][]string{append([]string{}, contractHdr...)}
	}
	for i, row := range rows {
		for j, v := range row {
			cell, _ := excelize.CoordinatesToCellName(j+1, i+1)
			if i == 0 {
				_ = f.SetCellValue(contractSheetData, cell, v)
			} else {
				_ = f.SetCellValue(contractSheetData, cell, ExcelSafeText(v))
			}
		}
	}
	last, _ := excelize.ColumnNumberToName(len(rows[0]))
	_ = f.SetCellStyle(contractSheetData, "A1", last+"1", hdrFill)
	_ = f.SetRowHeight(contractSheetData, 1, 22)
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func BuildContractErrorWorkbook(prev *model.ContractImportPreview) ([]byte, error) {
	rows := [][]string{{"행", "처리", "사업명", "사유"}}
	if prev != nil {
		for _, r := range prev.Rows {
			if r.Action != "err" {
				continue
			}
			rows = append(rows, []string{strconv.Itoa(r.Line), r.Action, r.Name, r.Msg})
		}
	}
	return BuildContractWorkbook([]string{"오류 목록"}, rows)
}
