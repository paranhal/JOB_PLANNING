package handler

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
	"customer-support/internal/imageproc"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func (h *AuthHandler) uploadsRoot() string {
	if h != nil && strings.TrimSpace(h.uploadDir) != "" {
		return h.uploadDir
	}
	return "data/uploads"
}

func signatureStoredPath(uploadDir, userID string) string {
	return signatureStoredPathSlot(uploadDir, userID, 1)
}

func signatureStoredPathSlot(uploadDir, userID string, slot int) string {
	id := strings.TrimSpace(userID)
	if slot == 2 {
		return filepath.Join(uploadDir, "signatures", id+"_2.png")
	}
	return filepath.Join(uploadDir, "signatures", id+".png")
}

func parseSignatureSlot(raw string) int {
	if strings.TrimSpace(raw) == "2" {
		return 2
	}
	return 1
}

func signatureSlotHint(c echo.Context) (slot int, explicit bool) {
	if c == nil {
		return 1, false
	}
	raw := strings.TrimSpace(c.FormValue("slot"))
	if raw == "" {
		raw = strings.TrimSpace(c.QueryParam("slot"))
	}
	if raw == "" {
		return 1, false
	}
	return parseSignatureSlot(raw), true
}

func userHasSignatureSlot(u *model.User, slot int) bool {
	if u == nil {
		return false
	}
	if slot == 2 {
		return signatureFileExists(u.SignaturePath2)
	}
	return signatureFileExists(u.SignaturePath)
}

func nextEmptySignatureSlot(u *model.User) (int, error) {
	s1 := userHasSignatureSlot(u, 1)
	s2 := userHasSignatureSlot(u, 2)
	if !s1 {
		return 1, nil
	}
	if !s2 {
		return 2, nil
	}
	return 0, errSignatureBothFull
}

func signaturePathOf(u *model.User, slot int) string {
	if u == nil {
		return ""
	}
	if slot == 2 {
		return u.SignaturePath2
	}
	return u.SignaturePath
}

func signatureFileExists(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && st.Size() > 0
}

func readSignaturePNG(path string) []byte {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) == 0 {
		return nil
	}
	return raw
}

func readUserSignaturePNG(users interface {
	GetByID(string) (*model.User, error)
}, userID string) []byte {
	userID = strings.TrimSpace(userID)
	if users == nil || userID == "" {
		return nil
	}
	u, err := users.GetByID(userID)
	if err != nil || u == nil {
		return nil
	}
	return readSignaturePNG(u.SignaturePath)
}

func (h *AuthHandler) AccountSignatureImage(c echo.Context) error {
	uid := ctxString(c, "user_id")
	u, _ := h.userRepo.GetByID(uid)
	if u == nil {
		return echo.ErrNotFound
	}
	slot, _ := signatureSlotHint(c)
	raw := readSignaturePNG(signaturePathOf(u, slot))
	if len(raw) == 0 {
		return echo.ErrNotFound
	}
	return c.Blob(http.StatusOK, "image/png", raw)
}

func storeSignaturePNGAt(users *repository.UserRepo, uploadDir, userID, slotRaw string, raw []byte) (string, error) {
	userID = strings.TrimSpace(userID)
	if users == nil || userID == "" {
		return "", errPlain("계정이 없습니다.")
	}
	u, err := users.GetByID(userID)
	if err != nil || u == nil {
		return "", errPlain("계정이 없습니다.")
	}
	slot := 1
	if strings.TrimSpace(slotRaw) != "" {
		slot = parseSignatureSlot(slotRaw)
	} else {
		slot, err = nextEmptySignatureSlot(u)
		if err != nil {
			return "", err
		}
	}
	png, err := imageproc.ProcessSignature(bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	dir := filepath.Join(uploadDir, "signatures")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", errPlain("저장 폴더를 만들지 못했습니다.")
	}
	path := signatureStoredPathSlot(uploadDir, userID, slot)
	if err := os.WriteFile(path, png, 0644); err != nil {
		return "", errPlain("사인을 저장하지 못했습니다.")
	}
	if slot == 2 {
		if err := users.UpdateSignaturePath2(userID, path); err != nil {
			return "", errPlain("경로를 기록하지 못했습니다.")
		}
	} else if err := users.UpdateSignaturePath(userID, path); err != nil {
		return "", errPlain("경로를 기록하지 못했습니다.")
	}
	return path, nil
}

func (h *AuthHandler) storeSignaturePNG(userID string, raw []byte, slotRaw string) (string, error) {
	return storeSignaturePNGAt(h.userRepo, h.uploadsRoot(), userID, slotRaw, raw)
}

func (h *AuthHandler) clearSignaturePNG(userID string, slot int) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return
	}
	u, _ := h.userRepo.GetByID(userID)
	if u != nil {
		if p := strings.TrimSpace(signaturePathOf(u, slot)); p != "" {
			_ = os.Remove(p)
		}
	}
	_ = os.Remove(signatureStoredPathSlot(h.uploadsRoot(), userID, slot))
	if slot == 2 {
		_ = h.userRepo.UpdateSignaturePath2(userID, "")
		return
	}
	_ = h.userRepo.UpdateSignaturePath(userID, "")
}

func (h *AuthHandler) AccountSignature(c echo.Context) error {
	uid := ctxString(c, "user_id")
	if uid == "" {
		return c.Redirect(http.StatusSeeOther, "/logout")
	}
	if other := strings.TrimSpace(c.FormValue("user_id")); other != "" && other != uid {
		return h.renderAccountErr(c, uid, "사인은 본인만 올릴 수 있습니다.")
	}
	raw, err := readSignatureUpload(c)
	if err != nil {
		return h.renderAccountErr(c, uid, err.Error())
	}
	if _, err := h.storeSignaturePNG(uid, raw, c.FormValue("slot")); err != nil {
		return h.renderAccountErr(c, uid, err.Error())
	}
	accessLog(c, auditlog.Record{
		Action:      auditlog.ActionUpdate,
		Result:      auditlog.ResultOK,
		TargetTable: "users",
		TargetID:    uid,
		SubjectType: "user",
		SubjectID:   uid,
		Detail:      "사인 저장",
	})
	return c.Redirect(http.StatusSeeOther, "/account?ok=signature")
}

func (h *AuthHandler) AccountSignatureDelete(c echo.Context) error {
	uid := ctxString(c, "user_id")
	if uid == "" {
		return c.Redirect(http.StatusSeeOther, "/logout")
	}
	slot, _ := signatureSlotHint(c)
	h.clearSignaturePNG(uid, slot)
	return c.Redirect(http.StatusSeeOther, "/account?ok=signature_cleared")
}

func (h *AuthHandler) UserSignature(c echo.Context) error {
	if !h.isAdmin(c) {
		return h.forbidden(c)
	}
	id := strings.TrimSpace(c.Param("id"))
	u, _ := h.userRepo.GetByID(id)
	if u == nil {
		return echo.ErrNotFound
	}
	raw, err := readSignatureUpload(c)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape(err.Error()))
	}
	if _, err := h.storeSignaturePNG(id, raw, c.FormValue("slot")); err != nil {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape(err.Error()))
	}
	accessLog(c, auditlog.Record{
		Action:      auditlog.ActionUpdate,
		Result:      auditlog.ResultOK,
		TargetTable: "users",
		TargetID:    id,
		SubjectType: "user",
		SubjectID:   id,
		SubjectName: u.FullName,
		Detail:      "사인 저장",
	})
	return c.Redirect(http.StatusSeeOther, "/users?ok=signature")
}

func (h *AuthHandler) UserSignatureDelete(c echo.Context) error {
	if !h.isAdmin(c) {
		return h.forbidden(c)
	}
	id := strings.TrimSpace(c.Param("id"))
	u, _ := h.userRepo.GetByID(id)
	if u == nil {
		return echo.ErrNotFound
	}
	slot, _ := signatureSlotHint(c)
	h.clearSignaturePNG(id, slot)
	return c.Redirect(http.StatusSeeOther, "/users?ok=signature_cleared")
}

func (h *AuthHandler) UserSignatureImage(c echo.Context) error {
	if !h.isAdmin(c) {
		return h.forbidden(c)
	}
	u, _ := h.userRepo.GetByID(c.Param("id"))
	if u == nil {
		return echo.ErrNotFound
	}
	slot, _ := signatureSlotHint(c)
	raw := readSignaturePNG(signaturePathOf(u, slot))
	if len(raw) == 0 {
		return echo.ErrNotFound
	}
	return c.Blob(http.StatusOK, "image/png", raw)
}

func (h *AuthHandler) renderAccountErr(c echo.Context, uid, msg string) error {
	u, _ := h.userRepo.GetByID(uid)
	return c.Render(http.StatusOK, "auth/account.html", map[string]interface{}{
		"Title": "내 계정", "Active": NavAccount, "User": u, "Error": msg,
		"HasSignature":  u != nil && signatureFileExists(u.SignaturePath),
		"HasSignature2": u != nil && signatureFileExists(u.SignaturePath2),
	})
}

func readSignatureUpload(c echo.Context) ([]byte, error) {
	fh, err := c.FormFile("file")
	if err == nil && fh != nil && fh.Size > 0 {
		if fh.Size > imageproc.SignatureMaxBytes {
			return nil, errSignatureSize
		}
		f, err := fh.Open()
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return io.ReadAll(io.LimitReader(f, imageproc.SignatureMaxBytes+1))
	}
	drawn := strings.TrimSpace(c.FormValue("drawn"))
	if drawn == "" {
		return nil, errSignatureEmpty
	}
	return decodeDataURL(drawn)
}

var (
	errSignatureEmpty    = errPlain("사인을 올리거나 그려 주세요.")
	errSignatureSize     = errPlain("사인은 1MB 이하여야 합니다")
	errSignatureBothFull = errPlain("어느 사인을 바꿀지 고르세요.")
)

type errPlain string

func (e errPlain) Error() string { return string(e) }

func decodeDataURL(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, ","); i >= 0 {
		s = s[i+1:]
	}
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(raw) == 0 {
		return nil, errSignatureEmpty
	}
	if len(raw) > imageproc.SignatureMaxBytes {
		return nil, errSignatureSize
	}
	return raw, nil
}

func resolveAssignableUser(users *repository.UserRepo, userID, name string) *model.User {
	if users == nil {
		return nil
	}
	if id := strings.TrimSpace(userID); id != "" {
		if u := users.FindAssignable(id); u != nil {
			return u
		}
		if u, _ := users.GetByID(id); u != nil {
			return u
		}
	}
	return users.FindAssignable(name)
}

func signaturePNGForUser(users *repository.UserRepo, userID, name string, slot int) []byte {
	u := resolveAssignableUser(users, userID, name)
	if u == nil {
		return nil
	}
	if slot == 2 {
		return readSignaturePNG(u.SignaturePath2)
	}
	if slot == 1 {
		png := readSignaturePNG(u.SignaturePath)
		if len(png) > 0 {
			return png
		}
		if strings.TrimSpace(userID) == "" {
			return readSignaturePNG(u.SignaturePath2)
		}
		return png
	}
	if png := readSignaturePNG(u.SignaturePath); len(png) > 0 {
		return png
	}
	return readSignaturePNG(u.SignaturePath2)
}

func signaturePNGForName(users *repository.UserRepo, name string) []byte {
	return signaturePNGForUser(users, "", name, 0)
}

func signatureSavedOnPath(path string) string {
	if !signatureFileExists(path) {
		return ""
	}
	st, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return st.ModTime().Format("2006-01-02")
}

func signatureDataURL(png []byte) string {
	if len(png) == 0 {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
}
