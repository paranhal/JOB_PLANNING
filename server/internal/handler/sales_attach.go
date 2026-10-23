package handler

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

func (h *AttachmentHandler) guardSalesProjectAttach(c echo.Context, salesID string, write bool) error {
	if h == nil || h.salesRepo == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "사업을 확인할 수 없습니다")
	}
	p, err := h.salesRepo.Get(strings.TrimSpace(salesID))
	if err != nil || p == nil {
		return echo.NewHTTPError(http.StatusNotFound, "영업 사업을 찾을 수 없습니다")
	}
	if write {
		if !canManageSalesProject(c, p) {
			return echo.ErrForbidden
		}
		return nil
	}
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	return nil
}

func (h *AttachmentHandler) saveSalesProjectFile(salesID, keywords string, file *multipart.FileHeader, seq int) error {
	if h == nil || file == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "파일이 필요합니다")
	}
	salesID, ok := safePathToken(salesID)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "잘못된 대상입니다")
	}
	n, err := h.repo.CountByRef(model.RefTypeSalesProject, salesID)
	if err != nil {
		return err
	}
	existing, _ := h.repo.ListByRef(model.RefTypeSalesProject, salesID)
	seen := map[string]bool{}
	for _, a := range existing {
		seen[strings.ToLower(strings.TrimSpace(a.FileName))] = true
	}
	head, err := peekUploadHead(file, 8)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "파일을 읽을 수 없습니다")
	}
	if msg := model.SalesAttachReject(file.Filename, file.Size, head, n, seen); msg != "" {
		return echo.NewHTTPError(http.StatusBadRequest, msg)
	}
	origName := safeUploadBaseName(file.Filename)
	storedName := fmt.Sprintf("%d_%d_%s", time.Now().UnixNano(), seq, origName)
	dir := filepath.Join(h.uploadDir, model.RefTypeSalesProject, salesID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	dstPath := filepath.Join(dir, storedName)
	if err := copyUploadFile(file, dstPath); err != nil {
		return err
	}
	return h.repo.Create(&model.Attachment{
		RefType:  model.RefTypeSalesProject,
		RefID:    salesID,
		FileName: origName,
		FilePath: filepath.ToSlash(dstPath),
		FileSize: file.Size,
		MIMEType: file.Header.Get("Content-Type"),
		Keywords: keywords,
	})
}

func peekUploadHead(file *multipart.FileHeader, n int) ([]byte, error) {
	src, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()
	buf := make([]byte, n)
	got, err := io.ReadFull(src, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, err
	}
	return buf[:got], nil
}

func (h *SalesHandler) saveInitialSalesFiles(c echo.Context, salesID string) []string {
	if h == nil || h.attach == nil || c == nil || c.Request() == nil {
		return nil
	}
	form := c.Request().MultipartForm
	if form == nil {
		_ = c.Request().ParseMultipartForm(model.SalesInitialAttachMaxRequest)
		form = c.Request().MultipartForm
	}
	if form == nil {
		return nil
	}
	files := nonemptyUploadHeaders(form.File[model.SalesInitialAttachField])
	if len(files) == 0 {
		return nil
	}
	var fails []string
	for i, fh := range files {
		if err := h.attach.saveSalesProjectFile(salesID, "", fh, i); err != nil {
			if he, ok := err.(*echo.HTTPError); ok {
				if msg, _ := he.Message.(string); msg != "" {
					fails = append(fails, msg)
					continue
				}
			}
			fails = append(fails, fh.Filename+" — 저장하지 못했습니다")
		}
	}
	return fails
}

func limitSalesUploadBody(c echo.Context) {
	if c == nil || c.Request() == nil {
		return
	}
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, model.SalesInitialAttachMaxRequest)
}
