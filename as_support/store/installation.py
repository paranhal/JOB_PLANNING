# 설치기본정보 저장소 — 기획서 §6.1

from as_support.store._base import load_json, save_json, next_id, now_iso, date_iso

FILENAME = "installation.json"

def _photo_urls(v, size: int) -> list:
    if isinstance(v, list):
        return [str(x).strip() if x else "" for x in (v + [""] * size)[:size]]
    return [""] * size

def load_all() -> list:
    return load_json(FILENAME)

def save_all(items: list) -> None:
    save_json(FILENAME, items)

def find_by_id(installation_id: str) -> dict | None:
    for r in load_all():
        if r.get("installation_id") == installation_id:
            if "photo_urls" not in r:
                r["photo_urls"] = [""] * 2
            return r
    return None

def list_by_customer(customer_id: str) -> list:
    items = [r for r in load_all() if r.get("customer_id") == customer_id]
    return sorted(items, key=lambda r: (r.get("product_name") or "", r.get("installation_id") or ""))

def add(data: dict) -> dict:
    items = load_all()
    iid = data.get("installation_id") or next_id("I")
    row = {
        "installation_id": iid,
        "customer_id": data.get("customer_id", ""),
        "product_name": (data.get("product_name") or "").strip() or None,
        "product_type_code": (data.get("product_type_code") or "").strip() or None,
        "model_name": (data.get("model_name") or "").strip() or None,
        "manufacturer": (data.get("manufacturer") or "").strip() or None,
        "serial_number": (data.get("serial_number") or "").strip() or None,
        "installed_at": date_iso(data.get("installed_at")),
        "removed_at": date_iso(data.get("removed_at")),
        "install_owner_code": (data.get("install_owner_code") or "").strip() or None,
        "install_owner_name": (data.get("install_owner_name") or "").strip() or None,
        "operation_status_code": (data.get("operation_status_code") or "").strip() or None,
        "management_type_code": (data.get("management_type_code") or "").strip() or None,
        "managed_by_us_yn": data.get("managed_by_us_yn"),
        "request_owner_type_code": (data.get("request_owner_type_code") or "").strip() or None,
        "request_owner_name": (data.get("request_owner_name") or "").strip() or None,
        "contact_id": data.get("contact_id") or None,
        "assignee_id": data.get("assignee_id") or None,
        "building_id": data.get("building_id") or None,
        "floor_id": data.get("floor_id") or None,
        "room_id": data.get("room_id") or None,
        "location_detail": (data.get("location_detail") or "").strip() or None,
        "remarks": (data.get("remarks") or "").strip() or None,
        "photo_urls": _photo_urls(data.get("photo_urls"), 2),
        "created_at": now_iso(),
        "updated_at": now_iso(),
    }
    items.append(row)
    save_all(items)
    return row

def update(installation_id: str, data: dict) -> None:
    items = load_all()
    for r in items:
        if r.get("installation_id") == installation_id:
            for k, v in data.items():
                if k in r and k not in ("installation_id", "created_at"):
                    r[k] = v
            for dk in ("installed_at", "removed_at"):
                if dk in data:
                    r[dk] = date_iso(data[dk])
            if "photo_urls" in data:
                r["photo_urls"] = _photo_urls(data["photo_urls"], 2)
            r["updated_at"] = now_iso()
            save_all(items)
            return
    raise ValueError(f"installation not found: {installation_id}")
