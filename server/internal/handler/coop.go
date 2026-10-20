package handler

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"customer-support/internal/imageproc"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const maxCoopPhotos = 5

type CoopHandler struct {
	repo   *repository.CoopRepo
	users  *repository.UserRepo
	orgs   *repository.OrgRepo
	proj   *repository.ProjectRepo
	groups *repository.WorkGroupRepo
	upload string
}

func NewCoopHandler(repo *repository.CoopRepo, users *repository.UserRepo, orgs *repository.OrgRepo, proj *repository.ProjectRepo, groups *repository.WorkGroupRepo, upload string) *CoopHandler {
	return &CoopHandler{repo: repo, users: users, orgs: orgs, proj: proj, groups: groups, upload: upload}
}

func coopActor(c echo.Context) model.CoopActor {
	org := strings.TrimSpace(ctxString(c, "org_id"))
	if org == "" {
		org = model.OrgIDLibrary
	}
	return model.CoopActor{
		UserID: currentUserID(c), OrgID: org, Role: currentRole(c),
		ReadOnly: isReadOnly(c), ViewAs: viewAsActive(c),
	}
}

func (h *CoopHandler) mapErr(c echo.Context, err error) error {
	if err == nil {
		return nil
	}
	if repository.IsCoopForbidden(err) {
		return echo.ErrForbidden
	}
	if repository.IsCoopNotFound(err) {
		return echo.ErrNotFound
	}
	return err
}

func (h *CoopHandler) ListInbox(c echo.Context) error {
	a := coopActor(c)
	items, err := h.repo.ListInbox(a)
	if err != nil {
		return h.mapErr(c, err)
	}
	return c.Render(http.StatusOK, "coop/list.html", map[string]interface{}{
		"Title": "사업·업무관리 › 받은 협조", "Active": NavCoopInbox, "Kind": "inbox",
		"Items": items, "CanRequest": a.CanRequest(), "CanAct": true, "OK": c.QueryParam("ok"), "Err": c.QueryParam("err"),
	})
}

func (h *CoopHandler) ListSent(c echo.Context) error {
	a := coopActor(c)
	items, err := h.repo.ListSent(a)
	if err != nil {
		return h.mapErr(c, err)
	}
	return c.Render(http.StatusOK, "coop/list.html", map[string]interface{}{
		"Title": "사업·업무관리 › 보낸 협조", "Active": NavCoopSent, "Kind": "sent",
		"Items": items, "CanRequest": a.CanRequest(), "OK": c.QueryParam("ok"), "Err": c.QueryParam("err"),
	})
}

func (h *CoopHandler) New(c echo.Context) error {
	a := coopActor(c)
	if !a.CanRequest() {
		return echo.ErrForbidden
	}
	orgs, _ := h.orgs.ListActive()
	var targets []model.Org
	for _, o := range orgs {
		if o.OrgID != a.HomeOrg() {
			targets = append(targets, o)
		}
	}
	projs, _ := h.proj.List(0, "")
	return RenderPartial(c, "coop/_form.html", map[string]interface{}{
		"Title": "새 협조 요청", "Targets": targets, "Projects": projs, "Err": c.QueryParam("err"),
	})
}

func (h *CoopHandler) Create(c echo.Context) error {
	a := coopActor(c)
	row := &model.Cooperation{
		TargetOrgID: c.FormValue("target_org_id"), Kind: c.FormValue("kind"),
		Title: c.FormValue("title"), Body: c.FormValue("body"),
		ProjectID: c.FormValue("project_id"), WantStart: c.FormValue("want_start"), WantEnd: c.FormValue("want_end"),
	}
	if err := h.repo.Create(a, row); err != nil {
		if repository.IsCoopForbidden(err) {
			return echo.ErrForbidden
		}
		return c.Redirect(http.StatusSeeOther, "/cooperations/sent?err="+err.Error())
	}
	if err := h.saveUploads(c, a, row.CoopID); err != nil {
		return c.Redirect(http.StatusSeeOther, "/cooperations/sent?err="+err.Error())
	}
	return c.Redirect(http.StatusSeeOther, "/cooperations/sent?ok=1")
}

func (h *CoopHandler) Show(c echo.Context) error {
	a := coopActor(c)
	got, err := h.repo.Get(c.Param("id"))
	if err != nil {
		return err
	}
	if got == nil || !h.repo.CanSee(a, got) {
		return echo.ErrNotFound
	}
	ev, _ := h.repo.Events(got.CoopID)
	files, _ := h.repo.ListFiles(got.CoopID)
	cands, _ := h.users.ListAssignable(got.TargetOrgID)
	groups, _ := h.groups.ListGroups(got.TargetOrgID, model.WorkGroupActive)
	onBehalf := h.behalf(a, got.TargetOrgID) != ""
	return RenderPartial(c, "coop/_detail.html", map[string]interface{}{
		"Title": got.Title, "Item": got, "Events": ev, "Files": files,
		"CanAct":     h.repo.CanActInbox(a, got.TargetOrgID) && a.CanWrite(),
		"CanCancel":  got.Status == model.CoopRequested && got.ReqUserID == a.UserID && a.CanRequest(),
		"CanRequest": a.CanRequest(),
		"ReadOnly":   !a.CanWrite(),
		"OnBehalf":   onBehalf,
		"OpenN":      h.openN(got.CoopID), "NeedReason": h.openN(got.CoopID) > 0,
		"Candidates": cands, "Groups": groups,
		"KindLabel": model.CoopKindLabel(got.Kind), "StatusLabel": model.CoopStatusLabel(got.Status),
	})
}

func (h *CoopHandler) openN(id string) int {
	return h.repo.OpenLinked(id)
}

func (h *CoopHandler) Cancel(c echo.Context) error {
	if err := h.repo.Cancel(coopActor(c), c.Param("id")); err != nil {
		return h.mapErr(c, err)
	}
	return c.Redirect(http.StatusSeeOther, "/cooperations/sent?ok=1")
}

func (h *CoopHandler) Accept(c echo.Context) error {
	a := coopActor(c)
	id := c.Param("id")
	got, _ := h.repo.Get(id)
	if got == nil {
		return echo.ErrNotFound
	}
	if err := h.repo.Accept(a, id, h.behalf(a, got.TargetOrgID)); err != nil {
		return h.mapErr(c, err)
	}
	return c.Redirect(http.StatusSeeOther, "/cooperations/inbox?ok=1&open="+id)
}

func (h *CoopHandler) Reject(c echo.Context) error {
	a := coopActor(c)
	id := c.Param("id")
	got, _ := h.repo.Get(id)
	if got == nil {
		return echo.ErrNotFound
	}
	if err := h.repo.Reject(a, id, c.FormValue("reject_reason"), h.behalf(a, got.TargetOrgID)); err != nil {
		if repository.IsCoopForbidden(err) {
			return echo.ErrForbidden
		}
		return c.Redirect(http.StatusSeeOther, "/cooperations/inbox?err="+err.Error())
	}
	return c.Redirect(http.StatusSeeOther, "/cooperations/inbox?ok=1")
}

func (h *CoopHandler) Assign(c echo.Context) error {
	a := coopActor(c)
	id := c.Param("id")
	got, _ := h.repo.Get(id)
	if got == nil {
		return echo.ErrNotFound
	}
	vals, _ := c.FormParams()
	userIDs := vals["user_id"]
	if err := h.repo.Assign(a, id, c.FormValue("plan_start"), c.FormValue("plan_end"), c.FormValue("instruction"), c.FormValue("work_group_id"), userIDs); err != nil {
		if repository.IsCoopForbidden(err) {
			return echo.ErrForbidden
		}
		return c.Redirect(http.StatusSeeOther, "/cooperations/inbox?err="+err.Error())
	}
	return c.Redirect(http.StatusSeeOther, "/cooperations/inbox?ok=1")
}

func (h *CoopHandler) Complete(c echo.Context) error {
	a := coopActor(c)
	id := c.Param("id")
	got, _ := h.repo.Get(id)
	if got == nil {
		return echo.ErrNotFound
	}
	if err := h.repo.Complete(a, id, c.FormValue("completed_reason"), h.behalf(a, got.TargetOrgID)); err != nil {
		if repository.IsCoopForbidden(err) {
			return echo.ErrForbidden
		}
		return c.Redirect(http.StatusSeeOther, "/cooperations/inbox?err="+err.Error())
	}
	return c.Redirect(http.StatusSeeOther, "/cooperations/inbox?ok=1")
}

func (h *CoopHandler) ReRequest(c echo.Context) error {
	n, err := h.repo.ReRequest(coopActor(c), c.Param("id"))
	if err != nil {
		return h.mapErr(c, err)
	}
	return c.Redirect(http.StatusSeeOther, "/cooperations/sent?ok=1&open="+n.CoopID)
}

func (h *CoopHandler) ProjectJSON(c echo.Context) error {
	v, err := h.repo.ProjectView(coopActor(c), c.Param("id"))
	if err != nil {
		return h.mapErr(c, err)
	}
	return c.JSON(http.StatusOK, v)
}

func (h *CoopHandler) CreateGroup(c echo.Context) error {
	a := coopActor(c)
	got, err := h.repo.Get(c.Param("id"))
	if err != nil || got == nil || !h.repo.CanActInbox(a, got.TargetOrgID) {
		return echo.ErrNotFound
	}
	wgA := model.WorkGroupActor{UserID: a.UserID, OrgID: got.TargetOrgID, Role: a.Role}
	g := &model.WorkGroup{OrgID: got.TargetOrgID, Name: c.FormValue("name")}
	if err := h.groups.Create(wgA, g); err != nil {
		if repository.IsWorkGroupForbidden(err) {
			return echo.ErrForbidden
		}
		return err
	}
	return c.JSON(http.StatusOK, map[string]string{"group_id": g.GroupID, "name": g.Name})
}

func (h *CoopHandler) File(c echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("file_id"), 10, 64)
	f, coopID, err := h.repo.File(id)
	if err != nil {
		return echo.ErrNotFound
	}
	got, _ := h.repo.Get(coopID)
	if got == nil || !h.repo.CanSee(coopActor(c), got) {
		return echo.ErrNotFound
	}
	return c.Attachment(f.Path, f.FileName)
}

func (h *CoopHandler) behalf(a model.CoopActor, target string) string {
	rle := model.NormalizeRole(a.Role)
	if rle == model.RoleVisionAdmin || rle == model.RoleOrgAdmin || rle == model.RoleAdmin {
		return ""
	}
	if h.repo.CanActInbox(a, target) {
		return a.UserID
	}
	return ""
}

func (h *CoopHandler) saveUploads(c echo.Context, a model.CoopActor, coopID string) error {
	form, err := c.MultipartForm()
	if err != nil || form == nil {
		return nil
	}
	files := form.File["photos"]
	if len(files) == 0 {
		return nil
	}
	if len(files) > maxCoopPhotos {
		return fmt.Errorf("사진은 최대 %d장입니다", maxCoopPhotos)
	}
	dir := filepath.Join(h.upload, "coop", coopID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, fh := range files {
		if fh.Size > model.MaxReceiptBytes {
			return fmt.Errorf("파일이 너무 큽니다")
		}
		if !imageproc.LooksLikeImage(fh.Filename, fh.Header.Get("Content-Type")) {
			return fmt.Errorf("이미지만 첨부할 수 있습니다")
		}
		src, err := fh.Open()
		if err != nil {
			return err
		}
		head := make([]byte, 16)
		n, _ := io.ReadFull(src, head)
		if !looksImageMagic(head[:n]) {
			src.Close()
			return fmt.Errorf("이미지만 첨부할 수 있습니다")
		}
		src.Close()
		src, err = fh.Open()
		if err != nil {
			return err
		}
		name := uuid.NewString() + filepath.Ext(fh.Filename)
		dst := filepath.Join(dir, name)
		out, err := os.Create(dst)
		if err != nil {
			src.Close()
			return err
		}
		_, err = io.Copy(out, src)
		src.Close()
		out.Close()
		if err != nil {
			return err
		}
		if err := h.repo.AddFile(coopID, fh.Filename, dst, fh.Header.Get("Content-Type"), a.UserID, fh.Size); err != nil {
			return err
		}
	}
	return nil
}

func looksImageMagic(b []byte) bool {
	if len(b) < 3 {
		return false
	}
	if b[0] == 0xFF && b[1] == 0xD8 {
		return true
	}
	if len(b) >= 8 && b[0] == 0x89 && b[1] == 0x50 && b[2] == 0x4E && b[3] == 0x47 {
		return true
	}
	if b[0] == 'G' && b[1] == 'I' && b[2] == 'F' {
		return true
	}
	if len(b) >= 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WEBP" {
		return true
	}
	return false
}
