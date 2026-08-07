package handler

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type AttachmentHandler struct {
	repo      *repository.AttachmentRepo
	uploadDir string
}

func (h *AttachmentHandler) Upload(c echo.Context) error {
	refType := c.FormValue("ref_type")
	refID := c.FormValue("ref_id")
	keywords := strings.TrimSpace(c.FormValue("keywords"))

	if !canUploadAttachment(c, refType) {
		return echo.ErrForbidden
	}

	file, err := c.FormFile("file")
	if err != nil {
		return c.String(http.StatusBadRequest, "파일이 필요합니다")
	}

	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	var (
		dstPath  string
		filename string
		slotNo   int
	)

	if refType == "asset" {
		n, _ := h.repo.CountByRef("asset", refID)
		if n >= 3 {
			return c.String(http.StatusBadRequest, "설치자산 이미지는 최대 3장까지입니다")
		}
		slotNo, err = h.repo.NextAssetImageSlot(refID)
		if err != nil {
			return err
		}
		if slotNo == 0 {
			return c.String(http.StatusBadRequest, "설치자산 이미지는 최대 3장까지입니다")
		}
		if v := strings.TrimSpace(c.FormValue("slot_no")); v != "" {
			if s, e := strconv.Atoi(v); e == nil && s >= 1 && s <= 3 {
				slotNo = s
			}
		}
		existing, _ := h.repo.ListByRef("asset", refID)
		for _, a := range existing {
			if a.SlotNo == slotNo {
				return c.String(http.StatusBadRequest, "해당 슬롯에 이미 이미지가 있습니다")
			}
		}
		dir := repository.AssetImageDir(h.uploadDir, refID)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		filename = repository.AssetImageFileName(refID, slotNo, file.Filename)
		dstPath = filepath.Join(dir, filename)
	} else {
		dir := filepath.Join(h.uploadDir, refType, refID)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		filename = fmt.Sprintf("%d_%s", time.Now().UnixNano(), file.Filename)
		dstPath = filepath.Join(dir, filename)
	}

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
		FileName: filename,
		FilePath: dstPath,
		FileSize: file.Size,
		MIMEType: file.Header.Get("Content-Type"),
		Keywords: keywords,
		SlotNo:   slotNo,
	}
	if err := h.repo.Create(att); err != nil {
		return err
	}

	redirect := c.FormValue("redirect")
	if redirect == "" {
		redirect = "/"
	}
	return c.Redirect(http.StatusSeeOther, redirect)
}

func canUploadAttachment(c echo.Context, refType string) bool {
	switch strings.TrimSpace(refType) {
	case "as":
		return canProcessAS(c) || canWriteMaster(c)
	default:
		return canWriteMaster(c)
	}
}

func (h *AttachmentHandler) UpdateKeywords(c echo.Context) error {
	id := c.Param("id")
	att, _ := h.repo.GetByID(id)
	if att != nil && !canUploadAttachment(c, att.RefType) {
		return echo.ErrForbidden
	}
	keywords := strings.TrimSpace(c.FormValue("keywords"))
	if err := h.repo.UpdateKeywords(id, keywords); err != nil {
		return err
	}
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
		if !canUploadAttachment(c, att.RefType) {
			return echo.ErrForbidden
		}
		os.Remove(att.FilePath)
	}
	h.repo.Delete(c.Param("id"))
	redirect := c.FormValue("redirect")
	if redirect == "" {
		redirect = c.QueryParam("redirect")
	}
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
