package handler

import "html/template"

const projectMatchPendingBadgeHTML = `<span class="badge badge-warning text-xs">사업명 매칭 필요</span>`

func projectMatchPendingBadge(on bool) template.HTML {
	if !on {
		return ""
	}
	return template.HTML(projectMatchPendingBadgeHTML)
}
