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
	return filepath.Join(uploadDir, "signatures", strings.TrimSpace(userID)+".png")
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
	raw := readSignaturePNG(u.SignaturePath)
	if len(raw) == 0 {
		return echo.ErrNotFound
	}
	return c.Blob(http.StatusOK, "image/png", raw)
}

func (h *AuthHandler) storeSignaturePNG(userID string, raw []byte) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", errPlain("계정이 없습니다.")
	}
	png, err := imageproc.ProcessSignature(bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	dir := filepath.Join(h.uploadsRoot(), "signatures")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", errPlain("저장 폴더를 만들지 못했습니다.")
	}
	path := signatureStoredPath(h.uploadsRoot(), userID)
	if err := os.WriteFile(path, png, 0644); err != nil {
		return "", errPlain("사인을 저장하지 못했습니다.")
	}
	if err := h.userRepo.UpdateSignaturePath(userID, path); err != nil {
		return "", errPlain("경로를 기록하지 못했습니다.")
	}
	return path, nil
}

func (h *AuthHandler) clearSignaturePNG(userID string) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return
	}
	u, _ := h.userRepo.GetByID(userID)
	if u != nil && strings.TrimSpace(u.SignaturePath) != "" {
		_ = os.Remove(u.SignaturePath)
	}
	_ = os.Remove(signatureStoredPath(h.uploadsRoot(), userID))
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
	if _, err := h.storeSignaturePNG(uid, raw); err != nil {
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
	h.clearSignaturePNG(uid)
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
	if _, err := h.storeSignaturePNG(id, raw); err != nil {
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
	h.clearSignaturePNG(id)
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
	raw := readSignaturePNG(u.SignaturePath)
	if len(raw) == 0 {
		return echo.ErrNotFound
	}
	return c.Blob(http.StatusOK, "image/png", raw)
}

func (h *AuthHandler) renderAccountErr(c echo.Context, uid, msg string) error {
	u, _ := h.userRepo.GetByID(uid)
	return c.Render(http.StatusOK, "auth/account.html", map[string]interface{}{
		"Title": "내 계정", "Active": NavAccount, "User": u, "Error": msg,
		"HasSignature": u != nil && signatureFileExists(u.SignaturePath),
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
	errSignatureEmpty = errPlain("사인을 올리거나 그려 주세요.")
	errSignatureSize  = errPlain("사인은 1MB 이하여야 합니다")
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

func signaturePNGForName(users *repository.UserRepo, name string) []byte {
	name = strings.TrimSpace(name)
	if users == nil || name == "" {
		return nil
	}
	list, err := users.ListAssignable()
	if err != nil {
		return nil
	}
	for i := range list {
		if strings.TrimSpace(list[i].FullName) == name || strings.TrimSpace(list[i].Username) == name {
			return readSignaturePNG(list[i].SignaturePath)
		}
	}
	return nil
}

func signatureDataURL(png []byte) string {
	if len(png) == 0 {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
}
