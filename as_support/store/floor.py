# 고객층 저장소 — 기획서 §5.2

from as_support.store._base import load_json, save_json, next_id, now_iso

FILENAME = "floor.json"

def load_all() -> list:
    return load_json(FILENAME)

def save_all(items: list) -> None:
    save_json(FILENAME, items)

def list_by_building(building_id: str) -> list:
    items = [r for r in load_all() if r.get("building_id") == building_id]
    return sorted(items, key=lambda r: (r.get("sort_order", 0), r.get("floor_name") or ""))

def add(data: dict) -> dict:
    items = load_all()
    fid = data.get("floor_id") or next_id("F")
    row = {
        "floor_id": fid,
        "building_id": data.get("building_id", ""),
        "floor_name": (data.get("floor_name") or "").strip(),
        "sort_order": data.get("sort_order", 0),
        "created_at": now_iso(),
        "updated_at": now_iso(),
    }
    items.append(row)
    save_all(items)
    return row

def update(floor_id: str, data: dict) -> None:
    items = load_all()
    for r in items:
        if r.get("floor_id") == floor_id:
            for k, v in data.items():
                if k in r and k not in ("floor_id", "created_at"):
                    r[k] = v
            r["updated_at"] = now_iso()
            save_all(items)
            return
    raise ValueError(f"floor not found: {floor_id}")
