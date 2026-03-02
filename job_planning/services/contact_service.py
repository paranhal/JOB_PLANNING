# -*- coding: utf-8 -*-
"""고객 담당자(contact) 서비스."""
from typing import Optional

from ..store import contact as store


def create(data: dict) -> Optional[int]:
    name = (data.get("name") or "").strip()
    customer_id = data.get("customer_id")
    if not name or customer_id is None:
        return None
    row = {
        "customer_id": int(customer_id),
        "name": name,
        "phone_office": (data.get("phone_office") or "").strip() or None,
        "phone_mobile": (data.get("phone_mobile") or "").strip() or None,
        "email": (data.get("email") or "").strip() or None,
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
        "phone_office": (data.get("phone_office") or "").strip() or None,
        "phone_mobile": (data.get("phone_mobile") or "").strip() or None,
        "email": (data.get("email") or "").strip() or None,
        "remarks": (data.get("remarks") or "").strip() or None,
    })
    return store.update(id, row)


def delete(id: int) -> bool:
    return store.delete(id)
