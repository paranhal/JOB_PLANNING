# 공간(건물/층/실) 서비스 — 기획서 §5.2

from as_support.store import building, floor, room

def list_buildings(customer_id: str, use_yn: bool = True):
    return building.list_by_customer(customer_id, use_yn=use_yn)

def list_floors(building_id: str):
    return floor.list_by_building(building_id)

def list_rooms(floor_id: str):
    return room.list_by_floor(floor_id)

def get_building(building_id: str):
    return building.find_by_id(building_id)

def add_building(data: dict):
    return building.add(data)

def update_building(building_id: str, data: dict):
    return building.update(building_id, data)

def add_floor(data: dict):
    return floor.add(data)

def update_floor(floor_id: str, data: dict):
    return floor.update(floor_id, data)

def add_room(data: dict):
    return room.add(data)

def update_room(room_id: str, data: dict):
    return room.update(room_id, data)
