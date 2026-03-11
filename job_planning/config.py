# -*- coding: utf-8 -*-
"""경로·기본 설정 — 구현_탭_파일구조_설계.md §0·§3 반영."""
import os

# 프로젝트 루트 = 이 패키지의 부모 디렉터리
_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
DATA_DIR = os.path.join(_ROOT, "data")

# JSON 파일 경로
WORK_LOG_PATH = os.path.join(DATA_DIR, "work_log.json")
CUSTOMERS_PATH = os.path.join(DATA_DIR, "customers.json")
CONTACTS_PATH = os.path.join(DATA_DIR, "contacts.json")
EQUIPMENT_PATH = os.path.join(DATA_DIR, "equipment.json")
INSPECTION_SCHEDULE_PATH = os.path.join(DATA_DIR, "inspection_schedule.json")

# 구분(제조사·품목·지역) — 통합 처리 이력 source, 확장 가능
SOURCES = ("NICOM", "원콜", "세종KLAS")

# 접수 방법 (유선/대면/메일/기타/원콜센터 — 기획 에이전트 규칙·기획서 §2-1)
RECEPTION_METHODS = ("유선", "대면", "메일", "기타", "원콜센터")

# 원콜/세종KLAS 분류 (category)
CATEGORIES = ("메일문의", "전화문의", "홈페이지", "정기방문", "현장방문", "기타")

# 진행 상태 (status) — 접수대장 현재 상태
STATUS_LIST = ("진행중", "완료", "처리완료", "보류")

# 유/무상 (billing_type) — 장비·유지보수 등급
BILLING_TYPES = ("유상", "무상")
