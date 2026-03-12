# 고객건물 저장소 — 기획서 §5.2

from as_support.store._base import load_json, save_json, next_id, now_iso

FILENAME = "building.json"

def load_all() -> list:
    return load_json(FILENAME)

def save_all(items: list) -> None:
    save_json(FILENAME, items)

def find_by_id(building_id: str) -> dict | None:
    for r in load_all():
        if r.get("building_id") == building_id:
            return r
    return None

def list_by_customer(customer_id: str, use_yn: bool = True) -> list:
    items = [r for r in load_all() if r.get("customer_id") == customer_id]
    if use_yn:
        items = [r for r in items if r.get("use_yn", True)]
    return sorted(items, key=lambda r: r.get("building_name") or "")

def add(data: dict) -> dict:
    items = load_all()
    bid = data.get("building_id") or next_id("B")
    row = {
        "building_id": bid,
        "customer_id": data.get("customer_id", ""),
        "building_name": (data.get("building_name") or "").strip(),
        "building_type_code": (data.get("building_type_code") or "").strip() or None,
        "address": (data.get("address") or "").strip() or None,
        "use_yn": data.get("use_yn", True),
        "created_at": now_iso(),
        "updated_at": now_iso(),
    }
    items.append(row)
    save_all(items)
    return row

def update(building_id: str, data: dict) -> None:
    items = load_all()
    for r in items:
        if r.get("building_id") == building_id:
            for k, v in data.items():
                if k in r and k not in ("building_id", "created_at"):
                    r[k] = v
            r["updated_at"] = now_iso()
            save_all(items)
            return
    raise ValueError(f"building not found: {building_id}")
