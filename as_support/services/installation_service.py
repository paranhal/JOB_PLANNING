# 설치자산 서비스 — 기획서 §6

from as_support.store import installation, installation_sw

def list_by_customer(customer_id: str):
    return installation.list_by_customer(customer_id)

def get(installation_id: str):
    return installation.find_by_id(installation_id)

def add(data: dict):
    return installation.add(data)

def update(installation_id: str, data: dict):
    return installation.update(installation_id, data)

def list_sw(installation_id: str):
    return installation_sw.list_by_installation(installation_id)

def add_sw(data: dict):
    return installation_sw.add(data)

def update_sw(sw_detail_id: str, data: dict):
    return installation_sw.update(sw_detail_id, data)

def search(keyword: str):
    """제품명, S/N 검색 (§15)"""
    kw = (keyword or "").strip().lower()
    if not kw:
        return installation.load_all()
    return [r for r in installation.load_all() if kw in (r.get("product_name") or "").lower() or kw in (r.get("serial_number") or "").lower()]
