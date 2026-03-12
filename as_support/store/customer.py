# 고객(기관) 마스터 저장소 — 기획서 §5.1, 데이터_처리_설계

from as_support.store._base import load_json, save_json, next_id, now_iso

FILENAME = "customer_master.json"

def _photo_urls(v, size: int) -> list:
    if isinstance(v, list):
        return [str(x).strip() if x else "" for x in (v + [""] * size)[:size]]
    return [""] * size

def load_all() -> list:
    return load_json(FILENAME)

def save_all(items: list) -> None:
    save_json(FILENAME, items)

def find_by_id(customer_id: str) -> dict | None:
    for r in load_all():
        if r.get("customer_id") == customer_id:
            if "photo_urls" not in r:
                r["photo_urls"] = [""] * 4
            return r
    return None

def add(data: dict) -> dict:
    items = load_all()
    cid = data.get("customer_id") or next_id("C")
    row = {
        "customer_id": cid,
        "name": data.get("name", "").strip(),
        "official_name": data.get("official_name", "").strip(),
        "email": (data.get("email") or "").strip(),
        "phone": (data.get("phone") or "").strip(),
        "homepage": (data.get("homepage") or "").strip(),
        "business_number": (data.get("business_number") or "").strip(),
        "representative": (data.get("representative") or "").strip(),
        "industry_code": (data.get("industry_code") or "").strip(),
        "parent_customer_id": data.get("parent_customer_id") or None,
        "address": (data.get("address") or "").strip(),
        "address_detail": (data.get("address_detail") or "").strip(),
        "use_yn": data.get("use_yn", True),
        "remarks": (data.get("remarks") or "").strip(),
        "photo_urls": _photo_urls(data.get("photo_urls"), 4),
        "created_at": now_iso(),
        "updated_at": now_iso(),
    }
    items.append(row)
    save_all(items)
    return row

ALLOWED_UPDATE_KEYS = {
    "name", "official_name", "email", "phone", "homepage", "business_number",
    "representative", "industry_code", "parent_customer_id", "address", "address_detail",
    "use_yn", "remarks", "photo_urls",
}

def update(customer_id: str, data: dict) -> None:
    items = load_all()
    for r in items:
        if r.get("customer_id") == customer_id:
            for k, v in data.items():
                if k in ALLOWED_UPDATE_KEYS and k not in ("customer_id", "created_at"):
                    if k == "photo_urls":
                        r["photo_urls"] = _photo_urls(v, 4)
                    else:
                        r[k] = v
            r["updated_at"] = now_iso()
            save_all(items)
            return
    raise ValueError(f"customer not found: {customer_id}")

def list_use(use_yn: bool = True) -> list:
    items = load_all()
    if use_yn is not None:
        items = [r for r in items if r.get("use_yn", True) == use_yn]
    return sorted(items, key=lambda r: (r.get("name") or "", r.get("customer_id") or ""))
