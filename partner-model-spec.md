# 협력사(Partner) 등급·DB 구조 설계서 (v2)

> v2 = 사용자 답변 반영(2026-10-19). v1 대비 변경: 접속 보안 항목 추가, **AS 업무 공유·조치 보고 추가**, **사업(협력 건)별 권한표 도입**(고정 규칙 → 권한 항목×단계), 현재 협력사 인원 확인.
> 결정: D-105, D-106. 구현 지시서: `커서_작업지시서_96단계_협력사_DB구조.md` (이번 단계는 DB + 등급 + 기존 작업자 호환까지. 협력 건 관리 화면 97, 협력사 포털 98)

## 1. 현재 상태 (코드·개발 DB 확인)
- 협력사 인원은 `users` 한 줄로 등록되어 AS·업무의 담당자(작업자)로 쓰인다. 소속 회사를 나타내는 컬럼이 없다.
- **현재 협력업체 인원: 도서관사업팀의 문성진(`user1`), 김남기(`user2`)** — 둘 다 role=support, org=O01. 로그인해서 데이터를 고친 기록(`data_change_logs`)은 없다(로그인 기록 자체는 시스템에 없음).
- **AS 비중**: AS 접수 약 1,400건 중 문성진 459건(약 33%)이 협력사 인원 담당, 김남기 1건. 즉 AS는 이미 협력사 엔지니어가 상당수를 수행한다.
- `as_receipts.external_assignee`(050 마이그레이션)가 이미 있어 **계정이 없는 외부 담당자 이름**을 적는 칸으로 쓰인다(이효준 13건, 최영성(앤로보틱스) 1건). 이들도 협력사 인원일 가능성이 높다(확인 필요).
- 회사 정보는 `customers.party_kind`(customer/own/partner/vendor)로 구분. 개발 DB의 partner: (주)채움씨앤아이, 누미노(주). 어느 회사 소속인지는 아직 미확인.
- 접속은 **일반 인터넷 + 아이디/비밀번호**만 쓴다.

## 2. 핵심 결정
1. **협력사는 새 등급(`role='partner'`)**. 직원 등급과 섞지 않는다.
2. 계정 상태 3종: **직원** / **협력사 작업자 전용**(can_login=0, 지금 방식) / **협력사 포털 계정**(향후, can_login=1).
3. 회사 정보는 `customers`를 그대로 쓰고 협력사 속성만 `partner_companies`(1:1 확장)에 둔다.
4. **접근 경계는 협력 건(engagement)**. 보통 "협력사 × 사업(프로젝트)" 한 건이고, AS 지원용은 사업 없이 `support` 건으로 만든다. 협력사 직원은 자기가 참여한 협력 건에서 공유된 업무·AS·자료만 본다.
5. **권한은 협력 건(사업)별 권한표**로 정한다. 권한 항목(`partner_perm_defs`)×단계(none/view/…)를 협력 건마다 저장하고, 조직(도서관/미디어/SI)별 **기본 템플릿**을 만들어 새 협력 건에 복사한다. 미디어·SI처럼 사업마다 달라지는 경우를 코드 수정 없이 처리한다.
6. 협력사 직원의 `users.org_id` = 담당 조직 → 93-1의 `ListAssignable(orgID)`에 그대로 걸려 지금처럼 일정표·AS 담당자로 배정된다.
7. 협력사는 직원 화면에 못 들어온다. `/partner/**`만 허용, 나머지 403(deny-by-default).
8. **AS 조치는 "조치 보고 → 직원이 반영"이 기본**: 협력사 엔지니어가 방문일시·소요시간·조치내용·원인을 보고하면 직원이 한 번 눌러 공식 AS 조치 기록(`as_processes`)으로 반영한다. 협력 건 권한표에서 `as_action`을 `direct`로 올리면 직접 기록 가능(후속 단계에서 구현).
9. 협력사 계정은 **직원이 등록**한다(협력사의 자체 초대 없음).
10. **인터넷+ID/비밀번호만이면 최소 보안 장치가 필요**하다(§4-3). 포털(98)을 열기 전에 적용한다.

## 3. 데이터 모델
```
customers(party_kind='partner') 1──1 partner_companies
partner_companies 1──N partner_org_links N──1 orgs            (어느 조직과 거래하는가)
partner_companies 1──N users(account_kind='partner')          (협력사 직원)
partner_companies 1──N partner_engagements N──1 orgs          (협력 건 = 협력사×사업 또는 AS 지원)
partner_engagements N──1 work_projects (선택)
partner_engagements 1──N partner_engagement_members N──1 users
partner_engagements 1──N partner_engagement_perms N──1 partner_perm_defs   (사업별 권한표)
orgs 1──N partner_perm_templates 1──N partner_perm_template_items          (조직별 기본 권한 템플릿)
partner_engagements 1──N partner_shares  → work_tasks | as_receipts       (공유한 업무/AS)
partner_engagements 1──N partner_notes / partner_engagement_files
users 1──N partner_access_log
```

### 3-1. `users` 열 추가 (기존 행은 모두 staff)
| 열 | 타입 | 기본 | 설명 |
|---|---|---|---|
| account_kind | TEXT NOT NULL | 'staff' | `staff` \| `partner` |
| partner_id | TEXT | '' | partner면 필수, staff면 비어야 함(트리거) |
| partner_role | TEXT | '' | `manager` \| `worker` (이번엔 구분만, 권한 차이는 후속) |
| can_login | INTEGER NOT NULL | 1 | 0이면 로그인 불가(작업자 전용) |
| access_expires_at | TEXT | '' | 접속 만료일. 포털 계정은 필수 |
| invited_by / invited_at | TEXT | '' | 등록한 직원 |
| failed_login_count | INTEGER NOT NULL | 0 | 연속 실패 횟수 |
| locked_until | TEXT | '' | 잠금 해제 시각 |
| last_login_at / last_login_ip | TEXT | '' | 마지막 접속 |

### 3-2. 신규 테이블
| 테이블 | 목적 | 핵심 열 |
|---|---|---|
| `partner_companies` | 협력사 속성(거래처 1:1) | partner_id PK, customer_id UNIQUE, status, portal_enabled, partner_type, contract_ref, nda_signed_at, security_pledge_at, **allowed_ips**, **require_otp** |
| `partner_org_links` | 협력사↔조직 거래 관계 | (partner_id, org_id) PK, status |
| `partner_engagements` | 협력 건(접근 경계) | engagement_id, partner_id, org_id, project_id?, title, scope, kind(subcontract/joint/maintenance/support), 기간, status, **auto_share_assigned** |
| `partner_engagement_members` | 참여 협력사 직원 | (engagement_id, user_id), member_role, is_active |
| `partner_perm_defs` | 권한 항목 정의(참조 데이터) | perm_key PK, label, grp, levels, default_level |
| `partner_perm_templates` / `_items` | 조직별 기본 권한 템플릿 | (org_id, name), (template_id, perm_key, level) |
| `partner_engagement_perms` | **협력 건(사업)별 권한표** | (engagement_id, perm_key) PK, level |
| `partner_perm_log` | 권한 변경 이력 | engagement_id, perm_key, old/new_level, changed_by, reason |
| `partner_shares` | 공유한 업무/AS | target_type(work_task/as_receipt), target_id, engagement_id, is_active |
| `partner_notes` | 진행 기록·조치 보고 | target_type/id, kind(comment/progress/issue/done_request/done_confirm/reject/**action_report/action_applied**), body, progress_pct, payload_json, applied_ref_* |
| `partner_engagement_files` | 주고받는 자료 | direction(to_partner/from_partner), target_type/id(선택), stored_path |
| `partner_access_log` | 보안 감사 | user_id, partner_id, engagement_id, action, target, ip |

### 3-3. 권한 항목 (초기 목록, `partner_perm_defs` 시드)
| perm_key | 의미 | 단계 | 기본 |
|---|---|---|---|
| task_view | 공유된 업무 보기 | none, view | view |
| task_note | 진행 기록(코멘트·진행률·이슈) | none, write | write |
| task_complete | 업무 완료 | none, request, direct | request |
| as_view | 공유된 AS 보기 | none, view | none |
| as_action | AS 조치 | none, report, direct | none |
| file_view | 자료 내려받기 | none, view | view |
| file_upload | 자료 올리기 | none, write | write |
| customer_info | 고객·현장 정보(**공유된 항목에 한함**) | none, view | none |
| amount_view | 금액 | none, view | none |
| member_view | 참여자 목록 | none, view | view |
- 단계는 정의 테이블의 `levels`에 있는 값만 저장 가능(트리거). 새 권한 항목은 **정의 행 추가**만으로 늘린다(마이그레이션에서 참조 데이터 시드).
- AS 지원용 협력 건 템플릿 예: as_view=view, as_action=report, customer_info=view, task_*=none. 사업 협력 건 템플릿 예: task_view/note=on, as_*=none, amount_view=none. 실제 값은 조직관리자가 템플릿 화면에서 정한다(데이터는 마이그레이션에 넣지 않음).

### 3-4. 무결성 (트리거 + 앱 검증)
- `account_kind='partner'` ⇔ `partner_id` 비어 있지 않음.
- 협력 건 생성 시 (협력사, 조직)의 `partner_org_links`가 active.
- 참여자의 `users.partner_id` = 협력 건의 `partner_id`.
- 공유 대상(업무/AS)의 `org_id` = 협력 건의 `org_id`.
- 권한 단계 값은 `partner_perm_defs.levels`에 있는 값만.
- 같은 협력사×사업의 진행 중(draft/active) 협력 건은 1개만(부분 유니크 인덱스).
- 협력사 status가 suspended/terminated면 소속 사용자 로그인·모든 접근 거부(요청마다 확인).

## 4. 접근 규칙

### 4-1. 볼 수 있는 범위 (권한표 단계와 별개로 항상 적용되는 경계)
- 협력 건 **참여자**이고 협력 건이 **active**이며 협력사·계정이 유효할 때만.
- 업무/AS는 `partner_shares`에 활성 공유된 것. 단 `auto_share_assigned=1`인 `support` 협력 건은 **참여자 본인이 담당자로 지정된 AS(`assigned_user_id`)를 자동 공유**로 본다(문성진처럼 수백 건을 일일이 공유하지 않기 위해).
- 항상 못 보는 것: 영업·견적, 결재, 휴가, 통계, 직원 휴대폰·메일, 다른 협력사·다른 협력 건, 공유되지 않은 업무·AS.
- 고객 정보(`customer_info`)는 공유된 항목의 고객명·설치장소·현장 담당자까지만. 고객 목록·검색은 없음.

### 4-2. 할 수 있는 일 (권한표 단계에 따름)
| 행동 | 필요한 권한 |
|---|---|
| 업무 보기/기록 남기기 | task_view / task_note=write |
| 업무 완료 | task_complete = request(요청) 또는 direct |
| AS 보기 | as_view=view |
| AS 조치 보고 | as_action=report (직원이 "조치로 반영") 또는 direct |
| 자료 보기/올리기 | file_view / file_upload |
| 금액·고객 정보 | amount_view / customer_info |
- 항상 못 하는 것: 기한·담당자 변경, 업무·협력 건 생성/삭제, 다른 사람 계정 관리.
- 직원 측: 협력 건 생성·종료·참여자 지정, 권한표 편집(협력 건 조직의 조직관리자·비젼관리자), 공유/해제, 완료 확인·반려, 조치 반영. **권한 변경은 즉시 적용되고 `partner_perm_log`에 남는다.**

### 4-3. 접속 보안 (일반 인터넷 + ID/비밀번호 전제)
인터넷에 열린 아이디/비밀번호 로그인은 가장 약한 방식이므로 포털을 열기 **전에** 다음을 적용한다(98단계, 컬럼은 96에서 준비).
1. **HTTPS 필수**: 로그인 쿠키 `Secure`·`HttpOnly`·`SameSite`, HSTS. 지금 운영 주소가 HTTPS인지 먼저 확인(HTTP만이면 포털 공개 불가).
2. 비밀번호: 10자 이상·영문+숫자+기호, 첫 로그인 변경 강제(`must_change_password` 재사용), 재사용 방지.
3. 잠금: 5회 연속 실패 → 15분 잠금(`failed_login_count`, `locked_until`), 로그인 요청 횟수 제한.
4. 세션: 유휴 30분 만료, 최대 8시간, 협력사 계정은 동시 로그인 1개.
5. 접속 만료일 필수(`access_expires_at`), 협력 건 종료/회사 중지 시 즉시 차단.
6. 선택(권장): 회사별 **허용 IP**(`allowed_ips`), 그리고 **2단계 인증(OTP)**(`require_otp`)은 99단계 이후 도입 검토. 사용자가 정책을 정하면 컬럼은 이미 있으므로 마이그레이션 없이 켠다.
7. 모든 로그인·열람·내려받기·거부를 `partner_access_log`에 기록하고, 직원 화면에서 협력사별 접속 이력을 본다.

## 5. 수명주기
1. 회사 등록(거래처 선택/신규) → 거래 조직 연결.
2. 직원 등록: 협력사 소속 사용자 생성(기본 작업자 전용). 기존 사용자 전환은 사용자 승인 후 관리자 화면에서.
3. 협력 건 개설: 조직·사업 선택 → **조직 권한 템플릿 복사** → 필요하면 권한표 수정 → 참여자 지정 → 공유(또는 AS 자동 공유 켬).
4. 포털 접속 허용: 회사 `portal_enabled=1` + 사용자 `can_login=1` + 만료일 지정 + 초기 비밀번호 변경 강제.
5. 종료: 협력 건 closed → 참여 해제·공유 비활성. 회사 중지 → 전 계정 차단.

## 6. 기존 기능과의 관계
- **담당자 목록(93-1)**: `ListAssignable(orgID)`는 `org_id=?`인 협력사 사용자(회사 active)를 포함. AS·업무 담당 배정 흐름은 지금과 같다.
- **`as_receipts.external_assignee`**: 그대로 둔다. 97단계의 "외부 담당자 정리" 화면에서 이름 → 협력사 사용자로 연결(데이터 변경은 사용자 승인 후).
- **AS 통계**: 협력사 처리 건수 비교는 `users.account_kind`로 구분 가능. 이번 단계에서 통계 값은 바꾸지 않고, 포함되는 곳 목록만 보고.
- **조직 격리(92·93)**: 협력사 포털은 조직 데이터 직접 조회가 아니라 협력 건→공유로만. 정적 감사 allowlist에 `/partner/**`를 사유와 함께 등록.
- **업무 협조(87)**: 87 마이그레이션 적용 후 `work_cooperations`에 `target_partner_id`, `engagement_id`를 추가(99단계).
- **권한표 화면**: 협력사는 직원 권한표 대상이 아니다. 협력 건 권한표는 별도 화면(97단계).
- **경영 통합(94)**: 협력사 직원은 직원 수 집계 대상 아님. 향후 "협력 현황" 카드(협력사 처리 AS 건수 등)는 99단계.

## 7. 단계 계획
| 단계 | 내용 |
|---|---|
| **96** | DB(마이그레이션·권한 항목 시드), 협력사 등급·모델, 협력사 회사 관리, 사용자 관리에서 협력사 지정, 로그인 차단, `ListAssignable` 호환, 현황 보고 |
| 97 | 협력 건 관리(개설·참여자·공유·**권한표/템플릿 화면**·AS 자동 공유)·외부 담당자 정리 |
| 98 | 협력사 포털(`/partner`): 내 협력 건, 공유 업무·AS, 기록·자료·완료 요청, **AS 조치 보고**, 보안 장치(§4-3), 직원 측 확인/반영·접속 로그 |
| 99 | 업무 협조(87) 연동, 알림, 만료/종료 자동 처리, 허용 IP·OTP, 경영 통합 "협력 현황" |

## 8. 확정된 답변과 남은 질문
| 질문 | 답 | 반영 |
|---|---|---|
| 접속 경로 | 일반 인터넷 + ID/비밀번호 | §4-3 보안 장치, `allowed_ips`·`require_otp` 컬럼 준비 |
| 협력사의 상태 변경 | 현재 없음, 실제로는 AS 일부를 협력사 엔지니어가 작업 | AS 공유·조치 보고(2-8), 권한표의 `as_action` |
| 계정 등록 | 우리 직원이 등록, 사업별로 권한이 자주 바뀜(특히 미디어·SI) | 협력 건별 권한표 + 조직 템플릿 |
| 현재 협력사 인원 | 도서관사업팀 문성진, 김남기 | 전환 절차(96 §E) |

**남은 질문**
1. 문성진·김남기는 **어느 협력사 소속**인가(개발 DB 후보: (주)채움씨앤아이, 누미노(주), 또는 새 회사)? 이효준·최영성(앤로보틱스)도 협력사 인원인가?
2. 문성진(`user1`)·김남기(`user2`) 계정으로 **지금 실제로 로그인해서 쓰는가**? (로그인 기록이 없어 코드로 확인 불가. 협력사로 전환하면 포털 오픈 전까지 로그인이 막힘)
3. AS 조치 기록을 협력사가 **직접 남기는 방식**을 원하는지, **보고 후 직원 반영**으로 시작할지(기본: 보고 후 반영).
4. 운영 주소는 **HTTPS**인가?

## 9. 부록 — 마이그레이션 SQL 초안 (SQLite, 번호는 Cursor가 다음 빈 번호로 부여)
```sql
-- A. users 확장 (기존 행은 모두 staff)
ALTER TABLE users ADD COLUMN account_kind TEXT NOT NULL DEFAULT 'staff';
ALTER TABLE users ADD COLUMN partner_id TEXT DEFAULT '';
ALTER TABLE users ADD COLUMN partner_role TEXT DEFAULT '';
ALTER TABLE users ADD COLUMN can_login INTEGER NOT NULL DEFAULT 1;
ALTER TABLE users ADD COLUMN access_expires_at TEXT DEFAULT '';
ALTER TABLE users ADD COLUMN invited_by TEXT DEFAULT '';
ALTER TABLE users ADD COLUMN invited_at TEXT DEFAULT '';
ALTER TABLE users ADD COLUMN failed_login_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN locked_until TEXT DEFAULT '';
ALTER TABLE users ADD COLUMN last_login_at TEXT DEFAULT '';
ALTER TABLE users ADD COLUMN last_login_ip TEXT DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_users_partner ON users(partner_id);

-- B. 협력사 회사 / 조직 연결
CREATE TABLE IF NOT EXISTS partner_companies (
  partner_id TEXT PRIMARY KEY,
  customer_id TEXT NOT NULL UNIQUE REFERENCES customers(customer_id),
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','suspended','terminated')),
  portal_enabled INTEGER NOT NULL DEFAULT 0,
  partner_type TEXT NOT NULL DEFAULT '',
  contract_ref TEXT NOT NULL DEFAULT '',
  nda_signed_at TEXT NOT NULL DEFAULT '',
  security_pledge_at TEXT NOT NULL DEFAULT '',
  allowed_ips TEXT NOT NULL DEFAULT '',
  require_otp INTEGER NOT NULL DEFAULT 0,
  note TEXT NOT NULL DEFAULT '',
  created_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS partner_org_links (
  partner_id TEXT NOT NULL REFERENCES partner_companies(partner_id),
  org_id TEXT NOT NULL REFERENCES orgs(org_id),
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','ended')),
  started_at TEXT NOT NULL DEFAULT '',
  ended_at TEXT NOT NULL DEFAULT '',
  note TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (partner_id, org_id)
);

-- C. 권한 항목 정의(참조 데이터) / 조직별 템플릿
CREATE TABLE IF NOT EXISTS partner_perm_defs (
  perm_key TEXT PRIMARY KEY,
  label TEXT NOT NULL,
  grp TEXT NOT NULL DEFAULT '',
  levels TEXT NOT NULL,          -- 예: 'none,view' 낮은 단계 → 높은 단계
  default_level TEXT NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0
);
INSERT OR IGNORE INTO partner_perm_defs (perm_key,label,grp,levels,default_level,sort_order) VALUES
 ('task_view','업무 보기','업무','none,view','view',10),
 ('task_note','진행 기록(코멘트·진행률·이슈)','업무','none,write','write',20),
 ('task_complete','업무 완료','업무','none,request,direct','request',30),
 ('as_view','AS 보기','AS','none,view','none',40),
 ('as_action','AS 조치','AS','none,report,direct','none',50),
 ('file_view','자료 내려받기','자료','none,view','view',60),
 ('file_upload','자료 올리기','자료','none,write','write',70),
 ('customer_info','고객·현장 정보(공유 항목 한정)','정보','none,view','none',80),
 ('amount_view','금액','정보','none,view','none',90),
 ('member_view','참여자 목록','정보','none,view','view',100);

CREATE TABLE IF NOT EXISTS partner_perm_templates (
  template_id TEXT PRIMARY KEY,
  org_id TEXT NOT NULL REFERENCES orgs(org_id),
  name TEXT NOT NULL,
  is_default INTEGER NOT NULL DEFAULT 0,
  created_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (org_id, name)
);
CREATE TABLE IF NOT EXISTS partner_perm_template_items (
  template_id TEXT NOT NULL REFERENCES partner_perm_templates(template_id),
  perm_key TEXT NOT NULL REFERENCES partner_perm_defs(perm_key),
  level TEXT NOT NULL,
  PRIMARY KEY (template_id, perm_key)
);

-- D. 협력 건 / 참여자 / 사업별 권한표
CREATE TABLE IF NOT EXISTS partner_engagements (
  engagement_id TEXT PRIMARY KEY,
  partner_id TEXT NOT NULL REFERENCES partner_companies(partner_id),
  org_id TEXT NOT NULL REFERENCES orgs(org_id),
  project_id TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL,
  scope TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL DEFAULT 'subcontract' CHECK (kind IN ('subcontract','joint','maintenance','support')),
  start_date TEXT NOT NULL DEFAULT '',
  end_date TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','active','closed','cancelled')),
  auto_share_assigned INTEGER NOT NULL DEFAULT 0,
  created_by_user_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  closed_at TEXT NOT NULL DEFAULT '',
  close_reason TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_pe_partner ON partner_engagements(partner_id, status);
CREATE INDEX IF NOT EXISTS idx_pe_org ON partner_engagements(org_id, status);
CREATE UNIQUE INDEX IF NOT EXISTS ux_pe_partner_project
  ON partner_engagements(partner_id, project_id)
  WHERE project_id <> '' AND status IN ('draft','active');

CREATE TABLE IF NOT EXISTS partner_engagement_members (
  engagement_id TEXT NOT NULL REFERENCES partner_engagements(engagement_id),
  user_id TEXT NOT NULL REFERENCES users(user_id),
  member_role TEXT NOT NULL DEFAULT 'worker' CHECK (member_role IN ('lead','worker')),
  is_active INTEGER NOT NULL DEFAULT 1,
  added_by TEXT NOT NULL DEFAULT '',
  added_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  ended_at TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (engagement_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_pem_user ON partner_engagement_members(user_id, is_active);

CREATE TABLE IF NOT EXISTS partner_engagement_perms (
  engagement_id TEXT NOT NULL REFERENCES partner_engagements(engagement_id),
  perm_key TEXT NOT NULL REFERENCES partner_perm_defs(perm_key),
  level TEXT NOT NULL,
  updated_by TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (engagement_id, perm_key)
);
CREATE TABLE IF NOT EXISTS partner_perm_log (
  log_id INTEGER PRIMARY KEY AUTOINCREMENT,
  at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  engagement_id TEXT NOT NULL,
  perm_key TEXT NOT NULL,
  old_level TEXT NOT NULL DEFAULT '',
  new_level TEXT NOT NULL,
  changed_by TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL DEFAULT ''
);

-- E. 공유 / 기록 / 자료 / 접속로그
CREATE TABLE IF NOT EXISTS partner_shares (
  share_id INTEGER PRIMARY KEY AUTOINCREMENT,
  target_type TEXT NOT NULL CHECK (target_type IN ('work_task','as_receipt')),
  target_id TEXT NOT NULL,
  engagement_id TEXT NOT NULL REFERENCES partner_engagements(engagement_id),
  is_active INTEGER NOT NULL DEFAULT 1,
  shared_by_user_id TEXT NOT NULL DEFAULT '',
  shared_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  unshared_at TEXT NOT NULL DEFAULT '',
  UNIQUE (target_type, target_id, engagement_id)
);
CREATE INDEX IF NOT EXISTS idx_ps_eng ON partner_shares(engagement_id, is_active);
CREATE INDEX IF NOT EXISTS idx_ps_target ON partner_shares(target_type, target_id);

CREATE TABLE IF NOT EXISTS partner_notes (
  note_id INTEGER PRIMARY KEY AUTOINCREMENT,
  target_type TEXT NOT NULL CHECK (target_type IN ('work_task','as_receipt')),
  target_id TEXT NOT NULL,
  engagement_id TEXT NOT NULL REFERENCES partner_engagements(engagement_id),
  author_user_id TEXT NOT NULL,
  author_side TEXT NOT NULL CHECK (author_side IN ('partner','staff')),
  kind TEXT NOT NULL DEFAULT 'comment' CHECK (kind IN
    ('comment','progress','issue','done_request','done_confirm','reject','action_report','action_applied')),
  body TEXT NOT NULL DEFAULT '',
  progress_pct INTEGER,
  payload_json TEXT NOT NULL DEFAULT '',   -- 조치 보고: 방문일시·소요시간·원인·부품 등
  applied_ref_type TEXT NOT NULL DEFAULT '',  -- 'as_process'
  applied_ref_id TEXT NOT NULL DEFAULT '',
  applied_by TEXT NOT NULL DEFAULT '',
  applied_at TEXT NOT NULL DEFAULT '',
  is_deleted INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_pn_target ON partner_notes(target_type, target_id, created_at);

CREATE TABLE IF NOT EXISTS partner_engagement_files (
  file_id TEXT PRIMARY KEY,
  engagement_id TEXT NOT NULL REFERENCES partner_engagements(engagement_id),
  target_type TEXT NOT NULL DEFAULT '',
  target_id TEXT NOT NULL DEFAULT '',
  direction TEXT NOT NULL CHECK (direction IN ('to_partner','from_partner')),
  orig_name TEXT NOT NULL,
  stored_path TEXT NOT NULL,
  size_bytes INTEGER NOT NULL DEFAULT 0,
  mime TEXT NOT NULL DEFAULT '',
  uploaded_by_user_id TEXT NOT NULL DEFAULT '',
  uploaded_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  is_deleted INTEGER NOT NULL DEFAULT 0,
  deleted_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_pef_eng ON partner_engagement_files(engagement_id, is_deleted);

CREATE TABLE IF NOT EXISTS partner_access_log (
  log_id INTEGER PRIMARY KEY AUTOINCREMENT,
  at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  user_id TEXT NOT NULL DEFAULT '',
  partner_id TEXT NOT NULL DEFAULT '',
  engagement_id TEXT NOT NULL DEFAULT '',
  action TEXT NOT NULL CHECK (action IN ('login','logout','view','create','update','download','denied','locked')),
  target_type TEXT NOT NULL DEFAULT '',
  target_id TEXT NOT NULL DEFAULT '',
  ip TEXT NOT NULL DEFAULT '',
  detail TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_pal_user ON partner_access_log(user_id, at);

-- F. 무결성 트리거
CREATE TRIGGER IF NOT EXISTS trg_users_partner_ins BEFORE INSERT ON users
WHEN (NEW.account_kind='partner' AND TRIM(COALESCE(NEW.partner_id,''))='')
  OR (NEW.account_kind!='partner' AND TRIM(COALESCE(NEW.partner_id,''))!='')
BEGIN SELECT RAISE(ABORT,'users: account_kind 와 partner_id 규칙 위반'); END;

CREATE TRIGGER IF NOT EXISTS trg_users_partner_upd BEFORE UPDATE OF account_kind, partner_id ON users
WHEN (NEW.account_kind='partner' AND TRIM(COALESCE(NEW.partner_id,''))='')
  OR (NEW.account_kind!='partner' AND TRIM(COALESCE(NEW.partner_id,''))!='')
BEGIN SELECT RAISE(ABORT,'users: account_kind 와 partner_id 규칙 위반'); END;

CREATE TRIGGER IF NOT EXISTS trg_pe_link BEFORE INSERT ON partner_engagements
WHEN NOT EXISTS (SELECT 1 FROM partner_org_links
                 WHERE partner_id=NEW.partner_id AND org_id=NEW.org_id AND status='active')
BEGIN SELECT RAISE(ABORT,'협력 건: 조직과 협력사의 거래 관계가 없음'); END;

CREATE TRIGGER IF NOT EXISTS trg_pem_match BEFORE INSERT ON partner_engagement_members
WHEN (SELECT partner_id FROM partner_engagements WHERE engagement_id=NEW.engagement_id)
     IS NOT (SELECT partner_id FROM users WHERE user_id=NEW.user_id)
BEGIN SELECT RAISE(ABORT,'참여자: 협력사가 협력 건과 다름'); END;

CREATE TRIGGER IF NOT EXISTS trg_ps_org BEFORE INSERT ON partner_shares
WHEN (CASE NEW.target_type
        WHEN 'work_task'  THEN (SELECT COALESCE(org_id,'') FROM work_tasks  WHERE task_id=NEW.target_id)
        WHEN 'as_receipt' THEN (SELECT COALESCE(org_id,'') FROM as_receipts WHERE as_id=NEW.target_id)
      END)
     IS NOT (SELECT org_id FROM partner_engagements WHERE engagement_id=NEW.engagement_id)
BEGIN SELECT RAISE(ABORT,'공유: 대상과 협력 건의 조직이 다름'); END;

CREATE TRIGGER IF NOT EXISTS trg_pep_level_ins BEFORE INSERT ON partner_engagement_perms
WHEN NOT EXISTS (SELECT 1 FROM partner_perm_defs d
                 WHERE d.perm_key=NEW.perm_key AND (','||d.levels||',') LIKE '%,'||NEW.level||',%')
BEGIN SELECT RAISE(ABORT,'권한 단계가 정의에 없음'); END;

CREATE TRIGGER IF NOT EXISTS trg_pep_level_upd BEFORE UPDATE OF level ON partner_engagement_perms
WHEN NOT EXISTS (SELECT 1 FROM partner_perm_defs d
                 WHERE d.perm_key=NEW.perm_key AND (','||d.levels||',') LIKE '%,'||NEW.level||',%')
BEGIN SELECT RAISE(ABORT,'권한 단계가 정의에 없음'); END;

CREATE TRIGGER IF NOT EXISTS trg_ppti_level_ins BEFORE INSERT ON partner_perm_template_items
WHEN NOT EXISTS (SELECT 1 FROM partner_perm_defs d
                 WHERE d.perm_key=NEW.perm_key AND (','||d.levels||',') LIKE '%,'||NEW.level||',%')
BEGIN SELECT RAISE(ABORT,'권한 단계가 정의에 없음'); END;
```
- `as_receipts.as_id`, `work_tasks.task_id`, `customers.customer_id`, `users.user_id`, `orgs.org_id`의 **실제 컬럼 타입·이름을 확인**해 맞춘다.
- 이 마이그레이션이 만드는 데이터는 **권한 항목 정의(참조 데이터) 10행뿐**이다. 협력사·협력 건·템플릿·사용자 전환은 만들지 않는다.
- 롤백(`.down.sql`): 신규 테이블·트리거·인덱스 DROP, users 열은 기존 관례대로 "운영 롤백은 백업 복원" 주석.
