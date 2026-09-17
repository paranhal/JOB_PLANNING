package handler

import (
	"net/url"
	"strings"
)

type sortCol struct {
	Key, Label string
}

// sortLinkHrefs 필터 쿼리를 유지한 채 열별 정렬 링크를 만든다. §35.7
func sortLinkHrefs(path string, filter url.Values, cols []string, curSort, curDir string) map[string]string {
	out := make(map[string]string, len(cols))
	for _, col := range cols {
		v := cloneURLValues(filter)
		v.Del("page")
		v.Del("offset")
		dir := "asc"
		if col == curSort && curDir == "asc" {
			dir = "desc"
		}
		v.Set("sort", col)
		v.Set("dir", dir)
		enc := v.Encode()
		if enc == "" {
			out[col] = path
			continue
		}
		out[col] = path + "?" + enc
	}
	return out
}

func sortSelectOptions(cols []sortCol, hrefs map[string]string, cur, dir string) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(cols))
	for _, col := range cols {
		active := col.Key == cur
		mark := "⇅"
		if active {
			if dir == "desc" {
				mark = "▼"
			} else {
				mark = "▲"
			}
		}
		out = append(out, map[string]interface{}{
			"Label":  col.Label,
			"Href":   hrefs[col.Key],
			"Active": active,
			"Mark":   mark,
		})
	}
	return out
}

func workListSortCols(showAssignee bool) []sortCol {
	cols := []sortCol{
		{Key: "task_id", Label: "번호"},
		{Key: "prefix", Label: "구분"},
		{Key: "title", Label: "업무명"},
		{Key: "customer", Label: "거래처"},
	}
	if showAssignee {
		cols = append(cols, sortCol{Key: "assignee", Label: "담당자"})
	}
	return append(cols,
		sortCol{Key: "work_date", Label: "시작일"},
		sortCol{Key: "due_date", Label: "종료일"},
		sortCol{Key: "status", Label: "상태"},
	)
}

func adminWorkSortCols() []sortCol {
	return []sortCol{
		{Key: "task_id", Label: "번호"},
		{Key: "customer", Label: "거래처"},
		{Key: "title", Label: "업무명"},
		{Key: "assignee", Label: "담당자"},
		{Key: "work_date", Label: "시작일"},
		{Key: "due_date", Label: "종료일"},
		{Key: "status", Label: "상태"},
		{Key: "duration", Label: "소요시간"},
		{Key: "created_at", Label: "등록일"},
	}
}

func asListSortCols() []sortCol {
	return []sortCol{
		{Key: "as_number", Label: "접수번호"},
		{Key: "org_name", Label: "기관(고객명)"},
		{Key: "status", Label: "상태"},
		{Key: "visit", Label: "예정일"},
		{Key: "assigned", Label: "담당"},
		{Key: "days", Label: "경과"},
	}
}

func unplannedSortCols(showAssignee bool) []sortCol {
	cols := []sortCol{}
	if showAssignee {
		cols = append(cols, sortCol{Key: "assignee", Label: "담당자"})
	}
	return append(cols,
		sortCol{Key: "prefix", Label: "구분"},
		sortCol{Key: "task_id", Label: "번호"},
		sortCol{Key: "customer", Label: "기관 / 제목"},
		sortCol{Key: "due_date", Label: "예정일"},
	)
}

func salesListSortCols() []sortCol {
	return []sortCol{
		{Key: "name", Label: "사업명"},
		{Key: "stage", Label: "단계"},
		{Key: "customer", Label: "고객"},
		{Key: "period", Label: "시기"},
		{Key: "amount", Label: "금액"},
	}
}

func quoteListSortCols() []sortCol {
	return []sortCol{
		{Key: "quote_no", Label: "번호"},
		{Key: "quote_date", Label: "견적일"},
		{Key: "recipient", Label: "수신"},
		{Key: "title", Label: "건명"},
		{Key: "total", Label: "합계"},
		{Key: "status", Label: "상태"},
	}
}

func parseOptionalSort(sort, dir, allowed string) (string, string) {
	sort = strings.TrimSpace(sort)
	dir = strings.TrimSpace(dir)
	if dir != "asc" && dir != "desc" {
		dir = "asc"
	}
	for _, a := range strings.Split(allowed, ",") {
		if sort == a {
			return sort, dir
		}
	}
	return "", ""
}
