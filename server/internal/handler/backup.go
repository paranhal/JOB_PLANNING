package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/audit"
	"customer-support/internal/backup"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type BackupHandler struct {
	cfg        backup.Config
	projects   *repository.ProjectRepo
	settings   *repository.SettingsRepo
	importRepo *repository.ASImportRepo
	customers  *repository.CustomerRepo
}

func dataDirFromEnv() string {
	p := strings.TrimSpace(os.Getenv("DB_PATH"))
	if p == "" {
		p = "data/app.db"
	}
	return filepath.Dir(p)
}

func (h *BackupHandler) RedirectLegacy(c echo.Context) error {
	q := c.QueryString()
	target := "/admin/data"
	if q != "" {
		target += "?" + q
	}
	return c.Redirect(http.StatusSeeOther, target)
}

func (h *BackupHandler) Page(c echo.Context) error {
	if _, err := backup.SyncIndex(h.cfg); err != nil {
		return err
	}
	items, err := backup.List(h.cfg.DataDir)
	if err != nil {
		return err
	}
	logs, _ := audit.ListLogs(150)
	logCount, oldest := audit.LogStats()
	tab := strings.TrimSpace(c.QueryParam("tab"))
	if tab == "" {
		tab = "backup"
	}
	name := strings.TrimSpace(c.QueryParam("name"))
	var stats *audit.SnapshotStats
	if name != "" {
		st := audit.AnalyzeBackup(h.cfg.DataDir, name)
		stats = &st
		tab = "analyze"
	}
	ok := strings.TrimSpace(c.QueryParam("ok"))
	errMsg := strings.TrimSpace(c.QueryParam("err"))
	var unresolved []model.UnresolvedProjectRow
	if tab == "unassigned" && h.projects != nil {
		unresolved, _ = h.projects.ListUnresolvedProjectRows()
	}
	var checks []repository.AppendixCCheck
	if tab == "checks" && h.cfg.DB != nil {
		checks = repository.RunAppendixC(h.cfg.DB, true)
	}
	metricsBase := ""
	if h.settings != nil {
		metricsBase, _ = h.settings.Get(repository.SettingMetricsBaseDate)
	}
	var importBatches []model.ASImportBatch
	var importBatch *model.ASImportBatch
	var importRows []model.ASImportRow
	var importCustomers []model.Customer
	if tab == "import" && h.importRepo != nil {
		importBatches, _ = h.importRepo.ListBatches()
		if bid := strings.TrimSpace(c.QueryParam("batch")); bid != "" {
			importBatch, _ = h.importRepo.GetBatch(bid)
			importRows, _ = h.importRepo.ListRows(bid)
		}
		if h.customers != nil {
			importCustomers, _ = h.customers.ListAll()
		}
	}
	var unmatched []repository.UnmatchedAssignee
	if tab == "assignees" && h.cfg.DB != nil {
		unmatched = repository.ListUnmatchedAssignees(h.cfg.DB)
	}
	var processConflicts []repository.ActionProcessConflict
	if tab == "processes" && h.cfg.DB != nil {
		processConflicts = repository.ListActionConflicts(h.cfg.DB)
	}
	return c.Render(http.StatusOK, "admin/data.html", map[string]interface{}{
		"Title":           "데이터 관리",
		"Active":          NavData,
		"Tab":             tab,
		"Items":           items,
		"Logs":            logs,
		"LogCount":        logCount,
		"OldestLog":       oldest,
		"Stats":           stats,
		"Unresolved":      unresolved,
		"Checks":          checks,
		"UnmatchedAssignees": unmatched,
		"ProcessConflicts": processConflicts,
		"OK":              ok,
		"Error":           errMsg,
		"DataPath":        filepath.ToSlash(filepath.Join(h.cfg.DataDir, "backups")),
		"Due":             audit.DueForArchive(),
		"MetricsBaseDate": metricsBase,
		"ImportBatches":   importBatches,
		"ImportBatch":     importBatch,
		"ImportRows":      importRows,
		"ImportErrors":    importIssueRows(importRows, true),
		"ImportReady":     importIssueRows(importRows, false),
		"ImportCustomers": importCustomers,
	})
}

func (h *BackupHandler) Save(c echo.Context) error {
	name, err := backup.Snapshot(h.cfg, backup.KindManual)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/data?err="+url.QueryEscape(fmt.Sprintf("저장 실패: %v", err)))
	}
	return c.Redirect(http.StatusSeeOther, "/admin/data?ok="+url.QueryEscape("현재 데이터를 저장했습니다. 폴더: "+name))
}

func (h *BackupHandler) Rollback(c echo.Context) error {
	id := strings.TrimSpace(c.FormValue("log_id"))
	if err := audit.Rollback(id); err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/data?tab=logs&err="+url.QueryEscape(err.Error()))
	}
	return c.Redirect(http.StatusSeeOther, "/admin/data?tab=logs&ok="+url.QueryEscape("이력을 롤백했습니다."))
}

func (h *BackupHandler) Archive(c echo.Context) error {
	name, n, err := audit.ArchiveLogs(h.cfg.DataDir)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/data?tab=logs&err="+url.QueryEscape(err.Error()))
	}
	msg := fmt.Sprintf("관리 로그 %d건을 백업한 뒤 테이블을 비웠습니다. 폴더: %s", n, name)
	return c.Redirect(http.StatusSeeOther, "/admin/data?ok="+url.QueryEscape(msg))
}

func (h *BackupHandler) SaveMetrics(c echo.Context) error {
	redirect := func(msg, errMsg string) error {
		q := "/admin/data?tab=metrics"
		if errMsg != "" {
			return c.Redirect(http.StatusSeeOther, q+"&err="+url.QueryEscape(errMsg))
		}
		return c.Redirect(http.StatusSeeOther, q+"&ok="+url.QueryEscape(msg))
	}
	if h.settings == nil {
		return redirect("", "설정 저장소를 쓸 수 없습니다")
	}
	base := strings.TrimSpace(c.FormValue("metrics_base_date"))
	if base != "" {
		if _, err := time.Parse("2006-01-02", base); err != nil {
			return redirect("", "기준일은 YYYY-MM-DD 형식이어야 합니다")
		}
	}
	if err := h.settings.Set(repository.SettingMetricsBaseDate, base); err != nil {
		return redirect("", fmt.Sprintf("기준일 저장 실패: %v", err))
	}
	return redirect("지표 기준을 저장했습니다. 통계·보고서·대시보드 KPI가 함께 바뀝니다.", "")
}
