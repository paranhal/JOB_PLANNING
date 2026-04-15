package repository

import (
	"database/sql"
	"time"

	"customer-support/internal/model"
)

type AssetRepo struct{ db *sql.DB }

func NewAssetRepo(db *sql.DB) *AssetRepo { return &AssetRepo{db: db} }

func (r *AssetRepo) List(customerID, search string, page, pageSize int) ([]model.Asset, int, error) {
	offset := (page - 1) * pageSize

	base := `
		SELECT a.asset_id, a.customer_id, a.product_name, COALESCE(a.product_type,''),
		       COALESCE(a.model_name,''), COALESCE(a.manufacturer,''), COALESCE(a.serial_number,''),
		       COALESCE(a.install_date,''), COALESCE(a.operation_status,'operating'),
		       COALESCE(a.management_type,''), a.is_managed,
		       c.org_name,
		       COALESCE(NULLIF(TRIM(a.loc_building_name),''), b.building_name,'') AS bname,
		       COALESCE(NULLIF(TRIM(a.loc_floor_name),''), f.floor_name,'') AS fname,
		       COALESCE(NULLIF(TRIM(a.loc_room_name),''), rm.room_name,'') AS rname,
		       (SELECT COUNT(*) FROM as_receipts ar WHERE ar.asset_id=a.asset_id) AS as_cnt,
		       CASE WHEN a.install_date!='' THEN CAST((julianday('now')-julianday(a.install_date))/365 AS INTEGER) ELSE 0 END AS yrs
		FROM assets a
		JOIN customers c ON c.customer_id=a.customer_id
		LEFT JOIN customer_buildings b ON b.building_id=a.building_id
		LEFT JOIN customer_floors f ON f.floor_id=a.floor_id
		LEFT JOIN customer_rooms rm ON rm.room_id=a.room_id
		WHERE 1=1`

	cnt := `SELECT COUNT(*) FROM assets a JOIN customers c ON c.customer_id=a.customer_id WHERE 1=1`
	args, cntArgs := []interface{}{}, []interface{}{}

	if customerID != "" {
		base += ` AND a.customer_id=?`
		cnt += ` AND a.customer_id=?`
		args = append(args, customerID)
		cntArgs = append(cntArgs, customerID)
	}
	if search != "" {
		like := "%" + search + "%"
		f := ` AND (a.product_name LIKE ? OR a.serial_number LIKE ? OR c.org_name LIKE ? OR a.model_name LIKE ?)`
		base += f
		cnt += f
		args = append(args, like, like, like, like)
		cntArgs = append(cntArgs, like, like, like, like)
	}

	var total int
	if err := r.db.QueryRow(cnt, cntArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	base += ` ORDER BY c.org_name, a.product_name LIMIT ? OFFSET ?`
	args = append(args, pageSize, offset)

	rows, err := r.db.Query(base, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []model.Asset
	for rows.Next() {
		var a model.Asset
		var managed int
		if err := rows.Scan(
			&a.AssetID, &a.CustomerID, &a.ProductName, &a.ProductType,
			&a.ModelName, &a.Manufacturer, &a.SerialNumber,
			&a.InstallDate, &a.OperationStatus, &a.ManagementType, &managed,
			&a.OrgName, &a.BuildingName, &a.FloorName, &a.RoomName,
			&a.AsCount, &a.InstallYears,
		); err != nil {
			return nil, 0, err
		}
		a.IsManaged = managed == 1
		items = append(items, a)
	}
	return items, total, rows.Err()
}

func (r *AssetRepo) GetByID(id string) (*model.Asset, error) {
	q := `
		SELECT a.asset_id, a.customer_id, a.product_name, COALESCE(a.product_type,''),
		       COALESCE(a.model_name,''), COALESCE(a.manufacturer,''), COALESCE(a.serial_number,''),
		       COALESCE(a.install_date,''), COALESCE(a.retire_date,''),
		       COALESCE(a.installer_type,''), COALESCE(a.original_installer,''),
		       COALESCE(a.operation_status,'operating'), COALESCE(a.management_type,''),
		       a.is_managed, COALESCE(a.requester_type,''), COALESCE(a.requester_name,''),
		       COALESCE(a.customer_contact_id,''), COALESCE(a.our_contact,''),
		       COALESCE(a.building_id,''), COALESCE(a.floor_id,''), COALESCE(a.room_id,''),
		       COALESCE(a.loc_building_name,''), COALESCE(a.loc_floor_name,''), COALESCE(a.loc_room_name,''),
		       COALESCE(a.location_detail,''), COALESCE(a.notes,''),
		       a.created_at, a.updated_at,
		       c.org_name,
		       COALESCE(NULLIF(TRIM(a.loc_building_name),''), b.building_name,''),
		       COALESCE(NULLIF(TRIM(a.loc_floor_name),''), f.floor_name,''),
		       COALESCE(NULLIF(TRIM(a.loc_room_name),''), rm.room_name,'')
		FROM assets a
		JOIN customers c ON c.customer_id=a.customer_id
		LEFT JOIN customer_buildings b ON b.building_id=a.building_id
		LEFT JOIN customer_floors f ON f.floor_id=a.floor_id
		LEFT JOIN customer_rooms rm ON rm.room_id=a.room_id
		WHERE a.asset_id=?`

	var a model.Asset
	var managed int
	var createdAt, updatedAt string
	err := r.db.QueryRow(q, id).Scan(
		&a.AssetID, &a.CustomerID, &a.ProductName, &a.ProductType,
		&a.ModelName, &a.Manufacturer, &a.SerialNumber,
		&a.InstallDate, &a.RetireDate,
		&a.InstallerType, &a.OriginalInstaller,
		&a.OperationStatus, &a.ManagementType,
		&managed, &a.RequesterType, &a.RequesterName,
		&a.CustomerContactID, &a.OurContact,
		&a.BuildingID, &a.FloorID, &a.RoomID,
		&a.LocBuildingName, &a.LocFloorName, &a.LocRoomName,
		&a.LocationDetail, &a.Notes,
		&createdAt, &updatedAt,
		&a.OrgName, &a.BuildingName, &a.FloorName, &a.RoomName,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.IsManaged = managed == 1
	a.CreatedAt = parseTime(createdAt)
	a.UpdatedAt = parseTime(updatedAt)
	return &a, nil
}

func (r *AssetRepo) Create(a *model.Asset) error {
	a.AssetID = newID("AST")
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := r.db.Exec(`
		INSERT INTO assets (
			asset_id, customer_id, product_name, product_type, model_name,
			manufacturer, serial_number, install_date, retire_date,
			installer_type, original_installer, operation_status, management_type,
			is_managed, requester_type, requester_name,
			customer_contact_id, our_contact,
			building_id, floor_id, room_id,
			loc_building_name, loc_floor_name, loc_room_name,
			location_detail, notes,
			created_at, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.AssetID, a.CustomerID, a.ProductName, a.ProductType, a.ModelName,
		a.Manufacturer, a.SerialNumber, a.InstallDate, a.RetireDate,
		a.InstallerType, a.OriginalInstaller, a.OperationStatus, a.ManagementType,
		boolToInt(a.IsManaged), a.RequesterType, a.RequesterName,
		a.CustomerContactID, a.OurContact,
		nullStr(a.BuildingID), nullStr(a.FloorID), nullStr(a.RoomID),
		a.LocBuildingName, a.LocFloorName, a.LocRoomName,
		a.LocationDetail, a.Notes, now, now,
	)
	return err
}

func (r *AssetRepo) Update(a *model.Asset) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := r.db.Exec(`
		UPDATE assets SET
			customer_id=?, product_name=?, product_type=?, model_name=?,
			manufacturer=?, serial_number=?, install_date=?, retire_date=?,
			installer_type=?, original_installer=?, operation_status=?, management_type=?,
			is_managed=?, requester_type=?, requester_name=?,
			customer_contact_id=?, our_contact=?,
			building_id=?, floor_id=?, room_id=?,
			loc_building_name=?, loc_floor_name=?, loc_room_name=?,
			location_detail=?, notes=?,
			updated_at=?
		WHERE asset_id=?`,
		a.CustomerID, a.ProductName, a.ProductType, a.ModelName,
		a.Manufacturer, a.SerialNumber, a.InstallDate, a.RetireDate,
		a.InstallerType, a.OriginalInstaller, a.OperationStatus, a.ManagementType,
		boolToInt(a.IsManaged), a.RequesterType, a.RequesterName,
		a.CustomerContactID, a.OurContact,
		nullStr(a.BuildingID), nullStr(a.FloorID), nullStr(a.RoomID),
		a.LocBuildingName, a.LocFloorName, a.LocRoomName,
		a.LocationDetail, a.Notes, now, a.AssetID,
	)
	return err
}

func (r *AssetRepo) Delete(id string) error {
	_, err := r.db.Exec(
		`UPDATE assets SET operation_status='disposed', updated_at=? WHERE asset_id=?`,
		time.Now().Format("2006-01-02 15:04:05"), id)
	return err
}

func (r *AssetRepo) ListByCustomer(customerID string) ([]model.Asset, error) {
	rows, err := r.db.Query(
		`SELECT asset_id, product_name, COALESCE(product_type,''), COALESCE(serial_number,'')
		 FROM assets WHERE customer_id=? AND operation_status!='disposed' ORDER BY product_name`,
		customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.Asset
	for rows.Next() {
		var a model.Asset
		if err := rows.Scan(&a.AssetID, &a.ProductName, &a.ProductType, &a.SerialNumber); err != nil {
			return nil, err
		}
		items = append(items, a)
	}
	return items, rows.Err()
}
