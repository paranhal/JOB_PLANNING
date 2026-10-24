package handler

import (
	"strings"

	"customer-support/internal/model"
)

// asIDFromHref Create 등 리다이렉트 Location 에서 접수 ID 를 꺼낸다.
func asIDFromHref(loc string) string {
	loc = strings.TrimSpace(loc)
	if i := strings.IndexAny(loc, "?#"); i >= 0 {
		loc = loc[:i]
	}
	loc = strings.TrimPrefix(loc, "/as/")
	return strings.TrimSuffix(loc, "/action")
}

// asOpenHref AS 건 기본 진입. 템플릿 asHref 와 같다. 95-33.
// 타팀 지원(95-26)·AssigneePicker(95-25)는 이 단계 범위 밖이다. 링크만 통일한다.
func asOpenHref(asID string) string {
	return model.ASOpenHref(asID)
}

func asOpenHrefQuery(asID, rawQuery string) string {
	return model.ASOpenHrefQuery(asID, rawQuery)
}

func asLegacyHref(asID string) string {
	return model.ASLegacyHref(asID)
}

func asEmbedHref(asID string) string {
	return model.ASEmbedHref(asID)
}
