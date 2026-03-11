# -*- coding: utf-8 -*-
"""통합 처리 이력(work_log) JSON 저장소 — 접수·조치·접수대장 (구현_탭_파일구조_설계.md §3)."""
from datetime import datetime
from typing import Optional

from .. import config
from ._json_store import load_items, save_items, next_id


def _load() -> list:
    return load_items(config.WORK_LOG_PATH)


def _save(rows: list) -> None:
    save_items(config.WORK_LOG_PATH, rows)


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


def list_(
    source=None,
    date_from=None,
    date_to=None,
    customer_id=None,
    status=None,
    reception_method=None,
    limit=500,
) -> list:
    rows = _load()
    if source:
        rows = [r for r in rows if r.get("source") == source]
    if date_from:
        rows = [r for r in rows if (r.get("occurred_at") or "")[:10] >= date_from]
    if date_to:
        rows = [r for r in rows if (r.get("occurred_at") or "")[:10] <= date_to]
    if customer_id is not None:
        rows = [r for r in rows if r.get("customer_id") == customer_id]
    if status:
        rows = [r for r in rows if r.get("status") == status]
    if reception_method:
        rows = [r for r in rows if r.get("reception_method") == reception_method]
    rows.sort(key=lambda r: (r.get("occurred_at") or "") + (r.get("updated_at") or ""), reverse=True)
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
