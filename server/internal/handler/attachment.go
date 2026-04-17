package handler

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type AttachmentHandler struct {
	repo       *repository.AttachmentRepo
	uploadDir  string
}

func (h *AttachmentHandler) Upload(c echo.Context) error {
	refType := c.FormValue("ref_type")
	refID := c.FormValue("ref_id")

	file, err := c.FormFile("file")
	if err != nil {
		return c.String(http.StatusBadRequest, "파일이 필요합니다")
	}

	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	dir := filepath.Join(h.uploadDir, refType, refID)
	os.MkdirAll(dir, 0755)

	filename := fmt.Sprintf("%d_%s", time.Now().UnixNano(), file.Filename)
	dstPath := filepath.Join(dir, filename)

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err = io.Copy(dst, src); err != nil {
		return err
	}

	att := &model.Attachment{
		RefType:  refType,
		RefID:    refID,
		FileName: file.Filename,
		FilePath: dstPath,
		FileSize: file.Size,
		MIMEType: file.Header.Get("Content-Type"),
	}
	h.repo.Create(att)

	redirect := c.FormValue("redirect")
	if redirect == "" {
		redirect = "/"
	}
	return c.Redirect(http.StatusSeeOther, redirect)
}

func (h *AttachmentHandler) Download(c echo.Context) error {
	att, err := h.repo.GetByID(c.Param("id"))
	if err != nil || att == nil {
		return echo.ErrNotFound
	}
	return c.Attachment(att.FilePath, att.FileName)
}

func (h *AttachmentHandler) Delete(c echo.Context) error {
	att, _ := h.repo.GetByID(c.Param("id"))
	if att != nil {
		os.Remove(att.FilePath)
	}
	h.repo.Delete(c.Param("id"))
	redirect := c.QueryParam("redirect")
	if redirect == "" {
		redirect = "/"
	}
	return c.Redirect(http.StatusSeeOther, redirect)
}

func (h *AttachmentHandler) ListJSON(c echo.Context) error {
	refType := c.QueryParam("ref_type")
	refID := c.QueryParam("ref_id")
	items, _ := h.repo.ListByRef(refType, refID)
	return c.JSON(http.StatusOK, items)
}
