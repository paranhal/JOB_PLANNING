package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
	"customer-support/internal/service/leaveimport"
)

func (h *LeaveHandler) ImportForm(c echo.Context) error {
	if !h.canLeaveAdmin(c) {
		return echo.ErrForbidden
	}
	return h.renderLeaveImport(c, nil, "", nil, "")
}

func (h *LeaveHandler) ImportTemplate(c echo.Context) error {
	if !h.canLeaveAdmin(c) {
		return echo.ErrForbidden
	}
	raw, err := leaveimport.TemplateXLSX()
	if err != nil {
		return err
	}
	c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="leave_opening_template.xlsx"`)
	return c.Blob(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", raw)
}

func (h *LeaveHandler) ImportPreview(c echo.Context) error {
	if !h.canLeaveAdminWrite(c) {
		return echo.ErrForbidden
	}
	raw, err := readLeaveXLSX(c)
	if err != nil {
		return h.renderLeaveImport(c, nil, "", nil, err.Error())
	}
	y := time.Now().Year()
	if d := h.nowDate(); len(d) >= 4 {
		if n, err := strconv.Atoi(d[:4]); err == nil && n > 2000 {
			y = n
		}
	}
	parsed, err := leaveimport.Parse(raw, y)
	if err != nil {
		return h.renderLeaveImport(c, nil, "", nil, err.Error())
	}
	rows, err := leaveimport.Match(h.svc.DB, parsed)
	if err != nil {
		return err
	}
	tok := leaveimport.PutPreview(rows, y)
	accessLog(c, auditlog.Record{
		Action: auditlog.ActionView, Result: auditlog.ResultOK, TargetTable: "leave_grants",
		Detail: fmt.Sprintf("기초잔액이관 미리보기 %d행", len(rows)),
	})
	return h.renderLeaveImport(c, rows, tok, h.importPickUsers(c), "")
}

func (h *LeaveHandler) ImportConfirm(c echo.Context) error {
	if !h.canLeaveAdminWrite(c) {
		return echo.ErrForbidden
	}
	tok := strings.TrimSpace(c.FormValue("token"))
	sess, ok := leaveimport.GetPreview(tok)
	if !ok {
		return h.renderLeaveImport(c, nil, "", nil, "미리보기가 만료되었습니다. 다시 올리세요.")
	}
	picks := map[int]string{}
	if f, err := c.FormParams(); err == nil {
		for k, vs := range f {
			if !strings.HasPrefix(k, "match-") || len(vs) == 0 {
				continue
			}
			n, err := strconv.Atoi(strings.TrimPrefix(k, "match-"))
			if err != nil {
				continue
			}
			picks[n] = strings.TrimSpace(vs[0])
		}
	}
	rows := leaveimport.ApplyPicks(h.svc.DB, sess.Rows, picks)
	led := h.ledger()
	if h.svc != nil {
		led.Now = h.svc.Now
	}
	rep, err := leaveimport.Apply(led, rows, sess.Year)
	if err != nil {
		return h.renderLeaveImport(c, rows, tok, h.importPickUsers(c), err.Error())
	}
	leaveimport.DropPreview(tok)
	accessLog(c, auditlog.Record{
		Action: auditlog.ActionCreate, Result: auditlog.ResultOK, TargetTable: "leave_grants",
		Detail: leaveimport.CountsOnly(rep),
	})
	return c.Redirect(http.StatusSeeOther, "/admin/leaves/import?ok="+url.QueryEscape(leaveimport.CountsOnly(rep)))
}

func (h *LeaveHandler) renderLeaveImport(c echo.Context, rows []leaveimport.PreviewRow, tok string, users []leaveimport.Candidate, errMsg string) error {
	ok := strings.TrimSpace(c.QueryParam("ok"))
	return c.Render(http.StatusOK, "leave/import.html", map[string]interface{}{
		"Title":    "기초 잔액 이관",
		"Active":   NavLeaveAdmin,
		"Rows":     rows,
		"Token":    tok,
		"Users":    users,
		"CanWrite": h.canLeaveAdminWrite(c),
		"Error":    errMsg,
		"OK":       ok,
		"Report":   ok,
	})
}

func (h *LeaveHandler) importPickUsers(c echo.Context) []leaveimport.Candidate {
	scope := h.leaveScopeOrg(c)
	q := `SELECT user_id, full_name, COALESCE(org_id,'') FROM users WHERE is_active=1`
	args := []interface{}{}
	if scope != "" {
		q += ` AND org_id=?`
		args = append(args, scope)
	}
	q += ` ORDER BY full_name`
	rs, err := h.svc.DB.Query(q, args...)
	if err != nil {
		return nil
	}
	defer rs.Close()
	var out []leaveimport.Candidate
	for rs.Next() {
		var u leaveimport.Candidate
		if err := rs.Scan(&u.UserID, &u.Name, &u.OrgID); err != nil {
			return out
		}
		out = append(out, u)
	}
	return out
}

func readLeaveXLSX(c echo.Context) ([]byte, error) {
	fh, err := c.FormFile("file")
	if err != nil {
		return nil, fmt.Errorf("엑셀 파일을 선택하세요")
	}
	src, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()
	raw, err := io.ReadAll(io.LimitReader(src, 4<<20+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 4<<20 {
		return nil, fmt.Errorf("파일은 4MB 이하여야 합니다")
	}
	return raw, nil
}
