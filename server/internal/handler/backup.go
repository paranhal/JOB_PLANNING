package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/audit"
	"customer-support/internal/backup"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type BackupHandler struct {
	cfg      backup.Config
	projects *repository.ProjectRepo
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
	return c.Render(http.StatusOK, "admin/data.html", map[string]interface{}{
		"Title":      "데이터 관리",
		"Active":     "data",
		"Tab":        tab,
		"Items":      items,
		"Logs":       logs,
		"LogCount":   logCount,
		"OldestLog":  oldest,
		"Stats":      stats,
		"Unresolved": unresolved,
		"OK":         ok,
		"Error":      errMsg,
		"DataPath":   filepath.ToSlash(filepath.Join(h.cfg.DataDir, "backups")),
		"Due":        audit.DueForArchive(),
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
