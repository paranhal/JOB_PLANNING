package handler

import (
	"customer-support/internal/repository"
)

// siteBadgeMap 사업의 사이트별(고객 ID 기준) 하부 업무 배지. 사업이 없거나 조회에 실패하면 빈 맵이다(표시만 빠진다). 95-179D.
func siteBadgeMap(repo *repository.WBRepo, projectID string) map[string][]repository.SiteBadge {
	if repo == nil || projectID == "" {
		return map[string][]repository.SiteBadge{}
	}
	m, err := repository.NewProjectRepo(repo.DB()).SiteBadgesByCustomer(projectID)
	if err != nil || m == nil {
		return map[string][]repository.SiteBadge{}
	}
	return m
}

func attachSiteBadges(data map[string]interface{}, repo *repository.WBRepo, projectID string) {
	data["SiteBadgeMap"] = siteBadgeMap(repo, projectID)
}
