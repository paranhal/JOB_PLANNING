# -*- coding: utf-8 -*-
"""고객(customer) JSON 저장소."""
from datetime import datetime
from typing import Optional

from .. import config
from ._json_store import load_items, save_items, next_id


def _load() -> list:
    return load_items(config.CUSTOMERS_PATH)


def _save(rows: list) -> None:
    save_items(config.CUSTOMERS_PATH, rows)


def create(row: dict) -> int:
    now = datetime.now().strftime("%Y-%m-%dT%H:%M:%S")
    row = dict(row)
    row.setdefault("created_at", now)
    row.setdefault("updated_at", now)
    rows = _load()
    row["id"] = next_id(rows)
    rows.append(row)
    _save(rows)
    return row["id"]


def get_by_id(id: int) -> Optional[dict]:
    for r in _load():
        if r.get("id") == id:
            return dict(r)
    return None


def list_(division=None, limit=1000) -> list:
    rows = _load()
    if division:
        rows = [r for r in rows if r.get("division") == division]
    rows.sort(key=lambda r: (r.get("name") or "").lower())
    return rows[:limit]


def update(id: int, row: dict) -> bool:
    row = dict(row)
    row["updated_at"] = datetime.now().strftime("%Y-%m-%dT%H:%M:%S")
    rows = _load()
    for i, r in enumerate(rows):
        if r.get("id") == id:
            rows[i] = {**r, **row, "id": id}
            _save(rows)
            return True
    return False


def delete(id: int) -> bool:
    rows = _load()
    new_rows = [r for r in rows if r.get("id") != id]
    if len(new_rows) == len(rows):
        return False
    _save(new_rows)
    return True
