# 코드관리 저장소 — 기획서 §11
# code_group, code_value, code_name, sort_order, use_yn

from as_support.store._base import load_json, save_json, next_id, now_iso
from as_support.config import DEFAULT_CODES, CODE_GROUPS

FILENAME = "code_master.json"

def load_all() -> list:
    return load_json(FILENAME)

def save_all(items: list) -> None:
    save_json(FILENAME, items)

def ensure_defaults() -> None:
    items = load_all()
    existing = {(r.get("code_group"), r.get("code_value")) for r in items if r.get("code_group")}
    added = 0
    for group in CODE_GROUPS:
        for i, name in enumerate(DEFAULT_CODES.get(group, [])):
            if (group, name) in existing:
                continue
            items.append({
                "code_group": group,
                "code_value": name,
                "code_name": name,
                "sort_order": i,
                "use_yn": True,
                "created_at": now_iso(),
                "updated_at": now_iso(),
            })
            added += 1
    if added:
        save_all(items)

def list_by_group(code_group: str, use_only: bool = True) -> list:
    items = load_all()
    rows = [r for r in items if r.get("code_group") == code_group]
    if use_only:
        rows = [r for r in rows if r.get("use_yn", True)]
    return sorted(rows, key=lambda r: (r.get("sort_order", 0), r.get("code_value", "")))

def add(code_group: str, code_value: str, code_name: str = None, sort_order: int = 0) -> dict:
    items = load_all()
    code_name = code_name or code_value
    row = {
        "code_group": code_group,
        "code_value": code_value,
        "code_name": code_name,
        "sort_order": sort_order,
        "use_yn": True,
        "created_at": now_iso(),
        "updated_at": now_iso(),
    }
    items.append(row)
    save_all(items)
    return row

def update(row: dict, **kwargs) -> None:
    items = load_all()
    for i, r in enumerate(items):
        if r.get("code_group") == row.get("code_group") and r.get("code_value") == row.get("code_value"):
            for k, v in kwargs.items():
                if k in r:
                    r[k] = v
            r["updated_at"] = now_iso()
            save_all(items)
            return
    raise ValueError("code not found")

def delete(row: dict) -> None:
    update(row, use_yn=False)
