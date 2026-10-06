package repository

import (
	"database/sql"
	"strings"
	"time"

	"customer-support/internal/model"
)

type SpaceRepo struct{ db *sql.DB }

func NewSpaceRepo(db *sql.DB) *SpaceRepo { return &SpaceRepo{db: db} }

func spaceOrgFrag(orgID string) (string, []interface{}, error) {
	return AppendOrgSQL("c", orgID)
}

// ── 건물 ──

func (r *SpaceRepo) ListBuildings(orgID, customerID string) ([]model.CustomerBuilding, error) {
	orgFrag, orgArgs, err := spaceOrgFrag(orgID)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.Query(
		`SELECT b.building_id, b.customer_id, b.building_name, COALESCE(b.building_type,''),
		        COALESCE(b.address,''), b.is_active
		 FROM customer_buildings b
		 JOIN customers c ON c.customer_id = b.customer_id
		 WHERE b.customer_id=?`+orgFrag+` ORDER BY b.building_name`,
		append([]interface{}{customerID}, orgArgs...)...)
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

func (r *SpaceRepo) GetBuilding(orgID, id string) (*model.CustomerBuilding, error) {
	orgFrag, orgArgs, err := spaceOrgFrag(orgID)
	if err != nil {
		return nil, err
	}
	var b model.CustomerBuilding
	var active int
	err = r.db.QueryRow(
		`SELECT b.building_id, b.customer_id, b.building_name, COALESCE(b.building_type,''),
		        COALESCE(b.address,''), b.is_active
		 FROM customer_buildings b
		 JOIN customers c ON c.customer_id = b.customer_id
		 WHERE b.building_id=?`+orgFrag,
		append([]interface{}{id}, orgArgs...)...).
		Scan(&b.BuildingID, &b.CustomerID, &b.BuildingName, &b.BuildingType, &b.Address, &active)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	b.IsActive = active == 1

	b.Floors, _ = r.ListFloors(orgID, id)
	return &b, nil
}

func (r *SpaceRepo) CreateBuilding(b *model.CustomerBuilding) error {
	b.BuildingID = newID("BLD")
	_, err := r.db.Exec(
		`INSERT INTO customer_buildings (building_id,customer_id,building_name,building_type,address,is_active,created_at)
		 VALUES (?,?,?,?,?,?,?)`,
		b.BuildingID, b.CustomerID, b.BuildingName, b.BuildingType, b.Address,
		boolToInt(b.IsActive), time.Now().Format("2006-01-02 15:04:05"))
	if err != nil {
		return err
	}
	logCreate(r.db, "customer_buildings", "building_id", b.BuildingID, b.BuildingName)
	return nil
}

func (r *SpaceRepo) UpdateBuilding(b *model.CustomerBuilding) error {
	return touchUpdate(r.db, "customer_buildings", "building_id", b.BuildingID, b.BuildingName, func() error {
		_, err := r.db.Exec(
			`UPDATE customer_buildings SET building_name=?,building_type=?,address=?,is_active=? WHERE building_id=?`,
			b.BuildingName, b.BuildingType, b.Address, boolToInt(b.IsActive), b.BuildingID)
		return err
	})
}

func (r *SpaceRepo) DeleteBuilding(id string) error {
	return touchDelete(r.db, "customer_buildings", "building_id", id, "건물", func() error {
		_, err := r.db.Exec(`DELETE FROM customer_buildings WHERE building_id=?`, id)
		return err
	})
}

// ── 층 ──

func (r *SpaceRepo) ListFloors(orgID, buildingID string) ([]model.CustomerFloor, error) {
	orgFrag, orgArgs, err := spaceOrgFrag(orgID)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.Query(
		`SELECT f.floor_id, f.building_id, f.floor_name, f.sort_order
		 FROM customer_floors f
		 JOIN customer_buildings b ON b.building_id = f.building_id
		 JOIN customers c ON c.customer_id = b.customer_id
		 WHERE f.building_id=?`+orgFrag+` ORDER BY f.sort_order`,
		append([]interface{}{buildingID}, orgArgs...)...)
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
		f.Rooms, _ = r.ListRooms(orgID, f.FloorID)
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
	if err != nil {
		return err
	}
	logCreate(r.db, "customer_floors", "floor_id", f.FloorID, f.FloorName)
	return nil
}

func (r *SpaceRepo) UpdateFloor(f *model.CustomerFloor) error {
	return touchUpdate(r.db, "customer_floors", "floor_id", f.FloorID, f.FloorName, func() error {
		_, err := r.db.Exec(
			`UPDATE customer_floors SET floor_name=?,sort_order=? WHERE floor_id=?`,
			f.FloorName, f.SortOrder, f.FloorID)
		return err
	})
}

func (r *SpaceRepo) DeleteFloor(id string) error {
	return touchDelete(r.db, "customer_floors", "floor_id", id, "층", func() error {
		r.db.Exec(`DELETE FROM customer_rooms WHERE floor_id=?`, id)
		_, err := r.db.Exec(`DELETE FROM customer_floors WHERE floor_id=?`, id)
		return err
	})
}

// ── 실 ──

func (r *SpaceRepo) ListRooms(orgID, floorID string) ([]model.CustomerRoom, error) {
	orgFrag, orgArgs, err := spaceOrgFrag(orgID)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.Query(
		`SELECT r.room_id, r.floor_id, r.room_name, COALESCE(r.room_number,''), COALESCE(r.purpose,'')
		 FROM customer_rooms r
		 JOIN customer_floors f ON f.floor_id = r.floor_id
		 JOIN customer_buildings b ON b.building_id = f.building_id
		 JOIN customers c ON c.customer_id = b.customer_id
		 WHERE r.floor_id=?`+orgFrag+` ORDER BY r.room_name`,
		append([]interface{}{floorID}, orgArgs...)...)
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
	if err != nil {
		return err
	}
	logCreate(r.db, "customer_rooms", "room_id", rm.RoomID, rm.RoomName)
	return nil
}

func (r *SpaceRepo) UpdateRoom(rm *model.CustomerRoom) error {
	return touchUpdate(r.db, "customer_rooms", "room_id", rm.RoomID, rm.RoomName, func() error {
		_, err := r.db.Exec(
			`UPDATE customer_rooms SET room_name=?,room_number=?,purpose=? WHERE room_id=?`,
			rm.RoomName, rm.RoomNumber, rm.Purpose, rm.RoomID)
		return err
	})
}

func (r *SpaceRepo) UpdateRoomName(roomID, roomName string) error {
	return touchUpdate(r.db, "customer_rooms", "room_id", roomID, roomName, func() error {
		_, err := r.db.Exec(`UPDATE customer_rooms SET room_name=? WHERE room_id=?`, roomName, roomID)
		return err
	})
}

func (r *SpaceRepo) DeleteRoom(id string) error {
	return touchDelete(r.db, "customer_rooms", "room_id", id, "호실", func() error {
		_, err := r.db.Exec(`DELETE FROM customer_rooms WHERE room_id=?`, id)
		return err
	})
}

// ── 위치 조회용 (드롭다운) ──

func (r *SpaceRepo) AllBuildingsForCustomer(orgID, customerID string) ([]model.CustomerBuilding, error) {
	return r.ListBuildings(orgID, customerID)
}

func (r *SpaceRepo) AllFloorsForBuilding(orgID, buildingID string) ([]model.CustomerFloor, error) {
	orgFrag, orgArgs, err := spaceOrgFrag(orgID)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.Query(
		`SELECT f.floor_id, f.building_id, f.floor_name, f.sort_order
		 FROM customer_floors f
		 JOIN customer_buildings b ON b.building_id = f.building_id
		 JOIN customers c ON c.customer_id = b.customer_id
		 WHERE f.building_id=?`+orgFrag+` ORDER BY f.sort_order`,
		append([]interface{}{buildingID}, orgArgs...)...)
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

func (r *SpaceRepo) AllRoomsForFloor(orgID, floorID string) ([]model.CustomerRoom, error) {
	return r.ListRooms(orgID, floorID)
}

type SpaceRow struct {
	CustomerID, OrgName      string
	BuildingID, BuildingName string
	FloorID, FloorName       string
	RoomID, RoomName         string
}

type SpaceRowCounts struct {
	Customers, Buildings, Floors, Rooms int
}

func spaceRowFromWhere(orgID, customerID, q string, includeInactive bool) (string, []interface{}, error) {
	orgFrag, orgArgs, err := spaceOrgFrag(orgID)
	if err != nil {
		return "", nil, err
	}
	from := `
		 FROM customer_buildings b
		 JOIN customers c ON c.customer_id = b.customer_id
		 LEFT JOIN customer_floors f ON f.building_id = b.building_id
		 LEFT JOIN customer_rooms r ON r.floor_id = f.floor_id
		 WHERE 1=1` + orgFrag
	args := append([]interface{}{}, orgArgs...)
	if customerID != "" {
		from += ` AND b.customer_id=?`
		args = append(args, customerID)
	}
	if !includeInactive {
		from += ` AND b.is_active=1`
	}
	q = strings.TrimSpace(q)
	if q != "" {
		like := "%" + q + "%"
		from += ` AND (c.org_name LIKE ? OR b.building_name LIKE ? OR COALESCE(f.floor_name,'') LIKE ? OR COALESCE(r.room_name,'') LIKE ?)`
		args = append(args, like, like, like, like)
	}
	return from, args, nil
}

func spaceOrderSQL(sort, dir string) string {
	d := "ASC"
	if strings.EqualFold(strings.TrimSpace(dir), "desc") {
		d = "DESC"
	}
	switch strings.TrimSpace(sort) {
	case "org":
		return `c.org_name ` + d + `, b.building_name, f.sort_order, f.floor_name, CAST(r.room_name AS INTEGER), r.room_name`
	case "building":
		return `b.building_name ` + d + `, c.org_name, f.sort_order, f.floor_name, CAST(r.room_name AS INTEGER), r.room_name`
	case "floor":
		return `f.sort_order ` + d + `, f.floor_name ` + d + `, c.org_name, b.building_name, CAST(r.room_name AS INTEGER), r.room_name`
	case "room":
		return `CAST(r.room_name AS INTEGER) ` + d + `, r.room_name ` + d + `, c.org_name, b.building_name, f.sort_order, f.floor_name`
	default:
		return `c.org_name, b.building_name, f.sort_order, f.floor_name, CAST(r.room_name AS INTEGER), r.room_name`
	}
}

// ListSpaceRows 공간 평면 목록. q 는 기관·건물·층·호실 통합 검색. limit<=0 이면 전부.
func (r *SpaceRepo) ListSpaceRows(orgID, customerID, q, sort, dir string, includeInactive bool, limit, offset int) ([]SpaceRow, int, SpaceRowCounts, error) {
	from, args, err := spaceRowFromWhere(orgID, customerID, q, includeInactive)
	if err != nil {
		return nil, 0, SpaceRowCounts{}, err
	}
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*)`+from, args...).Scan(&total); err != nil {
		return nil, 0, SpaceRowCounts{}, err
	}
	var counts SpaceRowCounts
	if err := r.db.QueryRow(
		`SELECT COUNT(DISTINCT c.customer_id), COUNT(DISTINCT b.building_id),
		        COUNT(DISTINCT f.floor_id), COUNT(DISTINCT r.room_id)`+from, args...).
		Scan(&counts.Customers, &counts.Buildings, &counts.Floors, &counts.Rooms); err != nil {
		return nil, 0, SpaceRowCounts{}, err
	}

	sel := `SELECT c.customer_id, c.org_name, b.building_id, b.building_name,
		        COALESCE(f.floor_id,''), COALESCE(f.floor_name,''),
		        COALESCE(r.room_id,''), COALESCE(r.room_name,'')` + from +
		` ORDER BY ` + spaceOrderSQL(sort, dir)
	qargs := append([]interface{}{}, args...)
	if limit > 0 {
		if offset < 0 {
			offset = 0
		}
		sel += ` LIMIT ? OFFSET ?`
		qargs = append(qargs, limit, offset)
	}
	rows, err := r.db.Query(sel, qargs...)
	if err != nil {
		return nil, 0, SpaceRowCounts{}, err
	}
	defer rows.Close()
	var items []SpaceRow
	for rows.Next() {
		var row SpaceRow
		if err := rows.Scan(&row.CustomerID, &row.OrgName, &row.BuildingID, &row.BuildingName,
			&row.FloorID, &row.FloorName, &row.RoomID, &row.RoomName); err != nil {
			return nil, 0, SpaceRowCounts{}, err
		}
		items = append(items, row)
	}
	return items, total, counts, rows.Err()
}

type SpaceStatRow struct {
	CustomerID, OrgName      string
	Buildings, Floors, Rooms int
}

// SpaceStats 기관별 건물·층·호실 수. COUNT(DISTINCT). is_active 는 JOIN ON 에 둔다.
func (r *SpaceRepo) SpaceStats(orgID, customerID, q string, includeInactive bool) ([]SpaceStatRow, error) {
	orgFrag, orgArgs, err := spaceOrgFrag(orgID)
	if err != nil {
		return nil, err
	}
	onB := ` ON b.customer_id = c.customer_id`
	if !includeInactive {
		onB += ` AND b.is_active=1`
	}
	sql := `
		SELECT c.customer_id, c.org_name,
		       COUNT(DISTINCT b.building_id) AS buildings,
		       COUNT(DISTINCT f.floor_id)    AS floors,
		       COUNT(DISTINCT r.room_id)     AS rooms
		  FROM customers c
		  LEFT JOIN customer_buildings b` + onB + `
		  LEFT JOIN customer_floors    f ON f.building_id = b.building_id
		  LEFT JOIN customer_rooms     r ON r.floor_id    = f.floor_id
		 WHERE 1=1` + orgFrag
	args := append([]interface{}{}, orgArgs...)
	if customerID != "" {
		sql += ` AND c.customer_id=?`
		args = append(args, customerID)
	}
	q = strings.TrimSpace(q)
	if q != "" {
		like := "%" + q + "%"
		sql += ` AND (c.org_name LIKE ? OR b.building_name LIKE ? OR COALESCE(f.floor_name,'') LIKE ? OR COALESCE(r.room_name,'') LIKE ?)`
		args = append(args, like, like, like, like)
	}
	sql += ` GROUP BY c.customer_id, c.org_name ORDER BY c.org_name`
	rows, err := r.db.Query(sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []SpaceStatRow
	for rows.Next() {
		var s SpaceStatRow
		if err := rows.Scan(&s.CustomerID, &s.OrgName, &s.Buildings, &s.Floors, &s.Rooms); err != nil {
			return nil, err
		}
		items = append(items, s)
	}
	return items, rows.Err()
}
