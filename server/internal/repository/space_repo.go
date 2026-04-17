package repository

import (
	"database/sql"
	"time"

	"customer-support/internal/model"
)

type SpaceRepo struct{ db *sql.DB }

func NewSpaceRepo(db *sql.DB) *SpaceRepo { return &SpaceRepo{db: db} }

// ── 건물 ──

func (r *SpaceRepo) ListBuildings(customerID string) ([]model.CustomerBuilding, error) {
	rows, err := r.db.Query(
		`SELECT building_id, customer_id, building_name, COALESCE(building_type,''),
		        COALESCE(address,''), is_active
		 FROM customer_buildings WHERE customer_id=? ORDER BY building_name`, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.CustomerBuilding
	for rows.Next() {
		var b model.CustomerBuilding
		var active int
		if err := rows.Scan(&b.BuildingID, &b.CustomerID, &b.BuildingName,
			&b.BuildingType, &b.Address, &active); err != nil {
			return nil, err
		}
		b.IsActive = active == 1
		items = append(items, b)
	}
	return items, rows.Err()
}

func (r *SpaceRepo) GetBuilding(id string) (*model.CustomerBuilding, error) {
	var b model.CustomerBuilding
	var active int
	err := r.db.QueryRow(
		`SELECT building_id, customer_id, building_name, COALESCE(building_type,''),
		        COALESCE(address,''), is_active
		 FROM customer_buildings WHERE building_id=?`, id).
		Scan(&b.BuildingID, &b.CustomerID, &b.BuildingName, &b.BuildingType, &b.Address, &active)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	b.IsActive = active == 1

	b.Floors, _ = r.ListFloors(id)
	return &b, nil
}

func (r *SpaceRepo) CreateBuilding(b *model.CustomerBuilding) error {
	b.BuildingID = newID("BLD")
	_, err := r.db.Exec(
		`INSERT INTO customer_buildings (building_id,customer_id,building_name,building_type,address,is_active,created_at)
		 VALUES (?,?,?,?,?,?,?)`,
		b.BuildingID, b.CustomerID, b.BuildingName, b.BuildingType, b.Address,
		boolToInt(b.IsActive), time.Now().Format("2006-01-02 15:04:05"))
	return err
}

func (r *SpaceRepo) UpdateBuilding(b *model.CustomerBuilding) error {
	_, err := r.db.Exec(
		`UPDATE customer_buildings SET building_name=?,building_type=?,address=?,is_active=? WHERE building_id=?`,
		b.BuildingName, b.BuildingType, b.Address, boolToInt(b.IsActive), b.BuildingID)
	return err
}

func (r *SpaceRepo) DeleteBuilding(id string) error {
	_, err := r.db.Exec(`DELETE FROM customer_buildings WHERE building_id=?`, id)
	return err
}

// ── 층 ──

func (r *SpaceRepo) ListFloors(buildingID string) ([]model.CustomerFloor, error) {
	rows, err := r.db.Query(
		`SELECT floor_id, building_id, floor_name, sort_order
		 FROM customer_floors WHERE building_id=? ORDER BY sort_order`, buildingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.CustomerFloor
	for rows.Next() {
		var f model.CustomerFloor
		if err := rows.Scan(&f.FloorID, &f.BuildingID, &f.FloorName, &f.SortOrder); err != nil {
			return nil, err
		}
		f.Rooms, _ = r.ListRooms(f.FloorID)
		items = append(items, f)
	}
	return items, rows.Err()
}

func (r *SpaceRepo) CreateFloor(f *model.CustomerFloor) error {
	f.FloorID = newID("FL")
	_, err := r.db.Exec(
		`INSERT INTO customer_floors (floor_id,building_id,floor_name,sort_order,created_at)
		 VALUES (?,?,?,?,?)`,
		f.FloorID, f.BuildingID, f.FloorName, f.SortOrder, time.Now().Format("2006-01-02 15:04:05"))
	return err
}

func (r *SpaceRepo) UpdateFloor(f *model.CustomerFloor) error {
	_, err := r.db.Exec(
		`UPDATE customer_floors SET floor_name=?,sort_order=? WHERE floor_id=?`,
		f.FloorName, f.SortOrder, f.FloorID)
	return err
}

func (r *SpaceRepo) DeleteFloor(id string) error {
	r.db.Exec(`DELETE FROM customer_rooms WHERE floor_id=?`, id)
	_, err := r.db.Exec(`DELETE FROM customer_floors WHERE floor_id=?`, id)
	return err
}

// ── 실 ──

func (r *SpaceRepo) ListRooms(floorID string) ([]model.CustomerRoom, error) {
	rows, err := r.db.Query(
		`SELECT room_id, floor_id, room_name, COALESCE(room_number,''), COALESCE(purpose,'')
		 FROM customer_rooms WHERE floor_id=? ORDER BY room_name`, floorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.CustomerRoom
	for rows.Next() {
		var rm model.CustomerRoom
		if err := rows.Scan(&rm.RoomID, &rm.FloorID, &rm.RoomName, &rm.RoomNumber, &rm.Purpose); err != nil {
			return nil, err
		}
		items = append(items, rm)
	}
	return items, rows.Err()
}

func (r *SpaceRepo) CreateRoom(rm *model.CustomerRoom) error {
	rm.RoomID = newID("RM")
	_, err := r.db.Exec(
		`INSERT INTO customer_rooms (room_id,floor_id,room_name,room_number,purpose,created_at)
		 VALUES (?,?,?,?,?,?)`,
		rm.RoomID, rm.FloorID, rm.RoomName, rm.RoomNumber, rm.Purpose,
		time.Now().Format("2006-01-02 15:04:05"))
	return err
}

func (r *SpaceRepo) UpdateRoom(rm *model.CustomerRoom) error {
	_, err := r.db.Exec(
		`UPDATE customer_rooms SET room_name=?,room_number=?,purpose=? WHERE room_id=?`,
		rm.RoomName, rm.RoomNumber, rm.Purpose, rm.RoomID)
	return err
}

func (r *SpaceRepo) DeleteRoom(id string) error {
	_, err := r.db.Exec(`DELETE FROM customer_rooms WHERE room_id=?`, id)
	return err
}

// ── 위치 조회용 (드롭다운) ──

func (r *SpaceRepo) AllBuildingsForCustomer(customerID string) ([]model.CustomerBuilding, error) {
	return r.ListBuildings(customerID)
}

func (r *SpaceRepo) AllFloorsForBuilding(buildingID string) ([]model.CustomerFloor, error) {
	rows, err := r.db.Query(
		`SELECT floor_id, building_id, floor_name, sort_order
		 FROM customer_floors WHERE building_id=? ORDER BY sort_order`, buildingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.CustomerFloor
	for rows.Next() {
		var f model.CustomerFloor
		if err := rows.Scan(&f.FloorID, &f.BuildingID, &f.FloorName, &f.SortOrder); err != nil {
			return nil, err
		}
		items = append(items, f)
	}
	return items, rows.Err()
}

func (r *SpaceRepo) AllRoomsForFloor(floorID string) ([]model.CustomerRoom, error) {
	return r.ListRooms(floorID)
}
