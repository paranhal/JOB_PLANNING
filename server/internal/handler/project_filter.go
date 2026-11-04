package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const projectFilterEmptyOrgName = "조직 미지정"

type projectFilterItem struct {
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	ShortName string `json:"short_name,omitempty"`
}

type projectOrgGroup struct {
	OrgID   string              `json:"org_id"`
	OrgName string              `json:"org_name"`
	Items   []projectFilterItem `json:"items"`
}

// splitProjectsByOrg 주관=내 조직 또는 유관에 내 조직 포함 → own. 조직 빈 값 → other(조직 미지정).
func splitProjectsByOrg(projects []model.WorkProject, myOrg string) (own, other []model.WorkProject) {
	myOrg = strings.TrimSpace(myOrg)
	for _, p := range projects {
		host := strings.TrimSpace(p.OrgID)
		if myOrg != "" && host != "" && host == myOrg {
			p.Related = false
			own = append(own, p)
			continue
		}
		if myOrg != "" {
			hit := false
			for _, id := range p.RelatedOrgIDs {
				if strings.TrimSpace(id) == myOrg {
					hit = true
					break
				}
			}
			if hit {
				p.Related = true
				own = append(own, p)
				continue
			}
		}
		other = append(other, p)
	}
	return own, other
}

func groupProjectsByOrg(projects []model.WorkProject) []projectOrgGroup {
	idx := map[string]int{}
	var groups []projectOrgGroup
	for _, p := range projects {
		oid := strings.TrimSpace(p.OrgID)
		key := oid
		if key == "" {
			key = "__empty__"
		}
		i, ok := idx[key]
		if !ok {
			name := strings.TrimSpace(p.OrgLabel)
			if oid == "" {
				name = projectFilterEmptyOrgName
			} else if name == "" {
				name = oid
			}
			idx[key] = len(groups)
			i = len(groups)
			groups = append(groups, projectOrgGroup{OrgID: oid, OrgName: name})
		}
		nm := strings.TrimSpace(p.Name)
		if s := strings.TrimSpace(p.ShortName); s != "" && nm == "" {
			nm = s
		}
		groups[i].Items = append(groups[i].Items, projectFilterItem{
			ProjectID: p.ProjectID,
			Name:      nm,
			ShortName: strings.TrimSpace(p.ShortName),
		})
	}
	return groups
}

func projectFilterMyOrg(c echo.Context) string {
	if DataScopeFor(c).All {
		return ""
	}
	home := loginHomeOrg(c)
	if home != "" {
		return home
	}
	o := currentOrg(c)
	if o == "" || o == repository.OrgAll {
		return ""
	}
	return o
}

func projectDisplayName(p model.WorkProject) string {
	if s := strings.TrimSpace(p.ShortName); s != "" {
		return s
	}
	return strings.TrimSpace(p.Name)
}

func findProjectByID(projects []model.WorkProject, id string) *model.WorkProject {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	for i := range projects {
		if projects[i].ProjectID == id {
			return &projects[i]
		}
	}
	return nil
}

func myOrgLabel(projects []model.WorkProject, myOrg string) string {
	myOrg = strings.TrimSpace(myOrg)
	if myOrg == "" {
		return ""
	}
	for _, p := range projects {
		if strings.TrimSpace(p.OrgID) == myOrg {
			if s := strings.TrimSpace(p.OrgLabel); s != "" {
				return s
			}
		}
	}
	if myOrg == model.OrgIDLibrary {
		return model.OrgNameLibrary
	}
	return myOrg
}

func matchProjectQuery(p model.WorkProject, q string) bool {
	q = strings.TrimSpace(strings.ToLower(q))
	if q == "" {
		return true
	}
	name := strings.ToLower(strings.TrimSpace(p.Name))
	short := strings.ToLower(strings.TrimSpace(p.ShortName))
	return strings.Contains(name, q) || strings.Contains(short, q)
}

// allowedProjectFilter 목록에 없는 ID 는 무시(빈 필터). filterTasksByProject 자체는 바꾸지 않는다.
func allowedProjectFilter(c echo.Context, id string, all []model.WorkProject) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	p := findProjectByID(all, id)
	if p == nil {
		return ""
	}
	if DataScopeFor(c).All {
		return id
	}
	own, other := splitProjectsByOrg(all, projectFilterMyOrg(c))
	for _, x := range own {
		if x.ProjectID == id {
			return id
		}
	}
	for _, x := range other {
		if x.ProjectID == id {
			return id
		}
	}
	return ""
}

func (h *WorkboardHandler) sanitizeProjectQuery(c echo.Context) string {
	raw := strings.TrimSpace(c.QueryParam("project"))
	if raw == "" {
		return ""
	}
	all, err := h.repo.ListActiveProjectsLabeled()
	if err != nil {
		return raw
	}
	return allowedProjectFilter(c, raw, all)
}

func attachProjectFilter(c echo.Context, h *WorkboardHandler, data map[string]interface{}, fieldID string) {
	if data == nil || h == nil {
		return
	}
	all, _ := h.repo.ListActiveProjectsLabeled()
	myOrg := projectFilterMyOrg(c)
	own, other := splitProjectsByOrg(all, myOrg)
	allOrgs := DataScopeFor(c).All
	raw := strings.TrimSpace(c.QueryParam("project"))
	selID := allowedProjectFilter(c, raw, all)
	cur := "사업 전체"
	orgTag := ""
	if p := findProjectByID(all, selID); p != nil {
		cur = projectDisplayName(*p)
		if allOrgs {
			orgTag = strings.TrimSpace(p.OrgLabel)
			if strings.TrimSpace(p.OrgID) == "" {
				orgTag = projectFilterEmptyOrgName
			}
		} else {
			inOwn := false
			for _, x := range own {
				if x.ProjectID == p.ProjectID {
					inOwn = true
					break
				}
			}
			if !inOwn {
				orgTag = strings.TrimSpace(p.OrgLabel)
				if strings.TrimSpace(p.OrgID) == "" {
					orgTag = projectFilterEmptyOrgName
				}
			}
		}
	}
	otherN := len(other)
	if allOrgs {
		own = nil
		otherN = 0
	}
	data["OwnProjects"] = own
	data["OtherCount"] = otherN
	data["ProjectFilter"] = selID
	data["ProjectFilterUI"] = map[string]interface{}{
		"ID":         fieldID,
		"Name":       "project",
		"Value":      selID,
		"Current":    cur,
		"OrgTag":     orgTag,
		"Own":        own,
		"OtherCount": otherN,
		"OtherURL":   "/workboard/projects/other",
		"AllOrgs":    allOrgs,
		"MyOrgName":  myOrgLabel(all, myOrg),
	}
}

func (h *WorkboardHandler) ListOtherProjects(c echo.Context) error {
	if strings.TrimSpace(currentUserID(c)) == "" {
		return c.JSON(http.StatusForbidden, map[string]interface{}{
			"groups": []projectOrgGroup{},
			"total":  0,
		})
	}
	all, err := h.repo.ListActiveProjectsLabeled()
	if err != nil {
		return err
	}
	q := strings.TrimSpace(c.QueryParam("q"))
	var src []model.WorkProject
	if DataScopeFor(c).All {
		src = all
	} else {
		_, src = splitProjectsByOrg(all, projectFilterMyOrg(c))
	}
	var matched []model.WorkProject
	for _, p := range src {
		if matchProjectQuery(p, q) {
			matched = append(matched, p)
		}
	}
	groups := groupProjectsByOrg(matched)
	if groups == nil {
		groups = []projectOrgGroup{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"groups": groups,
		"total":  len(matched),
	})
}
