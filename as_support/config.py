# 고객지원 시스템 — 설정
# 기준: 데이터_처리_설계_PostgreSQL확장.md, 기획서 §11 코드관리

import os

# 프로젝트 루트 기준 데이터 디렉터리
_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
DATA_DIR = os.environ.get("AS_SUPPORT_DATA_DIR", os.path.join(_ROOT, "data"))

def ensure_data_dir():
    os.makedirs(DATA_DIR, exist_ok=True)

# 코드 그룹 (기획서 §11)
CODE_GROUPS = [
    "industry",           # 업종
    "duty",               # 담당업무
    "product_type",       # 제품구분
    "install_owner",      # 설치주체
    "management_type",    # 관리유형
    "request_owner_type", # 요청주체유형
    "as_status",          # AS상태
    "cause",              # 원인분류
    "action_type",        # 처리유형
    "operation_status",   # 운영상태
    "building_type",      # 건물구분
    "room_use",           # 실용도
]

# 코드 그룹 표시명
CODE_GROUP_LABELS = {
    "industry": "업종",
    "duty": "담당업무",
    "product_type": "제품구분",
    "install_owner": "설치주체",
    "management_type": "관리유형",
    "request_owner_type": "요청주체유형",
    "as_status": "AS상태",
    "cause": "원인분류",
    "action_type": "처리유형",
    "operation_status": "운영상태",
    "building_type": "건물구분",
    "room_use": "실용도",
}

# 기본 코드값 (기획서 §11 예시)
DEFAULT_CODES = {
    "industry": ["도서관", "학교", "공공기관", "민간"],
    "duty": ["전산", "네트워크", "도서관리시스템", "행정시스템"],
    "product_type": ["SW", "HW", "서버", "네트워크장비", "주변장비"],
    "install_owner": ["자사", "타사", "제조사", "협력사", "미상"],
    "management_type": ["직접유지보수", "장애대응", "정기점검", "요청시지원", "참고관리"],
    "request_owner_type": ["고객직접", "제조사", "협력사", "원청", "내부"],
    "as_status": ["접수", "진행중", "보류", "완료", "종료"],
    "cause": ["HW고장", "SW오류", "네트워크", "환경문제", "사용자오류"],
    "action_type": ["원격지원", "방문", "교체", "설정변경", "문의응대"],
    "operation_status": ["운영중", "점검중", "장애", "철수", "폐기"],
    "building_type": ["본관", "별관", "기타"],
    "room_use": ["전산실", "자료실", "서버실", "기타"],
}
