# 코드 서비스 — 기획서 §11

from as_support.store import code_master
from as_support.config import CODE_GROUPS

def ensure_defaults():
    code_master.ensure_defaults()

def list_by_group(code_group: str, use_only: bool = True):
    return code_master.list_by_group(code_group, use_only=use_only)

def add_code(code_group: str, code_value: str, code_name: str = None, sort_order: int = 0):
    return code_master.add(code_group, code_value, code_name, sort_order)

def update_code(row: dict, **kwargs):
    code_master.update(row, **kwargs)

def delete_code(row: dict):
    code_master.delete(row)

def get_all_groups():
    return code_master.load_all()

def save_codes(items: list):
    code_master.save_all(items)
