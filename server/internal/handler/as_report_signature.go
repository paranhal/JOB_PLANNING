package handler

import (
	"bytes"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
	"customer-support/internal/imageproc"
	"customer-support/internal/model"
)

func (h *ASHandler) loadInspectorSignature(c echo.Context, userID, inspectorName string) []byte {
	if strings.TrimSpace(c.FormValue("use_saved_sig")) == "0" {
		return formSignaturePNG(c, "inspector_sig")
	}
	if png := formSignaturePNG(c, "inspector_sig"); len(png) > 0 {
		return png
	}
	slot := parseSignatureSlot(c.FormValue("inspector_sig_slot"))
	if id := strings.TrimSpace(userID); id != "" {
		return signaturePNGForUser(h.userRepo, id, inspectorName, slot)
	}
	return signaturePNGForUser(h.userRepo, c.FormValue("inspector_id"), inspectorName, slot)
}

func (h *ASHandler) loadConfirmerSignature(c echo.Context, userID, confirmerName string) []byte {
	if strings.TrimSpace(c.FormValue("use_saved_sig")) == "0" {
		return formSignaturePNG(c, "confirmer_sig")
	}
	if png := formSignaturePNG(c, "confirmer_sig"); len(png) > 0 {
		return png
	}
	slot := parseSignatureSlot(c.FormValue("confirmer_sig_slot"))
	if id := strings.TrimSpace(userID); id != "" {
		return signaturePNGForUser(h.userRepo, id, confirmerName, slot)
	}
	return signaturePNGForUser(h.userRepo, c.FormValue("confirmer_id"), confirmerName, slot)
}

func (h *ASHandler) logSavedSignatureIfUsed(c echo.Context, slot, name string, png []byte) {
	if len(png) == 0 || c == nil {
		return
	}
	if strings.TrimSpace(c.FormValue("use_saved_sig")) == "0" {
		return
	}
	key := "inspector_sig"
	if slot == "confirmer" {
		key = "confirmer_sig"
	}
	if len(formSignaturePNG(c, key)) > 0 {
		return
	}
	accessLog(c, auditlog.Record{
		Action:      auditlog.ActionUpdate,
		Result:      auditlog.ResultOK,
		TargetTable: "as_receipts",
		TargetID:    c.Param("id"),
		SubjectType: "as_report_signature",
		SubjectID:   slot,
		SubjectName: name,
		Detail:      "보고서 발급에 저장된 사인",
	})
}

func signatureSavedOn(u *model.User) string {
	return signatureSavedOnPath(signaturePathOf(u, 1))
}

func (h *ASHandler) reportSlotUser(slot, name, loginID string) *model.User {
	return h.reportSlotUserID(slot, "", name, loginID)
}

func (h *ASHandler) reportSlotUserID(slot, userID, name, loginID string) *model.User {
	if h == nil || h.userRepo == nil {
		return nil
	}
	if u := resolveAssignableUser(h.userRepo, userID, name); u != nil {
		return u
	}
	if strings.TrimSpace(slot) == "inspector" {
		u, _ := h.userRepo.GetByID(strings.TrimSpace(loginID))
		return u
	}
	return nil
}

func reportSigSource(png []byte, u *model.User) string {
	if len(png) == 0 {
		return ""
	}
	if u != nil && (signatureFileExists(u.SignaturePath) || signatureFileExists(u.SignaturePath2)) {
		return "계정에 저장됨"
	}
	return ""
}

func formSignaturePNG(c echo.Context, key string) []byte {
	if c == nil {
		return nil
	}
	raw, err := decodeDataURL(c.FormValue(key))
	if err != nil || len(raw) == 0 {
		return nil
	}
	png, err := imageproc.ProcessSignature(bytes.NewReader(raw))
	if err != nil {
		return raw
	}
	return png
}

type reportSigJSONItem struct {
	Slot    int    `json:"slot"`
	SavedAt string `json:"saved_at"`
	DataURL string `json:"data_url"`
}

type reportSigJSONSet struct {
	Has    bool                `json:"has"`
	UserID string              `json:"user_id,omitempty"`
	Items  []reportSigJSONItem `json:"items"`
}

func (h *ASHandler) ReportSignatures(c echo.Context) error {
	as, err := h.loadAS(c.Param("id"))
	if err != nil {
		return err
	}
	if !canProcessAS(c) {
		return echo.ErrForbidden
	}
	inspID := strings.TrimSpace(c.QueryParam("inspector_id"))
	confID := strings.TrimSpace(c.QueryParam("confirmer_id"))
	inspName := strings.TrimSpace(c.QueryParam("inspector"))
	confName := strings.TrimSpace(c.QueryParam("confirmer"))
	if inspName == "" {
		inspName = strings.TrimSpace(as.AssignedTo)
	}
	if inspID == "" {
		inspID = strings.TrimSpace(as.AssignedUserID)
	}
	if confName == "" {
		confName = strings.TrimSpace(as.CustomerConfirmer)
		if confName == "" {
			confName = strings.TrimSpace(as.ConfirmTarget)
		}
	}
	return c.JSON(http.StatusOK, map[string]reportSigJSONSet{
		"inspector": h.reportSignatureSet(inspID, inspName),
		"confirmer": h.reportSignatureSet(confID, confName),
	})
}

func (h *ASHandler) reportSignatureSet(userID, name string) reportSigJSONSet {
	u := resolveAssignableUser(h.userRepo, userID, name)
	out := reportSigJSONSet{Items: []reportSigJSONItem{}}
	if u == nil {
		return out
	}
	out.UserID = u.UserID
	for _, slot := range []int{1, 2} {
		png := readSignaturePNG(signaturePathOf(u, slot))
		if len(png) == 0 {
			continue
		}
		out.Items = append(out.Items, reportSigJSONItem{
			Slot:    slot,
			SavedAt: signatureSavedOnPath(signaturePathOf(u, slot)),
			DataURL: signatureDataURL(png),
		})
	}
	out.Has = len(out.Items) > 0
	return out
}

func (h *ASHandler) ReportApplySignature(c echo.Context) error {
	if _, err := h.loadAS(c.Param("id")); err != nil {
		return err
	}
	if !canProcessAS(c) {
		return echo.ErrForbidden
	}
	slot := strings.TrimSpace(c.FormValue("slot"))
	if slot != "inspector" && slot != "confirmer" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "점검자 또는 확인자 칸이 아닙니다"})
	}
	raw, err := decodeDataURL(c.FormValue("drawn"))
	if err != nil || len(raw) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "사인을 그려 주세요"})
	}
	png, err := imageproc.ProcessSignature(bytes.NewReader(raw))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	intent := strings.TrimSpace(c.FormValue("intent"))
	source := "이 보고서만"
	if intent == "save" {
		name := strings.TrimSpace(c.FormValue("name"))
		id := strings.TrimSpace(c.FormValue("user_id"))
		if id == "" {
			if slot == "inspector" {
				id = strings.TrimSpace(c.FormValue("inspector_id"))
			} else {
				id = strings.TrimSpace(c.FormValue("confirmer_id"))
			}
		}
		u := h.reportSlotUserID(slot, id, name, ctxString(c, "user_id"))
		if u == nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "계정이 없어 이 보고서에만 쓸 수 있습니다"})
		}
		if _, err := storeSignaturePNGAt(h.userRepo, h.reportUploadDir(), u.UserID, c.FormValue("sig_slot"), raw); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		source = "계정에 저장됨"
		accessLog(c, auditlog.Record{
			Action:      auditlog.ActionUpdate,
			Result:      auditlog.ResultOK,
			TargetTable: "users",
			TargetID:    u.UserID,
			SubjectType: "user",
			SubjectID:   u.UserID,
			SubjectName: u.FullName,
			Detail:      "보고서 사인 저장",
		})
	}
	return c.JSON(http.StatusOK, map[string]string{
		"ok":       "1",
		"data_url": signatureDataURL(png),
		"source":   source,
	})
}

func reportUserID(u *model.User) string {
	if u == nil {
		return ""
	}
	return u.UserID
}
