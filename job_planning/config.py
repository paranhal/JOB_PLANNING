# -*- coding: utf-8 -*-
"""경로·기본 설정 (JSON 저장, 엑셀 분석·PostgreSQL 확장 설계 반영)."""
import os

# 프로젝트 루트 = 이 패키지의 부모 디렉터리
_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
DATA_DIR = os.path.join(_ROOT, "data")

# JSON 파일 경로 (데이터 설계 §5-2)
WORK_LOG_PATH = os.path.join(DATA_DIR, "work_log.json")
CUSTOMERS_PATH = os.path.join(DATA_DIR, "customers.json")
CONTACTS_PATH = os.path.join(DATA_DIR, "contacts.json")
EQUIPMENT_PATH = os.path.join(DATA_DIR, "equipment.json")
INSPECTION_SCHEDULE_PATH = os.path.join(DATA_DIR, "inspection_schedule.json")

# 처리 이력 출처 (work_log.source)
SOURCES = ("NICOM", "원콜", "세종KLAS")

# 원콜/세종KLAS 분류 (category)
CATEGORIES = ("메일문의", "전화문의", "홈페이지", "정기방문", "현장방문", "기타")

# 진행 여부 (status)
STATUS_LIST = ("진행중", "완료", "처리완료", "보류")

# 유/무상 (billing_type)
BILLING_TYPES = ("유상", "무상")
