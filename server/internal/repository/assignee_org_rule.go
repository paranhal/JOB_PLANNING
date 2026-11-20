package repository

import (
	"database/sql"
	"strings"

	"customer-support/internal/model"
)

// 95-215 규칙 O: 한 조직의 화면에서 「담당자」로 표시되거나 선택·저장·복사되는 사람은 그 조직 소속 사용자뿐이다.
// 담당자가 들어가는 모든 경로는 이 파일의 함수 하나로 판정한다(경로마다 따로 검사하지 않는다).

// assigneeLibraryEmptyOrgAllowed 95-189 의 도서관 「조직 빈 사용자 허용」 예외(ListAssignable 과 같은 규칙).
// 데이터 정리(운영 SQL ⑤) 뒤 별도 지시서에서 false 로 바꾼다. 이번 지시서(95-215)에서는 건드리지 않는다.
const assigneeLibraryEmptyOrgAllowed = true

// UserInOrg 「조직이 userOrg 인 직원이 orgID 조직 소속인가」의 유일한 규칙. 담당자 후보 목록(ListAssignable)·입력 검사
// (checkAssigneeInOrg)·표시 판정·점검 상속이 모두 이 함수와 assigneeOrgCond 를 쓴다(95-215 후속: 규칙을 한 곳에 둔다).
func UserInOrg(userOrg, orgID string) bool {
	userOrg, orgID = strings.TrimSpace(userOrg), strings.TrimSpace(orgID)
	if orgID == "" || orgID == OrgAll {
		return false
	}
	if userOrg == orgID {
		return true
	}
	return assigneeLibraryEmptyOrgAllowed && orgID == model.OrgIDLibrary && userOrg == ""
}

// assigneeOrgCond users 테이블(별칭 없음)의 org_id 조건. UserInOrg 와 같은 규칙의 SQL 판이다.
func assigneeOrgCond(orgID string) string {
	if assigneeLibraryEmptyOrgAllowed && strings.TrimSpace(orgID) == model.OrgIDLibrary {
		return ` AND (org_id=? OR TRIM(COALESCE(org_id,''))='')`
	}
	return ` AND org_id=?`
}

type assigneeOrgUser struct {
	UserID string
	OrgID  string
}

// AssigneeOrgChecker 활성 내부 직원 목록을 한 번 읽어 두고 「이 사람이 이 조직 소속인가」를 답한다.
type AssigneeOrgChecker struct {
	byKey map[string]assigneeOrgUser // user_id · username · full_name → 사용자
}

// NewAssigneeOrgChecker ListAssignable 과 같은 역할·활성 조건의 사용자를 읽는다.
func NewAssigneeOrgChecker(db *sql.DB) *AssigneeOrgChecker {
	c := &AssigneeOrgChecker{byKey: map[string]assigneeOrgUser{}}
	if db == nil {
		return c
	}
	rows, err := db.Query(`SELECT user_id, COALESCE(username,''), COALESCE(full_name,''), COALESCE(org_id,'')
		FROM users
		WHERE is_active=1 AND TRIM(COALESCE(deleted_at,''))='' AND role IN ('admin','org_admin','tech','office','support','receipt','sales')`)
	if err != nil {
		return c
	}
	defer rows.Close()
	for rows.Next() {
		var id, un, fn, org string
		if rows.Scan(&id, &un, &fn, &org) != nil {
			continue
		}
		u := assigneeOrgUser{UserID: strings.TrimSpace(id), OrgID: strings.TrimSpace(org)}
		for _, k := range []string{id, un, fn} {
			if k = strings.TrimSpace(k); k != "" {
				if _, dup := c.byKey[k]; !dup || k == strings.TrimSpace(id) {
					c.byKey[k] = u
				}
			}
		}
	}
	return c
}

// Belongs uidOrName 이 orgID 소속 활성 직원이면 true. 빈 값·사용자 미등록(이름만)·조직 밖이면 false.
func (c *AssigneeOrgChecker) Belongs(orgID, uidOrName string) bool {
	orgID, uidOrName = strings.TrimSpace(orgID), strings.TrimSpace(uidOrName)
	if c == nil || orgID == "" || orgID == OrgAll || uidOrName == "" {
		return false
	}
	u, ok := c.byKey[uidOrName]
	if !ok {
		return false
	}
	return UserInOrg(u.OrgID, orgID)
}

// assigneeOrgState 필요한 사용자만 조회해 (활성 직원으로 알려진 사람인가, 그 조직 소속인가)를 돌려준다.
func assigneeOrgState(db *sql.DB, orgID, key string) (known, belongs bool) {
	orgID, key = strings.TrimSpace(orgID), strings.TrimSpace(key)
	if db == nil || orgID == "" || orgID == OrgAll || key == "" {
		return false, false
	}
	rows, err := db.Query(`SELECT COALESCE(org_id,'') FROM users
		WHERE is_active=1 AND TRIM(COALESCE(deleted_at,''))='' AND role IN ('admin','org_admin','tech','office','support','receipt','sales')
		  AND (user_id=? OR TRIM(COALESCE(username,''))=? OR TRIM(COALESCE(full_name,''))=?)`, key, key, key)
	if err != nil {
		return false, false
	}
	defer rows.Close()
	for rows.Next() {
		var org string
		if rows.Scan(&org) != nil {
			continue
		}
		known = true
		if UserInOrg(org, orgID) {
			belongs = true
		}
	}
	return known, belongs
}

// AssigneeBelongsToOrg 한 번만 확인할 때의 공통 함수(필요한 사용자만 조회한다). uid 가 있으면 uid 를, 없으면 이름을 쓴다.
// 목록을 여러 건 확인할 때는 NewAssigneeOrgChecker 를 한 번 만들어 쓴다. 판정 규칙은 같다.
func AssigneeBelongsToOrg(db *sql.DB, orgID, uid, name string) bool {
	key := strings.TrimSpace(uid)
	if key == "" {
		key = strings.TrimSpace(name)
	}
	_, belongs := assigneeOrgState(db, orgID, key)
	return belongs
}

// AssigneeNeedsReassign 화면 표시용: 이름이 있는데 그 조직 소속이 아니면 true(「담당 재배정 필요」).
// done 이 true 면(완료된 방문·업무는 실적 이력) 이름을 그대로 둔다.
func (c *AssigneeOrgChecker) AssigneeNeedsReassign(orgID, assignee string, done bool) bool {
	if done || strings.TrimSpace(assignee) == "" || strings.TrimSpace(orgID) == "" || strings.TrimSpace(orgID) == OrgAll {
		return false
	}
	// 어느 직원과도 맞지 않는 이름만 있는 담당(사용자 미등록)은 종전대로 둔다. 다른 조직의 직원일 때만 재배정 대상이다.
	return c.KnownUser(assignee) && !c.Belongs(orgID, assignee)
}

// AssigneeOrgChecker UserRepo 에서 판정기를 만든다(화면 쪽에서 쓴다).
func (r *UserRepo) AssigneeOrgChecker() *AssigneeOrgChecker {
	if r == nil {
		return NewAssigneeOrgChecker(nil)
	}
	return NewAssigneeOrgChecker(r.db)
}

// KnownUser 이름·ID 가 활성 내부 직원(어느 조직이든) 한 명과 맞으면 true. 「사용자 미등록」과 「조직 밖 직원」을 가를 때 쓴다.
func (c *AssigneeOrgChecker) KnownUser(uidOrName string) bool {
	if c == nil {
		return false
	}
	_, ok := c.byKey[strings.TrimSpace(uidOrName)]
	return ok
}

// NeedsReassign 카드 한 장 단위 판정(필요한 사용자만 조회). 목록을 훑을 때는 AssigneeOrgChecker 를 한 번 만들어 쓴다.
func (r *UserRepo) NeedsReassign(orgID, assignee string, done bool) bool {
	orgID, assignee = strings.TrimSpace(orgID), strings.TrimSpace(assignee)
	if r == nil || done || assignee == "" || orgID == "" || orgID == OrgAll {
		return false
	}
	known, belongs := assigneeOrgState(r.db, orgID, assignee)
	return known && !belongs
}
