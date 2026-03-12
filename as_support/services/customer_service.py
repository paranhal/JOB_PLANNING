# 고객(기관) 서비스 — 기획서 §5.1, §14 중복 방지

from as_support.store import customer

def list_all(use_yn=None):
    if use_yn is None:
        return customer.load_all()
    return customer.list_use(use_yn=use_yn)

def get(customer_id: str):
    return customer.find_by_id(customer_id)

def add(data: dict):
    bn = (data.get("business_number") or "").strip()
    if bn:
        for r in customer.load_all():
            if (r.get("business_number") or "").strip() == bn:
                raise ValueError("동일 사업자번호가 이미 등록되어 있습니다.")
    return customer.add(data)

def update(customer_id: str, data: dict):
    return customer.update(customer_id, data)

def search(keyword: str):
    """기관명, 공식명칭 검색 (§15)"""
    kw = (keyword or "").strip().lower()
    if not kw:
        return list_all()
    return [r for r in customer.load_all() if kw in (r.get("name") or "").lower() or kw in (r.get("official_name") or "").lower()]
