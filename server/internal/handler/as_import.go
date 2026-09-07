package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func (h *BackupHandler) ImportUpload(c echo.Context) error {
	redir := func(errMsg string) error {
		q := "/admin/data?tab=import"
		if errMsg != "" {
			return c.Redirect(http.StatusSeeOther, q+"&err="+url.QueryEscape(errMsg))
		}
		return c.Redirect(http.StatusSeeOther, q)
	}
	if h.importRepo == nil {
		return redir("반입 저장소를 쓸 수 없습니다")
	}
	fh, err := c.FormFile("file")
	if err != nil || fh == nil {
		return redir("엑셀 또는 CSV 파일을 선택하세요")
	}
	if fh.Size > 8<<20 {
		return redir("파일은 8MB까지입니다")
	}
	f, err := fh.Open()
	if err != nil {
		return redir("파일을 열 수 없습니다")
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return redir("파일을 읽을 수 없습니다")
	}
	drafts, err := repository.ParseASImportFile(fh.Filename, data)
	if err != nil {
		return redir(err.Error())
	}
	b, err := h.importRepo.CreateBatch(fh.Filename, ctxString(c, "user_name"), drafts)
	if err != nil {
		return redir(err.Error())
	}
	return c.Redirect(http.StatusSeeOther, "/admin/data?tab=import&batch="+url.QueryEscape(b.BatchID)+
		"&ok="+url.QueryEscape("검증 리포트입니다. 바로 반영하지 않았습니다."))
}

func (h *BackupHandler) ImportExclude(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	rowNo, _ := strconv.Atoi(c.FormValue("row_no"))
	excluded := c.FormValue("excluded") == "1"
	if err := h.importRepo.SetExcluded(id, rowNo, excluded); err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/data?tab=import&batch="+url.QueryEscape(id)+"&err="+url.QueryEscape(err.Error()))
	}
	return c.Redirect(http.StatusSeeOther, "/admin/data?tab=import&batch="+url.QueryEscape(id))
}

func (h *BackupHandler) ImportPatch(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	rowNo, _ := strconv.Atoi(c.FormValue("row_no"))
	err := h.importRepo.PatchRow(id, rowNo,
		strings.TrimSpace(c.FormValue("customer_id")),
		strings.TrimSpace(c.FormValue("receipt_date")),
		strings.TrimSpace(c.FormValue("visit_date")),
		strings.TrimSpace(c.FormValue("symptom")))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/data?tab=import&batch="+url.QueryEscape(id)+"&err="+url.QueryEscape(err.Error()))
	}
	return c.Redirect(http.StatusSeeOther, "/admin/data?tab=import&batch="+url.QueryEscape(id)+"&ok="+url.QueryEscape("행을 고쳤습니다. 검증을 다시 했습니다."))
}

func (h *BackupHandler) ImportApply(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	n, err := h.importRepo.ApplyBatch(id)
	if err != nil {
		msg := err.Error()
		if err == repository.ErrImportNotReported || err == repository.ErrImportHasErrors {
			msg = err.Error()
		}
		return c.Redirect(http.StatusSeeOther, "/admin/data?tab=import&batch="+url.QueryEscape(id)+"&err="+url.QueryEscape(msg))
	}
	return c.Redirect(http.StatusSeeOther, "/admin/data?tab=import&batch="+url.QueryEscape(id)+
		"&ok="+url.QueryEscape(fmt.Sprintf("%d건을 반영했습니다. data_origin=import · 배치 %s. 지표에서는 기본 제외됩니다.", n, id)))
}

func (h *BackupHandler) ImportCancel(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	n, err := h.importRepo.CancelBatch(id)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/data?tab=import&batch="+url.QueryEscape(id)+"&err="+url.QueryEscape(err.Error()))
	}
	return c.Redirect(http.StatusSeeOther, "/admin/data?tab=import&batch="+url.QueryEscape(id)+
		"&ok="+url.QueryEscape(fmt.Sprintf("배치 %s 접수 %d건을 일괄 취소했습니다.", id, n)))
}

func importIssueRows(rows []model.ASImportRow, blockers bool) []model.ASImportRow {
	var out []model.ASImportRow
	for _, r := range rows {
		if blockers {
			if r.HasBlocker() {
				out = append(out, r)
			}
		} else if !r.Excluded && !r.HasBlocker() {
			out = append(out, r)
		}
	}
	return out
}
