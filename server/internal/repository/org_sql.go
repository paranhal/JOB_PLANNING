package repository

import (
	"errors"
	"fmt"
	"strings"
)

// OrgAll 비젼관리자만 넘긴다. 빈 문자열은 전 조직이 아니라 오류다 (§52.4).
const OrgAll = "*"

var ErrEmptyOrgID = errors.New("org_id 가 비었습니다")

func RequireOrgID(orgID string) error {
	if strings.TrimSpace(orgID) == "" {
		return ErrEmptyOrgID
	}
	return nil
}

func RequireInsertOrg(orgID string) (string, error) {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" || orgID == OrgAll {
		return "", ErrEmptyOrgID
	}
	return orgID, nil
}

func VisibleInOrg(rowOrg, orgID string) bool {
	rowOrg = strings.TrimSpace(rowOrg)
	if err := RequireOrgID(orgID); err != nil {
		return false
	}
	if orgID == OrgAll {
		return true
	}
	return rowOrg == orgID
}

// AppendOrgSQL 빈 org_id 는 오류. OrgAll 은 추가 조건 없음(비젼관리자).
func AppendOrgSQL(alias, orgID string) (string, []interface{}, error) {
	if err := RequireOrgID(orgID); err != nil {
		return "", nil, err
	}
	col := "org_id"
	if strings.TrimSpace(alias) != "" {
		col = alias + ".org_id"
	}
	if orgID == OrgAll {
		return "", nil, nil
	}
	return fmt.Sprintf(` AND %s = ?`, col), []interface{}{orgID}, nil
}

func mustOrgSQL(alias, orgID string) (string, []interface{}) {
	frag, args, err := AppendOrgSQL(alias, orgID)
	if err != nil {
		return ` AND 1=0`, nil
	}
	return frag, args
}

func appendOrg(q string, args []interface{}, alias, orgID string) (string, []interface{}) {
	frag, a := mustOrgSQL(alias, orgID)
	return q + frag, append(args, a...)
}

func orgIDOrAll(orgID string) string {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return OrgAll
	}
	return orgID
}
