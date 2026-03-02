# -*- coding: utf-8 -*-
"""처리 이력(work_log) 비즈니스 로직 — NICOM/원콜/세종KLAS 통합."""
from typing import Optional

from .. import config
from ..store import work_log as store


def create(data: dict) -> Optional[int]:
    """처리 이력 생성. source, occurred_at, content, status 필수."""
    source = (data.get("source") or "").strip()
    occurred_at = (data.get("occurred_at") or "").strip()
    content = (data.get("content") or "").strip()
    status = (data.get("status") or "").strip()
    if not source or not occurred_at or not content or not status:
        return None
    if source not in config.SOURCES:
        return None
    if status not in config.STATUS_LIST:
        status = config.STATUS_LIST[0] if config.STATUS_LIST else "진행중"

    row = {
        "source": source,
        "occurred_at": occurred_at,
        "content": content,
        "status": status,
        "customer_id": data.get("customer_id") or None,
        "contact_id": data.get("contact_id") or None,
        "processed_at": (data.get("processed_at") or "").strip() or None,
        "category": (data.get("category") or "").strip() or None,
        "title": (data.get("title") or "").strip() or None,
        "received_content": (data.get("received_content") or "").strip() or None,
        "handled_content": (data.get("handled_content") or "").strip() or None,
        "reply": (data.get("reply") or "").strip() or None,
        "equipment_name": (data.get("equipment_name") or "").strip() or None,
        "equipment_id": data.get("equipment_id") or None,
        "equipment_location": (data.get("equipment_location") or "").strip() or None,
        "billing_type": (data.get("billing_type") or "").strip() or None,
        "remarks": (data.get("remarks") or "").strip() or None,
    }
    return store.create(row)


def get(id: int) -> Optional[dict]:
    return store.get_by_id(id)


def list_(source=None, date_from=None, date_to=None, customer_id=None, status=None):
    return store.list_(
        source=source,
        date_from=date_from,
        date_to=date_to,
        customer_id=customer_id,
        status=status,
    )


def update(id: int, data: dict) -> bool:
    row = store.get_by_id(id)
    if not row:
        return False
    source = (data.get("source") or "").strip()
    occurred_at = (data.get("occurred_at") or "").strip()
    content = (data.get("content") or "").strip()
    status = (data.get("status") or "").strip()
    if not source or not occurred_at or not content or not status:
        return False
    if source not in config.SOURCES:
        return False

    row.update({
        "source": source,
        "occurred_at": occurred_at,
        "content": content,
        "status": status,
        "customer_id": data.get("customer_id") or None,
        "contact_id": data.get("contact_id") or None,
        "processed_at": (data.get("processed_at") or "").strip() or None,
        "category": (data.get("category") or "").strip() or None,
        "title": (data.get("title") or "").strip() or None,
        "received_content": (data.get("received_content") or "").strip() or None,
        "handled_content": (data.get("handled_content") or "").strip() or None,
        "reply": (data.get("reply") or "").strip() or None,
        "equipment_name": (data.get("equipment_name") or "").strip() or None,
        "equipment_id": data.get("equipment_id") or None,
        "equipment_location": (data.get("equipment_location") or "").strip() or None,
        "billing_type": (data.get("billing_type") or "").strip() or None,
        "remarks": (data.get("remarks") or "").strip() or None,
    })
    return store.update(id, row)


def delete(id: int) -> bool:
    return store.delete(id)
