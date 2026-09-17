package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/repository"
)

// ProcessConflictsExcel 접수·이력 조치 본문이 다른 건 xlsx. 사람이 보기 전에는 지우지 않는다. §44.8.4
func (h *BackupHandler) ProcessConflictsExcel(c echo.Context) error {
	if h.cfg.DB == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "DB 없음")
	}
	items := repository.ListActionConflicts(h.cfg.DB)
	file, err := buildActionConflictExcel(items)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	buf, err := file.WriteToBuffer()
	if err != nil {
		return err
	}
	return writeExcelDownload(c, buf.Bytes(), "as_process_conflicts")
}

func buildActionConflictExcel(items []repository.ActionProcessConflict) (*excelize.File, error) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	if sheet == "" {
		sheet = "Sheet1"
	}
	if err := f.SetSheetName(sheet, "조치어긋남"); err != nil {
		return nil, err
	}
	sheet = "조치어긋남"
	headers := []string{
		"AS번호", "사이트",
		"접수_조치", "이력_조치",
		"접수_원인", "이력_원인",
		"접수_부품", "이력_부품",
		"접수_결과", "이력_결과",
		"접수_처리유형", "이력_처리유형",
		"접수_이관", "이력_이관",
		"as_id",
	}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}
	for r, it := range items {
		vals := []interface{}{
			it.ASNumber, it.OrgName,
			it.ReceiptAction, it.ProcessAction,
			it.ReceiptCause, it.ProcessCause,
			it.ReceiptParts, it.ProcessParts,
			it.ReceiptResult, it.ProcessResult,
			it.ReceiptProcessType, it.ProcessWorkType,
			it.ReceiptTransfer, it.ProcessTransfer,
			it.ASID,
		}
		for c, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
			_ = f.SetCellValue(sheet, cell, v)
		}
	}
	return f, nil
}
