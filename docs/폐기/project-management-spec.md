# 사업관리 시스템 설계서 (v0.1)

작성: 2026-10-14 · 근거: D-51(독립 운영 + 통합 관리), 사용자 답변 8항(2026-10-14)
상태: 설계 확정 대상. 현업 자료가 필요한 부분(서류 양식·진행률 공식·사업 유형별 단계)은 **자리(확장점)만 만들고 값은 비워 둔다**.

---

## 1. 범위와 원칙

1. **영업 → 사업관리 이관 규칙**: 계약 완료 전까지 사업은 영업(`sales_projects`)에 있고, **계약 완료가 확정되면 사업관리(`work_projects`)로 이관**한다. 이관 후 계약·수행 정보의 **원본(source of truth)은 사업관리**이며, 영업 쪽은 읽기 전용(이력 보존)이다.
2. 사업관리는 **독립 시스템처럼** 자기 목록·상세·서류·일지를 가진다. 업무관리(홈/내 업무/팀 업무)는 사업관리 데이터를 **읽어서 모아 보여 줄 뿐**, 별도 사본을 만들지 않는다.
3. **이중 입력 금지**: 사업 일지는 일일 업무(`work_tasks`)를 사업 기준으로 모아 보는 **뷰**다. 같은 사실을 두 번 적게 하지 않는다.
4. 현업 자료 미수령 항목은 **데이터(템플릿 테이블)로 두고 코드에 박지 않는다.** 관리자가 화면에서 채울 수 있게 한다.
5. 기존 mock(예시 데이터) 화면은 **실데이터 화면으로 단계 교체**한다. 실데이터가 준비되기 전까지 mock이 실제 목록을 가리지 않게 한다.
6. 결재는 71단계 **결재 엔진**을 재사용한다(작성 → 승인, 검토 단계 없음, D-13). mock의 「작성→검토→승인」은 폐기.

---

## 2. 현재 문제 8항과 해결 방향 (요약표)

| # | 문제 | 해결 | 단계 |
|---|------|------|------|
| 1 | 화면이 mock이고 실제 목록을 가림 | 실데이터 교체, `SHOW_PROJECTS_SECTOR`는 「메뉴 노출」 용도로만 축소 | 72-A, 72-I |
| 2 | 영업/사업 계약정보 이중, 원본 불명 | 이관 규칙(§3) + 이관 이력 테이블 | 72-C |
| 3 | PM·팀·유형·상태·단계 컬럼 없음 | `work_projects` 컬럼 추가 + 구성원·단계 테이블 | 72-B |
| 4 | 서류 모델 없음 | 템플릿형 + 수작업형 겸용 모델(§6) | 72-G |
| 5 | 하위 업무 `project_id` NULL 허용 | 연결 규칙 + 미분류 큐 + 백필(§4) | 72-D |
| 6 | 발주처 이름/ID 이중 | 고객 마스터 ID가 원본, 이름은 표시용 스냅샷(§5) | 72-E |
| 7 | 일지/투입 vs 일일업무 중복 | §7 제안안 채택 | 72-F |
| 8 | 미커밋 작업 혼재 | 주제별 커밋 분리 | 72-A |
| + | 진행률 공식 없음 | 계산기 확장점(§8), 1차는 수동/단계완료비율 | 72-H |

---

## 3. 이관 규칙 (영업 → 사업관리)

- **트리거**: 영업 사업 상태가 「계약 완료」로 바뀌는 시점(또는 계약 정보 입력 완료 + 「사업관리로 이관」 버튼). **자동 생성이 아니라 「이관 확정」 한 번의 확인**을 거친다(계약 금액·기간 오기입 방지). 한 영업 사업은 한 번만 이관된다(멱등, `UNIQUE(sales_project_id)`).
- **이관 시 복사되는 값(스냅샷)**: 사업명, 발주처(고객 ID), 계약금액, 계약번호, 계약일, 계약방식/대상, 계약 기간(착수·완료 예정), 영업 담당자(참고용). 복사 후 사업관리가 원본이다.
- **이관 후**: 영업 사업은 `transferred` 표시 + 읽기 전용, 영업 상세에 「사업관리 상세로 이동」 링크. 영업 쪽 금액 변경은 차단(변경은 사업관리에서 하고 D-48 이력 유지).
- **이미 `work_projects.sales_project_id`로 연결된 기존 건**: 새로 만들지 않고 **연결만 확정**하여 이관 이력에 `kind='linked_existing'`으로 기록.
- **이관 담당**: 확정은 영업 담당자 또는 관리자. 이관 직후 PM 미지정이면 「PM 미지정」 알림을 관리자 업무에 표시.
- **되돌리기**: 일반 사용자에게 제공하지 않는다. 관리자만 사유 필수로 「이관 취소」(연결 데이터가 없을 때만, `data_change_logs` 기록).

---

## 4. 하위 업무와 사업 연결

대상: `as_receipts`, `assets`, `maintenance_visits`, `work_tasks`, `weekly_report_rows`, `project_scope_rules`.

- SQLite라 `NOT NULL` 일괄 전환은 하지 않는다. **서비스 계층에서 필수 검증 + 린트 테스트**로 강제한다.
- **「내부업무(비사업)」 시스템 사업**을 조직당 1개 자동 생성(`work_projects.is_system=1`). 사업에 속하지 않는 업무(일반 사무, 교육 등)는 여기에 연결 → **모든 신규 업무는 반드시 사업 하나에 연결**된다.
- **연결 규칙(신규 입력)**:
  - AS 접수: 고객에 **활성 유지보수/공사 사업이 1개**면 자동 제안, 여러 개면 선택, 없으면 내부업무.
  - 정기점검: 점검 계획이 속한 사업으로 고정.
  - 일일업무·주간보고: 사업 선택 필수(최근 사용 사업 우선 표시).
- **백필(기존 NULL)**: 마이그레이션이 아니라 **관리자 화면의 「미분류 큐」**로 처리한다.
  1. 1차 자동: 고객+활성 사업이 정확히 1개인 건만 자동 연결(드라이런 리포트 → 승인 후 실행).
  2. 나머지는 미분류 큐에서 일괄 선택(여러 건 체크 → 사업 지정).
  3. 모든 변경은 `data_change_logs` 기록.
- 사업 병합(`project_merges`)이 있으므로 자식 이동은 기존 병합 로직과 동일 경로를 쓴다.

---

## 5. 발주처 정리

- 원본은 **고객 마스터 ID**(`work_projects.ordering_party_id` → `customers`). 현재 텍스트 `ordering_party`는 **표시용 스냅샷**으로만 두고 입력은 ID 선택만 허용한다(자동완성 선택, 없으면 「고객 신규 등록」 팝업).
- **정규화**: `customers.name_normalized`(공백·법인표기(주)/㈜/주식회사·대소문자 제거) 컬럼 추가, 같은 조직 내 정규화 이름 중복 검사.
- **데이터 정리 순서**: ① 드라이런 리포트(정규화 이름 일치 / 유사 / 불일치 3분류) ② 정확 일치는 ID 자동 채움 ③ 유사·불일치는 관리자가 「고객 병합/연결」 화면에서 결정. 자동 병합은 정확 일치 외에는 하지 않는다.
- 고객 병합은 하위 참조(사업, AS, 자산, 영업 사업)를 함께 이동하며 이력 기록.

---

## 6. 서류 모델 (템플릿형 + 수작업형 겸용)

현업 양식이 오기 전에도 **수작업형으로 바로 운영**되고, 양식이 오면 **템플릿형으로 전환**할 수 있는 구조.

- `project_doc_templates`: 서류 양식(조직별). 이름, 서류 종류, 적용 사업 유형, 버전, 상태(초안/사용/중지), `schema_json`(섹션·필드 정의), 결재 필요 여부. **1차 출시는 빈 상태**, 관리자 화면에서 등록.
- `project_documents`: 개별 서류. `mode` = `template` | `adhoc`, `template_id`·`template_version`(NULL 가능), `phase_id`(NULL 가능, 해당 수행 단계), 제목, 종류(자유 코드), `content_json`(템플릿형: 필드 값 / 수작업형: 본문 단일 필드), 상태(작성중/결재중/승인/반려/보관), 작성자, `approval_request_id`(결재 엔진 연결, NULL 가능).
- `project_document_files`: 첨부(파일 경로/크기/체크섬, 폐쇄망 로컬 저장).
- 버전: 제출(결재 요청) 시점의 `content_json`을 불변 스냅샷으로 보존, 반려 후 수정은 새 리비전.
- 결재: 71 엔진에 `project_doc` 유형 등록(작성 → 승인(팀장) / 문서별 「결재 불필요」 선택 가능). mock의 검토 단계는 만들지 않는다.
- 현업에 받을 자료: 서류 종류 목록, 종류별 양식 파일(HWPX/XLSX/DOCX), 필수 항목, 결재 필요 서류, 사업 유형별 필수 제출 서류.

---

## 7. 업무일지/투입 vs 일일업무 — 통합안 (사용자 요청: 아이디어 제시)

### 현황
일일업무(`work_tasks`)는 「무엇을 했나/할 건가」, mock의 사업 일지는 「사업, 내용, 투입 인원·시간」. 같은 날 같은 일을 두 번 적게 될 위험.

### 후보안
| 안 | 내용 | 장점 | 단점 |
|----|------|------|------|
| **A** | 일일업무가 유일한 원본. 사업 일지 = `project_id`로 모아 본 뷰. 일일업무 항목을 일지형 필드(사업·내용·결과·투입)로 보이게 함 | 이중 입력 없음, 구현 최소, 사용자 의도와 일치 | 일지 고유 필드(날씨·인원 등)가 필요하면 확장 필요 |
| B | 일지 별도 테이블 + 일일업무에서 자동 생성 | 일지 고유 필드 자유 | 동기화 버그, 수정 시 어느 쪽이 원본인지 혼란 |
| C | 일지만 쓰고 일일업무는 일지에서 파생 | 현장 일지 중심 | 일일 계획/배정(D-49 미배정 열 등) 모델과 충돌 |

### 권고: **A + 투입 선택 옵션**
1. 일일업무 한 건 = 일지 한 줄: `[사업] [유형] [내용] [결과/메모] [투입]`. 기본 화면이 이미 이 모양이 되게 한다(별도 일지 입력 화면 없음).
2. **투입 기본 단위는 공수(일 비율: 0.25/0.5/0.75/1.0)**. 입력은 선택. 입력한 날은 합계가 1.0을 넘으면 **경고만**(차단 안 함).
3. **시간별 체크는 옵션**: 조직 설정 `time_tracking = off|optional|required`(기본 off). 켜면 업무 항목에 시작/종료 시각(또는 분)을 입력할 수 있고, 투입 공수는 시간에서 자동 계산(8시간=1.0, D-18과 동일 기준).
4. **사업 일지 화면**(`/projects/:id/diary`, 기존 `/projects/diary`는 사업 선택 후 동일 뷰)은 `work_tasks`를 날짜·사업으로 모아 **읽기 중심**으로 보여 주고, 「이 날 일지에 한 줄 추가」는 일일업무 등록 폼을 사업 고정으로 여는 것이다.
5. 사업 대시보드의 투입 합계·최근 일지는 같은 집계를 재사용한다.
6. 확장: 공사 현장처럼 날씨·인원 등 일지 고유 항목이 필요해지면 `work_task_extras`(JSON) 또는 **사업 단위 일일 메모**(`project_day_notes`, 날짜+사업 1행)로 추가한다. 일지 본문을 둘로 나누지 않고 「일일 메모」만 별도로 둔다.

→ D-54로 확정 제안.

---

## 8. 진행률 확장점

현업 자료가 없으므로 **계산기를 끼울 자리**만 만든다.

- Go 인터페이스 `ProgressCalculator{ Key() string; Compute(ctx, projectID) (Result{Pct float64, Detail map[string]any}, error) }` + 레지스트리.
- 1차 제공: `manual`(PM이 직접 입력), `phase_ratio`(완료 단계 수 / 전체 단계 수, 가중치 컬럼 `weight` 선택 사용).
- `work_projects.progress_method`(기본 `manual`), `progress_pct`(캐시), `project_progress_snapshots`(이력: 방식, 값, 계산시각, 근거 JSON).
- 공식은 현업 자료 도착 후 `ProgressCalculator` 구현 추가 + 사업 유형별 `progress_method` 매핑만 바꾸면 된다(화면·DB 변경 없음).
- 화면에는 진행률 옆에 **산출 방식 이름**을 작게 표시(근거 없는 수치 오해 방지).

---

## 9. DB 스키마 변경 (마이그레이션 060 이후)

> 번호는 현재 최신 059 기준. Cursor는 착수 시점 최신 번호를 확인하고 이어 붙인다.

### 060_project_core
```sql
ALTER TABLE work_projects ADD COLUMN project_type TEXT;            -- construction|build|dev|maintenance|other (NULL 허용, 코드표는 data)
ALTER TABLE work_projects ADD COLUMN exec_status TEXT NOT NULL DEFAULT 'preparing'; -- preparing|running|inspection|closed|on_hold
ALTER TABLE work_projects ADD COLUMN pm_user_id INTEGER;           -- 사업 책임자
ALTER TABLE work_projects ADD COLUMN is_system INTEGER NOT NULL DEFAULT 0; -- 내부업무(비사업) 사업
ALTER TABLE work_projects ADD COLUMN progress_method TEXT NOT NULL DEFAULT 'manual';
ALTER TABLE work_projects ADD COLUMN progress_pct REAL;            -- 캐시
ALTER TABLE work_projects ADD COLUMN contract_date TEXT;
ALTER TABLE work_projects ADD COLUMN transferred_at TEXT;

CREATE TABLE project_members (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id INTEGER NOT NULL, user_id INTEGER NOT NULL,
  role TEXT NOT NULL DEFAULT 'member',   -- pm|member
  from_date TEXT, to_date TEXT,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  UNIQUE(project_id, user_id, role)
);
CREATE TABLE project_phase_templates (        -- 데이터로 관리, 1차 최소 시드(준비/수행/검수/종료)
  id INTEGER PRIMARY KEY AUTOINCREMENT, org_id INTEGER, project_type TEXT,
  key TEXT NOT NULL, name TEXT NOT NULL, sort_order INTEGER NOT NULL, weight REAL, is_active INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE project_phases (
  id INTEGER PRIMARY KEY AUTOINCREMENT, project_id INTEGER NOT NULL,
  key TEXT NOT NULL, name TEXT NOT NULL, sort_order INTEGER NOT NULL, weight REAL,
  planned_start TEXT, planned_end TEXT, actual_start TEXT, actual_end TEXT,
  status TEXT NOT NULL DEFAULT 'todo'     -- todo|doing|done|skipped
);
CREATE INDEX idx_projects_pm ON work_projects(pm_user_id);
CREATE INDEX idx_phases_project ON project_phases(project_id, sort_order);
```
(project_type/exec_status 값 목록은 코드 상수 + 관리 화면에서 라벨 변경 가능, CHECK 제약은 두지 않아 현업 유형 확정 시 마이그레이션 없이 확장.)

### 061_project_transfer
```sql
CREATE TABLE project_transfers (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  sales_project_id INTEGER NOT NULL UNIQUE,
  project_id INTEGER NOT NULL,
  kind TEXT NOT NULL,                      -- created|linked_existing
  snapshot_json TEXT NOT NULL,             -- 이관 시점 영업 값
  transferred_by INTEGER NOT NULL,
  transferred_at TEXT NOT NULL DEFAULT (datetime('now')),
  cancelled_at TEXT, cancelled_by INTEGER, cancel_reason TEXT
);
CREATE UNIQUE INDEX uq_projects_sales ON work_projects(sales_project_id) WHERE sales_project_id IS NOT NULL;
ALTER TABLE sales_projects ADD COLUMN transferred_project_id INTEGER;
```
(`uq_projects_sales` 생성 전 중복 `sales_project_id` 유무 점검 필수 — 있으면 드라이런 리포트 후 처리.)

### 062_project_linkage
- 조직별 시스템 사업 「내부업무(비사업)」 생성(멱등).
- `project_unclassified_queue`는 **테이블을 만들지 않고 뷰/쿼리**(자식 테이블 `project_id IS NULL`)로 구현.
- 인덱스: 자식 테이블 `project_id`에 없는 인덱스 추가.

### 063_party_normalization
```sql
ALTER TABLE customers ADD COLUMN name_normalized TEXT;
CREATE INDEX idx_customers_norm ON customers(org_id, name_normalized);
ALTER TABLE work_projects ADD COLUMN ordering_party_snapshot TEXT; -- 표시용(기존 ordering_party 값 복사)
```
(데이터 정리는 마이그레이션이 아닌 관리 화면/CLI 드라이런 후 적용.)

### 064_effort_and_timetrack
```sql
ALTER TABLE work_tasks ADD COLUMN effort REAL;           -- 공수(일 비율), NULL=미입력
ALTER TABLE work_tasks ADD COLUMN started_at TEXT;       -- 옵션: 시간 추적
ALTER TABLE work_tasks ADD COLUMN ended_at TEXT;
CREATE TABLE project_day_notes (
  id INTEGER PRIMARY KEY AUTOINCREMENT, project_id INTEGER NOT NULL, note_date TEXT NOT NULL,
  body TEXT, created_by INTEGER, updated_at TEXT, UNIQUE(project_id, note_date)
);
-- 조직 설정: time_tracking (off|optional|required)은 기존 설정 테이블 사용, 없으면 org_settings 추가
```

### 065_project_docs
```sql
CREATE TABLE project_doc_templates (
  id INTEGER PRIMARY KEY AUTOINCREMENT, org_id INTEGER, name TEXT NOT NULL, doc_kind TEXT,
  project_type TEXT, version INTEGER NOT NULL DEFAULT 1, status TEXT NOT NULL DEFAULT 'draft',
  schema_json TEXT, needs_approval INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE project_documents (
  id INTEGER PRIMARY KEY AUTOINCREMENT, project_id INTEGER NOT NULL, phase_id INTEGER,
  mode TEXT NOT NULL DEFAULT 'adhoc', template_id INTEGER, template_version INTEGER,
  title TEXT NOT NULL, doc_kind TEXT, content_json TEXT, status TEXT NOT NULL DEFAULT 'draft',
  revision INTEGER NOT NULL DEFAULT 1, approval_request_id INTEGER,
  created_by INTEGER NOT NULL, created_at TEXT NOT NULL DEFAULT (datetime('now')), updated_at TEXT
);
CREATE TABLE project_document_files (
  id INTEGER PRIMARY KEY AUTOINCREMENT, document_id INTEGER NOT NULL, file_name TEXT NOT NULL,
  stored_path TEXT NOT NULL, size_bytes INTEGER, sha256 TEXT, uploaded_by INTEGER,
  uploaded_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_docs_project ON project_documents(project_id, status);
```

### 066_project_progress
```sql
CREATE TABLE project_progress_snapshots (
  id INTEGER PRIMARY KEY AUTOINCREMENT, project_id INTEGER NOT NULL, method TEXT NOT NULL,
  pct REAL NOT NULL, detail_json TEXT, computed_at TEXT NOT NULL DEFAULT (datetime('now')), computed_by INTEGER
);
CREATE INDEX idx_progress_project ON project_progress_snapshots(project_id, computed_at);
```

공통: 모든 마이그레이션은 **멱등**(재실행 안전), 롤백 방법을 작업 보고서에 명시, 운영 적용은 백업 후 별도 단계.

---

## 10. 화면 대응 (mock → 실데이터)

| mock 화면 | 실데이터 대체 | 비고 |
|-----------|---------------|------|
| `/projects` 목록 | `work_projects` 실제 목록(유형·상태·PM·기간·진행률) | `WorkSeed` 제거 |
| `/projects/dashboard` | 집계 쿼리(상태별 건수, 지연, 최근 일지, 미분류 큐 건수) | 진행률은 `progress_pct` |
| `/projects/docs` | `project_documents` 목록, 수작업 작성 + 첨부 | 템플릿 없으면 수작업형만 |
| `/projects/diary` | `work_tasks` 사업별 집계 뷰 + 일 메모 | 별도 입력 화면 없음 |
| `/projects/:id` | PM·구성원·단계·서류·일지·연결 업무 탭 | |
| (신규) 영업 상세 | 「사업관리로 이관」 버튼 | 계약 완료 상태에서만 |
| (신규) 관리 | 미분류 큐, 발주처 정리, 서류 양식, 단계 템플릿 | 관리자 |

mock에만 있던 자재, 서류 검토 단계, 임의 진행률 수치는 **제거**(필요 시 현업 확인 후 재도입).
