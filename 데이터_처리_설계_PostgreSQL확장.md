# 데이터 처리 설계 — 고객지원 시스템 (PostgreSQL 확장)

**기준 문서:** `기획_업무일지_프로그램_요구사항.md` (고객지원 시스템 구축 기획서)

**목표:** 기획서 §5~§12에 따른 기준정보·설치자산·AS·코드의 데이터 구조와 저장 방식을 정의한다. 1단계는 JSON 또는 SQLite로 운영하고, 확장 시 PostgreSQL로 전환할 수 있도록 설계한다.

**설계 원칙(기획서 §2.2):**
- 모든 주요 엔터티는 이름이 아닌 **고유 ID**로 연결한다.
- **현재 정보**와 **이력 정보**를 분리하여 변경 추적이 가능하도록 한다.
- 설치 위치는 **건물-층-실** 구조 기반으로 표준화한다.
- AS 정보는 **설치자산과 연결**하여 동일 장비의 반복 장애·교체주기를 추적한다.
- 비밀번호 등 **민감정보는 DB에 평문 저장하지 않는다.**

---

## 1. 저장 전략 요약

| 단계 | 저장소 | 용도 |
|------|--------|------|
| 1단계 | JSON 파일 또는 SQLite | 단일 사용자·로컬 운영, 기준정보·AS 1차 구축 |
| 확장 | PostgreSQL | 다중 사용자, 원격 DB, 백업·이관·영업분석 확장 |

본 문서는 **논리 스키마**를 먼저 정의하고, JSON/SQLite 구현 시 필드명을 동일하게 유지하여 이후 PostgreSQL 마이그레이션 시 매핑을 일치시킨다.

---

## 2. 코드관리 (공통)

기획서 §11. 모든 업무 입력의 표준화를 위해 **코드 그룹**을 먼저 정의한다. 1단계에서는 JSON 또는 SQLite 단일 테이블(또는 코드그룹별 파일)로 관리한다.

| 코드그룹 | 설명 | 예시 값 |
|----------|------|---------|
| 업종 | 기관 분류 | 도서관, 학교, 공공기관, 민간 |
| 담당업무 | 담당자 업무 영역 | 전산, 네트워크, 도서관리시스템, 행정시스템 |
| 제품구분 | 설치자산 제품 유형 | SW, HW, 서버, 네트워크장비, 주변장비 |
| 설치주체 | 설치 수행 주체 | 자사, 타사, 제조사, 협력사, 미상 |
| 관리유형 | 자산 관리 방식 | 직접유지보수, 장애대응, 정기점검, 요청시지원, 참고관리 |
| 요청주체유형 | AS 요청 주체 | 고객직접, 제조사, 협력사, 원청, 내부 |
| AS상태 | 접수 건 상태 | 접수, 진행중, 보류, 완료, 종료 |
| 원인분류 | 장애 원인 | HW고장, SW오류, 네트워크, 환경문제, 사용자오류 |
| 처리유형 | AS 처리 방식 | 원격지원, 방문, 교체, 설정변경, 문의응대 |
| 운영상태 | 설치자산 운영 상태 | 운영중, 점검중, 장애, 철수, 폐기 |
| 건물구분 | 건물 용도(선택) | 본관, 별관, 기타 |
| 실용도 | 실 용도 | 전산실, 자료실, 서버실 등 |

**저장 구조(예시):**
- `code_group`: 코드그룹 식별
- `code_value`: 코드 값
- `code_name`: 표시명
- `sort_order`: 정렬 순서
- `use_yn`: 사용 여부

---

## 3. 기준정보 — 고객(기관) 마스터

기획서 §5.1.

| 필드명 | 타입 | 필수 | 설명 |
|--------|------|------|------|
| customer_id | string (UUID 또는 내부 ID) | Y | 내부 고유 식별자 |
| name | string | Y | 기관명(실무상 명칭) |
| official_name | string | Y | 공식명칭(계약서·공문 기준) |
| email | string | N | 기관 대표/공식 이메일 |
| phone | string | Y | 대표전화 |
| homepage | string | N | 기관 홈페이지 |
| business_number | string | N | 사업자번호(중복 판별) |
| representative | string | N | 대표자명 |
| industry_code | string | Y | 업종(코드) |
| parent_customer_id | string | N | 상위기관 ID(본청-산하) |
| address | string | N | 기본주소 |
| address_detail | string | N | 상세주소 |
| use_yn | boolean | Y | 사용 여부 |
| remarks | string | N | 비고(통합/폐쇄/휴면 등) |
| created_at | datetime | Y | 등록일시 |
| updated_at | datetime | Y | 수정일시 |

**제약:** 사업자번호 또는 내부 규칙으로 동일 기관 중복 등록 방지(기획서 §14).

---

## 4. 공간정보 — 건물·층·실

기획서 §5.2. 설치자산의 위치는 건물-층-실 기준 선택 + 상세위치 보조 입력.

### 4.1 고객건물

| 필드명 | 타입 | 필수 | 설명 |
|--------|------|------|------|
| building_id | string | Y | 건물 고유 ID |
| customer_id | string | Y | 소속 기관 |
| building_name | string | Y | 건물명 |
| building_type_code | string | N | 건물구분(코드) |
| address | string | N | 주소 |
| use_yn | boolean | Y | 사용 여부 |
| created_at, updated_at | datetime | Y | |

### 4.2 고객층

| 필드명 | 타입 | 필수 | 설명 |
|--------|------|------|------|
| floor_id | string | Y | 층 고유 ID |
| building_id | string | Y | 소속 건물 |
| floor_name | string | Y | 층명(B1, 1F, 2F 등) |
| sort_order | int | N | 정렬 순서 |
| created_at, updated_at | datetime | Y | |

### 4.3 고객실

| 필드명 | 타입 | 필수 | 설명 |
|--------|------|------|------|
| room_id | string | Y | 실 고유 ID |
| floor_id | string | Y | 소속 층 |
| room_name | string | Y | 실명 |
| room_number | string | N | 실번호 |
| room_use_code | string | N | 용도(전산실, 자료실 등) |
| created_at, updated_at | datetime | Y | |

**상세위치**는 설치자산(설치기본정보)에만 저장(복도 좌측 등 자유 서술).

---

## 5. 담당자·담당자 이력

기획서 §5.3, §5.4. 담당자는 삭제하지 않고 재직상태·이력으로 관리한다.

### 5.1 고객담당자(현재 담당자)

| 필드명 | 타입 | 필수 | 설명 |
|--------|------|------|------|
| contact_id | string | Y | 담당자 고유 ID |
| customer_id | string | Y | 소속 기관 |
| name | string | Y | 담당자 실명 |
| duty_code | string | N | 담당업무(코드) |
| job_title | string | N | 직함/직급 |
| phone | string | N | 전화번호 |
| mobile | string | N | 핸드폰번호 |
| email | string | N | 개별 이메일 |
| appointed_at | date | N | 부임일 |
| retired_at | date | N | 퇴임일 |
| in_office_yn | boolean | Y | 재직 여부 |
| main_contact_yn | boolean | N | 주담당 여부 |
| created_at, updated_at | datetime | Y | |

### 5.2 고객담당자이력

변경 시 과거 이력을 별도 행으로 보존. AS 건 발생 시점의 담당자 재현 가능.

| 필드명 | 타입 | 필수 | 설명 |
|--------|------|------|------|
| contact_history_id | string | Y | 이력 고유 ID |
| contact_id | string | Y | 담당자 |
| customer_id | string | Y | 기관 |
| start_date | date | Y | 시작일 |
| end_date | date | N | 종료일 |
| department_name | string | N | 부서명 |
| duty_code | string | N | 담당업무 |
| job_title | string | N | 직함 |
| phone | string | N | 연락처 |
| email | string | N | 이메일 |
| status_code | string | N | 상태(재직, 전보, 퇴직, 변경 등) |
| change_reason | string | N | 변경사유 |
| created_at | datetime | Y | 등록일시 |
| created_by | string | N | 등록자 |

---

## 6. 설치자산

기획서 §6.1, §6.2, §6.3.

### 6.1 설치기본정보

| 필드명 | 타입 | 필수 | 설명 |
|--------|------|------|------|
| installation_id | string | Y | 설치자산 고유 ID |
| customer_id | string | Y | 소속 기관 |
| product_name | string | N | 제품명 |
| product_type_code | string | N | 제품구분(SW, HW 등) |
| model_name | string | N | 모델명 |
| manufacturer | string | N | 제조사명 |
| serial_number | string | N | S/N |
| installed_at | date | N | 설치일 |
| removed_at | date | N | 철수일 |
| install_owner_code | string | N | 설치주체(자사, 타사 등) |
| install_owner_name | string | N | 원설치사명 |
| operation_status_code | string | N | 운영상태(운영중, 점검중 등) |
| management_type_code | string | N | 관리유형 |
| managed_by_us_yn | boolean | N | 현재관리대상 여부 |
| request_owner_type_code | string | N | 요청주체유형 |
| request_owner_name | string | N | 요청주체명 |
| contact_id | string | N | 고객 담당자(현행) |
| assignee_id | string | N | 당사 담당자(내부 사용자 ID 등) |
| building_id | string | N | 건물 |
| floor_id | string | N | 층 |
| room_id | string | N | 실 |
| location_detail | string | N | 상세위치(자유 서술) |
| remarks | string | N | 비고 |
| created_at, updated_at | datetime | Y | |

### 6.2 설치SW상세

설치기본정보와 1:0..1 또는 1:N(동일 설치에 SW 다수).

| 필드명 | 타입 | 필수 | 설명 |
|--------|------|------|------|
| sw_detail_id | string | Y | 상세 고유 ID |
| installation_id | string | Y | 설치자산 |
| software_name | string | N | 소프트웨어명 |
| software_version | string | N | 버전 |
| install_type_code | string | N | 설치형태(PC설치형, 서버형, 웹형) |
| hardware_info | string | N | 설치 HW 정보 |
| os_name | string | N | OS |
| os_version | string | N | OS 버전 |
| dbms_name | string | N | DBMS |
| dbms_version | string | N | DB 버전 |
| access_type | string | N | 접속방식 |
| access_address | string | N | 접속주소 |
| install_path | string | N | 설치경로 |
| backup_path | string | N | 백업경로 |
| config_file_path | string | N | 환경설정파일 위치 |
| created_at, updated_at | datetime | Y | |

### 6.3 접속정보참조

기획서 §6.3. ID/PASSWORD는 DB에 저장하지 않음. 경로·관리주체만 저장.

| 필드명 | 타입 | 필수 | 설명 |
|--------|------|------|------|
| access_ref_id | string | Y | 참조 고유 ID |
| installation_id | string | Y | 설치자산(또는 sw_detail_id) |
| storage_type | string | N | 저장방식(암호화파일, 금고 등) |
| storage_path | string | N | 파일/저장소 경로(비밀 제외) |
| last_verified_at | date | N | 최종 확인일 |
| managed_by | string | N | 관리주체 |
| created_at, updated_at | datetime | Y | |

---

## 7. 수행관계

기획서 §7. 제조사/협력사/원청/요청주체 관계. 기관 또는 설치건 단위 연결.

| 필드명 | 타입 | 필수 | 설명 |
|--------|------|------|------|
| relation_id | string | Y | 수행관계 고유 ID |
| customer_id | string | Y | 기관 |
| installation_id | string | N | 설치건(선택) |
| relation_type_code | string | N | 관계구분 |
| partner_type_code | string | N | 상대회사구분 |
| partner_name | string | N | 상대회사명 |
| contact_person | string | N | 담당자명 |
| contact_phone | string | N | 연락처 |
| contact_email | string | N | 이메일 |
| start_date | date | N | 시작일 |
| end_date | date | N | 종료일 |
| current_yn | boolean | Y | 현재 유효 여부 |
| remarks | string | N | 비고 |
| created_at, updated_at | datetime | Y | |

---

## 8. AS 접수·처리

기획서 §8. AS는 가급적 설치자산(installation_id)과 연결. 연결 불가 시 임시 접수 후 자산 매핑.

### 8.1 AS접수(접수 및 상태)

| 필드명 | 타입 | 필수 | 설명 |
|--------|------|------|------|
| as_id | string | Y | 접수 건 고유 ID(AS번호) |
| received_at | datetime | Y | 접수일시 |
| customer_id | string | Y | 기관 |
| installation_id | string | N | 설치자산(가급적 연결) |
| channel_code | string | N | 접수채널(전화, 이메일, 방문 등) |
| requester_name | string | N | 요청자(실명) |
| symptom | string | N | 증상내용 |
| urgency_code | string | N | 긴급도(상/중/하) |
| importance_code | string | N | 중요도 |
| request_owner_type_code | string | N | 요청주체유형 |
| request_owner_name | string | N | 요청주체명 |
| assignee_id | string | Y | 배정담당자(당사) |
| status_code | string | Y | AS상태(접수, 진행중, 보류, 완료, 종료) |
| created_at, updated_at | datetime | Y | |

### 8.2 AS처리(처리 세부 이력)

AS접수와 1:N 가능(한 건에 처리 이력 여러 번).

| 필드명 | 타입 | 필수 | 설명 |
|--------|------|------|------|
| as_action_id | string | Y | 처리 이력 고유 ID |
| as_id | string | Y | AS접수 건 |
| started_at | datetime | N | 처리시작일시 |
| completed_at | datetime | N | 완료일시 |
| action_type_code | string | N | 처리유형 |
| cause_code | string | N | 원인분류 |
| action_detail | string | N | 조치내용 |
| parts_used | string | N | 사용부품/교체품 |
| recurrence_yn | boolean | N | 재발 여부 |
| reopen_yn | boolean | N | 재오픈 여부 |
| result_code | string | N | 처리결과코드 |
| customer_confirmed_by | string | N | 고객 확인자 |
| customer_confirmed_at | datetime | N | 확인일시 |
| follow_up | string | N | 후속조치 |
| replacement_candidate_yn | boolean | N | 교체대상 여부 |
| sales_memo | string | N | 영업메모 |
| created_at, updated_at | datetime | Y | |

---

## 9. 첨부파일관리

기획서 §12. 고객, 설치, AS, 수행관계 등에 범용 연결.

| 필드명 | 타입 | 필수 | 설명 |
|--------|------|------|------|
| attachment_id | string | Y | 첨부 고유 ID |
| entity_type | string | Y | 연결 대상 유형(customer, installation, as, relation 등) |
| entity_id | string | Y | 연결 대상 ID |
| file_name | string | Y | 파일명 |
| file_path | string | Y | 저장 경로(또는 스토리지 키) |
| file_type | string | N | 유형(설치사진, 점검표, 장애사진 등) |
| file_size | long | N | 크기 |
| created_at, created_by | datetime/string | Y | |

---

## 10. PostgreSQL 확장 시 적용

### 10.1 전환 시 유의사항

- 위 필드명을 **그대로 컬럼명**으로 사용하면 애플리케이션 수정을 최소화할 수 있다.
- ID는 UUID 또는 `SERIAL`/`IDENTITY`로 생성.
- **코드관리**는 `code_group`, `code_value` 테이블로 이관.
- **담당자 이력**, **AS처리**는 1:N 관계로 FK 설정.
- **접속정보참조**에는 비밀번호·평문 계정 정보 컬럼을 두지 않는다.

### 10.2 PostgreSQL 스키마 예시(요약)

```sql
-- 예: 고객마스터
CREATE TABLE customer_master (
  customer_id   VARCHAR(36) PRIMARY KEY,
  name          VARCHAR(200) NOT NULL,
  official_name VARCHAR(200) NOT NULL,
  email         VARCHAR(200),
  phone         VARCHAR(50) NOT NULL,
  homepage      VARCHAR(500),
  business_number VARCHAR(20),
  representative  VARCHAR(100),
  industry_code   VARCHAR(20) NOT NULL,
  parent_customer_id VARCHAR(36) REFERENCES customer_master(customer_id),
  address       VARCHAR(500),
  address_detail VARCHAR(500),
  use_yn        BOOLEAN NOT NULL DEFAULT true,
  remarks       TEXT,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 나머지 테이블: 고객건물, 고객층, 고객실, 고객담당자, 고객담당자이력,
-- 설치기본정보, 설치SW상세, 접속정보참조, 수행관계정보, AS접수, AS처리, 첨부파일관리, 코드관리
-- 는 위 논리 스키마에 맞춰 동일한 필드명으로 CREATE TABLE 정의.
```

### 10.3 JSON/SQLite → PostgreSQL 마이그레이션

1. 코드·고객·공간·담당자·설치·수행관계·AS·첨부 순으로 **마스터 → 트랜잭션** 순서로 INSERT.
2. FK 제약은 마이그레이션 후 활성화하거나, 임시로 비활성화 후 데이터 적재 뒤 활성화.
3. 기존 JSON/파일은 백업 보관 후, 검증 완료 시 제거.

---

## 11. 정기 점검 일정(기획서 §17)

월간 정기점검 계획·방문·사이트별 점검 설정을 SQLite/PostgreSQL 공통 논리로 정의한다.

### 11.1 maintenance_site_config (사이트별 점검 기준)

| 필드 | 타입 | 필수 | 설명 |
|------|------|------|------|
| customer_id | TEXT (PK, FK→customers) | Y | 점검 대상 기관 |
| short_name | TEXT | Y | 달력·엑셀에 쓰는 짧은 표시명 |
| region | TEXT | N | 지역(자동 배정 시 묶음·정렬) |
| has_klas | INTEGER 0/1 | N | KLAS 점검 대상 |
| has_rfid | INTEGER 0/1 | N | RFID 자동화 장비 점검 대상 |
| entry_category | TEXT | Y | `normal` \| `fixed` \| `office` — 엑셀 글자색 구분(기획서 §17.11.4) |
| fixed_rule | TEXT | N | 예: `LAST_MONDAY_OF_MONTH` (매월 마지막 월요일) |
| updated_at | DATETIME | Y | 수정 시각 |

### 11.2 maintenance_plans (연도 계획 헤더)

| 필드 | 타입 | 필수 | 설명 |
|------|------|------|------|
| plan_id | TEXT (PK) | Y | 내부 ID |
| plan_year | INTEGER | Y | 연도(연도당 1건 UNIQUE) |
| title | TEXT | N | 계획 제목 |
| status | TEXT | Y | `draft` \| `approved` |
| created_at, updated_at | DATETIME | Y | |

### 11.3 maintenance_visits (일자별 방문)

| 필드 | 타입 | 필수 | 설명 |
|------|------|------|------|
| visit_id | TEXT (PK) | Y | |
| plan_id | TEXT (FK) | Y | |
| visit_date | TEXT (YYYY-MM-DD) | Y | 방문일 |
| customer_id | TEXT (FK) | Y | |
| sort_order | INTEGER | N | 같은 날 복수 건 정렬 |
| auto_generated | INTEGER 0/1 | Y | 1=자동 배정(재생성 시 삭제 대상) |
| entry_category | TEXT | Y | 엑셀 색상·표시용 |
| notes | TEXT | N | |
| created_at | DATETIME | Y | |

**제약:** `(plan_id, visit_date, customer_id)` UNIQUE — 동일일·동일 사이트 중복 방지.

### 11.4 구현 참고

- 공휴일·자동 배정 로직은 애플리케이션 계층(`internal/service`)에서 처리하고, DB에는 결과 방문만 저장한다.
- 엑셀(§17.11)은 `maintenance_visits` + `maintenance_site_config` 조인 표시명으로 생성한다.

---

## 12. 단계별 반영 요약

| 단계 | 범위(기획서 §16) | 데이터 반영 |
|------|------------------|-------------|
| 1단계 | 고객마스터, 공간정보, 담당자, 설치자산, 코드관리 | §3~§6, §2 코드관리 |
| 2단계 | AS 접수, 처리, 현황, 첨부파일 | §8, §9 첨부파일 |
| 3단계 | 수행관계, 교체대상 분석, 영업활용 | §7, §9 영업활용(집계/뷰) |
| 4단계 | 정기점검 일정·엑셀 | **§11** |
| 확장 | PostgreSQL 전환 | §10 적용 |

이 문서는 기획서를 유일 기준으로 하며, 구현 시 필드 추가·코드값 확장은 기획서의 코드관리·핵심 업무 규칙과 맞춰 진행한다.
