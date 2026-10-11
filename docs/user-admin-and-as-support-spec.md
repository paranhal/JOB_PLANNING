# 사용자·계정 관리 개선 + 영업 대시보드 + AS 지원업무 명세 (v0.1)

작성일: 2026-10-11 · 결정·미결 목록: **`claude/decisions-log.md`** (D-30~D-38)
범위: 기획·DB 스키마·테스트 기준. **구현은 Cursor, 테스트 필수.**
기존 코드 확인 결과(사실)
- 관리 섹션 잠금: `isAdminSectionPath`가 `/users`, `/codes`, `/admin/*`를 잠금 대상으로 보고, 면제(`skipsAdminSectionUnlock`)는 읽기 전용·테스트 계정뿐 → vsadmin·org_admin도 비밀번호를 묻는다.
- 계정 삭제: `POST /users/:id/delete`가 있고(비젼관리자 비밀번호 + 담당 업무 0건) **`DELETE FROM users` 하드 삭제**. 화면에는 버튼이 없다.
- `users`를 참조하는 외래키 없음. 업무 테이블에 담당자가 **이름 문자열 + user_id** 로 따로 저장된다 → 하드 삭제 시 고아 기록.
- `UserRenameUsername`(아이디 변경)이 있다 → 옛 아이디가 비면 재사용될 수 있다.
- AS 하부업무 `as_work_items`(work_kind = `confirm`·`revisit`)가 있고, 행정/지원 `work_tasks`에는 AS 연결 컬럼이 없다. `ToAdminWork`는 AS를 닫는 「이관」이다.
- 마이그레이션 최신은 055, 결재·휴가 설계가 056~061을 쓴다 → **이 문서의 마이그레이션은 062부터**(구현 순서에 따라 Cursor가 번호 조정).

---

## 1. 영업 역할 첫 화면 (D-30)

- `GET /` 에서 `NormalizeRole(role) == sales` 이면 영업 대시보드(`/sales/dashboard`)로 **302**. 단 `?home=tech` 가 있으면 기존 대시보드를 그대로 보여 준다(영업 대시보드 상단에 「기술 대시보드 보기」 링크, 기술 대시보드 상단에 「영업 대시보드로」 링크).
- 영업 대시보드 접근 권한(`sales.view` 등)이 없는 영업 계정은 이동하지 않는다(권한 없는 화면으로 보내지 않음).
- vsadmin·org_admin·기술·지원 등 다른 역할은 현재 동작 그대로.
- 로그인 직후 팝업(손봐야 할 업무)은 이동 후 화면에서도 동일하게 뜬다(이동 때문에 팝업이 사라지지 않게).
- 테스트: 영업 로그인 → `/` 리다이렉트, `?home=tech` 예외, 영업이지만 권한 없음, 타 역할 불변, HTMX 요청(`HX-Request`)에서의 동작, 리다이렉트 루프 없음, 팝업 표시.

## 2. 사용자·계정 관리

### 2.1 권한·잠금 (D-31, D-32)
| 역할 | 관리 섹션 잠금 | 사용자 목록 | 직원 추가 | 삭제·복구·삭제내역·승인 |
|------|---------------|------------|----------|------------------------|
| vsadmin | **묻지 않음** | 전체 | 직접 등록(+요청 승인) | **가능(전용)** |
| org_admin | **묻지 않음** | **자기 조직만** | **등록 요청**만 | 불가 |
| 그 외 | 접근 불가(기존) | — | — | — |

- 잠금 면제 범위는 **`/users`·`/codes` 중 사용자 관리 관련 경로**와 org_admin이 쓰는 관리 화면. 다른 `/admin/*`의 잠금 정책이 바뀌는 범위는 Cursor가 목록으로 보고하고, 임의로 넓히지 않는다(미확정이면 질문).
- 조직 범위는 **서버 쿼리에서 필터**한다(템플릿에서 숨기지 않음). 다른 조직 사용자의 `/users/:id/*` 직접 접근은 403/404. 조직 전환 기능(`canSwitchOrg`)이 있는 역할만 조직을 바꿀 수 있다.
- 기존 **비밀번호 변경·권한 수정·서명** 등 org_admin이 현재 쓰는 기능은 동작을 유지하되 조직 범위 필터를 적용한다. (영향 점검 후 보고)
- 잠금 해제 시도 로그(`adminUnlock.LogAttempt`)는 유지(면제 계정은 시도 자체가 없음).

### 2.2 등록 요청 → vsadmin 승인 (D-33)
```sql
-- 062
CREATE TABLE account_requests (
  request_id     TEXT PRIMARY KEY,
  org_id         TEXT NOT NULL,
  requested_by   TEXT NOT NULL,                -- org_admin user_id
  full_name      TEXT NOT NULL,
  username       TEXT NOT NULL,                -- 희망 아이디
  role           TEXT NOT NULL,                -- sales | tech | support 등 일반 역할만
  mobile         TEXT,
  reason         TEXT,
  status         TEXT NOT NULL DEFAULT 'pending',   -- pending | approved | rejected | cancelled
  restore_user_id TEXT,                        -- 삭제 계정과 일치해 「복구」로 처리한 경우
  decided_by     TEXT,
  decided_at     DATETIME,
  decision_note  TEXT,                         -- 반려 사유 필수
  created_user_id TEXT,                        -- 승인으로 생성된 user_id
  created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
);
```
- org_admin 화면: 「직원 등록 요청」 폼(비밀번호 입력란 **없음**), 내 요청 목록·상태, 대기 중 요청 취소.
- 요청 가능한 역할은 **일반 역할만**. `org_admin`·`vision_admin` 요청은 서버에서 거부(권한 상승 방지). 조직은 요청자의 조직으로 고정.
- 아이디 검사: 요청 시와 승인 시 **두 번**(§2.4 예약 테이블 기준).
- vsadmin 화면: 「등록 요청」 목록 + 사이드바 배지 + **로그인 팝업 「손봐야 할 업무」에 「계정 등록 요청 N건」 섹션 추가**(기존 `LoginHelps` 패턴 재사용). 승인 / 반려(사유 필수) / (이름·연락처가 삭제된 계정과 같으면) **복구로 승인** 안내.
- 승인 시: 계정 생성(`is_active=1`), **임시 비밀번호 무작위 생성 → 해시만 저장 → 승인 결과 화면에 1회만 표시**(새로고침·재조회 불가, 로그에 평문 금지), `must_change_password=1`(첫 로그인 변경 강제 — 기존 `ProfileDone` 흐름 확인 후 재사용 또는 컬럼 추가). 임시 비밀번호는 vsadmin이 요청자에게 직접 전달한다(메일 연동 없음).
- 감사 로그: 요청·승인·반려·취소.

### 2.3 소프트 삭제 · 삭제 내역 · 복구 (D-34, D-35)
```sql
-- 063
ALTER TABLE users ADD COLUMN deleted_at     DATETIME;
ALTER TABLE users ADD COLUMN deleted_by     TEXT;
ALTER TABLE users ADD COLUMN delete_reason  TEXT;
ALTER TABLE users ADD COLUMN delete_snapshot TEXT;      -- 삭제 당시 정보(JSON, 비밀번호 해시 제외)
ALTER TABLE users ADD COLUMN must_change_password INTEGER NOT NULL DEFAULT 0;   -- 이미 있으면 생략
CREATE TABLE user_lifecycle_events (                     -- append-only
  event_id   TEXT PRIMARY KEY,
  user_id    TEXT NOT NULL,
  kind       TEXT NOT NULL,        -- created | deactivated | reactivated | deleted | restored | renamed
  actor_id   TEXT,
  note       TEXT,
  detail     TEXT,                 -- JSON (예: 복구 시 근속 처리 모드, 이전/이후 조직·역할)
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```
- **하드 삭제 제거**: `UserRepo.Delete`의 `DELETE FROM users`를 없애고 소프트 삭제 메서드로 교체. 소스 전체에서 `DELETE FROM users`가 없음을 확인하는 **정적 테스트**를 추가한다.
- 삭제 = `is_active=0` + `deleted_at` 설정 + 로그인 불가 + 사용자 목록·담당자 선택지·팔레트 검색에서 제외. 진행 중 세션(JWT)은 다음 요청에서 차단(활성·삭제 확인 미들웨어 점검).
- **삭제 전 점검(차단/이관)**
  1. 담당 업무가 남아 있으면 **이관 대상자를 골라 일괄 이관**(AS·하부업무·정기점검·행정/지원·영업 등 담당 컬럼 전체). 이관 대상자는 같은 조직 활성 직원.
  2. 팀장(`is_org_head`)이면 후임 지정 전에는 삭제 불가(결재 설계와 연결, 결재 도입 전에는 해당 항목 생략).
  3. 진행 중 결재·휴가 신청이 있으면 삭제 불가(결재 도입 후 적용).
  4. vsadmin 비밀번호 재확인 + 사유 입력 필수.
- 삭제된 사용자의 기록 표시: 이름은 그대로, 필요한 곳에 「(삭제됨)」 표기. 업무·결재·연차 이력은 **삭제하지 않고 `user_id` 기준으로 보존**.
- **삭제 내역 화면**(vsadmin 전용, `/users/deleted`): 삭제일시·삭제자·사유·삭제 당시 소속/역할·**복구** 버튼. 검색·기간 필터.
- 개인정보(휴대폰 등) 비식별화는 이번 범위에서 만들지 않는다(보존 정책은 사내 규정 확인 후 별도 과제).

### 2.4 아이디 영구 유일성 (D-34)
```sql
-- 063 (계속)
CREATE TABLE username_reservations (
  username_norm TEXT PRIMARY KEY,      -- 정규화: 앞뒤 공백 제거 + 유니코드 NFC + 소문자
  user_id       TEXT NOT NULL,
  reason        TEXT NOT NULL,         -- current | renamed_from | deleted
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);
```
- **모든 아이디**(현재, 이름 변경 전, 삭제된 것)가 이 표에 남는다. 새 아이디·아이디 변경은 정규화 값이 이 표에 있으면 **거부**(본인의 현재 아이디는 제외).
- 기존 사용자 아이디를 마이그레이션에서 채우고, 정규화 결과가 겹치는 기존 데이터가 있으면 **중단하지 말고 목록으로 보고**(자동 변경 금지).
- 아이디 변경(`UserRenameUsername`)도 옛 아이디를 `renamed_from`으로 예약한다.
- 사용자에게 보이는 오류 문구는 「이미 사용했거나 삭제된 아이디입니다」(삭제 여부를 제3자에게 노출하지 않는 일반 문구, vsadmin 화면에서만 어떤 계정인지 표시).

### 2.5 재입사 = 삭제 계정 복구, 근속은 새로 시작 (D-36)
- 같은 사람이 돌아오면 새 계정을 만들지 않고 vsadmin이 삭제 내역에서 **복구**한다(같은 아이디·같은 `user_id` → 이전 업무·결재 기록이 이어짐).
- 복구 시: 임시 비밀번호 재발급(1회 표시, 변경 강제), **역할·조직 재지정**(이전 권한 자동 복원 금지), `user_lifecycle_events`에 `restored` 기록(복구 사유 포함).
- 등록 요청의 이름·휴대폰이 삭제된 계정과 같으면 승인 화면에 「삭제된 계정과 일치합니다. 복구하시겠습니까?」 표시(복구 승인 시 `restore_user_id` 기록).
- **연차: 근속은 새로 시작**(D-36). 복구 시 `employee_profiles.hire_date` = 재입사일로 갱신하고, 이전 입사 이력은 이벤트의 `detail`(이전 입사일·퇴사일)로 보존한다. 연차 원장은 **삭제하지 않는다**. 퇴사 전 미사용 부여분은 **복구 시점에 `expire` 소진 기록으로 닫고**(원장 append-only 유지), 새 입사일 기준으로 새로 발생한다. 이 방식이 회사 규정(퇴사 시 정산 등)과 맞는지는 확인이 필요하다(임시 가정, Q-19).
- 동명이인 대비로 아이디와 별개인 **사번** 컬럼(`employee_profiles.employee_no`, 선택)을 둘 수 있게 설계만 해 둔다(이번 구현 필수 아님).

### 2.6 필수 테스트 (Go)
- 잠금: vsadmin·org_admin은 `/users` 접근 시 `/admin/unlock`으로 가지 않음 / 그 외 역할 기존 동작 / 읽기전용·테스트 계정 기존 동작
- 조직 범위: org_admin 목록에 타 조직 없음, 타 조직 `/users/:id/*` 접근 403·404, 조직 전환 불가
- 등록 요청: 비밀번호 입력 없이 생성 / 일반 역할만 / `org_admin`·`vision_admin` 요청 거부 / 조직 고정 / 승인 시 계정 생성·임시 비밀번호 1회 표시·재조회 불가·평문 미기록 / 반려 사유 필수 / 아이디 중복(요청 시·승인 시) / 승인 후 첫 로그인 변경 강제 / org_admin의 승인 시도 403
- 소프트 삭제: vsadmin만 가능 / org_admin 403 / 담당 업무 이관 후 삭제 / 사유·비밀번호 재확인 필수 / 삭제 후 로그인 불가·진행 중 세션 차단 / 목록·담당자 선택지·팔레트에서 제외 / 이전 기록의 이름 표시 / **`DELETE FROM users` 없음(정적 테스트)**
- 삭제 내역·복구: 목록 정확성 / 복구 시 같은 `user_id`·이전 기록 연결 / 역할·권한 자동 복원 안 됨 / 임시 비밀번호 재발급 / 복구 이벤트 기록
- 아이디 예약: 삭제된 아이디로 신규 등록 거부 / 옛 아이디(이름 변경 전) 재사용 거부 / 대소문자·공백·유니코드 정규화 우회 불가 / 본인 현재 아이디는 통과 / 마이그레이션 시 기존 충돌 목록 보고
- 마이그레이션 up/down 왕복, `InitDB` 재실행 멱등

---

## 3. AS 지원업무 — 제조사 입고·회수 등 (D-37, D-38)

### 3.1 방향
AS를 **열어 둔 채** 직접 조치가 아닌 지원업무를 붙인다. 기존 하부업무(`as_work_items`)에 **지원 유형을 추가**한다. AS 번호 체계(AS번호-NN), 담당자, 예정일, 완료, 일일 업무·미배정 팝업 연동을 그대로 쓴다. `ToAdminWork`(AS를 닫는 이관)와는 별개다.

### 3.2 유형과 데이터
| work_kind | support_type | 라벨 |
|-----------|--------------|------|
| `support` | `factory_inbound` | 제조사 입고 |
| `support` | `factory_return` | 제조사 회수 |
| `support` | `loaner_send` | 대체장비 발송 |
| `support` | `courier` | 택배 발송·수령 |
| `support` | `other` | 기타 지원 |

```sql
-- 064  as_work_items는 그대로 두고 1:1 상세 표 추가 (기존 쿼리 영향 최소화)
CREATE TABLE as_support_details (
  work_id          TEXT PRIMARY KEY REFERENCES as_work_items(work_id),
  support_type     TEXT NOT NULL,        -- factory_inbound | factory_return | loaner_send | courier | other
  vendor_name      TEXT,                 -- 제조사/업체
  tracking_no      TEXT,                 -- 송장번호
  action_date      TEXT,                 -- 발송/입고 처리일 또는 회수일(유형별 의미는 3.3)
  due_date         TEXT,                 -- 입고/회수 예정일
  asset_id         TEXT,                 -- 대상 장비(자산 연결, 선택)
  memo             TEXT
);
```
- `work_kind='support'` 추가에 맞춰 **`workKindLabel`, `statusBadge`, 하부업무 번호 발급, 일일 업무 등록(`workAssignKey`의 `wi:`), 미배정 수집(`CollectUnassigned`의 wi 조회)** 등 `confirm`/`revisit`를 가정한 분기를 전수 점검해 지원 유형을 처리한다(누락 시 조용히 무시되지 않게).
- 담당자는 원 AS 담당자 복사(기존 규칙)이되 독립 변경 가능. **업무 구분은 지원업무로 표시**(`[지원]` 배지).
- 권한: AS 처리 권한(`canProcessAS`)이 있는 사용자가 등록·수정·완료. 열람 전용 역할은 불가.

### 3.3 화면
- **AS 상세**: 「지원업무」 섹션(목록 + 「+ 지원업무 추가」). 입고·회수 유형은 장비 선택(해당 접수의 자산 중), 제조사, 예정일 입력.
- **진행 표시(스테퍼)**: 입고/회수 두 건이 모두 있을 때 `입고 예정 → 입고 완료(제조사 수리 중) → 회수 예정 → 회수 완료`로 **계산해서** 표시한다(상태를 별도 저장하지 않고 두 건의 완료·날짜에서 파생).
- **자산 이력**: 해당 장비 이력에 「제조사 수리 중(입고 ~ 회수)」 구간을 표시한다. 같은 장비로 새 접수가 들어와도 알아볼 수 있게 한다.
- **하부업무 조치 화면**(`as/work_action.html`)에서 지원업무 입력 항목(제조사, 송장, 일자, 메모)을 입력·완료한다. 지원업무 완료는 해당 건에만 반영되고 원 접수 상태는 바뀌지 않는다(기존 하부업무 규칙).

### 3.4 AS 완료와의 관계 (D-37)
- 열린 지원업무가 있는 상태에서 AS를 「완료」하면 **차단하지 않고 경고**한다: 「미완료 지원업무 N건이 있습니다. 그대로 완료하시겠습니까?」 → 확인하면 완료. 경고에 응한 사실을 이력에 남긴다.
- 완료된 AS에 열린 지원업무가 남은 경우, 일일 업무·대시보드에서 계속 보이되 AS 완료와 구분한다.

### 3.5 통계: 제조사 대기 구간 (D-38)
- 접수→조치완료 영업일 KPI는 **지금처럼 계산**하고, 값을 **제외·보정하지 않는다.**
- 별도로 **「제조사 대기」 구간**을 표시한다: 입고 처리일(`factory_inbound`의 `action_date`) ~ 회수일(`factory_return`의 `action_date`, 미회수면 오늘). 영업일 기준(기존 영업일 계산 함수 재사용).
- 표시 위치: AS 상세(전체 소요 옆에 「제조사 대기 N영업일」) + 통계 화면 카드 1개(해당 기간 AS 중 제조사 대기가 있던 건수·평균 대기일). 측정 불가(입고만 있고 회수 없음 등)는 「진행 중」, 데이터 없음은 `—`.
- 기존 KPI 문구에 영향이 없는지(표본·기준일 표기 포함) 확인하고 보고.

### 3.6 필수 테스트
- 지원업무 생성(5개 유형) / AS를 닫지 않음 / 번호 `AS번호-NN` 순번 중복 없음(동시 생성 포함) / 권한(열람 전용 불가, 타 조직 AS 불가)
- 담당자 초기값 복사 후 독립 변경, 지원업무 완료가 원 AS 상태에 영향 없음
- 일일 업무 등록·미배정 목록·**배정 시 담당자 저장(`wi` 버그 수정과 함께)**·이관·보류에서 `support` 종류가 누락 없이 동작
- 스테퍼 파생 상태 표(입고만/입고+회수/회수 완료/미완료) / 자산 이력 구간 표시
- AS 완료 시 경고(열린 지원업무 N건) → 확인 후 완료, 이력 기록 / 열린 지원업무 없으면 경고 없음
- 제조사 대기 영업일: 주말·공휴일 제외, 입고만 있는 경우 「진행 중」, 데이터 없음 `—`, **기존 접수→조치완료 KPI 값이 변하지 않음**(회귀 테스트)
- 마이그레이션 up/down 왕복, 기존 `confirm`/`revisit` 하부업무 회귀

---

## 4. 진행 순서와 Cursor 프롬프트

| 순서 | 내용 | 비고 |
|------|------|------|
| U1 | §1 영업 대시보드 이동 + §2.1 잠금 면제·조직 범위 | 작음, 바로 가능 |
| U2 | §2.2~2.5 등록 요청·소프트 삭제·삭제 내역·복구·아이디 예약 | 마이그레이션 포함 |
| S1 | §3 AS 지원업무 | 로그인 팝업 점검(배정 `wi` 버그)과 같이 하면 좋음 |

### U1
```
@agent-implementation @agent-test
docs/user-admin-and-as-support-spec.md 의 §1과 §2.1만 구현해 줘. docs/decisions-log.md D-30~D-32 참고.
- 영업 역할 로그인 후 `/` → 영업 대시보드 이동(?home=tech 예외, 권한 없는 영업은 이동 안 함), 양쪽 화면에 전환 링크.
- vsadmin·org_admin은 /users 등 사용자 관리 접근 시 관리 섹션 잠금 비밀번호를 묻지 않는다. 면제 범위에서 다른 /admin/* 에 미치는 영향은 목록으로 보고하고 임의로 넓히지 말 것.
- org_admin의 사용자 목록·사용자별 경로는 서버 쿼리에서 자기 조직으로 제한(직접 URL 접근 403/404).
- §2.6 중 잠금·조직 범위·영업 이동 테스트와 §1 테스트를 작성. 완료 조건: go build/vet/test green, 변경 파일·영향 범위 보고. 불명확하면 질문.
```

### U2
```
@agent-implementation @agent-test @agent-db
docs/user-admin-and-as-support-spec.md 의 §2.2~2.6을 구현해 줘 (U1 완료 후). docs/decisions-log.md D-33~D-36 참고.
- 마이그레이션 번호는 현재 최신 다음으로 맞추고 up/down 왕복 테스트, InitDB 멱등.
- 등록 요청(account_requests), 소프트 삭제·삭제 내역·복구, 아이디 영구 예약(username_reservations), 임시 비밀번호 1회 표시(평문 저장·로깅 금지), 첫 로그인 변경 강제.
- UserRepo.Delete의 DELETE FROM users 제거 + 정적 테스트. 삭제 전 담당 업무 일괄 이관 UI.
- 로그인 팝업에 「계정 등록 요청 N건」 섹션 추가(vsadmin만).
- 연차 모듈(approval/leave)이 아직 없으면 §2.5의 연차 처리 부분은 인터페이스(이벤트 기록)까지만 만들고 TODO로 남길 것.
- §2.6 테스트 전부 작성. 완료 조건: go build/vet/test green, 테스트 목록·결과, 기존 users 관련 테스트 영향 보고. 불명확하면 질문.
```

### S1
```
@agent-implementation @agent-test @agent-db
docs/user-admin-and-as-support-spec.md 의 §3을 구현해 줘. docs/decisions-log.md D-37, D-38 참고.
- as_work_items에 work_kind='support' 추가 + as_support_details 1:1 표. confirm/revisit를 가정한 분기(workKindLabel, statusBadge, 번호 발급, wi 키 처리, 미배정 수집, 통계)를 전수 점검해서 누락 없이 처리.
- AS 상세에 지원업무 섹션·스테퍼(파생 계산)·자산 이력 표시, 하부업무 조치 화면에 지원 입력 항목.
- AS 완료 시 열린 지원업무는 경고 후 허용(차단 금지), 확인 이력 기록.
- 제조사 대기 구간은 별도 표시만. 기존 접수→조치완료 KPI 값이 바뀌지 않는 회귀 테스트 필수.
- §3.6 테스트 전부. 완료 조건: go build/vet/test green, 변경 파일·테스트 목록·기존 통계 영향 보고. 불명확하면 질문.
```
