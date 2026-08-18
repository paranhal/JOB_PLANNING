# 영업관리 연동 API 설명서

고객지원 시스템(본 서버)의 **고객(기관) 마스터**와 **고객 담당자**를 영업관리 시스템이 조회할 때 쓰는 읽기 전용 API입니다.

기준: 기획서 v2 §18 고객 마스터 · §20 담당자 정보.

---

## 1. 용어 — 담당자가 둘입니다

이 시스템에는 이름이 비슷한 정보가 두 종류 있습니다. **영업관리에서 가져갈 대상은 ①입니다.**

|구분|이 API|설명|식별자|
|-|-|-|-|
|**① 고객 담당자**|`contacts`|도서관·학교 등 **기관 쪽 사람** (사서, 전산담당)|`contact_id`|
|② 수행 담당자|제공하지 않음|우리 회사 기술·영업 직원 (`users`)|`user_id`|

기관의 기본 키는 **`customer_id`** 입니다. 기관명(`org_name`)은 바뀔 수 있으므로 동기화 키로 쓰지 마세요.

---

## 2. 접속

|항목|값|
|-|-|
|Base URL (개발)|`http://localhost:8080`|
|Base URL (운영)|실제 서버 주소 (포트 확인)|
|인증|`X-API-Key: {키}` 또는 `Authorization: Bearer {키}`|
|키 설정|서버 `INTEGRATION_API_KEY` 환경변수 (`server/.env`)|
|메서드|**GET만**. 등록·수정·삭제는 없습니다|
|형식|`application/json; charset=utf-8`|

키가 비어 있으면 `503`, 틀리면 `401`입니다.

```
GET /api/v1/customers?is_active=true
X-API-Key: (담당자에게 받은 키)
```

```bash
curl -sS -H "X-API-Key: $INTEGRATION_API_KEY" \
  "http://localhost:8080/api/v1/customers?page=1&page_size=100&is_active=true"
```

---

## 3. 공통 규칙

### 3.1 성공 (목록)

```json
{
  "ok": true,
  "total": 412,
  "page": 1,
  "page_size": 100,
  "items": [ ]
}
```

- `page` 기본 1, `page_size` 기본 100, 최대 500
- `total`은 필터 적용 후 전체 건수입니다. `total`을 넘을 때까지 `page`를 올립니다.

### 3.2 성공 (단건)

```json
{ "ok": true, "item": { } }
```

### 3.3 실패

|HTTP|상황|
|-|-|
|401|키 없음·불일치|
|404|해당 ID 없음|
|503|서버에 `INTEGRATION_API_KEY` 미설정|

```json
{ "ok": false, "error": "고객을 찾을 수 없습니다" }
```

날짜·시각은 JSON 기본값(RFC3339)입니다. 담당자의 부임일·퇴임일은 **문자열** `YYYY-MM-DD` 또는 빈 문자열입니다.

---

## 4. 엔드포인트

### 4.1 고객 목록

`GET /api/v1/customers`

|쿼리|설명|
|-|-|
|`q`|기관명 · 공식명칭 · 고객ID · 사업자번호 부분 검색|
|`is_active`|`true`/`1` 사용 중만, `false`/`0` 미사용만. 생략 시 전부|
|`page`, `page_size`|페이징|

### 4.2 고객 단건

`GET /api/v1/customers/{customer_id}`

### 4.3 한 기관의 담당자

`GET /api/v1/customers/{customer_id}/contacts`

|쿼리|설명|
|-|-|
|`status`|`active`(재직) · `transferred`(전보) · `resigned`(퇴직). 생략 시 전부|
|`q`|이름·전화·이메일 검색|
|`page`, `page_size`|페이징|

응답에 `customer_id`, `org_name`이 함께 붙습니다.

### 4.4 담당자 전체

`GET /api/v1/contacts`

|쿼리|설명|
|-|-|
|`customer_id`|특정 기관만|
|`status`|재직상태|
|`q`|이름 · 기관명 · 전화 · 이메일|
|`page`, `page_size`|페이징|

### 4.5 담당자 단건

`GET /api/v1/contacts/{contact_id}`

### 4.6 코드(콤보 값)

`GET /api/v1/codes`  
`GET /api/v1/codes?group=industry`

|group|내용|
|-|-|
|`industry`|업종|
|`contact_affiliation`|담당자 소속|

---

## 5. 필드

### 5.1 고객 (`customers`)

기획서 §18.

|JSON 키|의미|필수 동기화|비고|
|-|-|-|-|
|`customer_id`|고객 ID|**키**|불투명 문자열. 예: 지역번호 기반 ID|
|`org_name`|기관명(실무 명칭)|Y||
|`official_name`|공식명칭(계약·공문)|Y||
|`org_email`|기관 이메일|||
|`main_phone`|대표전화|권장|고객번호 지역코드에도 사용|
|`website`|홈페이지|||
|`business_number`|사업자번호|권장|중복 기관 판별|
|`representative`|대표자|||
|`industry`|업종|Y|화면에 보이는 값(예: `도서관`). 코드 그룹 `industry` 참고|
|`has_parent`|상위기관 여부|||
|`parent_customer_id`|상위기관 ID||본청–산하기관. 없으면 `""`|
|`postal_code`|우편번호|||
|`addr_sido`|시도|||
|`addr_sigungu`|시군구|||
|`addr_dong`|동읍면|||
|`address`|조합 주소(표시·검색용)|||
|`address_detail`|상세주소|||
|`is_active`|사용 여부|Y|`false`면 통합·폐쇄·휴면 등|
|`notes`|비고|||
|`needs_review`|확인 필요 표식||외부 반입 후 사람이 볼 기관|
|`review_reason`|확인 필요 사유|||
|`created_at`|등록 시각|||
|`updated_at`|수정 시각||증분 동기화에 사용 가능|

목록의 `as_count` / `asset_count`는 이 API에 넣지 않습니다. AS·자산은 영업 범위가 아닙니다.

### 5.2 담당자 (`contacts`)

기획서 §20.1. **기관 쪽 사람**입니다.

|JSON 키|의미|값|
|-|-|-|
|`contact_id`|담당자 ID|동기화 키|
|`customer_id`|소속 기관 ID|고객과 연결|
|`org_name`|기관명|JOIN. 단건·목록에 포함|
|`full_name`|이름||
|`affiliation`|소속|`institution` 소속기관 · `integrator` 통합사업자 · `partner` 협력업체 · `other` 기타|
|`job_role`|담당업무|전산, 도서관시스템 등|
|`title`|직함|과장, 사서 등|
|`job_grade`|직급|`librarian` 사서직 · `it` 전산직 · `other` 기타 · 또는 직접입력 문구|
|`phone`|전화번호||
|`mobile`|핸드폰||
|`email`|이메일||
|`start_date`|부임일|`YYYY-MM-DD` 또는 `""`|
|`end_date`|퇴임일|`YYYY-MM-DD` 또는 `""`|
|`status`|재직상태|`active` 재직 · `transferred` 전보 · `resigned` 퇴직|
|`contact_role`|담당구분|`primary` 주담당 · `secondary` 부담당 · `regular` 일반|
|`is_primary`|주담당 여부|`contact_role == primary` 와 같음|
|`notes`|비고||

한 기관에 담당자가 여러 명일 수 있습니다. 주담당은 `contact_role=primary` 또는 `is_primary=true`로 고릅니다.

---

## 6. 응답 예시

### 고객 단건

```json
{
  "ok": true,
  "item": {
    "customer_id": "02-0001",
    "org_name": "테스트도서관",
    "official_name": "테스트시도립도서관",
    "org_email": "lib@example.go.kr",
    "main_phone": "02-111-2222",
    "website": "",
    "business_number": "123-45-67890",
    "representative": "",
    "industry": "도서관",
    "has_parent": false,
    "parent_customer_id": "",
    "postal_code": "04524",
    "addr_sido": "서울특별시",
    "addr_sigungu": "중구",
    "addr_dong": "",
    "address": "서울특별시 중구",
    "address_detail": "세종대로 110",
    "is_active": true,
    "notes": "",
    "needs_review": false,
    "review_reason": "",
    "created_at": "2026-03-12T10:00:00+09:00",
    "updated_at": "2026-08-18T11:30:00+09:00"
  }
}
```

### 담당자 (목록 한 건)

```json
{
  "contact_id": "CONT-…",
  "customer_id": "02-0001",
  "full_name": "김사서",
  "affiliation": "institution",
  "job_role": "도서관시스템",
  "title": "주무관",
  "job_grade": "librarian",
  "phone": "02-111-0000",
  "mobile": "010-1234-5678",
  "email": "kim@example.go.kr",
  "start_date": "2023-03-01",
  "end_date": "",
  "status": "active",
  "contact_role": "primary",
  "is_primary": true,
  "notes": "",
  "org_name": "테스트도서관"
}
```

---

## 7. 권장 동기화

1. **최초:** `GET /api/v1/customers`를 페이지 끝까지 받고, 각 기관에 대해 `GET /api/v1/customers/{id}/contacts?status=active` 또는 `GET /api/v1/contacts?status=active` 한 번에 받습니다.
2. **이후:** 고객은 `updated_at`이 있습니다. 담당자 테이블에는 수정 시각 컬럼이 없으므로, 담당자는 **재직(`active`) 목록을 주기적으로 다시 받는 것**을 권장합니다.
3. `is_active=false` 고객 · `status`가 `transferred`/`resigned`인 담당자는 영업 화면에서 숨기거나 이력으로만 둡니다.
4. 쓰기는 하지 마세요. 원천은 고객지원 시스템의 기준정보 화면입니다.

---

## 8. 이 서버의 다른 `/api` 와 구분

화면용 API(`GET /api/contacts/:customer_id` 등)는 **로그인 쿠키(JWT)** 가 필요합니다. 영업관리 서버에서는 쓰지 마세요. 연동은 반드시 **`/api/v1/...` + API 키**만 사용합니다.

---

## 9. 운영 쪽 설정

1. `server/.env`에 키를 넣습니다. Git에 올리지 않습니다.

```
INTEGRATION_API_KEY=발급한-긴-임의문자열
```

2. 서버를 재기동합니다 (`docker compose up -d --build`).
3. 키는 영업관리 담당자에게만 전달합니다.

키가 없으면 `/api/v1/customers`가 `503` · `"INTEGRATION_API_KEY 가 설정되지 않았습니다"`를 반환합니다.
