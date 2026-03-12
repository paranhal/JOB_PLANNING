# 담당자 이력 서비스 — 기획서 §5.4

from as_support.store import contact_history

def list_by_contact(contact_id: str):
    return contact_history.list_by_contact(contact_id)

def list_by_customer(customer_id: str):
    return contact_history.list_by_customer(customer_id)

def add(data: dict):
    return contact_history.add(data)
