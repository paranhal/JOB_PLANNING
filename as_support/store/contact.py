# 고객담당자(현재) 저장소 — 기획서 §5.3

from as_support.store._base import load_json, save_json, next_id, now_iso, date_iso

FILENAME = "contact.json"

def load_all() -> list:
    return load_json(FILENAME)

def save_all(items: list) -> None:
    save_json(FILENAME, items)

def find_by_id(contact_id: str) -> dict | None:
    for r in load_all():
        if r.get("contact_id") == contact_id:
            return r
    return None

def list_by_customer(customer_id: str, in_office_only: bool = True) -> list:
    items = [r for r in load_all() if r.get("customer_id") == customer_id]
    if in_office_only:
        items = [r for r in items if r.get("in_office_yn", True)]
    return sorted(items, key=lambda r: (not r.get("main_contact_yn", False), r.get("name") or ""))

def add(data: dict) -> dict:
    items = load_all()
    cid = data.get("contact_id") or next_id("T")
    row = {
        "contact_id": cid,
        "customer_id": data.get("customer_id", ""),
        "name": (data.get("name") or "").strip(),
        "duty_code": (data.get("duty_code") or "").strip() or None,
        "job_title": (data.get("job_title") or "").strip() or None,
        "phone": (data.get("phone") or "").strip() or None,
        "mobile": (data.get("mobile") or "").strip() or None,
        "email": (data.get("email") or "").strip() or None,
        "appointed_at": date_iso(data.get("appointed_at")),
        "retired_at": date_iso(data.get("retired_at")),
        "in_office_yn": data.get("in_office_yn", True),
        "main_contact_yn": data.get("main_contact_yn", False),
        "created_at": now_iso(),
        "updated_at": now_iso(),
    }
    items.append(row)
    save_all(items)
    return row

def update(contact_id: str, data: dict) -> None:
    items = load_all()
    for r in items:
        if r.get("contact_id") == contact_id:
            for k, v in data.items():
                if k in r and k not in ("contact_id", "created_at"):
                    r[k] = v
            if "appointed_at" in data:
                r["appointed_at"] = date_iso(data["appointed_at"])
            if "retired_at" in data:
                r["retired_at"] = date_iso(data["retired_at"])
            r["updated_at"] = now_iso()
            save_all(items)
            return
    raise ValueError(f"contact not found: {contact_id}")
