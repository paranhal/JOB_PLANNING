# 고객실 저장소 — 기획서 §5.2

from as_support.store._base import load_json, save_json, next_id, now_iso

FILENAME = "room.json"

def load_all() -> list:
    return load_json(FILENAME)

def save_all(items: list) -> None:
    save_json(FILENAME, items)

def list_by_floor(floor_id: str) -> list:
    items = [r for r in load_all() if r.get("floor_id") == floor_id]
    return sorted(items, key=lambda r: (r.get("room_name") or "", r.get("room_number") or ""))

def add(data: dict) -> dict:
    items = load_all()
    rid = data.get("room_id") or next_id("R")
    row = {
        "room_id": rid,
        "floor_id": data.get("floor_id", ""),
        "room_name": (data.get("room_name") or "").strip(),
        "room_number": (data.get("room_number") or "").strip() or None,
        "room_use_code": (data.get("room_use_code") or "").strip() or None,
        "created_at": now_iso(),
        "updated_at": now_iso(),
    }
    items.append(row)
    save_all(items)
    return row

def update(room_id: str, data: dict) -> None:
    items = load_all()
    for r in items:
        if r.get("room_id") == room_id:
            for k, v in data.items():
                if k in r and k not in ("room_id", "created_at"):
                    r[k] = v
            r["updated_at"] = now_iso()
            save_all(items)
            return
    raise ValueError(f"room not found: {room_id}")
