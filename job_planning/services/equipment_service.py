# -*- coding: utf-8 -*-
"""장비(equipment) 서비스 — 구현_탭_파일구조_설계.md §4-3."""
from typing import Optional

from ..store import equipment as store


def create(data: dict) -> Optional[int]:
    name = (data.get("name") or "").strip()
    customer_id = data.get("customer_id")
    if not name or customer_id is None:
        return None
    row = {
        "customer_id": int(customer_id),
        "name": name,
        "product_category": (data.get("product_category") or "").strip() or None,
        "quantity": data.get("quantity") if data.get("quantity") is not None else 1,
        "location": (data.get("location") or "").strip() or None,
        "delivery_year": (data.get("delivery_year") or "").strip() or None,
        "inspection_interval": (data.get("inspection_interval") or "").strip() or None,
        "billing_type": (data.get("billing_type") or "").strip() or None,
        "billing_entity": (data.get("billing_entity") or "").strip() or None,
        "billing_interval": (data.get("billing_interval") or "").strip() or None,
        "remarks": (data.get("remarks") or "").strip() or None,
    }
    return store.create(row)


def get(id: int) -> Optional[dict]:
    return store.get_by_id(id)


def list_all():
    return store.list_()


def list_by_customer(customer_id: int):
    return store.list_by_customer(customer_id)


def update(id: int, data: dict) -> bool:
    row = store.get_by_id(id)
    if not row:
        return False
    name = (data.get("name") or "").strip()
    if not name:
        return False
    row.update({
        "customer_id": data.get("customer_id") if data.get("customer_id") is not None else row.get("customer_id"),
        "name": name,
        "product_category": (data.get("product_category") or "").strip() or None,
        "quantity": data.get("quantity") if data.get("quantity") is not None else row.get("quantity", 1),
        "location": (data.get("location") or "").strip() or None,
        "delivery_year": (data.get("delivery_year") or "").strip() or None,
        "inspection_interval": (data.get("inspection_interval") or "").strip() or None,
        "billing_type": (data.get("billing_type") or "").strip() or None,
        "billing_entity": (data.get("billing_entity") or "").strip() or None,
        "billing_interval": (data.get("billing_interval") or "").strip() or None,
        "remarks": (data.get("remarks") or "").strip() or None,
    })
    return store.update(id, row)


def delete(id: int) -> bool:
    return store.delete(id)
