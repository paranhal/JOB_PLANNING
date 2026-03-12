# 고객담당자이력 저장소 — 기획서 §5.4

from as_support.store._base import load_json, save_json, next_id, now_iso, date_iso

FILENAME = "contact_history.json"

def load_all() -> list:
    return load_json(FILENAME)

def save_all(items: list) -> None:
    save_json(FILENAME, items)

def list_by_contact(contact_id: str) -> list:
    items = [r for r in load_all() if r.get("contact_id") == contact_id]
    return sorted(items, key=lambda r: (r.get("start_date") or "", r.get("created_at") or ""), reverse=True)

def list_by_customer(customer_id: str) -> list:
    items = [r for r in load_all() if r.get("customer_id") == customer_id]
    return sorted(items, key=lambda r: (r.get("start_date") or "", r.get("created_at") or ""), reverse=True)

def add(data: dict) -> dict:
    items = load_all()
    hid = data.get("contact_history_id") or next_id("H")
    row = {
        "contact_history_id": hid,
        "contact_id": data.get("contact_id", ""),
        "customer_id": data.get("customer_id", ""),
        "start_date": date_iso(data.get("start_date")) or "",
        "end_date": date_iso(data.get("end_date")),
        "department_name": (data.get("department_name") or "").strip() or None,
        "duty_code": (data.get("duty_code") or "").strip() or None,
        "job_title": (data.get("job_title") or "").strip() or None,
        "phone": (data.get("phone") or "").strip() or None,
        "email": (data.get("email") or "").strip() or None,
        "status_code": (data.get("status_code") or "").strip() or None,
        "change_reason": (data.get("change_reason") or "").strip() or None,
        "created_at": now_iso(),
        "created_by": (data.get("created_by") or "").strip() or None,
    }
    items.append(row)
    save_all(items)
    return row
