# 데이터 처리 설계 — 엑셀 분석 기반 + PostgreSQL 확장

**기준 문서:** `엑셀_시트_분석결과.md`, `기획_업무일지_프로그램_요구사항.md`, `데이터_엑셀_구조_설계.md`

**목표:** 실제 엑셀 시트 구조를 반영한 입력·저장 설계. **DBMS 확장 전까지는 JSON 파일**을 주 저장소로 사용하고, 확장 시 **PostgreSQL**으로 전환하는 데이터 처리 계획.

---

## 1. 엑셀 분석 결과 기반 엔티티 매핑

실제 파일(`2025년_비젼아이티_유지보수현황(06161800).xlsx`)의 시트를 **데이터 입력/저장 관점**으로 분류한다.

| 엑셀 시트 | 역할 | 프로그램 엔티티 | 비고 |
|-----------|------|-----------------|------|
| NICOM_처리리스트 | 처리 이력(일지+AS 혼합) | `work_log` (라인타입으로 구분) | 일시·고객·접수/처리내용·진행여부 |
| 25년원콜지원센터통합 | 원콜 문의/처리 이력 | `work_log` (source=원콜) | 일자·처리일자·분류·제목·내용·진행·답변·도서관 |
| 세종시_KLAS 처리리스트 | KLAS 처리 이력 | `work_log` (source=세종KLAS) | 동일 구조 |
| NICOM_유지보수현황 | 고객+담당자+장비 마스터 | `customer` + `contact` + `equipment` | 구분·담당자·장비명·점검주기·유/무상 등 |
| VISIONIT_MaintenanceList | 동일 | 위와 통합 | 제품분류·유지보수품목 등 |
| 충남교육청_KLAS, 세종시도서관_KLAS | 고객+담당자 목록 | `customer` + `contact` | 장비 행 없을 수 있음 |
| AS (엑셀) | 유지보수 현황 요약(지역별) | 참고용 → 고객/점검일정과 연계 | 2차에서 계약·기간 매핑 검토 |
| 1월~12월 | 월별 달력(뷰) | 저장 테이블 없음, **조회/출력용** | 일지+점검일정에서 생성 |
| NICOM_유지보수_통계, 2025예산 | 집계/예산 | 2차 통계·예산 테이블 검토 | 1차는 생략 |

---

## 2. 실제 엑셀 컬럼 → 프로그램 필드 매핑

### 2-1. NICOM_처리리스트 (처리 이력)

| 엑셀 컬럼 | 프로그램 필드 | DB 타입(SQLite/PostgreSQL) | 필수 | 비고 |
|-----------|----------------|----------------------------|------|------|
| 번호 | (자동 id) | INTEGER / SERIAL | ✅ | PK |
| 일시 | `occurred_at` | DATETIME / TIMESTAMPTZ | ✅ | 접수·처리 일시 |
| 고객 | `customer_id` | INTEGER / FK | ✅ | 고객 마스터 연결 |
| 고객담당자 | `contact_id` | INTEGER / FK | ⬜ | 연락처 담당자 |
| 모델명 | `equipment_name` 또는 `equipment_id` | VARCHAR / FK | ⬜ | 장비명 또는 장비 마스터 |
| 장비위치 | `equipment_location` | VARCHAR(200) | ⬜ | 1층, 2층 등 |
| 유/무상 | `billing_type` | VARCHAR(20) | ⬜ | 유상/무상 |
| 접수내용 | `received_content` | TEXT | ⬜ | 접수/요청 내용 |
| 처리내용 | `handled_content` | TEXT | ⬜ | 처리/조치 내용 |
| 진행 여부 | `status` | VARCHAR(20) | ✅ | 완료/진행중 등 |
| (날짜 컬럼 25.06.16 등) | (미저장 또는 메타) | — | — | 엑셀 메타, 프로그램에서는 무시 |
| — | `source` | VARCHAR(20) | ✅ | 'NICOM' (시트 구분용) |
| — | `created_at`, `updated_at` | DATETIME/TIMESTAMPTZ | ✅ | 시스템 |

- **통합 시:** `work_log` 한 테이블에 두고 `source`(또는 `line_type`)로 NICOM / 원콜 / 세종KLAS 구분.

### 2-2. 25년원콜지원센터통합 · 세종시_KLAS 처리리스트 (문의/처리 이력)

| 엑셀 컬럼 | 프로그램 필드 | DB 타입 | 필수 | 비고 |
|-----------|----------------|---------|------|------|
| 번호 | (자동 id) | PK | ✅ | |
| 일자 | `occurred_at` (또는 date만) | DATE/TIMESTAMPTZ | ✅ | 접수 일자 |
| 처리일자 | `processed_at` | DATE/TIMESTAMPTZ | ⬜ | 처리 완료일 |
| 분류 | `category` | VARCHAR(50) | ⬜ | 메일문의/홈페이지/전화문의/정기방문/현장방문 등 |
| 제목 | `title` | VARCHAR(500) | ⬜ | |
| 내용 | `content` | TEXT | ✅ | |
| 진행 | `status` | VARCHAR(20) | ✅ | 처리완료 등 |
| 답변 | `reply` | TEXT | ⬜ | 처리 답변 내용 |
| 도서관 | `customer_id` | FK | ✅ | 고객 = 도서관 |
| 비고 | `remarks` | TEXT | ⬜ | |
| — | `source` | VARCHAR(20) | ✅ | '원콜' / '세종KLAS' |

- **NICOM과 통합 시:**  
  - `work_log`에 `title`, `reply`, `processed_at`, `category` 추가.  
  - NICOM 행은 `title`/`reply` null 또는 `received_content`/`handled_content`로만 채움.

### 2-3. NICOM_유지보수현황 / VISIONIT_MaintenanceList (고객·담당자·장비)

| 엑셀 컬럼 | 프로그램 필드 | 테이블 | DB 타입 | 비고 |
|-----------|----------------|--------|---------|------|
| No. | (id) | customer / contact / equipment | PK | 고객·담당자·장비는 별도 행으로 정규화 |
| 고객명 | `name` | customer | VARCHAR(200) | |
| 구분 | `division` | customer 또는 equipment | VARCHAR(50) | NICOM / K-LAS 등 |
| 담당자 | `name` | contact | VARCHAR(100) | contact 테이블 |
| 직장 | `phone_office` | contact | VARCHAR(50) | |
| 핸드폰 | `phone_mobile` | contact | VARCHAR(50) | |
| 이메일 | `email` | contact | VARCHAR(200) | |
| 장비명 / 유지보수품목 | `name` | equipment | VARCHAR(200) | |
| 제품분류 | `product_category` | equipment | VARCHAR(100) | VISIONIT용 |
| 수량 | `quantity` | equipment | INTEGER | 기본 1 |
| 설치위치 | `location` | equipment | VARCHAR(200) | |
| 납품연도 | `delivery_year` | equipment | VARCHAR(50) | |
| 점검주기 | `inspection_interval` | equipment 또는 inspection_schedule | VARCHAR(20) | 월/Call/분기 등 |
| 유/무상 | `billing_type` | equipment 또는 계약 | VARCHAR(20) | |
| 사이트기준/청구처 | `billing_entity` | equipment 또는 별도 | VARCHAR(100) | |
| 청구주기 | `billing_interval` | VARCHAR(100) | ⬜ | |
| 참고사항 | `remarks` | customer/contact/equipment | TEXT | |

- 한 엑셀 행에 “고객+담당자+장비”가 같이 있는 경우:  
  - **정규화:** `customer` 1건, `contact` 1건, `equipment` 1건(또는 N건)으로 나누어 저장.  
  - 고객·담당자는 중복 제거 후 FK로 연결.

### 2-4. 월별 시트 (1월~12월)

- **저장 구조:** 별도 테이블 없음.  
- **생성 방식:**  
  - `work_log` + `inspection_schedule`(다음 예정일)을 기간으로 조회한 뒤,  
  - 날짜별·요일별로 그룹핑하여 “해당 날짜 셀에 표시할 텍스트”를 만든다.  
- **출력:** 엑셀/PDF 달력 뷰 내보내기 시 위 결과를 셀에 채워 넣는 형태로 설계.

---

## 3. 통합 데이터 모델 (엑셀 라인타입 통합)

실제 엑셀의 “처리리스트” 3종(NICOM, 원콜, 세종KLAS)을 **하나의 work_log 테이블**로 수용하고, 원본 시트 구분은 `source`(및 선택 시 `line_type`)로 구분한다.

### 3-1. work_log (처리 이력 통합)

| 필드명 | 타입(SQLite) | 타입(PostgreSQL) | 필수 | 설명 |
|--------|----------------|-------------------|------|------|
| id | INTEGER PK | BIGSERIAL PK | ✅ | |
| source | TEXT | VARCHAR(20) | ✅ | 'NICOM' / '원콜' / '세종KLAS' |
| occurred_at | TEXT (ISO8601) | TIMESTAMPTZ | ✅ | 일시(또는 일자만 사용 시 DATE) |
| processed_at | TEXT | TIMESTAMPTZ NULL | ⬜ | 처리일자(원콜/KLAS) |
| customer_id | INTEGER | INTEGER FK→customer.id | ✅ | 고객(도서관) |
| contact_id | INTEGER NULL | INTEGER FK→contact.id NULL | ⬜ | 고객담당자(NICOM) |
| category | TEXT NULL | VARCHAR(50) NULL | ⬜ | 분류(원콜/KLAS) |
| title | TEXT NULL | VARCHAR(500) NULL | ⬜ | 제목(원콜/KLAS) |
| content | TEXT | TEXT | ✅ | 내용 (NICOM은 접수+처리 합치거나 별도 필드 사용) |
| received_content | TEXT NULL | TEXT NULL | ⬜ | NICOM 접수내용 |
| handled_content | TEXT NULL | TEXT NULL | ⬜ | NICOM 처리내용 |
| reply | TEXT NULL | TEXT NULL | ⬜ | 답변(원콜/KLAS) |
| status | TEXT | VARCHAR(30) | ✅ | 진행여부/진행 (완료, 처리완료, 진행중 등) |
| equipment_name | TEXT NULL | VARCHAR(200) NULL | ⬜ | 모델명(NICOM) |
| equipment_id | INTEGER NULL | INTEGER FK NULL | ⬜ | 장비 마스터 연결 |
| equipment_location | TEXT NULL | VARCHAR(200) NULL | ⬜ | 장비위치(NICOM) |
| billing_type | TEXT NULL | VARCHAR(20) NULL | ⬜ | 유/무상 |
| remarks | TEXT NULL | TEXT NULL | ⬜ | 비고 |
| created_at | TEXT | TIMESTAMPTZ | ✅ | |
| updated_at | TEXT | TIMESTAMPTZ | ✅ | |

- **인덱스 권장:** `(source, occurred_at)`, `(customer_id, occurred_at)`, `(status)`, `occurred_at` 단독.  
- PostgreSQL 확장 시: `occurred_at` 범위 검색·월별 집계용으로 인덱스 유지.

### 3-2. customer (고객)

| 필드명 | SQLite | PostgreSQL | 필수 | 설명 |
|--------|--------|------------|------|------|
| id | INTEGER PK | BIGSERIAL PK | ✅ | |
| name | TEXT | VARCHAR(200) | ✅ | 고객명 |
| division | TEXT NULL | VARCHAR(50) NULL | ⬜ | 구분(NICOM/K-LAS 등) |
| address | TEXT NULL | TEXT NULL | ⬜ | |
| phone | TEXT NULL | VARCHAR(50) NULL | ⬜ | 대표 연락처 |
| remarks | TEXT NULL | TEXT NULL | ⬜ | 참고사항 등 |
| created_at | TEXT | TIMESTAMPTZ | ✅ | |
| updated_at | TEXT | TIMESTAMPTZ | ✅ | |

### 3-3. contact (고객 담당자)

| 필드명 | SQLite | PostgreSQL | 필수 | 설명 |
|--------|--------|------------|------|------|
| id | INTEGER PK | BIGSERIAL PK | ✅ | |
| customer_id | INTEGER | INTEGER FK | ✅ | 소속 고객 |
| name | TEXT | VARCHAR(100) | ✅ | 담당자명 |
| phone_office | TEXT NULL | VARCHAR(50) NULL | ⬜ | 직장 |
| phone_mobile | TEXT NULL | VARCHAR(50) NULL | ⬜ | 핸드폰 |
| email | TEXT NULL | VARCHAR(200) NULL | ⬜ | |
| remarks | TEXT NULL | TEXT NULL | ⬜ | |
| created_at | TEXT | TIMESTAMPTZ | ✅ | |
| updated_at | TEXT | TIMESTAMPTZ | ✅ | |

### 3-4. equipment (장비)

| 필드명 | SQLite | PostgreSQL | 필수 | 설명 |
|--------|--------|------------|------|------|
| id | INTEGER PK | BIGSERIAL PK | ✅ | |
| customer_id | INTEGER | INTEGER FK | ✅ | 고객 |
| name | TEXT | VARCHAR(200) | ✅ | 장비명/유지보수품목 |
| product_category | TEXT NULL | VARCHAR(100) NULL | ⬜ | 제품분류 |
| quantity | INTEGER | INTEGER DEFAULT 1 | ⬜ | 수량 |
| location | TEXT NULL | VARCHAR(200) NULL | ⬜ | 설치위치 |
| delivery_year | TEXT NULL | VARCHAR(50) NULL | ⬜ | 납품연도 |
| inspection_interval | TEXT NULL | VARCHAR(20) NULL | ⬜ | 점검주기(월/Call 등) |
| billing_type | TEXT NULL | VARCHAR(20) NULL | ⬜ | 유/무상 |
| billing_entity | TEXT NULL | VARCHAR(100) NULL | ⬜ | 청구처 |
| billing_interval | TEXT NULL | VARCHAR(100) NULL | ⬜ | 청구주기 |
| remarks | TEXT NULL | TEXT NULL | ⬜ | |
| created_at | TEXT | TIMESTAMPTZ | ✅ | |
| updated_at | TEXT | TIMESTAMPTZ | ✅ | |

### 3-5. inspection_schedule (정기 점검 일정)

| 필드명 | SQLite | PostgreSQL | 필수 | 설명 |
|--------|--------|------------|------|------|
| id | INTEGER PK | BIGSERIAL PK | ✅ | |
| customer_id | INTEGER | INTEGER FK | ✅ | |
| equipment_id | INTEGER NULL | INTEGER FK NULL | ⬜ | |
| next_due_date | TEXT | DATE | ✅ | 다음 예정일 |
| interval_type | TEXT NULL | VARCHAR(20) NULL | ⬜ | 월/분기 등 |
| assignee_notes | TEXT NULL | TEXT NULL | ⬜ | 담당자 메모(내부용) |
| remarks | TEXT NULL | TEXT NULL | ⬜ | |
| created_at | TEXT | TIMESTAMPTZ | ✅ | |
| updated_at | TEXT | TIMESTAMPTZ | ✅ | |

### 3-6. 첨부(선택) — PostgreSQL 확장 시 유리

| 필드명 | PostgreSQL 권장 | 설명 |
|--------|----------------|------|
| attachments | JSONB | `[{ "path": "...", "name": "..." }]` 등. 검색·필터는 2차에서 |

- SQLite: TEXT로 JSON 문자열 저장해도 동일 구조 사용 가능.

---

## 4. PostgreSQL 확장 고려 사항

### 4-1. 타입·호환성

- **날짜/시간:**  
  - SQLite: TEXT ISO8601 권장.  
  - PostgreSQL: `TIMESTAMPTZ` 사용 시 타임존 일관성 유지.  
  - 애플리케이션에서 **동일한 필드명·논리 타입**을 쓰고, 드라이버에서 `date`/`timestamp`만 매핑하면 SQLite↔PostgreSQL 전환 시 스키마 변경 최소화.
- **자동 증가:**  
  - SQLite: `AUTOINCREMENT`.  
  - PostgreSQL: `BIGSERIAL`(또는 `SERIAL`).  
  - 마이그레이션 시 시퀀스 값 맞추기 필요할 수 있음.
- **문자열 길이:**  
  - SQLite는 길이 제한이 없으므로, PostgreSQL에서 `VARCHAR(n)` 제약을 두어도 1차는 넉넉히 잡으면 호환 유지 가능.

### 4-2. 스키마 구조 (PostgreSQL 확장 시)

- **1차:** 기본 `public` 스키마만 사용.  
- **확장(2차):**  
  - 예: `app.work_log`, `app.customer` 등으로 스키마 분리하면, 향후 다중 테넌트·다른 앱과 DB 공유 시 유리.  
  - 스키마가 바뀌어도 **테이블·컬럼명은 동일**하게 두면 코드 변경을 최소화할 수 있음.

### 4-3. 인덱스 (공통 설계, PostgreSQL에서 활용)

| 테이블 | 인덱스 | 용도 |
|--------|--------|------|
| work_log | (source, occurred_at) | 시트별·기간별 조회 |
| work_log | (customer_id, occurred_at) | 고객별 이력 |
| work_log | (status) | 진행상태 필터 |
| work_log | occurred_at | 월별·기간 검색 |
| customer | name (또는 name_trgm) | 고객명 검색 (PostgreSQL: pg_trgm 2차) |
| contact | (customer_id) | 고객별 담당자 |
| equipment | (customer_id) | 고객별 장비 |
| inspection_schedule | (next_due_date), (customer_id) | 점검 예정일·고객별 |

- PostgreSQL 전용 확장:  
  - **전문 검색:** `content`, `title` 등에 `tsvector`+GIN 인덱스(2차).  
  - **유사 검색:** `pg_trgm`으로 고객명/담당자명 검색.

### 4-4. 제약·참조 무결성

- 모든 FK에 `ON DELETE` 정책 명시(예: RESTRICT 또는 SET NULL).  
- SQLite에서도 FK를 활성화하고 동일한 제약을 두면, PostgreSQL 이전 시 동작이 맞춰진다.

---

## 5. 저장 계층: JSON 파일(1차) → PostgreSQL(DBMS 확장 시)

### 5-1. 추상화 원칙 (저장소 독립)

- **저장소에 의존하지 않는 인터페이스:**  
  - “연결 문자열/설정만 바꾸면 JSON(1차) 또는 PostgreSQL(확장 시) 사용”하도록,  
  - **CRUD 및 조회를 리포지토리(또는 서비스) 계층**에서 추상화.  
- **필드명·구조:** JSON 키와 PostgreSQL 컬럼명을 **동일**하게 두면 이관 스크립트가 단순해진다. (예: `WorkLogRepository.load_all()` → JSON이면 파일 읽어 파싱, PostgreSQL이면 SELECT 쿼리.)

### 5-2. 1차: JSON 파일 주 저장 (DBMS 확장 전)

- **DBMS 도입 전까지** 모든 데이터는 **로컬 JSON 파일**로 저장한다.

| 파일명 | 내용 | 배열 키 |
|--------|------|---------|
| `work_log.json` | 처리 이력 통합(NICOM/원콜/세종KLAS) | `items` |
| `customers.json` | 고객 마스터 | `items` |
| `contacts.json` | 고객 담당자 | `items` |
| `equipment.json` | 장비 | `items` |
| `inspection_schedule.json` | 정기 점검 일정 | `items` |

- **공통 포맷:** `{ "items": [ {...}, {...} ], "meta": { "updated_at": "..." } }` (또는 `items`만). 날짜는 ISO8601 문자열. ID는 정수, 새 건은 `max(id)+1`. FK(`customer_id` 등)는 숫자로 저장하고 참조 무결성은 앱에서 검증.
- **유의점:** 단일 사용자/단일 프로세스 전제. 저장 시 전체 파일 쓰기 → 쓰기 직렬화 권장. 건수 매우 많으면 DBMS 전환 권장.

### 5-3. 2차: DBMS(PostgreSQL) 확장 시 전환

- **전환:** JSON 파일의 `items`를 읽어 해당 테이블에 INSERT. ID는 JSON 값 그대로 사용 또는 SERIAL 후 FK 매핑. 설정만 PostgreSQL로 변경.
- **호환:** 테이블/컬럼명과 JSON 키를 동일하게 두었으므로 데이터만 이전하면 되고, 조회/검색/필터/월별 로직은 그대로 재사용 가능.

### 5-4. 엑셀 내보내기

- **1차(JSON):** JSON 파일에서 로드한 데이터를 pandas DataFrame으로 만든 뒤, openpyxl/pandas로 시트별 엑셀 생성.  
- **2차(PostgreSQL):** DB 조회 결과를 DataFrame으로 만든 뒤 동일하게 시트별 엑셀 생성.  
- **시트 매핑:**  
  - NICOM_처리리스트 → `work_log` WHERE source='NICOM'  
  - 원콜/KLAS → `work_log` WHERE source IN ('원콜','세종KLAS')  
  - 유지보수현황 → `customer` + `contact` + `equipment` 조인 후 기존 엑셀 컬럼 순서로 매핑.  
- **월별 시트:**  
  - `work_log` + `inspection_schedule` 기간 조회 후, 날짜별 텍스트 집계하여 달력 시트 생성.

---

## 6. 검색·필터·월별·출력 (설계 유지 + PostgreSQL 활용)

- **검색:**  
  - 기간(`occurred_at`), 고객(`customer_id`), 진행(`status`), 출처(`source`), 내용(`content`/`title` LIKE).  
  - PostgreSQL 2차: `content` 등에 전문검색(tsvector) 적용 시 검색 품질 향상.
- **필터:**  
  - 위 조건을 쿼리 파라미터로 전달, 동적 WHERE 절 구성.  
  - SQLite/PostgreSQL 모두 동일 로직.
- **월별:**  
  - `DATE(occurred_at)` 또는 `date_trunc('month', occurred_at)`로 월 단위 그룹.  
  - “이번 달 일지” / “특정 월” 조회·엑셀 출력.
- **출력:**  
  - 목록(페이징·정렬) + 시트별 엑셀 + 월별 달력 뷰 엑셀.  
  - 데이터는 모두 위 테이블에서만 조회.

---

## 7. 데이터 구조 요약 표 (PostgreSQL 확장 반영)

| 테이블 | PK | 주요 필드 | 비고 |
|--------|-----|-----------|------|
| **work_log** | id (BIGSERIAL) | source, occurred_at, processed_at, customer_id, contact_id, category, title, content, received_content, handled_content, reply, status, equipment_*, billing_type, remarks, created_at, updated_at | NICOM/원콜/세종KLAS 통합 |
| **customer** | id | name, division, address, phone, remarks, created_at, updated_at | 고객명·구분 |
| **contact** | id | customer_id, name, phone_office, phone_mobile, email, remarks, created_at, updated_at | 고객 담당자 |
| **equipment** | id | customer_id, name, product_category, quantity, location, delivery_year, inspection_interval, billing_type, billing_entity, billing_interval, remarks, created_at, updated_at | 장비·점검주기·유무상 |
| **inspection_schedule** | id | customer_id, equipment_id, next_due_date, interval_type, assignee_notes, remarks, created_at, updated_at | 다음 점검 예정일 |

- **저장:** 1차 **JSON 파일**(DBMS 확장 전), 확장 시 **PostgreSQL**.  
- **엑셀:** 위 테이블 기준 시트별·월별 내보내기로 실무 사용성 및 기존 양식 호환을 유지.

이 문서를 엑셀 분석 결과 반영 + PostgreSQL 확장까지 포함한 **데이터 처리 설계** 기준으로 사용하면 된다.
