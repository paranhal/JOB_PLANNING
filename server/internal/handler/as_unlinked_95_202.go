package handler

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-202: 「AS 접수·조치」(/as) 하단의 「자산 연결이 없는 작업」. 조회 전용이고 HTMX 부분 응답으로 지연 로딩한다.
// /as 첫 응답(List·Kanban)은 이 목록을 조회하지 않는다.

const unlinkedPageSize = 20

type unlinkedParams struct {
	Tab, Q, Status, From, To, Product, Who string
	// Sort 는 화이트리스트를 통과한 정렬 키(기본이면 빈 문자열), SortRaw 는 요청 값 그대로(탭이 정해진 뒤 다시 판정한다)
	Sort, SortRaw string
	Page          int
}

func clipRunesN(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

// readUnlinkedParams prefix 가 "" 이면 부분 응답용 이름(tab·q·…), "ul_" 이면 /as 페이지 주소에 남는 이름이다.
func readUnlinkedParams(c echo.Context, prefix string) unlinkedParams {
	g := func(k string) string { return strings.TrimSpace(c.QueryParam(prefix + k)) }
	p := unlinkedParams{Tab: g("tab"), Q: g("q"), Status: g("status"), From: g("from"), To: g("to"), Product: g("product"), Who: g("who")}
	if p.Tab != "mnt" {
		p.Tab = "as"
	}
	switch p.Status {
	case "open", "done":
	default:
		p.Status = ""
	}
	for _, d := range []*string{&p.From, &p.To} {
		if _, err := time.Parse("2006-01-02", *d); err != nil {
			*d = ""
		}
	}
	p.Page, _ = strconv.Atoi(g("page"))
	if p.Page < 1 {
		p.Page = 1
	}
	p.SortRaw = g("sort")
	p.Sort = repository.NormalizeUnlinkedSort(p.Tab, p.SortRaw)
	return p
}

// unlinkedScope /as 목록과 같은 조직·「내 배정」·업무그룹 범위. List 의 규칙을 그대로 따른다.
func (h *ASHandler) unlinkedScope(c echo.Context) repository.UnlinkedFilter {
	role := currentRole(c)
	mineParam := c.QueryParam("mine")
	assigneeID := strings.TrimSpace(c.QueryParam("assignee"))
	assigneeName := strings.TrimSpace(c.QueryParam("assignee_name"))
	mine := false
	switch {
	case assigneeID != "" || assigneeName != "":
		mine = true
	case role == "tech":
		mine = mineParam != "0"
	case mineParam == "1":
		mine = true
	}
	f := repository.UnlinkedFilter{OrgID: currentOrg(c), PageSize: unlinkedPageSize}
	if mine {
		switch {
		case assigneeID != "":
			f.MineUserID = assigneeID
			if u, _ := h.userRepo.GetByID(assigneeID); u != nil {
				f.MineKeys = []string{u.FullName, u.Username}
			}
		case assigneeName != "":
			f.MineKeys = []string{assigneeName}
		default:
			f.MineUserID = currentUserID(c)
			f.MineKeys = assigneeKeys(c)
		}
		f.ExcludeGroup = !viewGroupInclude(c)
	}
	return f
}

// scopeQuery 부분 응답 주소에 이어 붙이는 범위 파라미터(내 배정·담당자·그룹).
func scopeQuery(c echo.Context) url.Values {
	v := url.Values{}
	for _, k := range []string{"mine", "assignee", "assignee_name", "scope"} {
		if s := strings.TrimSpace(c.QueryParam(k)); s != "" {
			v.Set(k, s)
		}
	}
	return v
}

func (p unlinkedParams) values(prefix string, tab string, page int) url.Values {
	v := url.Values{}
	set := func(k, val string) {
		if val != "" {
			v.Set(prefix+k, val)
		}
	}
	set("tab", tab)
	set("q", p.Q)
	set("status", p.Status)
	set("who", p.Who)
	set("sort", repository.NormalizeUnlinkedSort(tab, p.SortRaw))
	set("from", p.From)
	set("to", p.To)
	if tab == "mnt" {
		set("product", p.Product)
	}
	if page > 1 {
		set("page", strconv.Itoa(page))
	}
	return v
}

// pageBase /as 화면 자신의 주소(경로·표시 옵션)에서 ul_ 파라미터를 뺀 것. HTMX 가 보내는 HX-Current-URL 을 쓴다.
func pageBase(c echo.Context) (string, url.Values) {
	path, q := "/as", url.Values{}
	if cur := strings.TrimSpace(c.Request().Header.Get("HX-Current-URL")); cur != "" {
		if u, err := url.Parse(cur); err == nil {
			if strings.HasPrefix(u.Path, "/as") && !strings.HasPrefix(u.Path, "/as/unlinked") {
				path = u.Path
			}
			q = u.Query()
		}
	}
	for k := range q {
		if strings.HasPrefix(k, "ul_") {
			q.Del(k)
		}
	}
	return path, q
}

type unlinkedPageLink struct {
	N     int
	Frag  string // hx-get 주소
	Push  string // 브라우저 주소(새로고침·뒤로가기 유지)
	Label string
	Cur   bool
}

func sortOrDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// unlinkedHeadLink 머리말 정렬 링크(1쪽으로, 필터 유지). 기본 정렬 키는 주소에서 뺀다.
func (h *ASHandler) unlinkedHeadLink(c echo.Context, p unlinkedParams, tab, key string) unlinkedPageLink {
	q := p
	q.SortRaw = key
	return h.unlinkedLinkFor(c, q, tab, 1, "")
}

type unlinkedHead struct {
	Link  unlinkedPageLink
	Arrow string // ▲ ▼ 또는 빈 문자열
}

// unlinkedHeads 정렬 가능한 머리말: 현재 정렬이면 방향 표시, 눌렀을 때 갈 다음 정렬 링크.
func (h *ASHandler) unlinkedHeads(c echo.Context, p unlinkedParams, tab string) map[string]unlinkedHead {
	cur := p.Sort
	mk := func(next string, arrow string) unlinkedHead {
		return unlinkedHead{Link: h.unlinkedHeadLink(c, p, tab, next), Arrow: arrow}
	}
	out := map[string]unlinkedHead{}
	if tab == "as" {
		c0 := sortOrDefault(cur, "recv_desc")
		recvNext, recvArrow := "recv_asc", ""
		switch c0 {
		case "recv_desc":
			recvArrow = "▼"
		case "recv_asc":
			recvNext, recvArrow = "recv_desc", "▲"
		}
		out["recv"] = mk(recvNext, recvArrow)
		for _, k := range []string{"customer", "assignee", "cand"} {
			a := ""
			if c0 == k {
				a = "▲"
				if k == "cand" {
					a = "▼"
				}
			}
			out[k] = mk(k, a)
		}
		return out
	}
	c0 := sortOrDefault(cur, "visit_desc")
	vNext, vArrow := "visit_asc", ""
	switch c0 {
	case "visit_desc":
		vArrow = "▼"
	case "visit_asc":
		vNext, vArrow = "visit_desc", "▲"
	}
	out["visit"] = mk(vNext, vArrow)
	for _, k := range []string{"customer", "product", "round", "cand"} {
		a := ""
		if c0 == k {
			a = "▲"
		}
		out[k] = mk(k, a)
	}
	return out
}

// unlinkedLinkFor n 쪽 링크: Frag(hx-get 주소)와 Push(브라우저 주소, 새로고침·뒤로가기 유지).
func (h *ASHandler) unlinkedLinkFor(c echo.Context, p unlinkedParams, tab string, n int, label string) unlinkedPageLink {
	path, base := pageBase(c)
	frag := p.values("", tab, n)
	for k, vs := range scopeQuery(c) {
		frag[k] = vs
	}
	push := url.Values{}
	for k, vs := range base {
		push[k] = vs
	}
	for k, vs := range p.values("ul_", tab, n) {
		push[k] = vs
	}
	l := unlinkedPageLink{N: n, Label: label, Frag: "/as/unlinked/" + tab + "?" + frag.Encode(), Push: path}
	if e := push.Encode(); e != "" {
		l.Push += "?" + e
	}
	return l
}

func (h *ASHandler) unlinkedLinks(c echo.Context, p unlinkedParams, tab string, cur, pages int) []unlinkedPageLink {
	var out []unlinkedPageLink
	mk := func(n int, label string) unlinkedPageLink {
		l := h.unlinkedLinkFor(c, p, tab, n, label)
		l.Cur = n == cur
		return l
	}
	if pages > 1 {
		if cur > 1 {
			out = append(out, mk(cur-1, "이전"))
		}
		for n := 1; n <= pages; n++ {
			if n == 1 || n == pages || (n >= cur-2 && n <= cur+2) {
				out = append(out, mk(n, strconv.Itoa(n)))
			}
		}
		if cur < pages {
			out = append(out, mk(cur+1, "다음"))
		}
	}
	return out
}

func (h *ASHandler) unlinkedError(c echo.Context, retry string) error {
	return c.Render(http.StatusOK, "as/unlinked_fragment.html", map[string]interface{}{"Kind": "error", "Retry": retry})
}

// UnlinkedCounts GET /as/unlinked/counts — 두 탭의 건수와 활성 탭의 목록 틀(목록 본문은 이어서 불러온다).
func (h *ASHandler) UnlinkedCounts(c echo.Context) error {
	scope := h.unlinkedScope(c)
	p := readUnlinkedParams(c, "ul_")
	if t := strings.TrimSpace(c.QueryParam("tab")); t == "as" || t == "mnt" {
		p.Tab = t
	}
	p.Sort = repository.NormalizeUnlinkedSort(p.Tab, p.SortRaw)
	asN, err1 := h.repo.CountUnlinkedAS(scope)
	mntN, err2 := h.repo.CountUnlinkedMnt(scope)
	retry := "/as/unlinked/counts?" + scopeQuery(c).Encode()
	if err1 != nil || err2 != nil {
		return h.unlinkedError(c, retry)
	}
	path, base := pageBase(c)
	tabURL := func(tab string) (string, string) {
		frag := scopeQuery(c)
		frag.Set("tab", tab)
		push := url.Values{}
		for k, vs := range base {
			push[k] = vs
		}
		push.Set("ul_tab", tab)
		return "/as/unlinked/counts?" + frag.Encode(), path + "?" + push.Encode()
	}
	asFrag, asPush := tabURL("as")
	mntFrag, mntPush := tabURL("mnt")
	bodyQ := p.values("", p.Tab, p.Page)
	for k, vs := range scopeQuery(c) {
		bodyQ[k] = vs
	}
	return c.Render(http.StatusOK, "as/unlinked_fragment.html", map[string]interface{}{
		"Kind": "counts", "Tab": p.Tab, "ASN": asN, "MntN": mntN,
		"ASFrag": asFrag, "ASPush": asPush, "MntFrag": mntFrag, "MntPush": mntPush,
		"BodyURL": "/as/unlinked/" + p.Tab + "?" + bodyQ.Encode(),
	})
}

type unlinkedASRow struct {
	repository.UnlinkedAS
	SymptomShort, ActionShort string
	Href                      string
	CauseLabel                string
	LinkOK                    bool   // 95-204: 일괄 연결 대상(체크박스)
	LinkNote                  string // 대상이 아닌 이유(툴팁)
}

type unlinkedMntRow struct {
	repository.UnlinkedMnt
	NotesShort string
	Href       string
	LinkOK     bool
	LinkNote   string
}

func unlinkedFilterView(c echo.Context, p unlinkedParams, tab string) map[string]interface{} {
	return map[string]interface{}{
		"Tab": tab, "P": p, "Action": "/as/unlinked/" + tab, "Scope": scopeQuery(c),
		"Products": []string{"KLAS", "앤로보틱스"},
	}
}

// UnlinkedAS GET /as/unlinked/as — AS 탭 목록 조각.
func (h *ASHandler) UnlinkedAS(c echo.Context) error {
	p := readUnlinkedParams(c, "")
	p.Tab = "as"
	p.Sort = repository.NormalizeUnlinkedSort(p.Tab, p.SortRaw)
	f := h.unlinkedScope(c)
	f.From, f.To, f.Status, f.Q, f.Page, f.Sort = p.From, p.To, p.Status, p.Q, p.Page, p.Sort
	if p.Who != "" { // 이 목록 안의 담당자 필터(/as 위쪽 목록에는 영향이 없다)
		f.MineUserID, f.MineKeys, f.ExcludeGroup = "", []string{p.Who}, false
	}
	items, total, err := h.repo.ListUnlinkedAS(f)
	if err == nil && len(items) == 0 && total > 0 && f.Page > 1 { // 연결로 마지막 쪽이 비었으면 마지막 쪽으로
		p.Page = (total + unlinkedPageSize - 1) / unlinkedPageSize
		f.Page = p.Page
		items, total, err = h.repo.ListUnlinkedAS(f)
	}
	if err != nil {
		return h.unlinkedError(c, "/as/unlinked/as?"+p.values("", "as", p.Page).Encode()+"&"+scopeQuery(c).Encode())
	}
	// 1차 분류 라벨(조치 화면과 같은 방식: 조직 분류표 → 기존 표). 조회 한 번.
	labels := map[string]string{}
	if cats, err := h.repo.ListCauseCategoriesForOrgAll(currentOrg(c)); err == nil {
		labels = model.CauseCategoryLabels(cats)
	}
	rows := make([]unlinkedASRow, 0, len(items))
	for _, it := range items {
		cause := ""
		if it.CauseCat1 != "" || it.CauseCat1Custom != "" {
			cause = model.CauseCat1Label(it.CauseCat1, it.CauseCat1Custom, labels)
		}
		ok, note := h.asRowLink(c, it)
		rows = append(rows, unlinkedASRow{UnlinkedAS: it, SymptomShort: clipRunesN(it.Symptom, 60), ActionShort: clipRunesN(it.ActionTaken, 60), Href: asOpenHref(it.ASID), CauseLabel: cause, LinkOK: ok, LinkNote: note})
	}
	pages := (total + unlinkedPageSize - 1) / unlinkedPageSize
	data := unlinkedFilterView(c, p, "as")
	data["Kind"], data["Rows"], data["Total"] = "as", rows, total
	data["Links"] = h.unlinkedLinks(c, p, "as", p.Page, pages)
	data["SortOptions"], data["CurSort"], data["Heads"] = repository.AsUnlinkedSortOptions, sortOrDefault(p.Sort, "recv_desc"), h.unlinkedHeads(c, p, "as")
	data["Filtered"] = p.Q != "" || p.Status != "" || p.From != "" || p.To != "" || p.Who != ""
	h.unlinkedBulkData(c, p, "as", data, len(rows))
	return c.Render(http.StatusOK, "as/unlinked_fragment.html", data)
}

// UnlinkedMnt GET /as/unlinked/mnt — 정기점검 탭 목록 조각.
func (h *ASHandler) UnlinkedMnt(c echo.Context) error {
	p := readUnlinkedParams(c, "")
	p.Tab = "mnt"
	p.Sort = repository.NormalizeUnlinkedSort(p.Tab, p.SortRaw)
	f := h.unlinkedScope(c)
	f.From, f.To, f.Status, f.Q, f.Page, f.Product, f.Sort = p.From, p.To, p.Status, p.Q, p.Page, p.Product, p.Sort
	if p.Who != "" {
		f.MineUserID, f.MineKeys, f.ExcludeGroup = "", []string{p.Who}, false
	}
	items, total, err := h.repo.ListUnlinkedMnt(f, time.Now().Format("2006-01-02"))
	if err == nil && len(items) == 0 && total > 0 && f.Page > 1 {
		p.Page = (total + unlinkedPageSize - 1) / unlinkedPageSize
		f.Page = p.Page
		items, total, err = h.repo.ListUnlinkedMnt(f, time.Now().Format("2006-01-02"))
	}
	if err != nil {
		return h.unlinkedError(c, "/as/unlinked/mnt?"+p.values("", "mnt", p.Page).Encode()+"&"+scopeQuery(c).Encode())
	}
	rows := make([]unlinkedMntRow, 0, len(items))
	for _, it := range items {
		ok, note := h.mntRowLink(c, it)
		rows = append(rows, unlinkedMntRow{UnlinkedMnt: it, NotesShort: clipRunesN(it.Notes, 40), Href: "/maintenance/visits/" + url.PathEscape(it.VisitID) + "/action", LinkOK: ok, LinkNote: note})
	}
	pages := (total + unlinkedPageSize - 1) / unlinkedPageSize
	data := unlinkedFilterView(c, p, "mnt")
	data["Kind"], data["Rows"], data["Total"] = "mnt", rows, total
	data["Links"] = h.unlinkedLinks(c, p, "mnt", p.Page, pages)
	data["SortOptions"], data["CurSort"], data["Heads"] = repository.MntUnlinkedSortOptions, sortOrDefault(p.Sort, "visit_desc"), h.unlinkedHeads(c, p, "mnt")
	data["Filtered"] = p.Q != "" || p.Status != "" || p.From != "" || p.To != "" || p.Product != "" || p.Who != ""
	h.unlinkedBulkData(c, p, "mnt", data, len(rows))
	return c.Render(http.StatusOK, "as/unlinked_fragment.html", data)
}

var _ = model.ASOpenHref
