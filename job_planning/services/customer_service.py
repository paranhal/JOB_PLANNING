# -*- coding: utf-8 -*-
"""통합 고객(customer) 서비스 — 구현_탭_파일구조_설계.md §4-1."""
from typing import Optional

from ..store import customer as store


def create(data: dict) -> Optional[int]:
    name = (data.get("name") or "").strip()
    if not name:
        return None
    row = {
        "name": name,
        "division": (data.get("division") or "").strip() or None,
        "address": (data.get("address") or "").strip() or None,
        "phone": (data.get("phone") or "").strip() or None,
        "remarks": (data.get("remarks") or "").strip() or None,
    }
    return store.create(row)


def get(id: int) -> Optional[dict]:
    return store.get_by_id(id)


def list_(division=None):
    return store.list_(division=division)


def update(id: int, data: dict) -> bool:
    row = store.get_by_id(id)
    if not row:
        return False
    name = (data.get("name") or "").strip()
    if not name:
        return False
    row["name"] = name
    row["division"] = (data.get("division") or "").strip() or None
    row["phone"] = (data.get("phone") or "").strip() or None
    if "address" in data:
        row["address"] = (data.get("address") or "").strip() or None
    if "remarks" in data:
        row["remarks"] = (data.get("remarks") or "").strip() or None
    return store.update(id, row)


def delete(id: int) -> bool:
    return store.delete(id)
