package repository

import (
	"errors"
	"fmt"
	"strings"

	"customer-support/internal/model"
)

// OrgAll 비젼관리자만 넘긴다. 빈 문자열은 전 조직이 아니라 오류다 (§52.4).
const OrgAll = "*"

var (
	ErrEmptyOrgID       = errors.New("org_id 가 비었습니다")
	ErrProjectForbidden = errors.New("이 사업을 변경할 수 없습니다")
)

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

// projectVisibleSQL 사업 등록 관리 조회. 소속 조직이거나 86 공개(is_shared, share_to 빈 값=전 팀·아니면 대상 팀).
func projectVisibleSQL(alias, orgID string) (string, []interface{}, error) {
	if err := RequireOrgID(orgID); err != nil {
		return "", nil, err
	}
	if orgID == OrgAll {
		return "", nil, nil
	}
	p := strings.TrimSpace(alias)
	if p == "" {
		p = "work_projects"
	}
	frag := fmt.Sprintf(` AND (COALESCE(NULLIF(TRIM(%s.org_id),''), ?) = ? OR (COALESCE(%s.is_shared,0)=1 AND (TRIM(COALESCE(%s.share_to,''))='' OR %s.share_to=?)))`,
		p, p, p, p)
	return frag, []interface{}{model.OrgIDLibrary, orgID, orgID}, nil
}

func projectHomeOrg(orgID string) string {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return model.OrgIDLibrary
	}
	return orgID
}

func ProjectWritableInOrg(rowOrg, orgID string) bool {
	return VisibleInOrg(projectHomeOrg(rowOrg), orgID)
}

func appendProjectVisible(q string, args []interface{}, alias, orgID string) (string, []interface{}) {
	frag, a, err := projectVisibleSQL(alias, orgID)
	if err != nil {
		return q + ` AND 1=0`, args
	}
	return q + frag, append(args, a...)
}

func orgIDOrAll(orgID string) string {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return OrgAll
	}
	return orgID
}

// workTaskVisibleOrgSQL 일일업무 조직 판정: 행 org_id → 담당자 소속 → 작성자 소속.
// 셋 다 비면 조직 지정 조회에서 제외한다. OrgAll이면 조건 없음.
func workTaskVisibleOrgSQL(alias, orgID string) (string, []interface{}) {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" || orgID == OrgAll {
		return "", nil
	}
	a := "t"
	if strings.TrimSpace(alias) != "" {
		a = alias
	}
	frag := fmt.Sprintf(` AND COALESCE(
		NULLIF(TRIM(%[1]s.org_id),''),
		(SELECT NULLIF(TRIM(ua.org_id),'') FROM users ua WHERE ua.user_id=%[1]s.assignee_user_id LIMIT 1),
		(SELECT NULLIF(TRIM(un.org_id),'') FROM users un WHERE TRIM(%[1]s.assignee)!='' AND TRIM(un.full_name)=TRIM(%[1]s.assignee) LIMIT 1),
		(SELECT NULLIF(TRIM(uc.org_id),'') FROM users uc WHERE uc.user_id=%[1]s.created_by_user_id LIMIT 1)
	)=?`, a)
	return frag, []interface{}{orgID}
}
