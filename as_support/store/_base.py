# 저장소 공통: JSON load/save, ID 생성
# 데이터_처리_설계_PostgreSQL확장.md 필드명 유지

import json
import os
import uuid
from datetime import datetime

from as_support.config import DATA_DIR, ensure_data_dir

def _path(filename: str) -> str:
    ensure_data_dir()
    return os.path.join(DATA_DIR, filename)

def load_json(filename: str) -> list:
    p = _path(filename)
    if not os.path.exists(p):
        return []
    with open(p, "r", encoding="utf-8") as f:
        data = json.load(f)
    return data if isinstance(data, list) else data.get("items", [])

def save_json(filename: str, items: list) -> None:
    p = _path(filename)
    ensure_data_dir()
    with open(p, "w", encoding="utf-8") as f:
        json.dump({"items": items, "meta": {"updated_at": datetime.now().isoformat()}}, f, ensure_ascii=False, indent=2)

def next_id(prefix: str = "") -> str:
    return f"{prefix}{uuid.uuid4().hex[:12]}" if prefix else uuid.uuid4().hex

def now_iso() -> str:
    return datetime.now().isoformat()

def date_iso(d) -> str | None:
    if d is None:
        return None
    if hasattr(d, "isoformat"):
        return d.isoformat()[:10]
    return str(d)[:10]
