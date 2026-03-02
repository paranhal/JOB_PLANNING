# -*- coding: utf-8 -*-
"""JSON 파일 공통: { "items": [ ... ] } 포맷 로드/저장."""
import json
import os
from datetime import datetime
from .. import config


def ensure_data_dir():
    os.makedirs(config.DATA_DIR, exist_ok=True)


def load_items(file_path: str) -> list:
    ensure_data_dir()
    if not os.path.exists(file_path):
        return []
    with open(file_path, "r", encoding="utf-8") as f:
        data = json.load(f)
    return data.get("items", data) if isinstance(data, dict) else data


def save_items(file_path: str, items: list) -> None:
    ensure_data_dir()
    payload = {"items": items, "meta": {"updated_at": datetime.now().strftime("%Y-%m-%dT%H:%M:%S")}}
    with open(file_path, "w", encoding="utf-8") as f:
        json.dump(payload, f, ensure_ascii=False, indent=2)


def next_id(items: list) -> int:
    if not items:
        return 1
    return max((x.get("id", 0) for x in items), default=0) + 1
