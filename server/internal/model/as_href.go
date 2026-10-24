package model

import "strings"

// ASOpenHref AS 건을 여는 기본 주소. 새 조치 화면 (95-33).
func ASOpenHref(asID string) string {
	id := strings.TrimSpace(asID)
	if id == "" {
		return "/as"
	}
	return "/as/" + id + "/action"
}

// ASOpenHrefQuery 조치 화면에 쿼리를 붙인다 (? 는 있어도 되고 없어도 된다).
func ASOpenHrefQuery(asID, rawQuery string) string {
	h := ASOpenHref(asID)
	q := strings.TrimSpace(rawQuery)
	if q == "" {
		return h
	}
	if strings.HasPrefix(q, "?") {
		return h + q
	}
	return h + "?" + q
}

// ASLegacyHref 예전 접수 상세. Show 는 legacy=1 이 있어야 렌더한다.
func ASLegacyHref(asID string) string {
	id := strings.TrimSpace(asID)
	if id == "" {
		return "/as?legacy=1"
	}
	return "/as/" + id + "?legacy=1"
}

// ASEmbedHref 임베드 조치 화면 (띠·할 일 팝업). 접수 iframe 은 /as/:id?embed=1 을 유지한다.
func ASEmbedHref(asID string) string {
	return ASOpenHrefQuery(asID, "embed=1")
}
