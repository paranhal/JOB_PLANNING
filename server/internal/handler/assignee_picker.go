package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const (
	PickerMulti  PickerMode = "multi"
	PickerSingle PickerMode = "single"
)

type PickerMode string

type PickerSpec struct {
	Field         string
	Mode          PickerMode
	Label         string
	Required      bool
	AllowGroups   bool
	AllowExternal bool
	AllowCrossOrg bool
	Selected      Selection
}

type Selection struct {
	PrimaryUserID string
	PrimaryName   string
	UserIDs       []string
	Names         []string
	GroupIDs      []string
	ExcludeIDs    []string
	External      string
	DurationByUID map[string]int
	People        []repository.AssigneePerson
}

func (s Selection) Primary() string {
	if strings.TrimSpace(s.PrimaryUserID) != "" {
		return s.PrimaryUserID
	}
	return strings.TrimSpace(s.PrimaryName)
}

func (s Selection) Empty() bool {
	if strings.TrimSpace(s.PrimaryUserID) != "" || strings.TrimSpace(s.PrimaryName) != "" {
		return false
	}
	if strings.TrimSpace(s.External) != "" {
		return false
	}
	return len(s.UserIDs) == 0 && len(s.GroupIDs) == 0 && len(s.People) == 0
}

func (s Selection) GroupID() string {
	if len(s.GroupIDs) > 0 {
		return strings.TrimSpace(s.GroupIDs[0])
	}
	return ""
}

type PickerDeps struct {
	Users  *repository.UserRepo
	Groups *repository.WorkGroupRepo
	Orgs   *repository.OrgRepo
}

func (d PickerDeps) mnt() *MaintenanceHandler {
	return &MaintenanceHandler{userRepo: d.Users, groups: d.Groups, orgs: d.Orgs}
}

func (h *MaintenanceHandler) pickerDeps() PickerDeps {
	if h == nil {
		return PickerDeps{}
	}
	return PickerDeps{Users: h.userRepo, Groups: h.groups, Orgs: h.orgs}
}

func (h *WorkboardHandler) pickerDeps() PickerDeps {
	if h == nil {
		return PickerDeps{}
	}
	return PickerDeps{Users: h.userRepo, Groups: h.groups}
}

func (h *ASHandler) pickerDeps() PickerDeps {
	if h == nil {
		return PickerDeps{}
	}
	return PickerDeps{Users: h.userRepo, Groups: h.groups}
}

func errPicker(msg string) error {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		msg = msgAssigneeRequired
	}
	return echo.NewHTTPError(http.StatusBadRequest, msg)
}

func ParseSelection(c echo.Context, deps PickerDeps, spec PickerSpec) (Selection, error) {
	if spec.Mode == "" {
		spec.Mode = PickerMulti
	}
	mh := deps.mnt()
	v := &model.MaintenanceVisit{}
	if err := mh.applyVisitAssigneesFromForm(c, v); err != nil {
		return Selection{}, errPicker(err.Error())
	}
	sel := selectionFromVisit(v)
	sel.ExcludeIDs = formVisitExcludeIDs(c)
	sel.DurationByUID = parseSupportMins(c)

	if spec.Mode == PickerSingle {
		sel = sel.singleOnly()
	}
	if !spec.AllowGroups && len(sel.GroupIDs) > 0 {
		return Selection{}, errPicker("이 화면에서는 업무 그룹을 지정할 수 없습니다")
	}
	ext := strings.TrimSpace(c.FormValue("assignee_external"))
	if ext == "" {
		ext = strings.TrimSpace(c.FormValue("assigned_to_custom"))
	}
	if ext == "" {
		ext = strings.TrimSpace(c.FormValue("assignee_name"))
	}
	if sel.Empty() {
		if code := strings.TrimSpace(c.FormValue("assigned_to_code")); code != "" && code != "custom" {
			if u := mh.lookupAssignableUser(c, code); u != nil {
				sel.PrimaryUserID = u.UserID
				sel.PrimaryName = u.FullName
				sel.UserIDs = []string{u.UserID}
				sel.Names = []string{u.FullName}
				sel.People = []repository.AssigneePerson{{
					UserID: u.UserID, Name: u.FullName, Role: model.WBMemberOwner,
				}}
			} else {
				return Selection{}, errPicker(errAssigneeSameOrg)
			}
		} else if code == "custom" && ext != "" {
			sel.External = ext
			sel.PrimaryName = ext
			sel.People = []repository.AssigneePerson{{Name: ext, Role: model.WBMemberOwner}}
		}
	}
	if spec.AllowExternal && ext != "" && sel.Empty() {
		sel.External = ext
		sel.PrimaryName = ext
		sel.People = []repository.AssigneePerson{{Name: ext, Role: model.WBMemberOwner}}
	}
	if !spec.AllowExternal && strings.TrimSpace(ext) != "" && sel.Empty() {
		return Selection{}, errPicker("직접입력 담당자는 이 화면에서 쓸 수 없습니다")
	}
	if spec.Required && sel.Empty() {
		return Selection{}, errPicker(msgAssigneeRequired)
	}
	if len(sel.People) == 0 && strings.TrimSpace(sel.PrimaryName) != "" {
		sel.People = []repository.AssigneePerson{{
			UserID: sel.PrimaryUserID, Name: sel.PrimaryName, Role: model.WBMemberOwner,
		}}
	}
	sel.applyDurations()
	sel.resolveNames(deps.Users)
	return sel, nil
}

func (s Selection) singleOnly() Selection {
	if s.Empty() {
		return s
	}
	p := repository.AssigneePerson{UserID: s.PrimaryUserID, Name: s.PrimaryName, Role: model.WBMemberOwner}
	if len(s.People) > 0 {
		p = s.People[0]
		for _, x := range s.People {
			if x.Role == model.WBMemberOwner {
				p = x
				break
			}
		}
	}
	out := Selection{
		PrimaryUserID: p.UserID,
		PrimaryName:   p.Name,
		UserIDs:       nil,
		Names:         nil,
		DurationByUID: s.DurationByUID,
		People:        []repository.AssigneePerson{p},
	}
	if p.UserID != "" {
		out.UserIDs = []string{p.UserID}
	}
	if p.Name != "" {
		out.Names = []string{p.Name}
	}
	if len(s.GroupIDs) > 0 {
		out.GroupIDs = s.GroupIDs[:1]
	}
	return out
}

func (s *Selection) resolveNames(users *repository.UserRepo) {
	if s == nil {
		return
	}
	if n := resolvePersonLabel(users, s.PrimaryUserID, s.PrimaryName); n != "" {
		s.PrimaryName = n
	} else if pickerPersonLabel(s.PrimaryName) == "" {
		s.PrimaryName = ""
	}
	if n := pickerPersonLabel(s.External); n != "" {
		s.External = n
	} else if strings.HasPrefix(strings.TrimSpace(s.External), "USR-") {
		s.External = ""
	}
	for i := range s.People {
		if n := resolvePersonLabel(users, s.People[i].UserID, s.People[i].Name); n != "" {
			s.People[i].Name = n
		}
	}
	s.Names = nil
	for _, p := range s.People {
		if n := pickerPersonLabel(p.Name); n != "" {
			s.Names = append(s.Names, n)
		}
	}
}

func (s *Selection) applyDurations() {
	if s == nil || len(s.DurationByUID) == 0 {
		return
	}
	for i := range s.People {
		uid := strings.TrimSpace(s.People[i].UserID)
		if n := s.DurationByUID[uid]; n > 0 && s.People[i].Role == model.WBMemberSupport {
			s.People[i].DurationMin = n
		}
	}
}

func selectionFromVisit(v *model.MaintenanceVisit) Selection {
	var s Selection
	if v == nil {
		return s
	}
	s.PrimaryName = strings.TrimSpace(v.Assignee)
	if gid := strings.TrimSpace(v.WorkGroupID); gid != "" {
		s.GroupIDs = []string{gid}
	}
	seenG := map[string]bool{}
	for _, g := range s.GroupIDs {
		seenG[g] = true
	}
	for _, p := range v.People {
		person := repository.AssigneePerson{
			UserID: p.UserID, Name: p.AssigneeName, Role: p.Role,
			WorkGroupID: p.WorkGroupID, SortOrder: p.SortOrder,
		}
		s.People = append(s.People, person)
		if uid := strings.TrimSpace(p.UserID); uid != "" {
			s.UserIDs = append(s.UserIDs, uid)
		}
		if n := strings.TrimSpace(p.AssigneeName); n != "" {
			s.Names = append(s.Names, n)
		}
		if gid := strings.TrimSpace(p.WorkGroupID); gid != "" && !seenG[gid] {
			seenG[gid] = true
			s.GroupIDs = append(s.GroupIDs, gid)
		}
		if p.Role == model.WBMemberOwner {
			s.PrimaryUserID = strings.TrimSpace(p.UserID)
			if n := strings.TrimSpace(p.AssigneeName); n != "" {
				s.PrimaryName = n
			}
		}
	}
	if s.PrimaryUserID == "" && len(s.UserIDs) > 0 {
		s.PrimaryUserID = s.UserIDs[0]
	}
	return s
}

func parseSupportMins(c echo.Context) map[string]int {
	out := map[string]int{}
	if c == nil {
		return out
	}
	_ = c.Request().ParseForm()
	ids := c.Request().PostForm["assignee_user_id"]
	mins := c.Request().PostForm["assignee_support_min"]
	for i, raw := range ids {
		uid := strings.TrimSpace(raw)
		if uid == "" || i >= len(mins) {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(mins[i]))
		if err != nil || n <= 0 {
			continue
		}
		out[uid] = n
	}
	return out
}

func assigneeFilterOptions(users *repository.UserRepo, c echo.Context) []model.User {
	return listAssignableForPicker(users, c)
}

func applySelectionToTask(t *model.WorkTask, sel Selection) {
	if t == nil {
		return
	}
	t.Assignee = sel.PrimaryName
	t.AssigneeUserID = sel.PrimaryUserID
	if gid := sel.GroupID(); gid != "" {
		t.WorkGroupID = gid
	}
}

func pickerSeedFromTask(t *model.WorkTask, members []model.WorkTaskMember, users []model.User) (ownerID string, ids []string) {
	byName := map[string]string{}
	for _, u := range users {
		n := strings.TrimSpace(u.FullName)
		if n != "" && strings.TrimSpace(u.UserID) != "" {
			byName[n] = u.UserID
		}
	}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		for _, x := range ids {
			if x == id {
				return
			}
		}
		ids = append(ids, id)
	}
	if t != nil {
		ownerID = strings.TrimSpace(t.AssigneeUserID)
		if ownerID == "" {
			ownerID = byName[strings.TrimSpace(t.Assignee)]
		}
		add(ownerID)
	}
	for _, m := range members {
		uid := strings.TrimSpace(m.UserID)
		if uid == "" {
			uid = byName[strings.TrimSpace(m.Assignee)]
		}
		add(uid)
	}
	return ownerID, ids
}

func ensureTaskProjectListed(t *model.WorkTask, list []model.WorkProject) []model.WorkProject {
	if t == nil {
		return list
	}
	pid := strings.TrimSpace(t.ProjectID)
	if pid == "" {
		return list
	}
	for _, p := range list {
		if p.ProjectID == pid {
			return list
		}
	}
	name := strings.TrimSpace(t.ProjectName)
	if name == "" {
		return list
	}
	return append([]model.WorkProject{{ProjectID: pid, Name: name}}, list...)
}

func (h *WorkboardHandler) saveSelectionOnTask(taskID string, sel Selection) error {
	if h == nil || h.repo == nil || strings.TrimSpace(taskID) == "" {
		return nil
	}
	return repository.SaveAssignees(h.repo.DB(), repository.AssigneeTargetWorkTask, taskID, sel.People, sel.GroupID())
}

func (h *WorkboardHandler) taskSelectionFromForm(c echo.Context, required bool) (Selection, error) {
	sel, err := ParseSelection(c, h.pickerDeps(), PickerSpec{
		AllowGroups: true, AllowExternal: true, Required: required, Mode: PickerMulti,
	})
	if err != nil {
		return sel, err
	}
	owner := strings.TrimSpace(c.FormValue("assignee"))
	supports := parseSupportMembers(c, firstNonEmpty(sel.PrimaryName, owner))
	if sel.Empty() {
		if owner == "" && len(supports) == 0 {
			if required {
				return sel, errPicker(msgAssigneeRequired)
			}
			return sel, nil
		}
		sel.PrimaryName = owner
		sel.People = []repository.AssigneePerson{{Name: owner, Role: model.WBMemberOwner}}
	}
	seen := map[string]bool{}
	for _, p := range sel.People {
		if n := strings.TrimSpace(p.Name); n != "" {
			seen[n] = true
		}
		if id := strings.TrimSpace(p.UserID); id != "" {
			seen[id] = true
		}
	}
	for i, m := range supports {
		if seen[strings.TrimSpace(m.Assignee)] {
			continue
		}
		sel.People = append(sel.People, repository.AssigneePerson{
			Name: m.Assignee, UserID: m.UserID, Role: model.WBMemberSupport,
			DurationMin: m.DurationMin, SortOrder: i + 1,
		})
		sel.Names = append(sel.Names, m.Assignee)
		seen[strings.TrimSpace(m.Assignee)] = true
	}
	if sel.PrimaryName == "" {
		sel.PrimaryName = owner
	}
	return sel, nil
}

func rejectPickerErr(c echo.Context, err error, fallback string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if he, ok := err.(*echo.HTTPError); ok {
		if s, ok := he.Message.(string); ok {
			msg = s
		}
		if he.Code == http.StatusBadRequest && (strings.Contains(msg, msgAssigneeRequired) || strings.Contains(msg, errAssigneeRequired)) {
			return rejectAssigneeRequired(c, fallback)
		}
	}
	return echo.NewHTTPError(http.StatusBadRequest, msg)
}
