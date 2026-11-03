package model

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// CauseCatalogEmpty 활성 1차가 없으면 자유 입력 모드(95-97 §3-4).
func CauseCatalogEmpty(cats []CauseCategory) bool {
	for _, c := range cats {
		if c.Level == 1 && c.IsActive && strings.TrimSpace(c.Code) != "" && c.Code != CauseCatCustom {
			return false
		}
	}
	return true
}

// CauseOrgPathCode 조직·1차>2차>3차 경로의 안정 코드. 같은 경로면 같은 코드.
func CauseOrgPathCode(orgID string, labels ...string) string {
	orgID = strings.TrimSpace(orgID)
	var path []string
	for _, l := range labels {
		l = strings.TrimSpace(l)
		if l == "" {
			break
		}
		path = append(path, l)
	}
	raw := orgID + "·" + strings.Join(path, ">")
	sum := sha256.Sum256([]byte(raw))
	return "g" + hex.EncodeToString(sum[:10])
}

type CauseCatImportPreview struct {
	NewN     int
	UpdN     int
	SameN    int
	ErrN     int
	Rows     []CauseCatImportRow
	FileName string
	OrgID    string
}

type CauseCatImportRow struct {
	Sheet        string
	Line         int
	Action       string
	Key          string
	Msg          string
	OrgID        string
	Code         string
	Level        int
	ParentCode   string
	Label        string
	CauseTypeMap string
	IsFault      bool
	IsActive     bool
	SortOrder    int
}
