package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
)

type companyWeeklyDownload struct {
	Data      []byte
	ASCIIName string
	UTF8Name  string
	Expires   time.Time
}

var companyWeeklyOut = struct {
	sync.Mutex
	items map[string]companyWeeklyDownload
}{items: map[string]companyWeeklyDownload{}}

func putCompanyWeeklyDownload(item companyWeeklyDownload) string {
	companyWeeklyOut.Lock()
	defer companyWeeklyOut.Unlock()
	now := time.Now()
	for k, v := range companyWeeklyOut.items {
		if now.After(v.Expires) {
			delete(companyWeeklyOut.items, k)
		}
	}
	token := uuid.NewString()
	item.Expires = now.Add(5 * time.Minute)
	companyWeeklyOut.items[token] = item
	return token
}

func takeCompanyWeeklyDownload(token string) (companyWeeklyDownload, bool) {
	companyWeeklyOut.Lock()
	defer companyWeeklyOut.Unlock()
	item, ok := companyWeeklyOut.items[token]
	if !ok || time.Now().After(item.Expires) {
		delete(companyWeeklyOut.items, token)
		return companyWeeklyDownload{}, false
	}
	return item, true
}

// CreateCompanyWeekly 전사 주간업무보고 시트 생성. §16.6.2 · §16.6.11 · §16.6.12
func (h *StatsHandler) CreateCompanyWeekly(c echo.Context) error {
	now := time.Now()
	anchor := reportMeetingMonday(now)
	if t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(c.FormValue("company_date")), now.Location()); err == nil {
		anchor = t
	}
	draft, err := h.repo.BuildCompanyWeeklyDraftOrg(anchor, currentOrg(c))
	if err != nil {
		return err
	}
	sheetName := strings.TrimSpace(c.FormValue("sheet_name"))
	if sheetName == "" {
		sheetName = draft.Period.SheetNameDefault
	}
	draft.Rows = applyCompanyWeeklyFormEdits(draft.Rows, c.FormValue)

	src, uploadName, err := readCompanyWeeklyUpload(c)
	if err != nil {
		return c.Render(http.StatusBadRequest, "stats/company_weekly_result.html", companyWeeklyResultData(draft, sheetName, "", nil, err.Error()))
	}

	result, err := buildCompanyWeeklyWorkbook(companyWeeklyBuildInput{
		Source:    src,
		SheetName: sheetName,
		Period:    draft.Period,
		Rows:      draft.Rows,
	})
	src = nil
	if err != nil {
		accessLog(c, auditlog.Record{
			Action:      auditlog.ActionDownload,
			TargetTable: "company_weekly_report",
			TargetID:    sheetName,
			SubjectType: "report",
			SubjectID:   draft.Period.Anchor,
			SubjectName: uploadName,
			Detail:      err.Error(),
			Reason:      "전사 주간업무보고 시트 생성",
			Result:      auditlog.ResultDeny,
		})
		return c.Render(http.StatusBadRequest, "stats/company_weekly_result.html", companyWeeklyResultData(draft, sheetName, "", nil, err.Error()))
	}

	token := putCompanyWeeklyDownload(companyWeeklyDownload{
		Data:      result.Data,
		ASCIIName: result.ASCIIName,
		UTF8Name:  result.UTF8Name,
	})
	persistCompanyWeeklyFile(h.dataDir, token, companyWeeklyDownload{
		Data: result.Data, ASCIIName: result.ASCIIName, UTF8Name: result.UTF8Name,
	})
	accessLog(c, auditlog.Record{
		Action:      auditlog.ActionDownload,
		TargetTable: "company_weekly_report",
		TargetID:    sheetName,
		SubjectType: "report",
		SubjectID:   draft.Period.Anchor,
		SubjectName: result.UTF8Name,
		Detail:      companyWeeklyAuditDetail(result, uploadName),
		Reason:      "전사 주간업무보고 시트 생성",
		Result:      auditlog.ResultOK,
	})
	return c.Render(http.StatusOK, "stats/company_weekly_result.html", companyWeeklyResultData(draft, sheetName, token, &result, ""))
}

// DownloadCompanyWeekly 생성 결과 한 번만 내려받는다. 업로드 원본은 보관하지 않는다. §16.6.2
func (h *StatsHandler) DownloadCompanyWeekly(c echo.Context) error {
	token := strings.TrimSpace(c.QueryParam("token"))
	item, ok := takeCompanyWeeklyDownload(token)
	if !ok {
		item, ok = loadCompanyWeeklyFile(h.dataDir, token)
	}
	if !ok {
		return c.Render(http.StatusGone, "stats/company_weekly_result.html", map[string]interface{}{
			"Title":  "보고서",
			"Active": NavStatsReports,
			"Error":  "내려받기 기한이 지났습니다. 확인 > 보고서에서 다시 생성하세요.",
		})
	}
	return writeExcelFile(c, item.Data, item.ASCIIName, item.UTF8Name)
}

func readCompanyWeeklyUpload(c echo.Context) ([]byte, string, error) {
	fh, err := c.FormFile("file")
	if err != nil || fh == nil {
		return nil, "", fmt.Errorf("회사 주간업무보고 xlsx를 선택하세요")
	}
	if fh.Size > companyWeeklyMaxUpload {
		return nil, fh.Filename, fmt.Errorf("파일이 너무 큽니다 (최대 20MB)")
	}
	src, err := fh.Open()
	if err != nil {
		return nil, fh.Filename, fmt.Errorf("업로드 파일을 읽을 수 없습니다")
	}
	defer src.Close()
	data, err := io.ReadAll(io.LimitReader(src, companyWeeklyMaxUpload+1))
	if err != nil {
		return nil, fh.Filename, fmt.Errorf("업로드 파일을 읽을 수 없습니다")
	}
	if int64(len(data)) > companyWeeklyMaxUpload {
		return nil, fh.Filename, fmt.Errorf("파일이 너무 큽니다 (최대 20MB)")
	}
	if !strings.HasSuffix(strings.ToLower(fh.Filename), ".xlsx") {
		return nil, fh.Filename, fmt.Errorf("xlsx 파일만 올릴 수 있습니다")
	}
	return data, fh.Filename, nil
}

func companyWeeklyAuditDetail(r companyWeeklyBuildResult, uploadName string) string {
	mode := "시트추가"
	if r.Mode != companyWeeklyModeSheetAdd {
		mode = "한장짜리"
	}
	name := strings.TrimSpace(uploadName)
	if name == "" {
		name = "-"
	}
	return fmt.Sprintf("mode=%s sheets=%d formulas=%d upload=%s", mode, r.KeptSheets, r.KeptFormulas, name)
}

func companyWeeklyResultData(draft interface{}, sheetName, token string, result *companyWeeklyBuildResult, errMsg string) map[string]interface{} {
	data := map[string]interface{}{
		"Title":            "보고서",
		"Active":           NavStatsReports,
		"CompanyWeekly":    draft,
		"CompanySheetName": sheetName,
		"DownloadToken":    token,
		"Error":            errMsg,
	}
	if result != nil {
		data["BuildMode"] = result.Mode
		data["OrigSheets"] = result.OrigSheets
		data["KeptSheets"] = result.KeptSheets
		data["KeptFormulas"] = result.KeptFormulas
		data["FallbackReason"] = result.FallbackReason
		data["DownloadName"] = result.UTF8Name
		data["SheetAdded"] = result.Mode == companyWeeklyModeSheetAdd
	}
	return data
}

func companyWeeklyPersistDir(dataDir string) string {
	if strings.TrimSpace(dataDir) == "" {
		dataDir = "data"
	}
	return filepath.Join(dataDir, "uploads", "company_weekly")
}

func persistCompanyWeeklyFile(dataDir, token string, item companyWeeklyDownload) {
	token = strings.TrimSpace(token)
	if token == "" || len(item.Data) == 0 {
		return
	}
	dir := companyWeeklyPersistDir(dataDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("company weekly persist mkdir: %v", err)
		return
	}
	if err := os.WriteFile(filepath.Join(dir, token+".xlsx"), item.Data, 0644); err != nil {
		log.Printf("company weekly persist xlsx: %v", err)
		return
	}
	meta, _ := json.Marshal(map[string]string{"ascii": item.ASCIIName, "utf8": item.UTF8Name})
	_ = os.WriteFile(filepath.Join(dir, token+".json"), meta, 0644)
}

func loadCompanyWeeklyFile(dataDir, token string) (companyWeeklyDownload, bool) {
	token = strings.TrimSpace(token)
	if token == "" || strings.ContainsAny(token, `/\`) {
		return companyWeeklyDownload{}, false
	}
	path := filepath.Join(companyWeeklyPersistDir(dataDir), token+".xlsx")
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return companyWeeklyDownload{}, false
	}
	item := companyWeeklyDownload{Data: data, ASCIIName: "company-weekly.xlsx", UTF8Name: "전사주간업무보고.xlsx"}
	if raw, err := os.ReadFile(filepath.Join(companyWeeklyPersistDir(dataDir), token+".json")); err == nil {
		var meta map[string]string
		if json.Unmarshal(raw, &meta) == nil {
			if meta["ascii"] != "" {
				item.ASCIIName = meta["ascii"]
			}
			if meta["utf8"] != "" {
				item.UTF8Name = meta["utf8"]
			}
		}
	}
	return item, true
}
