# 설치SW상세 저장소 — 기획서 §6.2

from as_support.store._base import load_json, save_json, next_id, now_iso

FILENAME = "installation_sw.json"

def load_all() -> list:
    return load_json(FILENAME)

def save_all(items: list) -> None:
    save_json(FILENAME, items)

def list_by_installation(installation_id: str) -> list:
    return [r for r in load_all() if r.get("installation_id") == installation_id]

def add(data: dict) -> dict:
    items = load_all()
    sid = data.get("sw_detail_id") or next_id("S")
    row = {
        "sw_detail_id": sid,
        "installation_id": data.get("installation_id", ""),
        "software_name": (data.get("software_name") or "").strip() or None,
        "software_version": (data.get("software_version") or "").strip() or None,
        "install_type_code": (data.get("install_type_code") or "").strip() or None,
        "hardware_info": (data.get("hardware_info") or "").strip() or None,
        "os_name": (data.get("os_name") or "").strip() or None,
        "os_version": (data.get("os_version") or "").strip() or None,
        "dbms_name": (data.get("dbms_name") or "").strip() or None,
        "dbms_version": (data.get("dbms_version") or "").strip() or None,
        "access_type": (data.get("access_type") or "").strip() or None,
        "access_address": (data.get("access_address") or "").strip() or None,
        "install_path": (data.get("install_path") or "").strip() or None,
        "backup_path": (data.get("backup_path") or "").strip() or None,
        "config_file_path": (data.get("config_file_path") or "").strip() or None,
        "created_at": now_iso(),
        "updated_at": now_iso(),
    }
    items.append(row)
    save_all(items)
    return row

def update(sw_detail_id: str, data: dict) -> None:
    items = load_all()
    for r in items:
        if r.get("sw_detail_id") == sw_detail_id:
            for k, v in data.items():
                if k in r and k not in ("sw_detail_id", "installation_id", "created_at"):
                    r[k] = v
            r["updated_at"] = now_iso()
            save_all(items)
            return
    raise ValueError(f"sw_detail not found: {sw_detail_id}")
