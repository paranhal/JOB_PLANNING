package repository

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

var assetImportDataHeaders = []string{
	"★ 기관명", "고객ID", "★ 제품명", "★ 설치위치", "건물명", "층", "호실",
	"▲ 제품구분", "▲ 모델명", "▲ 제조사", "▲ 시리얼번호(S/N)", "▲ 설치일", "▲ 운영상태",
	"▲ 설치주체", "▲ 관리유형", "▲ 유지보수 계약구분", "▲ 점검주기", "▲ 유지보수 시작일", "▲ 유지보수 종료일",
	"제품군", "상세위치", "고객 담당자", "당사 담당자", "사업명", "비고",
}

func (r *AssetRepo) PreviewAssetImport(cust *CustomerRepo, orgID, fileName string, raw []byte, createSpaces bool) (*model.AssetImportPreview, error) {
	out := &model.AssetImportPreview{FileName: fileName, CreateSpaces: createSpaces}
	rows, err := parseAssetImportWorkbook(raw)
	if err != nil {
		return nil, err
	}
	var idx *importSiteIndex
	if cust != nil {
		var err2 error
		idx, err2 = cust.LoadImportSiteIndex()
		if err2 != nil {
			return nil, err2
		}
	} else {
		idx = &importSiteIndex{}
	}
	existing := r.existingAssetKeys()
	fileSN := map[string]int{}
	for _, row := range rows {
		sn := strings.TrimSpace(row.Asset.SerialNumber)
		if sn != "" {
			fileSN[strings.ToUpper(sn)]++
		}
	}
	for i := range rows {
		ir := rows[i]
		ir.FileDup = strings.TrimSpace(ir.Asset.SerialNumber) != "" && fileSN[strings.ToUpper(ir.Asset.SerialNumber)] > 1
		if ir.FileDup {
			ir.Msgs = append(ir.Msgs, "파일 안 같은 시리얼이 여러 줄입니다. 모두 등록합니다")
		}
		matchCustomer(idx, orgID, &ir)
		if ir.Action != model.AssetImportError && ir.CustomerID != "" {
			if existing[assetExistKey(ir.CustomerID, ir.Asset.SerialNumber, ir.Asset.ProductName, ir.Asset.ModelName, ir.Asset.InstallLocation)] {
				ir.Action = model.AssetImportSkip
				ir.Msgs = append(ir.Msgs, "이미 등록")
			}
		}
		if ir.Action == "" {
			ir.Action = model.AssetImportRegister
		}
		pg := NewProductGroupRepo(r.db)
		cell := strings.TrimSpace(ir.Asset.ProductGroupID)
		ir.Asset.ProductGroupID = pg.ResolveImportCell(cell, ir.Asset.ProductType, ir.Asset.ProductCategory)
		if gid := ir.Asset.ProductGroupID; gid != "" {
			if g, _ := pg.Get(gid); g != nil {
				SyncAssetLegacyFromGroup(&ir.Asset, g)
			}
		}
		out.Rows = append(out.Rows, ir)
	}
	model.TallyAssetImport(out)
	return out, nil
}

func matchCustomer(idx *importSiteIndex, orgID string, ir *model.AssetImportRow) {
	if ir.Action == model.AssetImportError {
		return
	}
	name := strings.TrimSpace(ir.SiteName)
	if name == "" {
		ir.Action = model.AssetImportError
		ir.MatchKind = model.AssetImportErrMissing
		ir.Msgs = append(ir.Msgs, "기관명이 없습니다")
		return
	}
	d := idx.Decide(orgID, name)
	if d.Kind != "" {
		ir.Action = model.AssetImportError
		ir.MatchKind = d.Kind
		ir.Msgs = append(ir.Msgs, d.Msg)
		return
	}
	ir.CustomerID = d.Hit.ID
	ir.Asset.CustomerID = d.Hit.ID
}

func assetExistKey(customerID, serial, product, modelName, loc string) string {
	serial = strings.ToUpper(strings.TrimSpace(serial))
	if serial != "" {
		return "sn|" + customerID + "|" + serial
	}
	return "fp|" + customerID + "|" + strings.TrimSpace(product) + "|" + strings.TrimSpace(modelName) + "|" + strings.TrimSpace(loc)
}

func (r *AssetRepo) existingAssetKeys() map[string]bool {
	out := map[string]bool{}
	if r == nil || r.db == nil {
		return out
	}
	rows, err := r.db.Query(`
		SELECT COALESCE(customer_id,''), COALESCE(serial_number,''), COALESCE(product_name,''),
		       COALESCE(model_name,''), COALESCE(install_location,'')
		  FROM assets`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var cid, sn, pn, mn, loc string
		if err := rows.Scan(&cid, &sn, &pn, &mn, &loc); err != nil {
			continue
		}
		out[assetExistKey(cid, sn, pn, mn, loc)] = true
	}
	return out
}

func parseAssetImportWorkbook(raw []byte) ([]model.AssetImportRow, error) {
	if len(raw) > model.AssetImportMaxBytes {
		return nil, fmt.Errorf("파일은 2MB 이하여야 합니다")
	}
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("엑셀을 열 수 없습니다")
	}
	defer f.Close()
	sheet := pickAssetImportSheet(f.GetSheetList())
	grid, err := f.GetRows(sheet)
	if err != nil {
		return nil, fmt.Errorf("시트를 읽을 수 없습니다")
	}
	if len(grid) < 2 {
		return nil, fmt.Errorf("%s", model.AssetImportEmptySheet)
	}
	idx := mapAssetImportHeaders(grid[0])
	if _, ok := idx["기관명"]; !ok {
		return nil, fmt.Errorf("머리글에 기관명이 없습니다")
	}
	var out []model.AssetImportRow
	for i := 1; i < len(grid); i++ {
		row := grid[i]
		if assetImportRowEmpty(row) {
			continue
		}
		if len(out) >= model.AssetImportMaxRows {
			return nil, fmt.Errorf("한 번에 %d행까지입니다", model.AssetImportMaxRows)
		}
		ir := mapAssetImportRow(i+1, row, idx)
		out = append(out, ir)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s", model.AssetImportEmptySheet)
	}
	return out, nil
}

func pickAssetImportSheet(names []string) string {
	for _, n := range names {
		if n == "자산입력" {
			return n
		}
	}
	for _, n := range names {
		switch n {
		case "작성안내", "작성예시", "코드표", "안내":
			continue
		default:
			return n
		}
	}
	if len(names) > 0 {
		return names[0]
	}
	return "Sheet1"
}

func mapAssetImportHeaders(hdr []string) map[string]int {
	idx := map[string]int{}
	for i, h := range hdr {
		key := normalizeAssetHeader(h)
		if key == "" {
			continue
		}
		if _, ok := idx[key]; !ok {
			idx[key] = i
		}
	}
	return idx
}

func normalizeAssetHeader(h string) string {
	s := strings.TrimSpace(h)
	s = strings.TrimLeft(s, "★▲* ")
	s = strings.TrimSpace(s)
	switch {
	case s == "기관명" || strings.Contains(s, "기관명"):
		return "기관명"
	case s == "고객ID" || s == "고객id":
		return "고객ID"
	case s == "제품명":
		return "제품명"
	case s == "설치위치":
		return "설치위치"
	case s == "건물명":
		return "건물명"
	case s == "층":
		return "층"
	case s == "호실" || s == "호/실명" || s == "호":
		return "호실"
	case s == "제품구분":
		return "제품구분"
	case s == "모델명":
		return "모델명"
	case s == "제조사":
		return "제조사"
	case strings.Contains(s, "시리얼") || s == "S/N" || s == "SN":
		return "시리얼"
	case s == "설치일":
		return "설치일"
	case s == "운영상태":
		return "운영상태"
	case s == "설치주체":
		return "설치주체"
	case s == "관리유형":
		return "관리유형"
	case strings.Contains(s, "계약구분"):
		return "계약구분"
	case s == "점검주기":
		return "점검주기"
	case strings.Contains(s, "시작일"):
		return "유지보수시작"
	case strings.Contains(s, "종료일"):
		return "유지보수종료"
	case s == "제품군" || s == "제품분류":
		return "제품군"
	case s == "상세위치":
		return "상세위치"
	case strings.Contains(s, "고객") && strings.Contains(s, "담당"):
		return "고객담당"
	case strings.Contains(s, "당사") && strings.Contains(s, "담당"):
		return "당사담당"
	case s == "사업명":
		return "사업명"
	case s == "비고":
		return "비고"
	default:
		return ""
	}
}

func assetImportRowEmpty(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func cellAt(row []string, idx map[string]int, key string) string {
	i, ok := idx[key]
	if !ok || i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func mapAssetImportRow(line int, row []string, idx map[string]int) model.AssetImportRow {
	ir := model.AssetImportRow{Line: line, SiteName: cellAt(row, idx, "기관명")}
	for _, key := range []string{"기관명", "제품명", "설치위치", "건물명", "층", "호실", "모델명", "제조사", "시리얼", "비고"} {
		if v := cellAt(row, idx, key); excelFormulaUnsafe(v) {
			ir.Action = model.AssetImportError
			ir.Msgs = append(ir.Msgs, "수식처럼 보이는 값은 넣을 수 없습니다")
			return ir
		}
	}
	product := cellAt(row, idx, "제품명")
	locRaw := cellAt(row, idx, "설치위치")
	b := cellAt(row, idx, "건물명")
	fl := cellAt(row, idx, "층")
	rm := cellAt(row, idx, "호실")
	if product == "" {
		ir.Action = model.AssetImportError
		ir.Msgs = append(ir.Msgs, "제품명이 없습니다")
		return ir
	}
	if locRaw == "" && b == "" && fl == "" && rm == "" {
		ir.Action = model.AssetImportError
		ir.Msgs = append(ir.Msgs, "설치위치가 없습니다")
		return ir
	}
	if b == "" && fl == "" && rm == "" {
		if sb, sf, sr, ok := model.SplitSlashLocation(locRaw); ok {
			b, fl, rm = sb, sf, sr
		} else if locRaw != "" {
			ir.LocationWarn = true
			ir.Msgs = append(ir.Msgs, "건물·층·호실로 나누지 못해 설치위치만 저장합니다")
		}
	}
	install := locRaw
	if b != "" || fl != "" || rm != "" {
		install = model.JoinAssetInstallLocation(b, fl, rm)
	}
	if strings.TrimSpace(install) == "" {
		install = locRaw
	}
	mgmt, mgmtErr := mapImportManagementType(cellAt(row, idx, "관리유형"))
	if mgmtErr != "" {
		ir.Action = model.AssetImportError
		ir.Msgs = append(ir.Msgs, mgmtErr)
		return ir
	}
	ir.Asset = model.Asset{
		ProductName:       product,
		ProductType:       mapImportProductType(cellAt(row, idx, "제품구분")),
		ProductCategory:   mapImportProductCategory(cellAt(row, idx, "제품군")),
		ProductGroupID:    strings.TrimSpace(cellAt(row, idx, "제품군")),
		ModelName:         cellAt(row, idx, "모델명"),
		Manufacturer:      cellAt(row, idx, "제조사"),
		SerialNumber:      cellAt(row, idx, "시리얼"),
		InstallDate:       cellAt(row, idx, "설치일"),
		InstallerType:     mapImportInstaller(cellAt(row, idx, "설치주체")),
		OperationStatus:   mapImportOpStatus(cellAt(row, idx, "운영상태")),
		ManagementType:    mgmt,
		IsManaged:         true,
		MaintContractType: mapImportContract(cellAt(row, idx, "계약구분")),
		MaintCycle:        mapImportCycle(cellAt(row, idx, "점검주기")),
		MaintStartDate:    cellAt(row, idx, "유지보수시작"),
		MaintEndDate:      cellAt(row, idx, "유지보수종료"),
		LocBuildingName:   b,
		LocFloorName:      fl,
		LocRoomName:       rm,
		InstallLocation:   install,
		LocationDetail:    cellAt(row, idx, "상세위치"),
		RequesterName:     cellAt(row, idx, "고객담당"),
		OurContact:        cellAt(row, idx, "당사담당"),
		Notes:             cellAt(row, idx, "비고"),
	}
	if ir.Asset.OperationStatus == "" {
		ir.Asset.OperationStatus = model.AssetOpOperating
	}
	return ir
}

func excelFormulaUnsafe(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	switch s[0] {
	case '=', '+', '@':
		return true
	}
	return false
}

func mapImportProductType(s string) string {
	s = strings.TrimSpace(s)
	switch strings.ToUpper(s) {
	case "SW":
		return "SW"
	case "HW":
		return "HW"
	case "SERVER", "서버":
		return "SERVER"
	case "NETWORK", "네트워크장비":
		return "NETWORK"
	case "PERIPHERAL", "주변장비":
		return "PERIPHERAL"
	case "KIOSK", "키오스크":
		return "KIOSK"
	default:
		return s
	}
}

func mapImportProductCategory(s string) string {
	return ResolveProductCategoryValue(s)
}

func mapImportInstaller(s string) string {
	switch strings.TrimSpace(s) {
	case "자사", "self":
		return "self"
	case "타사", "other":
		return "other"
	case "제조사", "manufacturer":
		return "manufacturer"
	case "협력사", "partner":
		return "partner"
	case "미상", "unknown":
		return "unknown"
	default:
		return strings.TrimSpace(s)
	}
}

func mapImportOpStatus(s string) string {
	switch strings.TrimSpace(s) {
	case "운영중", "operating":
		return model.AssetOpOperating
	case "점검중", "maintenance":
		return model.AssetOpMaintenance
	case "장애", "fault":
		return model.AssetOpFault
	case "철수", "retired":
		return model.AssetOpRetired
	case "폐기", "disposed":
		return model.AssetOpDisposed
	default:
		return strings.TrimSpace(s)
	}
}

func mapImportManagementType(s string) (string, string) {
	s = strings.TrimSpace(s)
	switch s {
	case "", "장애대응", "fault":
		if s == "" {
			return "", ""
		}
		return model.ManagementTypeFault, ""
	case "정기점검", "periodic":
		return model.ManagementTypePeriodic, ""
	case "타사장비", "third_party":
		return model.ManagementTypeThirdParty, ""
	case "직접유지보수", "direct", "요청시지원", "on_demand", "참고관리", "reference":
		return "", "관리유형은 장애대응·정기점검·타사장비만 넣을 수 있습니다"
	default:
		if model.IsSelectableManagementType(s) {
			return s, ""
		}
		return "", "관리유형은 장애대응·정기점검·타사장비만 넣을 수 있습니다"
	}
}

func mapImportContract(s string) string {
	switch strings.TrimSpace(s) {
	case "유상", "paid":
		return model.MaintContractPaid
	case "무상", "free":
		return model.MaintContractFree
	case "미계약", "none":
		return model.MaintContractNone
	case "CALL", "call", "Call":
		return "call"
	default:
		return strings.TrimSpace(s)
	}
}

func mapImportCycle(s string) string {
	switch strings.TrimSpace(s) {
	case "월", "monthly":
		return "monthly"
	case "분기", "quarterly":
		return "quarterly"
	case "반기", "semi":
		return "semi"
	case "홀수격월", "odd_bimonthly":
		return "odd_bimonthly"
	case "짝수격월", "even_bimonthly":
		return "even_bimonthly"
	case "년1회", "yearly":
		return "yearly"
	case "CALL", "call", "Call":
		return model.MaintCycleCall
	default:
		return model.CanonicalMaintCycleCode(s)
	}
}
