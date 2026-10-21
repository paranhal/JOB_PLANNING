package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type dashRow struct {
	Row       model.ContractRow
	Run       string
	RunLabel  string
	Kind      model.EffectiveKind
	ASCount   int
	MntCount  int
	WaitCount int
	WorkCount int
	Href      string
	Link      bool
	CanLink   bool
	Period    string
	ASHref    string
	MntHref   string
	WaitHref  string
}

func dashboardKind(raw string) string {
	switch strings.TrimSpace(raw) {
	case "construction", "build", "dev", "maintenance", "other", "unclassified":
		return strings.TrimSpace(raw)
	case "network":
		return "build"
	default:
		return ""
	}
}

func dashQuery(kind, act string) string {
	q := url.Values{}
	if kind != "" {
		q.Set("kind", kind)
	}
	if act != "" {
		q.Set("act", act)
	}
	s := q.Encode()
	if s == "" {
		return "/projects/dashboard"
	}
	return "/projects/dashboard?" + s
}

func (h *ProjectHandler) Dashboard(c echo.Context) error {
	if !canViewProjects(c) {
		return echo.ErrForbidden
	}
	kind := dashboardKind(c.QueryParam("kind"))
	if kind == "" {
		kind = dashboardKind(c.QueryParam("tab"))
	}
	act := strings.TrimSpace(c.QueryParam("act"))
	today := time.Now().Format("2006-01-02")
	union, err := h.repo.ListContractUnion(repository.ContractUnionFilter{OrgID: SalesVisibilityScope(c).OrgID})
	if err != nil {
		return err
	}
	acts, err := h.repo.CountProjectActivities()
	if err != nil {
		return err
	}
	canSales := canViewSales(c)
	canWrite := canWriteProjects(c)
	canWriteSales := canWriteSales(c)
	kindN := map[string]int{}
	var rows []dashRow
	unclassN := 0
	for _, u := range union {
		if model.NormalizeContractDirection(u.Direction) == model.ContractDirCost {
			continue
		}
		dr := dashRow{Row: u, Href: model.ProjectContractHref(u.ProjectID, u.SalesID, u.Source), CanLink: canSales, Link: canSales}
		ek := u.EffectiveKind()
		if u.Source == model.ContractSourceSales {
			ek = model.EffectiveKind{Kind: model.ProjectKindUnclassified, Source: model.KindSourceUnclassified}
		}
		dr.Kind = ek
		if u.Source == model.ContractSourceWork {
			p := model.WorkProject{StartDate: u.StartDate, EndDate: u.EndDate, Status: u.Status}
			dr.Run = p.ContractRunStatus(today)
			dr.RunLabel = model.ContractRunLabel(dr.Run)
			dr.ASCount = acts.AS[u.ProjectID]
			dr.MntCount = acts.Mnt[u.ProjectID]
			dr.WaitCount = acts.Wait[u.ProjectID]
			dr.WorkCount = acts.Work[u.ProjectID]
			dr.ASHref = "/projects/" + u.ProjectID
			dr.MntHref = "/projects/" + u.ProjectID
			dr.WaitHref = "/workboard/tasks?project=" + url.QueryEscape(u.ProjectID)
			st, en := strings.TrimSpace(u.StartDate), strings.TrimSpace(u.EndDate)
			if st != "" || en != "" {
				dr.Period = st + " ~ " + en
			} else {
				dr.Period = "—"
			}
			kindN[ek.Kind]++
			if ek.Kind == model.ProjectKindUnclassified {
				unclassN++
			}
		} else {
			dr.RunLabel = "영업 계약 완료 · 계약 전환 필요"
			dr.Period = "—"
			unclassN++
			kindN[model.ProjectKindUnclassified]++
		}
		if !kindMatch(kind, dr) {
			continue
		}
		if !actMatch(act, dr) {
			continue
		}
		rows = append(rows, dr)
	}
	showProg, showWait, showNeed, showAct := 0, 0, 0, 0
	var filteredUnion []model.ContractRow
	for _, r := range rows {
		filteredUnion = append(filteredUnion, r.Row)
		if r.Row.Source != model.ContractSourceWork {
			continue
		}
		if r.Run == model.RunProgress {
			showProg++
			showAct += r.ASCount + r.MntCount + r.WaitCount + r.WorkCount
		}
		if r.Run == model.RunWait {
			showWait++
		}
		if r.Run == model.RunNeedComplete {
			showNeed++
		}
	}
	showKPI := model.BuildContractUnionKPI(filteredUnion, today)
	tabDefs := []struct{ Code, Label string }{
		{"", "사업 운영 현황"},
		{"construction", "공사"},
		{"build", "구축"},
		{"dev", "개발"},
		{"maintenance", "유지보수"},
		{"other", "기타"},
	}
	var tabs []map[string]string
	for _, t := range tabDefs {
		n := len(union)
		if t.Code != "" {
			n = kindN[t.Code]
		}
		tabs = append(tabs, map[string]string{
			"Code": t.Code, "Label": t.Label, "N": strconv.Itoa(n), "Href": dashQuery(t.Code, act),
		})
	}
	if unclassN > 0 {
		tabs = append(tabs, map[string]string{
			"Code": "unclassified", "Label": "미분류", "N": strconv.Itoa(unclassN), "Href": dashQuery("unclassified", act),
		})
	}
	return c.Render(http.StatusOK, "project/dashboard.html", map[string]interface{}{
		"Title":         "사업 대시보드",
		"Active":        NavProjectsDashboard,
		"Kind":          kind,
		"Act":           act,
		"Tab":           kind,
		"Tabs":          tabs,
		"Rows":          rows,
		"KindOptions":   model.ProjectKindOptions(),
		"CanWrite":      canWrite,
		"CanViewSales":  canSales,
		"CanWriteSales": canWriteSales,
		"KPITotal":      showKPI.TotalN,
		"KPIAmount":     model.FormatSalesMoney(showKPI.Amount),
		"KPIProgress":   showProg,
		"KPIWait":       showWait,
		"KPINeed":       showNeed,
		"KPIAct":        showAct,
		"UnspecN":       acts.Unspec,
		"EquipLine":     h.dashboardEquipLine(rows),
		"Back":          dashQuery(kind, act),
		"ActAll":        dashQuery(kind, ""),
		"ActAS":         dashQuery(kind, model.ActFilterAS),
		"ActMnt":        dashQuery(kind, model.ActFilterMnt),
		"ActTask":       dashQuery(kind, model.ActFilterTask),
		"ActWork":       dashQuery(kind, model.ActFilterWork),
		"ReportURL":     "/projects/diary",
		"FilterURL":     "/projects/registry",
	})
}

func kindMatch(kind string, r dashRow) bool {
	if kind == "" {
		return true
	}
	if kind == "unclassified" {
		return r.Kind.Kind == model.ProjectKindUnclassified || r.Row.Source == model.ContractSourceSales
	}
	if r.Row.Source == model.ContractSourceSales {
		return false
	}
	return r.Kind.Kind == kind
}

func actMatch(act string, r dashRow) bool {
	switch act {
	case model.ActFilterAS:
		return r.ASCount > 0
	case model.ActFilterMnt:
		return r.MntCount > 0
	case model.ActFilterTask:
		return r.WaitCount > 0
	case model.ActFilterWork:
		return r.WorkCount > 0
	default:
		return true
	}
}

func (h *ProjectHandler) dashboardEquipLine(rows []dashRow) string {
	if h == nil || h.pe == nil {
		return "—"
	}
	yearN, done, tot := 0, 0, 0
	for _, r := range rows {
		if r.Row.Source != model.ContractSourceWork || r.Row.ProjectID == "" {
			continue
		}
		if h.pe.CurrentYearLabel(r.Row.ProjectID) != "" {
			yearN++
		}
		d, n := h.pe.InstallRate(r.Row.ProjectID)
		done += d
		tot += n
	}
	if tot == 0 {
		if yearN == 0 {
			return "—"
		}
		return fmt.Sprintf("진행 연차 %d건 · 설치율 —", yearN)
	}
	return fmt.Sprintf("진행 연차 %d건 · 설치율 %d%%", yearN, done*100/tot)
}
