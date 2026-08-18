package handler

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/hwpx"
	"customer-support/internal/model"
)

func (h *ASHandler) mergeASReportData(c echo.Context, as *model.ASReceipt, data map[string]interface{}) {
	if as == nil {
		return
	}
	embed, _ := data["Embed"].(bool)
	var reports []model.Attachment
	if h.attachRepo != nil {
		reports, _ = h.attachRepo.ListByRef(model.RefTypeASReport, as.ASID)
	}
	closed := isASClosedStatus(as.Status)
	canIssue := !embed && canProcessAS(c)
	reason := ""
	if !closed {
		reason = "완료된 건만 발급할 수 있습니다. 현재 상태: " + model.ASStatusDisplayLabel(as.Status)
	}
	draft := h.buildASReportDraft(as, time.Now())
	data["ASReports"] = reports
	data["CanIssueReport"] = canIssue
	data["ReportReady"] = closed
	data["ReportBlockReason"] = reason
	data["ReportMissing"] = draft.MissingReportFields()
}

func (h *ASHandler) ReportPreview(c echo.Context) error {
	as, err := h.loadAS(c.Param("id"))
	if err != nil {
		return err
	}
	if !canProcessAS(c) {
		return echo.ErrForbidden
	}
	closed := isASClosedStatus(as.Status)
	draft := h.buildASReportDraft(as, time.Now())
	data := map[string]interface{}{
		"Title":             "조치완료보고서",
		"Active":            "as",
		"AS":                as,
		"Draft":             draft,
		"ReportReady":       closed,
		"ReportBlockReason": "",
		"ReportMissing":     draft.MissingReportFields(),
		"ActionErr":         c.QueryParam("err"),
	}
	if !closed {
		data["ReportBlockReason"] = "완료된 건만 발급할 수 있습니다. 현재 상태: " + model.ASStatusDisplayLabel(as.Status)
	}
	return c.Render(http.StatusOK, "as/report.html", data)
}

func (h *ASHandler) ReportIssue(c echo.Context) error {
	as, err := h.loadAS(c.Param("id"))
	if err != nil {
		return err
	}
	if !canProcessAS(c) {
		return echo.ErrForbidden
	}
	if !isASClosedStatus(as.Status) {
		return h.redirectReportErr(c, as.ASID, "완료된 건만 발급할 수 있습니다. 현재 상태: "+model.ASStatusDisplayLabel(as.Status))
	}

	draft := reportDraftFromForm(c)
	if miss := draft.MissingReportFields(); len(miss) > 0 {
		return h.redirectReportErr(c, as.ASID, strings.Join(miss, "·")+"이(가) 비어 있습니다. 미리보기에서 입력하거나 조치 화면에서 채워 주세요.")
	}

	tpl, err := h.loadASReportTemplate()
	if err != nil {
		return h.redirectReportErr(c, as.ASID, err.Error())
	}
	data, err := hwpx.Replace(tpl, draft.Values())
	if err != nil {
		return h.redirectReportErr(c, as.ASID, "보고서를 만들지 못했습니다: "+err.Error())
	}

	now := time.Now()
	utf8Name := draft.Filename(now)
	dir := filepath.Join(h.reportUploadDir(), "as_report", as.ASID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return h.redirectReportErr(c, as.ASID, "저장 폴더를 만들지 못했습니다")
	}
	stored := now.Format("20060102150405") + "_" + model.SanitizeReportFilename(utf8Name)
	path := filepath.Join(dir, stored)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return h.redirectReportErr(c, as.ASID, "파일을 저장하지 못했습니다")
	}

	if h.attachRepo != nil {
		att := &model.Attachment{
			RefType:  model.RefTypeASReport,
			RefID:    as.ASID,
			FileName: stored,
			FilePath: path,
			FileSize: int64(len(data)),
			MIMEType: "application/hwp+zip",
			Keywords: model.ASReportIssuerPrefix + strings.TrimSpace(ctxString(c, "user_name")),
		}
		if err := h.attachRepo.Create(att); err != nil {
			return h.redirectReportErr(c, as.ASID, "발급 이력을 남기지 못했습니다")
		}
	}

	return writeDownloadBytes(c, data, "application/hwp+zip", "as_report.hwpx", utf8Name)
}

func (h *ASHandler) loadAS(id string) (*model.ASReceipt, error) {
	as, err := h.repo.GetByID(id)
	if err != nil || as == nil {
		return nil, echo.ErrNotFound
	}
	return as, nil
}

func (h *ASHandler) loadASReportTemplate() ([]byte, error) {
	if len(h.reportTemplateBytes) > 0 {
		return append([]byte(nil), h.reportTemplateBytes...), nil
	}
	path := h.reportTemplatePath
	if path == "" {
		path = hwpx.DefaultTemplatePath()
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		// 한글 서식 템플릿이 아직 없으면 자리표시자만 있는 최소 HWPX 를 쓴다. 원본 경로는 덮어쓰지 않는다.
		return hwpx.BuildPlaceholderTemplate(), nil
	}
	return data, nil
}

func (h *ASHandler) reportUploadDir() string {
	if h.attach != nil && h.attach.uploadDir != "" {
		return h.attach.uploadDir
	}
	return "data/uploads"
}

func (h *ASHandler) buildASReportDraft(as *model.ASReceipt, now time.Time) model.ASReportDraft {
	var customer *model.Customer
	if as != nil && as.CustomerID != "" && h.customerRepo != nil {
		customer, _ = h.customerRepo.GetByID(as.CustomerID)
	}
	var asset *model.Asset
	if as != nil && as.AssetID != "" && h.assetRepo != nil {
		asset, _ = h.assetRepo.GetByID(as.AssetID)
	}
	var contacts []model.Contact
	if as != nil && as.CustomerID != "" && h.contactRepo != nil {
		contacts, _ = h.contactRepo.ListByCustomer(as.CustomerID)
	}
	var processes []model.ASProcess
	if as != nil && h.processRepo != nil {
		processes, _ = h.processRepo.ListByAS(as.ASID)
	}
	return model.BuildASReportDraft(as, processes, customer, asset, contacts, now)
}

func reportDraftFromForm(c echo.Context) model.ASReportDraft {
	return model.ASReportDraft{
		CustomerName: c.FormValue("customer_name"),
		Department:   c.FormValue("department"),
		Manager:      c.FormValue("manager"),
		Phone:        c.FormValue("phone"),
		Service:      c.FormValue("service"),
		Symptom:      c.FormValue("symptom"),
		CauseDetail:  c.FormValue("cause_detail"),
		Conclusion:   c.FormValue("conclusion"),
		ReportDate:   c.FormValue("report_date"),
		Inspector:    c.FormValue("inspector"),
		Confirmer:    c.FormValue("confirmer"),
		WorkDates:    c.FormValue("work_dates"),
		Actions:      c.FormValue("actions"),
	}
}

func (h *ASHandler) redirectReportErr(c echo.Context, asID, msg string) error {
	return c.Redirect(http.StatusSeeOther, "/as/"+asID+"/report?err="+url.QueryEscape(msg))
}
