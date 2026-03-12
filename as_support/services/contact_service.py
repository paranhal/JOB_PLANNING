# 담당자 서비스 — 기획서 §5.3

from as_support.store import contact

def list_by_customer(customer_id: str, in_office_only: bool = True):
    return contact.list_by_customer(customer_id, in_office_only=in_office_only)

def get(contact_id: str):
    return contact.find_by_id(contact_id)

def add(data: dict):
    return contact.add(data)

def update(contact_id: str, data: dict):
    return contact.update(contact_id, data)

def search(keyword: str):
    """담당자명 검색 (§15)"""
    kw = (keyword or "").strip().lower()
    if not kw:
        return contact.load_all()
    return [r for r in contact.load_all() if kw in (r.get("name") or "").lower()]
