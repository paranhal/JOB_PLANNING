# 업무일지 프로그램

엑셀 업무일지(유지보수현황·처리리스트)를 대체·보완하는 파이썬 실행형 프로그램.  
**기획·데이터 설계:** `기획_업무일지_프로그램_요구사항.md`, `데이터_처리_설계_PostgreSQL확장.md` 반영.

## 기능 요약

- **처리 이력 (work_log)**  
  NICOM 처리리스트 / 원콜·세종 KLAS 처리 이력을 하나의 목록으로 통합.  
  출처(source): NICOM, 원콜, 세종KLAS. 기간·출처별 조회, 수정·삭제.
- **고객·담당자·장비 마스터**  
  고객(customer), 담당자(contact), 장비(equipment) 목록 입력·수정·삭제.  
  처리 이력에서 고객/담당자 선택 시 마스터와 연동.

## 설치

```bash
pip install -r requirements.txt
```

## 실행

프로젝트 루트(`JOB_PLANNING`)에서:

```bash
python3 -m job_planning.main
```

## 데이터 저장 (JSON, 1차)

- **위치:** 프로젝트 루트 `data/` (실행 시 자동 생성)
- **파일:**  
  `work_log.json`, `customers.json`, `contacts.json`, `equipment.json`  
  포맷: `{ "items": [ ... ], "meta": { "updated_at": "..." } }`
- **확장:** DBMS(PostgreSQL) 전환 시 동일 필드명으로 이관 가능 (데이터 설계 문서 참고).

## 프로젝트 구조

- `job_planning/config.py` — 데이터 경로, SOURCES/CATEGORIES/STATUS 등
- `job_planning/store/` — work_log, customer, contact, equipment JSON CRUD
- `job_planning/services/` — work_log_service, customer_service, contact_service, equipment_service
- `job_planning/ui/` — PySide6: 처리 이력 탭, 고객·담당자·장비 탭
