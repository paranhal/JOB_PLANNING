package handler

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/backup"
)

// 빌드 값. Dockerfile -ldflags 로 main 에서 SetBuildInfo 한다. v2.0 §40
var (
	buildMu      sync.RWMutex
	buildVersion = "dev"
	buildCommit  = "unknown"
	buildTime    = "unknown"
	startedAt    time.Time
)

func SetBuildInfo(version, commit, built string, started time.Time) {
	buildMu.Lock()
	defer buildMu.Unlock()
	if strings.TrimSpace(version) != "" {
		buildVersion = strings.TrimSpace(version)
	}
	if strings.TrimSpace(commit) != "" {
		buildCommit = strings.TrimSpace(commit)
	}
	if strings.TrimSpace(built) != "" {
		buildTime = strings.TrimSpace(built)
	}
	startedAt = started
}

func currentBuild() (version, commit, built string, started time.Time) {
	buildMu.RLock()
	defer buildMu.RUnlock()
	return buildVersion, buildCommit, buildTime, startedAt
}

func formatClock(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return t.In(backup.SeoulLocation()).Format("2006-01-02 15:04")
}

func badgeShort(version, built string) string {
	return version + " · " + shortBuilt(built)
}

func shortBuilt(built string) string {
	t, ok := parseBuilt(built)
	if !ok {
		return built
	}
	return t.Format("01-02 15:04")
}

func parseBuilt(built string) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02 15:04:05", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, built, backup.SeoulLocation()); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func badgeFull(version, commit, built string) string {
	return version + " · 커밋 " + commit + " · 빌드 " + built + " KST"
}

// AssetQuery 정적 파일 주소에 붙인다. 빌드가 바뀌면 값이 바뀐다. §40.6
func AssetQuery() string {
	version, _, built, _ := currentBuild()
	q := version + "-" + built
	q = strings.ReplaceAll(q, " ", "-")
	q = strings.ReplaceAll(q, ":", "")
	return q
}

func injectBuildInfo(data map[string]interface{}) {
	version, commit, built, _ := currentBuild()
	data["BuildVersion"] = version
	data["BuildCommit"] = commit
	data["BuildTime"] = built
	data["AssetQuery"] = AssetQuery()
	data["BuildBadgeShort"] = badgeShort(version, built)
	data["BuildBadgeFull"] = badgeFull(version, commit, built)
	if notice := currentSchemaNotice(); notice != "" {
		data["SchemaNotice"] = notice
	}
}

// VersionJSON GET /version — 로그인 없이 현재 빌드만 준다. §40.4 ③ · §40.5.2
func VersionJSON(c echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-cache")
	version, commit, built, started := currentBuild()
	return c.JSON(http.StatusOK, map[string]string{
		"version": version,
		"commit":  commit,
		"built":   built,
		"started": formatClock(started),
	})
}
