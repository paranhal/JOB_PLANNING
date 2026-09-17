package handler

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/imageproc"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type AttachmentHandler struct {
	repo       *repository.AttachmentRepo
	asRepo     *repository.ASRepo
	unlockRepo *repository.ASUnlockRepo
	uploadDir  string
}

func (h *AttachmentHandler) Upload(c echo.Context) error {
	refType := strings.TrimSpace(c.FormValue("ref_type"))
	refID := strings.TrimSpace(c.FormValue("ref_id"))
	keywords := strings.TrimSpace(c.FormValue("keywords"))
	redirect := attachmentRedirect(c, "/")

	if !canUploadAttachment(c, refType) {
		return redirectAttachFailure(c, redirect, echo.ErrForbidden)
	}
	refType, ok := safePathToken(refType)
	if !ok {
		return redirectAttachErr(c, redirect, "잘못된 첨부 구분입니다", http.StatusBadRequest)
	}
	refID, ok = safePathToken(refID)
	if !ok {
		return redirectAttachErr(c, redirect, "잘못된 대상입니다", http.StatusBadRequest)
	}

	headers, err := uploadFileHeaders(c)
	if err != nil || len(headers) == 0 {
		return redirectAttachErr(c, redirect, "attach_file", http.StatusBadRequest)
	}

	if refType == model.AttachFormReceipt {
		if err := h.guardReceiptPhotoWrite(c, refID); err != nil {
			return redirectAttachFailure(c, redirect, err)
		}
		if err := h.saveReceiptPhotos(refID, keywords, headers); err != nil {
			return redirectAttachFailure(c, redirect, err)
		}
		return c.Redirect(http.StatusSeeOther, redirect)
	}

	if refType == model.RefTypeASActionPhoto {
		if err := h.guardActionPhotoWrite(c, refID); err != nil {
			return redirectAttachFailure(c, redirect, err)
		}
		if err := h.saveActionPhotos(refID, keywords, headers); err != nil {
			return redirectAttachFailure(c, redirect, err)
		}
		return c.Redirect(http.StatusSeeOther, redirect)
	}

	if refType == model.RefTypeASKB {
		if err := h.saveKBFiles(refID, keywords, headers); err != nil {
			return redirectAttachFailure(c, redirect, err)
		}
		return c.Redirect(http.StatusSeeOther, redirect)
	}

	if refType == model.RefTypeAsset {
		slotOverride := 0
		if v := strings.TrimSpace(c.FormValue("slot_no")); v != "" {
			if s, e := strconv.Atoi(v); e == nil && s >= 1 && s <= 3 {
				slotOverride = s
			}
		}
		if err := h.saveAssetFile(refID, keywords, headers[0], slotOverride); err != nil {
			if httpErr, ok := err.(*echo.HTTPError); ok {
				msg, _ := httpErr.Message.(string)
				return c.String(httpErr.Code, msg)
			}
			return err
		}
	} else {
		for i, fh := range headers {
			if err := h.saveGenericFile(refType, refID, keywords, fh, i); err != nil {
				return redirectAttachFailure(c, redirect, err)
			}
		}
	}

	return c.Redirect(http.StatusSeeOther, redirect)
}

func uploadFileHeaders(c echo.Context) ([]*multipart.FileHeader, error) {
	form, err := c.MultipartForm()
	if err == nil && form != nil && len(form.File["file"]) > 0 {
		return nonemptyUploadHeaders(form.File["file"]), nil
	}
	if err == nil && form != nil && len(form.File["receipt_photo"]) > 0 {
		return nonemptyUploadHeaders(form.File["receipt_photo"]), nil
	}
	fh, err := c.FormFile("file")
	if err != nil {
		return nil, err
	}
	return nonemptyUploadHeaders([]*multipart.FileHeader{fh}), nil
}

func nonemptyUploadHeaders(files []*multipart.FileHeader) []*multipart.FileHeader {
	out := make([]*multipart.FileHeader, 0, len(files))
	for _, fh := range files {
		if fh == nil {
			continue
		}
		if strings.TrimSpace(fh.Filename) == "" && fh.Size == 0 {
			continue
		}
		out = append(out, fh)
	}
	return out
}

func (h *AttachmentHandler) saveAssetFile(refID, keywords string, file *multipart.FileHeader, slotOverride int) error {
	n, _ := h.repo.CountByRef(model.AttachRefAsset, refID)
	if n >= 3 {
		return echo.NewHTTPError(http.StatusBadRequest, "설치자산 이미지는 최대 3장까지입니다")
	}
	slotNo, err := h.repo.NextAssetImageSlot(refID)
	if err != nil {
		return err
	}
	if slotOverride >= 1 && slotOverride <= 3 {
		slotNo = slotOverride
	}
	if slotNo == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "설치자산 이미지는 최대 3장까지입니다")
	}
	existing, _ := h.repo.ListByRef(model.AttachRefAsset, refID)
	for _, a := range existing {
		if a.SlotNo == slotNo {
			return echo.NewHTTPError(http.StatusBadRequest, "해당 슬롯에 이미 이미지가 있습니다")
		}
	}
	dir := repository.AssetImageDir(h.uploadDir, refID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	filename := repository.AssetImageFileName(refID, slotNo, file.Filename)
	dstPath := filepath.Join(dir, filename)
	if err := copyUploadFile(file, dstPath); err != nil {
		return err
	}
	return h.repo.Create(&model.Attachment{
		RefType:  model.AttachRefAsset,
		RefID:    refID,
		FileName: filename,
		FilePath: filepath.ToSlash(dstPath),
		FileSize: file.Size,
		MIMEType: file.Header.Get("Content-Type"),
		Keywords: keywords,
		SlotNo:   slotNo,
	})
}

func (h *AttachmentHandler) saveGenericFile(refType, refID, keywords string, file *multipart.FileHeader, seq int) error {
	origName := safeUploadBaseName(file.Filename)
	storedName := fmt.Sprintf("%d_%d_%s", time.Now().UnixNano(), seq, origName)
	dir := filepath.Join(h.uploadDir, refType, refID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	dstPath := filepath.Join(dir, storedName)
	if err := copyUploadFile(file, dstPath); err != nil {
		return err
	}
	return h.repo.Create(&model.Attachment{
		RefType:  refType,
		RefID:    refID,
		FileName: origName,
		FilePath: filepath.ToSlash(dstPath),
		FileSize: file.Size,
		MIMEType: file.Header.Get("Content-Type"),
		Keywords: keywords,
	})
}

func (h *AttachmentHandler) saveReceiptPhotos(asID, keywords string, files []*multipart.FileHeader) error {
	return h.saveProcessedPhotos(processedPhotoOpts{
		refType: model.AttachRefAS, asID: asID, keywords: keywords,
		max: model.MaxReceiptPhotos, dir: repository.ReceiptPhotoDir(h.uploadDir, asID),
		errMax: fmt.Sprintf("접수 사진은 최대 %d장입니다", model.MaxReceiptPhotos),
	}, files)
}

func (h *AttachmentHandler) saveActionPhotos(asID, keywords string, files []*multipart.FileHeader) error {
	return h.saveProcessedPhotos(processedPhotoOpts{
		refType: model.RefTypeASActionPhoto, asID: asID, keywords: keywords,
		max: model.MaxActionPhotos, dir: repository.ActionPhotoDir(h.uploadDir, asID),
		imageOnly: true,
		errMax:    "attach_photo_max",
	}, files)
}

func (h *AttachmentHandler) saveKBFiles(kbID, keywords string, files []*multipart.FileHeader) error {
	kbID = strings.TrimSpace(kbID)
	if kbID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "잘못된 대상입니다")
	}
	existing, err := h.repo.ListByRef(model.RefTypeASKB, kbID)
	if err != nil {
		return err
	}
	videos := 0
	for _, a := range existing {
		if a.IsVideo() {
			videos++
		}
	}
	dir := repository.KBAttachDir(h.uploadDir, kbID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	opt := processedPhotoOpts{
		refType: model.RefTypeASKB, asID: kbID, keywords: keywords, dir: dir,
	}
	for i, fh := range files {
		if fh == nil {
			continue
		}
		name := safeUploadBaseName(fh.Filename)
		mime := fh.Header.Get("Content-Type")
		if msg := model.KBAttachReject(name, mime, fh.Size, videos); msg != "" {
			return echo.NewHTTPError(http.StatusBadRequest, msg)
		}
		if model.LooksLikeVideo(name, mime) {
			if err := h.saveKBVideo(opt, fh, i); err != nil {
				return err
			}
			videos++
			continue
		}
		if imageproc.LooksLikeImage(name, mime) {
			if err := h.saveOneProcessedPhoto(opt, fh, i); err != nil {
				return err
			}
			continue
		}
		if err := h.saveKBDocument(opt, fh, i); err != nil {
			return err
		}
	}
	return nil
}

func (h *AttachmentHandler) saveKBVideo(opt processedPhotoOpts, file *multipart.FileHeader, seq int) error {
	origName := safeUploadBaseName(file.Filename)
	stored := fmt.Sprintf("%d_%d_%s", time.Now().UnixNano(), seq, origName)
	dstPath := filepath.Join(opt.dir, stored)
	if err := copyUploadFile(file, dstPath); err != nil {
		return err
	}
	st, err := os.Stat(dstPath)
	if err != nil {
		os.Remove(dstPath)
		return err
	}
	if st.Size() > model.MaxKBVideoBytes {
		os.Remove(dstPath)
		return echo.NewHTTPError(http.StatusBadRequest, model.SizeLimitMessage(model.MaxKBVideoBytes, st.Size()))
	}
	return h.repo.Create(&model.Attachment{
		RefType:  opt.refType,
		RefID:    opt.asID,
		FileName: origName,
		FilePath: filepath.ToSlash(dstPath),
		FileSize: st.Size(),
		MIMEType: model.VideoMIME(origName),
		Keywords: opt.keywords,
	})
}

func (h *AttachmentHandler) saveKBDocument(opt processedPhotoOpts, file *multipart.FileHeader, seq int) error {
	origName := safeUploadBaseName(file.Filename)
	stored := fmt.Sprintf("%d_%d_%s", time.Now().UnixNano(), seq, origName)
	dstPath := filepath.Join(opt.dir, stored)
	if err := copyUploadFile(file, dstPath); err != nil {
		return err
	}
	st, _ := os.Stat(dstPath)
	size := file.Size
	if st != nil {
		size = st.Size()
	}
	mime := file.Header.Get("Content-Type")
	if mime == "" {
		mime = "application/octet-stream"
	}
	return h.repo.Create(&model.Attachment{
		RefType:  opt.refType,
		RefID:    opt.asID,
		FileName: origName,
		FilePath: filepath.ToSlash(dstPath),
		FileSize: size,
		MIMEType: mime,
		Keywords: opt.keywords,
	})
}

type processedPhotoOpts struct {
	refType   string
	asID      string
	keywords  string
	max       int
	dir       string
	imageOnly bool
	errMax    string
}

func (h *AttachmentHandler) saveProcessedPhotos(opt processedPhotoOpts, files []*multipart.FileHeader) error {
	n, err := 0, error(nil)
	if opt.refType == model.AttachRefAS {
		n, err = h.repo.CountReceiptPhotos(opt.asID)
	} else {
		n, err = h.repo.CountByRef(opt.refType, opt.asID)
	}
	if err != nil {
		return err
	}
	if n+len(files) > opt.max {
		return echo.NewHTTPError(http.StatusBadRequest, opt.errMax)
	}
	if err := os.MkdirAll(opt.dir, 0755); err != nil {
		return err
	}
	for i, fh := range files {
		if fh.Size > model.MaxReceiptBytes {
			return echo.NewHTTPError(http.StatusBadRequest, "파일은 20MB 이하여야 합니다")
		}
		if err := h.saveOneProcessedPhoto(opt, fh, i); err != nil {
			return err
		}
	}
	return nil
}

func (h *AttachmentHandler) saveOneProcessedPhoto(opt processedPhotoOpts, file *multipart.FileHeader, seq int) error {
	origName := safeUploadBaseName(file.Filename)
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	mime := file.Header.Get("Content-Type")
	if imageproc.LooksLikeImage(origName, mime) {
		out, err := imageproc.Process(src, origName, mime)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		stored := fmt.Sprintf("%d_%d%s", time.Now().UnixNano(), seq, out.StoredExt)
		dstPath := filepath.Join(opt.dir, stored)
		if err := os.WriteFile(dstPath, out.Full, 0644); err != nil {
			return err
		}
		if err := os.WriteFile(imageproc.ThumbPath(dstPath), out.Thumb, 0644); err != nil {
			return err
		}
		return h.repo.Create(&model.Attachment{
			RefType:  opt.refType,
			RefID:    opt.asID,
			FileName: origName,
			FilePath: filepath.ToSlash(dstPath),
			FileSize: int64(len(out.Full)),
			MIMEType: out.MIMEType,
			Keywords: opt.keywords,
		})
	}

	if opt.imageOnly {
		return echo.NewHTTPError(http.StatusBadRequest, "attach_not_image")
	}

	stored := fmt.Sprintf("%d_%d_%s", time.Now().UnixNano(), seq, origName)
	dstPath := filepath.Join(opt.dir, stored)
	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(dst, src)
	dst.Close()
	if copyErr != nil {
		os.Remove(dstPath)
		return copyErr
	}
	if written > model.MaxReceiptBytes {
		os.Remove(dstPath)
		return echo.NewHTTPError(http.StatusBadRequest, "파일은 20MB 이하여야 합니다")
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	return h.repo.Create(&model.Attachment{
		RefType:  opt.refType,
		RefID:    opt.asID,
		FileName: origName,
		FilePath: filepath.ToSlash(dstPath),
		FileSize: written,
		MIMEType: mime,
		Keywords: opt.keywords,
	})
}

func copyUploadFile(file *multipart.FileHeader, dstPath string) error {
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	return err
}

// safePathToken 폴더명으로 쓸 값. 경로 탈출을 막는다.
func safePathToken(s string) (string, bool) {
	s = strings.TrimSpace(s)
	s = filepath.Base(strings.ReplaceAll(s, "\\", "/"))
	if s == "" || s == "." || s == ".." || strings.Contains(s, "..") {
		return "", false
	}
	if strings.ContainsAny(s, `/\`) {
		return "", false
	}
	return s, true
}

// safeUploadBaseName 원본 파일명만 남긴다. 경로·상위 폴더는 제거.
func safeUploadBaseName(orig string) string {
	name := filepath.Base(strings.ReplaceAll(strings.TrimSpace(orig), "\\", "/"))
	name = strings.Trim(name, " .")
	name = strings.ReplaceAll(name, "..", "_")
	name = strings.ReplaceAll(name, "/", "_")
	if name == "" || name == "." || name == "_" {
		return "file"
	}
	return name
}

func isReceiptPhoto(att *model.Attachment) bool {
	if att == nil {
		return false
	}
	if att.RefType == model.AttachFormReceipt {
		return true
	}
	if model.CanonicalAttachRef(att.RefType) != model.AttachRefAS {
		return false
	}
	p := filepath.ToSlash(att.FilePath)
	return strings.Contains(p, "/receipt/")
}

func canUploadAttachment(c echo.Context, refType string) bool {
	switch strings.TrimSpace(refType) {
	case model.RefTypeAS:
		return canProcessAS(c) || canWriteMaster(c)
	case model.RefTypeASReport:
		return canProcessAS(c) || canWriteMaster(c)
	case model.RefTypeASReceipt:
		return canReceiveAS(c) || canWriteMaster(c) || canProcessAS(c)
	case model.RefTypeASActionPhoto:
		return canProcessAS(c) || canWriteMaster(c)
	case model.RefTypeASKB:
		return canProcessAS(c) || canWriteMaster(c)
	case model.RefTypeWorkActivity:
		return canWriteWorkboard(c)
	default:
		return canWriteMaster(c)
	}
}

func canManageReceiptPhoto(c echo.Context, as *model.ASReceipt) bool {
	if canReceiveAS(c) || canWriteMaster(c) {
		return true
	}
	return canEditVisitDate(c, as)
}

func (h *AttachmentHandler) guardReceiptPhotoWrite(c echo.Context, asID string) error {
	if h.asRepo == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "접수 정보를 확인할 수 없습니다")
	}
	as, err := h.asRepo.GetByID(asID)
	if err != nil || as == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "접수를 찾을 수 없습니다")
	}
	if !canManageReceiptPhoto(c, as) {
		return echo.ErrForbidden
	}
	if isASClosedStatus(as.Status) && !h.receiptUnlocked(c, as) {
		return echo.NewHTTPError(http.StatusForbidden, "완료·종료 건은 읽기 전용입니다. 관리자는 수정 잠금 해제 후 이용하세요")
	}
	return nil
}

func (h *AttachmentHandler) guardActionPhotoWrite(c echo.Context, asID string) error {
	if h.asRepo == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "접수 정보를 확인할 수 없습니다")
	}
	as, err := h.asRepo.GetByID(asID)
	if err != nil || as == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "접수를 찾을 수 없습니다")
	}
	if !canProcessAS(c) && !canWriteMaster(c) {
		return echo.ErrForbidden
	}
	if isASClosedStatus(as.Status) && !h.receiptUnlocked(c, as) {
		return echo.NewHTTPError(http.StatusForbidden, "완료·종료 건은 읽기 전용입니다. 관리자는 수정 잠금 해제 후 이용하세요")
	}
	return nil
}

func (h *AttachmentHandler) receiptUnlocked(c echo.Context, as *model.ASReceipt) bool {
	if as == nil || !isAdminRole(c) || h.unlockRepo == nil {
		return false
	}
	ok, _, _ := h.unlockRepo.HasActive(as.ASID, currentUserID(c))
	return ok
}

func attachmentRedirect(c echo.Context, fallback string) string {
	redirect := c.FormValue("redirect")
	if redirect == "" {
		redirect = c.QueryParam("redirect")
	}
	if redirect == "" {
		redirect = fallback
	}
	return redirect
}

func redirectAttachFailure(c echo.Context, redirect string, err error) error {
	if err == nil {
		return nil
	}
	if err == echo.ErrForbidden {
		return redirectAttachErr(c, redirect, "attach_forbidden", http.StatusForbidden)
	}
	if he, ok := err.(*echo.HTTPError); ok {
		msg, _ := he.Message.(string)
		if he.Code == http.StatusForbidden {
			if strings.Contains(msg, "읽기 전용") || strings.Contains(msg, "완료") {
				return redirectAttachErr(c, redirect, "attach_closed", he.Code)
			}
			return redirectAttachErr(c, redirect, "attach_forbidden", he.Code)
		}
		if strings.Contains(msg, "최대") && strings.Contains(msg, "장") {
			return redirectAttachErr(c, redirect, "attach_photo_max", he.Code)
		}
		if msg == "attach_photo_max" || msg == "attach_not_image" || msg == "attach_file" || msg == "attach_forbidden" || msg == "attach_closed" {
			return redirectAttachErr(c, redirect, msg, he.Code)
		}
		if msg == "" {
			msg = http.StatusText(he.Code)
		}
		return redirectAttachErr(c, redirect, msg, he.Code)
	}
	return redirectAttachErr(c, redirect, err.Error(), http.StatusBadRequest)
}

func redirectAttachErr(c echo.Context, redirect, msg string, code int) error {
	if strings.TrimSpace(redirect) == "" || redirect == "/" {
		if msg == "attach_forbidden" {
			return echo.ErrForbidden
		}
		if msg == "attach_file" {
			return c.String(code, "파일이 필요합니다")
		}
		if msg == "attach_photo_max" {
			return c.String(code, fmt.Sprintf("사진은 최대 %d장입니다", model.MaxActionPhotos))
		}
		if msg == "attach_not_image" {
			return c.String(code, "조치 사진은 이미지만 올릴 수 있습니다")
		}
		if msg == "attach_closed" {
			return echo.NewHTTPError(http.StatusForbidden, "완료·종료 건은 읽기 전용입니다. 관리자는 수정 잠금 해제 후 이용하세요")
		}
		return c.String(code, msg)
	}
	sep := "?"
	if strings.Contains(redirect, "?") {
		sep = "&"
	}
	return c.Redirect(http.StatusSeeOther, redirect+sep+"err="+url.QueryEscape(msg))
}

func (h *AttachmentHandler) UpdateKeywords(c echo.Context) error {
	id := c.Param("id")
	att, _ := h.repo.GetByID(id)
	if att == nil {
		return echo.ErrNotFound
	}
	if isReceiptPhoto(att) {
		if err := h.guardReceiptPhotoWrite(c, att.RefID); err != nil {
			return err
		}
	} else if att.RefType == model.RefTypeASActionPhoto {
		if err := h.guardActionPhotoWrite(c, att.RefID); err != nil {
			return err
		}
	} else if !canUploadAttachment(c, att.RefType) {
		return echo.ErrForbidden
	}
	keywords := strings.TrimSpace(c.FormValue("keywords"))
	if err := h.repo.UpdateKeywords(id, keywords); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, attachmentRedirect(c, "/"))
}

func (h *AttachmentHandler) Download(c echo.Context) error {
	att, err := h.repo.GetByID(c.Param("id"))
	if err != nil || att == nil {
		return echo.ErrNotFound
	}
	if att.IsVideo() {
		return c.Inline(att.FilePath, att.DisplayName())
	}
	return c.Attachment(att.FilePath, att.DisplayName())
}

func (h *AttachmentHandler) Delete(c echo.Context) error {
	att, _ := h.repo.GetByID(c.Param("id"))
	if att == nil {
		return echo.ErrNotFound
	}
	if isReceiptPhoto(att) {
		if err := h.guardReceiptPhotoWrite(c, att.RefID); err != nil {
			return err
		}
	} else if att.RefType == model.RefTypeASActionPhoto {
		if err := h.guardActionPhotoWrite(c, att.RefID); err != nil {
			return err
		}
	} else if !canUploadAttachment(c, att.RefType) {
		return echo.ErrForbidden
	}
	removeAttachmentFiles(att)
	h.repo.Delete(c.Param("id"))
	return c.Redirect(http.StatusSeeOther, attachmentRedirect(c, "/"))
}

func removeAttachmentFiles(att *model.Attachment) {
	if att == nil || att.FilePath == "" {
		return
	}
	os.Remove(att.FilePath)
	os.Remove(imageproc.ThumbPath(att.FilePath))
}

func (h *AttachmentHandler) ListJSON(c echo.Context) error {
	refType := c.QueryParam("ref_type")
	refID := c.QueryParam("ref_id")
	items, _ := h.repo.ListByRef(refType, refID)
	return c.JSON(http.StatusOK, items)
}

// PromoteToAsset 접수 사진을 설치자산 슬롯으로 복사한다 (§12.9.6).
func (h *AttachmentHandler) PromoteToAsset(c echo.Context) error {
	if !canWriteMaster(c) && !canReceiveAS(c) {
		return echo.ErrForbidden
	}
	att, err := h.repo.GetByID(c.Param("id"))
	if err != nil || att == nil {
		return echo.ErrNotFound
	}
	if att.RefType != model.AttachRefAS && att.RefType != model.AttachFormReceipt {
		return echo.NewHTTPError(http.StatusBadRequest, "접수 사진만 자산 사진으로 옮길 수 있습니다")
	}
	if !isReceiptPhoto(att) {
		return echo.NewHTTPError(http.StatusBadRequest, "접수 사진만 자산 사진으로 옮길 수 있습니다")
	}
	if h.asRepo == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "접수 정보를 확인할 수 없습니다")
	}
	as, err := h.asRepo.GetByID(att.RefID)
	if err != nil || as == nil || strings.TrimSpace(as.AssetID) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "연결된 설치자산이 없습니다")
	}
	if !att.IsImage() {
		return echo.NewHTTPError(http.StatusBadRequest, "이미지만 자산 사진으로 옮길 수 있습니다")
	}
	redirect := attachmentRedirect(c, "/as/"+as.ASID)

	slotOverride := 0
	if v := strings.TrimSpace(c.FormValue("slot_no")); v != "" {
		if s, e := strconv.Atoi(v); e == nil && s >= 1 && s <= 3 {
			slotOverride = s
		}
	}
	slotNo, err := h.repo.NextAssetImageSlot(as.AssetID)
	if err != nil {
		return err
	}
	if slotNo == 0 {
		if slotOverride == 0 {
			return redirectAttachErr(c, redirect, "자산 사진 슬롯이 가득 찼습니다. 교체할 슬롯을 고르세요", http.StatusBadRequest)
		}
		if err := h.replaceAssetSlot(as.AssetID, slotOverride); err != nil {
			return err
		}
		slotNo = slotOverride
	} else if slotOverride >= 1 && slotOverride <= 3 {
		existing, _ := h.repo.ListByRef(model.RefTypeAsset, as.AssetID)
		for _, a := range existing {
			if a.SlotNo == slotOverride {
				if err := h.replaceAssetSlot(as.AssetID, slotOverride); err != nil {
					return err
				}
				break
			}
		}
		slotNo = slotOverride
	}

	dir := repository.AssetImageDir(h.uploadDir, as.AssetID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	filename := repository.AssetImageFileName(as.AssetID, slotNo, att.FileName)
	if ext := strings.ToLower(filepath.Ext(att.FilePath)); ext != "" {
		filename = repository.AssetImageFileName(as.AssetID, slotNo, "x"+ext)
	}
	dstPath := filepath.Join(dir, filename)
	if err := copyFile(att.FilePath, dstPath); err != nil {
		return err
	}
	if srcThumb := imageproc.ThumbPath(att.FilePath); srcThumb != "" {
		if _, err := os.Stat(srcThumb); err == nil {
			_ = copyFile(srcThumb, imageproc.ThumbPath(dstPath))
		}
	}
	info, _ := os.Stat(dstPath)
	size := att.FileSize
	if info != nil {
		size = info.Size()
	}
	if err := h.repo.Create(&model.Attachment{
		RefType:  model.RefTypeAsset,
		RefID:    as.AssetID,
		FileName: filename,
		FilePath: filepath.ToSlash(dstPath),
		FileSize: size,
		MIMEType: att.MIMEType,
		Keywords: att.Keywords,
		SlotNo:   slotNo,
	}); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, redirect)
}

func (h *AttachmentHandler) replaceAssetSlot(assetID string, slot int) error {
	existing, err := h.repo.ListByRef(model.RefTypeAsset, assetID)
	if err != nil {
		return err
	}
	for i := range existing {
		if existing[i].SlotNo == slot {
			removeAttachmentFiles(&existing[i])
			return h.repo.Delete(existing[i].AttachmentID)
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
