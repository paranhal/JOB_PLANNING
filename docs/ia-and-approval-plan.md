# 메뉴 구조(IA) · 결재 엔진 · 대리 결재 정책 · 모바일/태블릿 기준 (v0.3)

작성일: 2026-10-11 · 결정·미결 목록: **`claude/decisions-log.md`** (이 문서에는 결정 목록을 두지 않는다)
짝 문서: `claude/ui-design-system-cursor-spec.md`(디자인 시스템), `claude/leave-and-approval-spec.md`(**휴가·견적 결재 상세**)
범위: **설계·기획·스키마 제공까지.** 구현과 테스트는 Cursor가 하며, 테스트는 필수.
전제: 기획서 v2 §6의 섹션 구조와 **§6.3 URL 유지 정책**(기존 URL 불변, 신설 화면만 새 URL)을 따른다.
이 문서와 `leave-and-approval-spec.md`가 충돌하면 **그 문서를 따른다.**

변경 이력: v0.1 최초 / v0.2 결정 반영, 자기 결재 예외 / **v0.3 대리 결재 정책(관리 화면 체크박스) 추가, 견적 자기 결재 확정, 결정 목록을 `decisions-log.md`로 이관**

---

## 1. 메뉴 구조(IA) 개편안

### 1.1 현재 문제 (코드 확인)

- 사이드바 메뉴가 약 50개이고 전부 한 줄 나열이다. 영업·관리 섹션이 특히 길다.
- AS 메뉴가 역할에 따라 두 곳에 중복 정의되어 있다(`base.html`).
- 상단에 **조직 전환·시점 보기·스키마 경고** 배너가 색(보라/호박/파랑/슬레이트/빨강)을 달리해 쌓인다.
- 전사 시스템에 필요한 **결재·휴가** 자리가 없다.

### 1.2 제안 (섹션 이름과 URL은 최대한 유지, 추가·정리 중심)

```
[조직 스위처 ▾]            ← 현재 조직 배너를 대체 (전 조직 보기 등 상태는 칩으로 표시)
[검색 / 커맨드  Ctrl+K]

나의 업무   (기존 「홈」을 확장 · 로그인한 모든 사용자에게 노출)
 ├ 대시보드               /            (유지)
 ├ 결재  ▸ 결재할 문서 (n) /approvals/inbox    ★신설
 │         상신함          /approvals/sent     ★신설
 │         참조함          /approvals/cc       ★신설
 │         대리 결재 지정   /approvals/delegations  ★신설 (팀장만 · 정책 A가 켜져 있을 때)
 └ 휴가                   /leaves      ★신설  (내 휴가 현황 · 휴가 신청)

계획 · 실행 · 확인          (기존 §6.2 그대로. 메뉴 라벨/URL 변경 없음)

영업                        (기존 항목 유지. 견적 관리에 결재 상태 칩 표시)

기준정보                    (기존 유지. 접힘 기본)

관리  ▸ 기준·권한           코드관리 · 사용자/권한 · 역할 및 권한 · 소속 인원 · 전체 조직 · 휴무일 · **결재 설정 ★신설**
      ▸ 시스템 (접힘 기본)   데이터 · 시스템 정보 · 메일 보낸 함 · 알림 설정 · 영업 로그 · 테스터 통계
```

핵심 변경은 5가지다.

1. **「홈」 → 「나의 업무」**: 개인이 매일 여는 것(대시보드·결재·휴가)을 맨 위에 모은다.
2. **섹션 접기/펼치기**: 섹션 제목을 눌러 접는다. 접힘 상태는 사용자별로 기억(브라우저 저장은 편의 용도, 서버 의존 금지). 현재 화면이 속한 섹션은 자동 펼침.
3. **관리 섹션을 둘로 분리**하고 「시스템」은 기본 접힘. 「결재 설정」은 기준·권한 쪽에 둔다.
4. **배너 정리**: 조직 전환 → 사이드바 상단 스위처, 시점 보기(저장 불가 상태)는 **얇은 상단 띠 유지**(실수 방지), 스키마 경고는 관리자에게만.
5. **미결 건수 배지**: `결재할 문서 (n)`을 사이드바·모바일 하단 탭·헤더에 표시.

### 1.3 규칙·영향

- 용어는 기획서 §6.0을 따른다: 묶음 = **섹션**, 항목 = **메뉴**. 기획서 §6.2의 「홈」 섹션을 「나의 업무」로 고치고 §6.5 역할 표에 행 추가가 필요하다(기획서 개정은 Claude가 작성, 반영은 Cursor 규칙 `agent-planning`).
- 신설 URL: `/approvals/inbox`, `/approvals/sent`, `/approvals/cc`, `/approvals/delegations`, `/approvals/{id}`, `/leaves`, `/leaves/new`, `/leaves/{id}`, `/admin/approval-settings`.
- `nav.go` Active 키 추가: `approvals_inbox`, `approvals_sent`, `approvals_cc`, `approvals_delegations`, `leaves`, `approval_settings` (`base.html`의 `$a`와 값 동일해야 함).
- 「나의 업무」는 **모든 역할에 노출**한다(결정 D-10). 그 밖의 섹션을 기존 「준비 중」 역할에 어디까지 열지는 기획서 §6.5 개정 때 확정한다(Q-12).
- 팀장 여부는 역할이 아니라 직원 프로필(`employee_profiles.is_org_head`)로 판단한다.

### 1.4 IA 테스트 (필수)

- 역할별 사이드바 노출 테스트: 관리자 / 접수 / 기술 / 영업 / 열람 각각에 대해 메뉴 목록이 기대와 일치.
- **모든 역할에 「나의 업무」 노출**, 팀장이 아니면 `대리 결재 지정` 메뉴가 보이지 않음, 정책 A가 꺼져 있으면 팀장에게도 보이지 않음.
- 모든 사이드바 메뉴 URL이 200 (또는 권한 없을 때 정해진 응답).
- Active 키 ↔ 템플릿 `$a` 일치 테스트(`nav.go` 상수 누락 방지).
- 시점 보기에서 결재·휴가 **저장 UI 차단**.

---

## 2. 결재 — 공통 설계 (범용 엔진 1개)

### 2.1 원칙

- 견적서·휴가뿐 아니라 이후 다른 문서도 태울 수 있게 `(doc_type, doc_id)`로 대상을 참조하는 **하나의 결재 엔진**을 만든다. 기능마다 승인 컬럼을 늘리지 않는다.
- 상신 시점의 문서를 **스냅샷으로 고정**한다. 상신 후 원본 수정 차단, 반려 시 새 리비전으로 재상신.
- 결재 이력은 **삭제·수정 불가(append-only)**. 기존 `audit`/`auditlog` 구조와 연결 검토.
- 지표 원칙 유지: 결재 소요시간 등은 **분모 0이면 `—`**.
- 시점 보기·옵저버는 결재 불가.
- **검토 단계는 만들지 않는다**(D-13). 엔진은 단계가 늘어나도 동작하게 두되 기본 템플릿은 **작성 → 승인** 2단계.

### 2.2 상태 머신 (서버 공유 함수 한 곳)

```
draft ──상신──▶ in_progress ──마지막 결재자 승인──▶ approved
                  │  └─(반려)──▶ rejected ──재상신(새 리비전)──▶ in_progress
                  └─(기안자 회수)──▶ withdrawn

자기 결재(팀장 본인 신청): draft ──상신(같은 처리에서 self_approve)──▶ approved
```
단계(step) 상태: `waiting → pending → approved | rejected | skipped`
단계 유형: `approve`(결재) · `agree`(합의) · `cc`(참조, 결재 흐름에 영향 없음). 1차 범위는 **순차 결재 + 참조**, 합의·병렬은 2차.

### 2.2.1 자기 결재 (팀장 본인 신청 — 휴가·견적 공통, D-15)

- 승인자 규칙이 `org_head`로 해석된 결과가 **기안자 본인**이고 해당 문서 종류의 `head_self_approve=1`이면, 상신과 **같은 트랜잭션**에서 승인 처리한다. 이벤트는 `submit` → `self_approve`.
- 문서 종류별 부작용(휴가: 잔여 소진·`staff_leaves` 반영, 견적: 제출 가능 상태)도 **같은 트랜잭션**이며 하나라도 실패하면 전체 롤백.
- 일반 직원이 자기 자신을 승인자로 두는 것은 **금지**다. 예외는 위 조건 하나뿐.
- 감사: `self_approve` 이벤트 보존, 관리자 화면 필터 제공, 알림은 설정으로 선택(§2.5).
- 상세 규칙·화면·테스트는 `leave-and-approval-spec.md` §2.2~2.3, §10.

### 2.3 DB 스키마 초안 (Cursor 구현용, 마이그레이션 `056_*`)

기존 규칙(`NNN_name.sql` + `.down.sql`, SQLite 트리거 제약)을 따른다. 아래는 구조 초안이며 **확정 전 기획서 § 신설과 v2 부록 B 스타일의 제약 정의가 선행**된다. 휴가 관련 테이블(057~061)은 `leave-and-approval-spec.md` §8.

```sql
-- 결재 정책 (관리 > 결재 설정, 조직별) — 체크박스 값
CREATE TABLE approval_policies (
  org_id                     TEXT PRIMARY KEY,
  -- 팀장 부재 시 대리 결재 방식 (복수 선택 가능) §2.5
  deleg_by_head              INTEGER NOT NULL DEFAULT 1,   -- A 팀장이 직접 대리인 지정
  deleg_by_admin             INTEGER NOT NULL DEFAULT 1,   -- B 관리자가 대리인 지정
  deleg_with_leave           INTEGER NOT NULL DEFAULT 0,   -- C 팀장 휴가 신청 시 대리 결재자를 함께 지정
  admin_emergency_approve    INTEGER NOT NULL DEFAULT 0,   -- D 대리인이 없으면 관리자가 비상 승인
  -- 자기 결재 (D-15)
  self_approve_leave         INTEGER NOT NULL DEFAULT 1,   -- 휴가: 팀장 본인 신청 자기 결재 허용
  self_approve_quote         INTEGER NOT NULL DEFAULT 1,   -- 견적: 팀장 본인 작성 자기 결재 허용
  notify_admin_self_approve  INTEGER NOT NULL DEFAULT 0,   -- 자기 결재 발생 시 관리자 알림
  updated_by                 TEXT,
  updated_at                 DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 결재 템플릿(결재선 기본값). doc_type별, 조직별
CREATE TABLE approval_templates (
  template_id   TEXT PRIMARY KEY,
  org_id        TEXT NOT NULL,
  doc_type      TEXT NOT NULL,              -- 'quote' | 'leave' | ...
  name          TEXT NOT NULL,
  is_active     INTEGER NOT NULL DEFAULT 1,
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);
-- 자기 결재 허용 여부는 approval_policies.self_approve_<doc_type> 에서 읽는다(문서 종류 추가 시 컬럼 추가)
CREATE TABLE approval_template_steps (
  template_id   TEXT NOT NULL,
  step_no       INTEGER NOT NULL,
  step_type     TEXT NOT NULL DEFAULT 'approve',   -- approve | cc
  approver_rule TEXT NOT NULL,                     -- 'drafter' | 'org_head' | 'user:<id>' | 'role:<role>'
  PRIMARY KEY (template_id, step_no)
);
-- 기본 템플릿(휴가·견적 공통): step1 작성 = 'drafter', step2 승인 = 'org_head'

-- 결재 요청 (문서 1건당 리비전마다 1행)
CREATE TABLE approval_requests (
  request_id    TEXT PRIMARY KEY,
  org_id        TEXT NOT NULL,
  doc_type      TEXT NOT NULL,
  doc_id        TEXT NOT NULL,
  revision      INTEGER NOT NULL DEFAULT 1,
  drafter_id    TEXT NOT NULL,
  title         TEXT NOT NULL,
  status        TEXT NOT NULL DEFAULT 'draft',     -- draft|in_progress|approved|rejected|withdrawn
  snapshot_json TEXT,                              -- 상신 시점 문서 내용 고정
  submitted_at  DATETIME,
  completed_at  DATETIME,
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (doc_type, doc_id, revision)
);
CREATE INDEX idx_apr_status ON approval_requests(org_id, status);
CREATE INDEX idx_apr_doc ON approval_requests(doc_type, doc_id);

-- 결재 단계
CREATE TABLE approval_steps (
  step_id       TEXT PRIMARY KEY,
  request_id    TEXT NOT NULL REFERENCES approval_requests(request_id),
  step_no       INTEGER NOT NULL,
  step_type     TEXT NOT NULL DEFAULT 'approve',   -- approve | cc
  approver_id   TEXT NOT NULL,                     -- 원래 승인자(팀장)
  acted_by      TEXT,                              -- 대결 시 실제 처리자(대리인 또는 관리자)
  act_mode      TEXT,                              -- direct | self | delegate | admin_emergency
  status        TEXT NOT NULL DEFAULT 'waiting',   -- waiting|pending|approved|rejected|skipped
  comment       TEXT,
  signature_ref TEXT,                              -- 서명 이미지 참조
  acted_at      DATETIME,
  UNIQUE (request_id, step_no)
);
CREATE INDEX idx_aps_approver ON approval_steps(approver_id, status);

-- 대리 결재 지정 (§2.5)
CREATE TABLE approval_delegations (
  delegation_id TEXT PRIMARY KEY,
  org_id        TEXT NOT NULL,
  from_user_id  TEXT NOT NULL,                     -- 팀장
  to_user_id    TEXT NOT NULL,                     -- 대리인
  start_date    TEXT NOT NULL,
  end_date      TEXT NOT NULL,
  source        TEXT NOT NULL,                     -- head | admin | leave  (정책 A | B | C 로 만들어진 지정)
  leave_req_id  TEXT,                              -- source='leave' 일 때 연결(휴가 취소 시 함께 종료)
  created_by    TEXT NOT NULL,
  revoked_at    DATETIME,                          -- 조기 종료 시각(NULL이면 유효)
  revoked_by    TEXT,
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_deleg_from ON approval_delegations(from_user_id, start_date, end_date);

-- 이력 (append-only: UPDATE/DELETE 차단 트리거 필수)
CREATE TABLE approval_events (
  event_id      TEXT PRIMARY KEY,
  request_id    TEXT NOT NULL,
  event_type    TEXT NOT NULL,   -- submit|approve|self_approve|delegate_approve|admin_emergency_approve|reject|withdraw|resubmit|comment|auto_apply
  actor_id      TEXT NOT NULL,
  detail_json   TEXT,
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

필수 제약(트리거로 구현, SQLite 제약 때문):
1. `approval_events` UPDATE/DELETE 금지
2. `status='in_progress'`인 요청은 `approval_steps`가 1개 이상 `approve` 유형
3. 상신 후(`status<>'draft'`) `snapshot_json` 변경 금지
4. 같은 요청에서 `pending` 단계는 동시에 1개(순차 결재)
5. **승인 단계의 승인자는 기안자 본인일 수 없다. 예외: 규칙이 `org_head`이고 기안자가 그 조직 팀장이며 해당 문서 종류의 자기 결재 정책이 켜진 경우(`act_mode='self'`), 이때 `self_approve` 이벤트 필수**
6. 조직 경계: 결재자는 요청과 같은 org 소속이거나 명시 허용
7. 소속 조직에 팀장이 없어 `org_head`를 해석할 수 없으면 **상신 불가**(명확한 오류)
8. **대리 결재 지정은 같은 팀장에 대해 기간이 겹치는 유효 지정이 둘 이상 있을 수 없다**(`revoked_at IS NULL` 기준). 대리인은 같은 조직 활성 직원이고 팀장 본인이 아니다
9. `acted_by ≠ approver_id`이면 `act_mode`는 `delegate` 또는 `admin_emergency`여야 하고, 대응하는 이벤트가 반드시 있어야 한다

### 2.4 결재 필수 테스트 (Cursor 구현 시)

- 상태 머신 표 기반 테스트: 허용/불허 전이 전부(예: approved에서 reject 불가, withdrawn 후 승인 불가)
- 순차 결재: 앞 단계 미승인 시 뒤 단계가 처리 불가
- **자기 결재**: 팀장 본인 상신(휴가·견적 둘 다) 시 단일 트랜잭션으로 `approved`, 이벤트 순서 `submit → self_approve`, 부작용 실패 시 전체 롤백
- **일반 직원의 자기 승인 시도 차단**, 해당 문서 종류의 자기 결재 정책을 끄면 팀장 본인 상신 차단
- 팀장 미지정 조직 상신 차단
- 반려 → 재상신 시 리비전 증가, 이전 리비전 이력 보존
- 스냅샷 고정: 상신 후 원본 문서 수정 시 결재 화면은 스냅샷을 보여주고, 원본 수정 자체가 정책대로 차단
- 권한: 기안자/결재자/참조자/타인/옵저버/시점 보기 각각의 열람·처리 가능 범위
- append-only 트리거 위반 시도 실패
- 알림: 결재 요청·결과 알림 발송 큐에 적재(`notify`/`mailer` 경로)
- 마이그레이션 up/down 왕복 테스트
- 대리 결재 관련 테스트는 §2.5.5

### 2.5 대리 결재 정책 — 팀장이 자리에 없을 때 (D-16)

팀원 신청은 팀장이 승인해야 하므로, 팀장 부재 시 처리 방식을 **관리 > 결재 설정**에서 **체크박스로 선택**한다. 여러 방식을 동시에 켤 수 있다.

#### 2.5.1 관리 화면 「결재 설정」 (`/admin/approval-settings`, `approval.admin` 권한, 조직별)

```
팀장 부재 시 대리 결재 방식 (복수 선택 가능)
 [✔] A. 팀장이 직접 대리인을 지정한다          팀장이 「대리 결재 지정」에서 기간과 대리인을 정함
 [✔] B. 관리자가 대리인을 지정한다              관리자가 팀장 대신 기간과 대리인을 정함
 [ ] C. 팀장이 휴가를 신청할 때 대리 결재자를 함께 지정한다   휴가 기간이 대리 기간이 됨(승인 시 자동 생성)
 [ ] D. 대리인이 없으면 관리자가 직접 승인할 수 있다 (비상 승인, 사유 필수)

자기 결재 (팀장 본인이 신청한 문서)
 [✔] 휴가 신청은 팀장 본인이 승인한다
 [✔] 견적서는 팀장 본인이 승인한다
 [ ] 자기 결재가 일어나면 관리자에게 알린다
```
- 기본값은 위 그림과 같다(A·B 켬, C·D 끔, 자기 결재 켬). 아무것도 선택하지 않으면 팀장이 돌아올 때까지 **대기**하며, 화면에 경고를 표시한다.
- 변경 시 **누가·언제·무엇을** 바꿨는지 감사 로그에 남긴다.
- 체크를 해제해도 **이미 만들어진 지정은 종료일까지 유지**하고 신규 지정만 막는다(Q-05 임시 가정). 지정 목록에서 관리자가 개별 종료할 수 있다.

#### 2.5.2 방식별 동작

| 방식 | 화면/동작 | 만들어지는 지정 |
|------|-----------|-----------------|
| **A** 팀장 직접 지정 | 팀장 전용 「대리 결재 지정」 화면: 대리인(같은 조직 직원) + 시작일 + 종료일 | `source='head'`, `created_by`=팀장 |
| **B** 관리자 지정 | 관리 화면 지정 목록에서 팀장 선택 + 대리인 + 기간 | `source='admin'`, `created_by`=관리자 |
| **C** 휴가 연동 | **팀장이 휴가를 신청**할 때만 「대리 결재자」 입력란이 나타남(필수). 휴가가 최종 승인되면 휴가 기간의 지정이 자동 생성되고, 휴가를 취소하면 함께 종료 | `source='leave'`, `leave_req_id` 연결 |
| **D** 관리자 비상 승인 | 유효한 대리인이 없고 팀장이 처리하지 않은 문서에 한해 관리자에게 「비상 승인」 버튼. **사유 필수**, 문서에 「관리자 대결」 표시 | 지정 없음, `act_mode='admin_emergency'` |

#### 2.5.3 승인자 결정 규칙 (한 곳의 서버 함수)

문서의 승인 단계가 `pending`일 때 처리 가능한 사람은 다음 순서로 판단한다.
1. **팀장 본인** (항상 가능)
2. **오늘 날짜에 유효한 대리인** — `approval_delegations`에서 `start_date ≤ 오늘 ≤ end_date`, `revoked_at IS NULL`, 팀장당 최대 1명(제약 8)
3. **관리자 비상 승인** — 정책 D가 켜져 있고 2번이 없을 때만
- 대리인은 **자기 자신이 올린 문서를 대결로 승인할 수 없다**(자기 결재 예외는 팀장 본인뿐). 이 경우 팀장 복귀·관리자 비상 승인(D)을 기다린다.
- 대리인에게는 결재함에 **「OOO 팀장 대리」** 라벨로 해당 문서가 보인다. 지정 기간이 끝나면 즉시 보이지 않는다.
- 이미 승인된 문서에는 영향이 없고, 진행 중인 문서는 기간이 끝나는 순간부터 대리인이 처리할 수 없다.
- 이력·출력: 단계의 `approver_id`는 팀장, `acted_by`는 대리인, `act_mode='delegate'`. 출력 양식 승인란에는 **실제 처리자의 서명과 「(대결)」 표기**를 넣는 것을 가정한다(Q-04 확인 필요).
- 알림: 지정 시 대리인에게, 대결 처리 시 팀장에게 알림.

#### 2.5.4 화면 요구
- 팀장 「대리 결재 지정」: 현재 유효/예정 지정 목록, 새 지정, 조기 종료. **기간이 겹치면 저장 전에 안내하고 막는다.**
- 관리자 지정 목록: 팀장별 현재 상태(대리인 있음/없음), 정책 켜짐 여부 표시.
- 결재 상세: 대결로 처리된 단계에 `대리 결재 · 처리자 OOO` 배지, 비상 승인에는 `관리자 대결` 배지.
- 모바일(§4)에서도 대리 승인이 가능해야 한다(결재함 → 승인).

#### 2.5.5 대리 결재 필수 테스트
- **정책 조합 표 기반 테스트**: A/B/C/D를 켜고 끄는 조합별로 허용·거부 동작 확인(예: A 끔 → 팀장 지정 API 거부, B 끔 → 관리자 지정 거부, C 켬 → 팀장 휴가 신청에 대리 결재자 필수, C 끔 → 입력란 없음, D 켬/끔 → 비상 승인 가능/불가, 모두 끔 → 팀장 부재 중 문서 대기)
- 지정 기간 경계: 시작일 당일·종료일 당일 유효, 하루 전·후 무효, 조기 종료 즉시 반영
- **겹치는 지정 차단**(같은 팀장), 대리인 자격(같은 조직, 활성, 팀장 본인 아님) 위반 차단
- 대리인이 **자기 문서를 대결 승인 불가**, 팀장 본인 신청은 여전히 자기 결재
- C: 팀장 휴가 최종 승인 시 지정이 휴가 기간으로 생성, 휴가 취소·반려·회수 시 생성 안 됨/종료, 대리 결재자 미입력 시 상신 불가
- D: 사유 필수, 유효 대리인이 있으면 비상 승인 불가, `admin_emergency_approve` 이벤트와 배지
- 이력에 `acted_by`/`act_mode` 기록, 승인자 변경(팀장 교체) 시 기존 지정 처리
- 정책 변경은 `approval.admin`만 가능, 변경 감사 로그 기록, 해제 시 기존 지정 유지(Q-05 가정) 확인
- 시점 보기·옵저버는 지정·대결 불가
- 알림: 지정/대결 발생 시 대상자에게 큐 적재

---

## 3. 첫 적용 대상 2개 (요약 — 상세는 `leave-and-approval-spec.md`)

### 3.1 견적서 — 「제출(발송) 전 내부 결재」

현재 견적 상태: `draft(작성중) → sent(제출) → won/lost/expired/cancelled` (`model/quote.go`, 칸반 열 구성).

- 결재는 **모든 견적이 대상**이며(D-12) **`draft → sent` 사이**에 끼운다. `draft`에서 「결재 상신」 → 승인 완료 시에만 「제출」 가능.
- 결재선: **작성(기안자) → 승인(팀장)**. **팀장이 작성자이면 자기 결재**(D-15, 금액 예외 없음).
- 새 견적 상태를 늘리지 않는다. 결재 상태는 `approval_requests`에서 가져와 **칩으로 병기**(예: `작성중 · 결재중`). 칸반(§35)의 열 정의는 그대로 둔다.
- 견적 금액·품목이 바뀌면 안 되므로 **상신 시 스냅샷**(품목 라인·노임·합계) 필수. 상신 후 수정 불가, 반려 후 재상신 시 리비전 증가.
- 견적 출력물(엑셀/PDF)에 결재란을 넣을지는 Q-11.
- 결재 승인/반려는 견적 상세의 **사이드 패널**에서 처리.

### 3.2 휴가 신청서 — 「신청 → 결재 → 휴가 자동 반영」

- 직원이 `/leaves/new`에서 신청 → 결재 → **최종 승인 시 기존 `staff_leaves` 행을 자동 생성**(업무 배치 로직은 그대로). 연차 잔여일 원장, 양식(근태허가원 HWPX) 출력, 엑셀 일괄 등록은 모두 `leave-and-approval-spec.md`에 있다.

### 3.3 구현 순서

1. 기획서 v2에 「결재」「휴가 신청·연차」 절 신설 + §4.2 지표 입력 지점 표 갱신 (Claude)
2. 순수 계산 함수·HWPX 템플릿(DB·화면 무관) → 마이그레이션 → 결재 엔진(대리 결재 포함) → 화면 (Cursor + 테스트, 휴가 명세 §11)
3. **휴가 신청 → staff_leaves 자동 반영** (파일럿, 범위가 작음)
4. 견적서 결재 연동
5. 알림·출력(PDF/HWPX)·서명

---

## 4. 모바일·태블릿 기준 (반응형·터치)

### 4.1 현재 상태 점검 (코드 확인)

- 이미 있는 것: viewport 메타, 사이드바 `lg` 미만 오버레이, 표 정렬 모바일 셀렉트(`md`), 칸반 가로 스크롤, 서명 캔버스의 `touchstart` 처리.
- **빠져 있는 것**: ① 터치 환경 대응 규칙이 전혀 없음(`pointer: coarse` 0건) ② 입력 글자 크기가 작아 **iPhone에서 입력창을 누르면 화면이 확대**되는 문제 가능(Safari는 16px 미만 입력창에 자동 확대) ③ `viewport-fit=cover` 없음(노치/홈바 영역) ④ 칸반 드래그가 터치에서 동작하는지 불분명 ⑤ 태블릿(세로 768~1023px)에 대한 별도 기준 없음.

Linear 풍의 **조밀한 행(32px)과 13px 글자는 데스크톱 기준**이다. 터치 기기에서는 그대로 쓰면 누르기 어렵다. 그래서 아래 규칙으로 **기기 특성에 따라 밀도를 바꾼다.**

### 4.2 기기 구분과 프레임

| 구분 | 폭 | Tailwind | 레이아웃 |
|------|-----|----------|----------|
| 폰 | ~639 | 기본 | 1열, 사이드바는 드로어, 피크 패널·모달은 **전체 화면 시트** |
| 폰 가로 / 작은 태블릿 | 640~767 | `sm` | 폰과 동일 구조, 여백만 확대 |
| **태블릿 세로** (iPad 등) | 768~1023 | `md` | 사이드바 드로어, 2열 폼, 피크 패널은 **우측 오버레이(폭 ~480px)** |
| **태블릿 가로 / 노트북** | 1024~1279 | `lg` | 사이드바 상시 표시, 피크 패널은 오른쪽 분할 |
| 데스크톱 | 1280~ | `xl` | 위와 동일, 여백·표 폭 확대 |

Figma 프레임: **390(폰) · 820(태블릿 세로) · 1180(태블릿 가로) · 1440(데스크톱)** 4종.

### 4.3 터치 규칙 (CSS는 기기 폭이 아니라 **입력 방식**으로 분기)

```css
@media (pointer: coarse) {
  .btn, .select, .input, .textarea { min-height: 44px; }
  .input, .select, .textarea { font-size: 16px; }      /* iOS 입력 시 자동 확대 방지 */
  .list-row { min-height: 44px; }
  .sidebar-item { min-height: 44px; }
  .menu-item { min-height: 44px; }
  .kbd { display: none; }                               /* 단축키 칩은 터치에서 숨김 */
}
```
- 이렇게 하면 **iPad에 마우스를 붙이면 조밀, 손가락만 쓰면 넉넉**하게 자동 적용된다.
- `base.html` viewport: `width=device-width, initial-scale=1, viewport-fit=cover`. 하단 고정 바·시트는 `env(safe-area-inset-bottom)` 패딩.
- **hover에만 보이는 버튼 금지.** 터치에는 hover가 없으므로 액션은 항상 보이거나 「⋯」 메뉴에 둔다.
- **드래그 의존 금지.** 칸반 카드 이동은 드래그와 함께 **「상태 변경」 메뉴**를 반드시 제공(모바일·터치 대체 수단).
- 커맨드 팔레트·단축키는 데스크톱 전용. 터치에서는 헤더의 **검색 아이콘 → 같은 팔레트를 전체 화면**으로 연다.

### 4.4 컴포넌트별 모바일 규칙

| 대상 | 폰 | 태블릿 |
|------|-----|--------|
| 목록 | 표 대신 **카드형 행**(제목 + 상태 + 보조 2개). 가로 스크롤 표는 정말 넓은 데이터만, 첫 열 고정 | 표 유지 가능, 열 우선순위로 숨김 |
| 피크 패널 | **전체 화면 시트**(뒤로 가기 버튼, URL 유지) | 우측 오버레이 480px |
| 모달 | 전체 화면 시트, 하단 고정 액션 바 | 중앙 모달 |
| 폼 | 1열, 하단 고정 「저장」 바 | 2열 허용 |
| 견적 라인 편집 | 라인 1개 = 카드 1장 | 표 |
| 사이드바 | 드로어 + 헤더에 햄버거와 **결재 미결 배지** | 동일 |
| **하단 탭 바 (도입 확정)** | `대시보드 · 내 업무 · 결재 · 휴가 · 더보기`, 결재에 미결 건수 배지, 높이 56px + safe-area | 없음(사이드바 사용) |
| 차트 | 가로 스크롤 금지, 범례 아래로, 터치 툴팁 | 동일 |

하단 탭 바는 폭 `lg` 미만 **폰 구간**에서만 표시하고, 입력 폼 화면에서는 키보드와 하단 고정 「저장」 바와 겹치지 않게 **탭 바를 숨긴다**. 「더보기」는 사이드바 드로어를 연다.

### 4.5 결재·휴가의 모바일 (가장 중요한 사용 장면)

- 결재자는 **폰에서 승인**하는 경우가 많다. 결재 상세는 폰에서 `문서 요약 → 결재선 스테퍼 → 의견 → (고정 하단) 승인/반려`.
- 승인/반려 버튼은 하단 고정, 높이 44px 이상, 오터치 방지를 위해 **반려는 사유 입력 후 확정**.
- 휴가 신청은 폰에서 한 화면에 끝나게: 종류 → 기간(달력 선택) → 사유 → 제출.
- 서명은 터치/펜슬(Apple Pencil) 입력을 지원: 기존 `touchstart` 방식 대신 **Pointer Events**(`pointerdown/move/up` + `touch-action: none`)로 통일 권장.

### 4.6 반응형 필수 테스트

- **T8a 정적 검사**: 빌드된 `tailwind.css`에 `@media (pointer: coarse)` 규칙과 `.input` 16px 규칙이 존재.
- **T8b 템플릿 검사**: `base.html` viewport 메타에 `viewport-fit=cover` 포함, 새 템플릿에 hover 전용 액션(`group-hover:`만으로 노출되는 버튼) 금지 린트(T1에 추가).
- **T8c 렌더 스모크**: 주요 화면이 HX-Request/일반 요청 모두 정상 (피크 패널 직접 접근 폴백 포함).
- **T8d 하단 탭 바**: 폰 구간에서만 렌더, 결재 미결 배지 값이 서버 값과 일치, 폼 화면에서는 탭 바가 숨겨짐.
- **T4 확장 (개발기 전용, Playwright)**: 390 / 820 / 1180 / 1440 + **터치 에뮬레이션**으로 대표 화면 열기 → 가로 스크롤 발생 여부(`scrollWidth <= innerWidth`), 버튼 최소 크기(44px) 검사, 입력창 포커스 시 확대 없는지(글자 크기 16px 이상) 확인.
- **칸반 터치 대체 수단** 존재 테스트: 칸반 카드에 상태 변경 메뉴 요소가 렌더되는지.
- 결재·휴가 신청 화면, **대리 결재 지정·대결 승인**은 **390px에서 필수 동작 시나리오**(상신 → 승인) 수동 검수 항목에 포함.

### 4.7 메인 문서(v0.3) 대비 보완 사항

디자인 시스템 문서에는 390/1280 프레임과 `lg` 오버레이 정도만 있었다. **이 §4가 보완 기준**이며, 디자인 시스템 문서 §4(Figma 규칙)의 프레임 수는 **4종(390/820/1180/1440)** 으로, §3.1 컴포넌트에 `pointer: coarse` 규칙과 **하단 탭 바**를, §6 테스트에 T8을 추가해 다음 개정에서 합친다.

---

미결 질문은 `claude/decisions-log.md` §2를 본다.
