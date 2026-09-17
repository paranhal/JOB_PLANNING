package handler

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type SystemHandler struct {
	db        *sql.DB
	auth      *AuthHandler
	uploadDir string
}

func (h *SystemHandler) Page(c echo.Context) error {
	if !isAdminRole(c) {
		if h.auth != nil {
			return h.auth.forbidden(c)
		}
		return echo.ErrForbidden
	}
	version, commit, built, started := currentBuild()
	app := repository.AppSchemaVersion
	dbVer := repository.AppliedSchemaVersion(h.db)
	ok := app == dbVer
	hist, err := repository.ListAppVersions(h.db, 20)
	if err != nil {
		hist = nil
	}
	mismatch := repository.SchemaMismatchMessage(h.db)
	upRoot := strings.TrimSpace(h.uploadDir)
	if upRoot == "" {
		upRoot = "data/uploads"
	}
	upBytes, upFiles := repository.DirSize(upRoot)
	upWarn := ""
	upClass := "text-slate-700"
	if upBytes >= model.UploadAlertBytes {
		upWarn = "첨부 용량이 5GB를 넘었습니다. 백업에서 파일을 뺄지 정하세요."
		upClass = "text-red-700"
	} else if upBytes >= model.UploadWarnBytes {
		upWarn = "첨부 용량이 1GB를 넘었습니다. 백업 크기를 확인하세요."
		upClass = "text-amber-800"
	}
	return c.Render(http.StatusOK, "admin/system.html", map[string]interface{}{
		"Title":          "시스템 정보",
		"Active":         NavSystem,
		"Version":        version,
		"Commit":         commit,
		"Built":          built,
		"Started":        formatClock(started),
		"AppSchema":      repository.SchemaNo(app),
		"DBSchema":       repository.SchemaNo(dbVer),
		"SchemaOK":       ok,
		"SchemaMismatch": mismatch,
		"History":        hist,
		"UploadPath":     upRoot,
		"UploadBytes":    upBytes,
		"UploadFiles":    upFiles,
		"UploadLabel":    model.FormatByteSize(upBytes),
		"UploadWarn":     upWarn,
		"UploadClass":    upClass,
	})
}

var schemaNoticeCached string

func bindSchemaDB(db *sql.DB) {
	if db == nil {
		schemaNoticeCached = ""
		return
	}
	schemaNoticeCached = strings.TrimSpace(repository.SchemaMismatchMessage(db))
}

func currentSchemaNotice() string {
	return schemaNoticeCached
}
