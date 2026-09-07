package repository

import (
	"database/sql"
	"strings"
	"time"

	"customer-support/internal/model"
)

type AssetRepo struct{ db *sql.DB }

func NewAssetRepo(db *sql.DB) *AssetRepo { return &AssetRepo{db: db} }

func (r *AssetRepo) List(customerID, search, projectID, category, sort, dir string, page, pageSize int) ([]model.Asset, int, error) {
	offset := 0
	if page < 1 {
		page = 1
	}
	if pageSize > 0 {
		offset = (page - 1) * pageSize
	}

	base := `
		SELECT a.asset_id, a.customer_id, a.product_name, COALESCE(a.product_type,''),
		       COALESCE(a.product_category,''),
		       COALESCE(a.model_name,''), COALESCE(a.manufacturer,''), COALESCE(a.serial_number,''),
		       COALESCE(a.install_date,''), COALESCE(a.operation_status,'operating'),
		       COALESCE(a.management_type,''), a.is_managed,
		       COALESCE(a.maint_contract_type,''), COALESCE(a.maint_cycle,''),
		       c.org_name,
		       COALESCE(NULLIF(TRIM(a.loc_building_name),''), b.building_name,'') AS bname,
		       COALESCE(NULLIF(TRIM(a.loc_floor_name),''), f.floor_name,'') AS fname,
		       COALESCE(NULLIF(TRIM(a.loc_room_name),''), rm.room_name,'') AS rname,
		       COALESCE(a.install_location,''),
		       COALESCE(a.project_id,''),
		       COALESCE(NULLIF(TRIM(p.short_name),''), p.name, '') AS project_name,
		       (SELECT COUNT(*) FROM as_receipts ar WHERE ar.asset_id=a.asset_id) AS as_cnt,
		       CASE WHEN a.install_date!='' THEN CAST((julianday('now')-julianday(a.install_date))/365 AS INTEGER) ELSE 0 END AS yrs
		FROM assets a
		JOIN customers c ON c.customer_id=a.customer_id
		LEFT JOIN customer_buildings b ON b.building_id=a.building_id
		LEFT JOIN customer_floors f ON f.floor_id=a.floor_id
		LEFT JOIN customer_rooms rm ON rm.room_id=a.room_id
		LEFT JOIN work_projects p ON p.project_id=a.project_id
		WHERE 1=1`

	cnt := `SELECT COUNT(*) FROM assets a JOIN customers c ON c.customer_id=a.customer_id WHERE 1=1`
	args, cntArgs := []interface{}{}, []interface{}{}

	if customerID != "" {
		base += ` AND a.customer_id=?`
		cnt += ` AND a.customer_id=?`
		args = append(args, customerID)
		cntArgs = append(cntArgs, customerID)
	}
	if projectID != "" {
		base += ` AND a.project_id=?`
		cnt += ` AND a.project_id=?`
		args = append(args, projectID)
		cntArgs = append(cntArgs, projectID)
	}
	if category != "" {
		base += ` AND LOWER(TRIM(COALESCE(a.product_category,'')))=?`
		cnt += ` AND LOWER(TRIM(COALESCE(a.product_category,'')))=?`
		args = append(args, strings.ToLower(strings.TrimSpace(category)))
		cntArgs = append(cntArgs, strings.ToLower(strings.TrimSpace(category)))
	}
	if search != "" {
		like := "%" + search + "%"
		f := ` AND (a.product_name LIKE ? OR a.serial_number LIKE ? OR c.org_name LIKE ? OR a.model_name LIKE ? OR a.install_location LIKE ? OR a.asset_id LIKE ?)`
		base += f
		cnt += f
		args = append(args, like, like, like, like, like, like)
		cntArgs = append(cntArgs, like, like, like, like, like, like)
	}

	var total int
	if err := r.db.QueryRow(cnt, cntArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	orderDir := "ASC"
	if dir == "desc" {
		orderDir = "DESC"
	}
	base += " " + assetListOrderBy(sort, orderDir)
	if pageSize > 0 {
		base += ` LIMIT ? OFFSET ?`
		args = append(args, pageSize, offset)
	}

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
			&a.ProductCategory,
			&a.ModelName, &a.Manufacturer, &a.SerialNumber,
			&a.InstallDate, &a.OperationStatus, &a.ManagementType, &managed,
			&a.MaintContractType, &a.MaintCycle,
			&a.OrgName, &a.BuildingName, &a.FloorName, &a.RoomName,
			&a.InstallLocation, &a.ProjectID, &a.ProjectName,
			&a.AsCount, &a.InstallYears,
		); err != nil {
			return nil, 0, err
		}
		a.IsManaged = managed == 1
		items = append(items, a)
	}
	return items, total, rows.Err()
}

// ListExport 엑셀용 전체 목록 (현재 필터·정렬 반영, 페이징 없음)
func (r *AssetRepo) ListExport(customerID, search, projectID, category, sort, dir string) ([]model.Asset, error) {
	items, _, err := r.List(customerID, search, projectID, category, sort, dir, 1, 0)
	return items, err
}

// CountByProject 사업에 연결된 설치자산 건수
func (r *AssetRepo) CountByProject(projectID string) (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM assets WHERE project_id=?`, projectID).Scan(&n)
	return n, err
}

func assetListOrderBy(sort, orderDir string) string {
	switch sort {
	case "asset_id":
		return `ORDER BY a.asset_id ` + orderDir
	case "org_name":
		return `ORDER BY c.org_name COLLATE NOCASE ` + orderDir + `, a.product_name ASC`
	case "product_name":
		return `ORDER BY a.product_name COLLATE NOCASE ` + orderDir + `, c.org_name ASC`
	case "product_category":
		return `ORDER BY a.product_category ` + orderDir + `, c.org_name ASC`
	case "product_type":
		return `ORDER BY a.product_type ` + orderDir + `, a.product_name ASC`
	case "serial_number":
		return `ORDER BY a.serial_number ` + orderDir
	case "location":
		return `ORDER BY bname ` + orderDir + `, fname ` + orderDir + `, rname ` + orderDir
	case "install_location":
		return `ORDER BY a.install_location COLLATE NOCASE ` + orderDir
	case "maint_contract":
		return `ORDER BY a.maint_contract_type ` + orderDir + `, a.maint_cycle ` + orderDir
	case "status":
		return `ORDER BY a.operation_status ` + orderDir + `, c.org_name ASC`
	case "install_years":
		return `ORDER BY yrs ` + orderDir + `, a.install_date ` + orderDir
	case "as_count":
		return `ORDER BY as_cnt ` + orderDir + `, c.org_name ASC`
	case "project":
		return `ORDER BY project_name COLLATE NOCASE ` + orderDir + `, c.org_name ASC`
	default:
		return `ORDER BY c.org_name COLLATE NOCASE ASC, a.product_name ASC`
	}
}

func (r *AssetRepo) GetByID(id string) (*model.Asset, error) {
	q := `
		SELECT a.asset_id, a.customer_id, a.product_name, COALESCE(a.product_type,''),
		       COALESCE(a.product_category,''),
		       COALESCE(a.model_name,''), COALESCE(a.manufacturer,''), COALESCE(a.serial_number,''),
		       COALESCE(a.install_date,''), COALESCE(a.retire_date,''),
		       COALESCE(a.installer_type,''), COALESCE(a.original_installer,''),
		       COALESCE(a.operation_status,'operating'), COALESCE(a.management_type,''),
		       a.is_managed,
		       COALESCE(a.maint_contract_type,''), COALESCE(a.maint_cycle,''),
		       COALESCE(a.maint_start_date,''), COALESCE(a.maint_end_date,''),
		       COALESCE(a.maint_billing_party,''), COALESCE(a.maint_billing_cycle,''),
		       COALESCE(a.requester_type,''), COALESCE(a.requester_name,''),
		       COALESCE(a.customer_contact_id,''), COALESCE(a.our_contact,''),
		       COALESCE(a.building_id,''), COALESCE(a.floor_id,''), COALESCE(a.room_id,''),
		       COALESCE(a.loc_building_name,''), COALESCE(a.loc_floor_name,''), COALESCE(a.loc_room_name,''),
		       COALESCE(a.install_location,''),
		       COALESCE(a.location_detail,''), COALESCE(a.notes,''),
		       COALESCE(a.project_id,''), COALESCE(a.sales_order_id,''),
		       a.created_at, a.updated_at,
		       c.org_name,
		       COALESCE(NULLIF(TRIM(a.loc_building_name),''), b.building_name,''),
		       COALESCE(NULLIF(TRIM(a.loc_floor_name),''), f.floor_name,''),
		       COALESCE(NULLIF(TRIM(a.loc_room_name),''), rm.room_name,''),
		       COALESCE(NULLIF(TRIM(p.short_name),''), p.name, '')
		FROM assets a
		JOIN customers c ON c.customer_id=a.customer_id
		LEFT JOIN customer_buildings b ON b.building_id=a.building_id
		LEFT JOIN customer_floors f ON f.floor_id=a.floor_id
		LEFT JOIN customer_rooms rm ON rm.room_id=a.room_id
		LEFT JOIN work_projects p ON p.project_id=a.project_id
		WHERE a.asset_id=?`

	var a model.Asset
	var managed int
	var createdAt, updatedAt string
	err := r.db.QueryRow(q, id).Scan(
		&a.AssetID, &a.CustomerID, &a.ProductName, &a.ProductType,
		&a.ProductCategory,
		&a.ModelName, &a.Manufacturer, &a.SerialNumber,
		&a.InstallDate, &a.RetireDate,
		&a.InstallerType, &a.OriginalInstaller,
		&a.OperationStatus, &a.ManagementType,
		&managed,
		&a.MaintContractType, &a.MaintCycle,
		&a.MaintStartDate, &a.MaintEndDate,
		&a.MaintBillingParty, &a.MaintBillingCycle,
		&a.RequesterType, &a.RequesterName,
		&a.CustomerContactID, &a.OurContact,
		&a.BuildingID, &a.FloorID, &a.RoomID,
		&a.LocBuildingName, &a.LocFloorName, &a.LocRoomName,
		&a.InstallLocation,
		&a.LocationDetail, &a.Notes,
		&a.ProjectID, &a.SalesOrderID,
		&createdAt, &updatedAt,
		&a.OrgName, &a.BuildingName, &a.FloorName, &a.RoomName,
		&a.ProjectName,
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
	id, err := NextAssetID(r.db, a.ModelName, a.ProductName, a.ProductType, a.Manufacturer)
	if err != nil {
		return err
	}
	a.AssetID = id
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err = r.db.Exec(`
		INSERT INTO assets (
			asset_id, customer_id, product_name, product_type, product_category, model_name,
			manufacturer, serial_number, install_date, retire_date,
			installer_type, original_installer, operation_status, management_type,
			is_managed,
			maint_contract_type, maint_cycle, maint_start_date, maint_end_date,
			maint_billing_party, maint_billing_cycle,
			requester_type, requester_name,
			customer_contact_id, our_contact,
			building_id, floor_id, room_id,
			loc_building_name, loc_floor_name, loc_room_name,
			install_location, location_detail, notes, project_id, sales_order_id,
			created_at, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.AssetID, a.CustomerID, a.ProductName, a.ProductType, a.ProductCategory, a.ModelName,
		a.Manufacturer, a.SerialNumber, a.InstallDate, a.RetireDate,
		a.InstallerType, a.OriginalInstaller, a.OperationStatus, a.ManagementType,
		boolToInt(a.IsManaged),
		a.MaintContractType, a.MaintCycle, a.MaintStartDate, a.MaintEndDate,
		a.MaintBillingParty, a.MaintBillingCycle,
		a.RequesterType, a.RequesterName,
		a.CustomerContactID, a.OurContact,
		nullStr(a.BuildingID), nullStr(a.FloorID), nullStr(a.RoomID),
		a.LocBuildingName, a.LocFloorName, a.LocRoomName,
		a.InstallLocation, a.LocationDetail, a.Notes, nullStr(a.ProjectID), a.SalesOrderID, now, now,
	)
	if err != nil {
		return err
	}
	logCreate(r.db, "assets", "asset_id", a.AssetID, a.ProductName+" "+a.ModelName)
	return nil
}

func (r *AssetRepo) Update(a *model.Asset) error {
	return touchUpdate(r.db, "assets", "asset_id", a.AssetID, a.ProductName+" "+a.ModelName, func() error {
		now := time.Now().Format("2006-01-02 15:04:05")
		_, err := r.db.Exec(`
		UPDATE assets SET
			customer_id=?, product_name=?, product_type=?, product_category=?, model_name=?,
			manufacturer=?, serial_number=?, install_date=?, retire_date=?,
			installer_type=?, original_installer=?, operation_status=?, management_type=?,
			is_managed=?,
			maint_contract_type=?, maint_cycle=?, maint_start_date=?, maint_end_date=?,
			maint_billing_party=?, maint_billing_cycle=?,
			requester_type=?, requester_name=?,
			customer_contact_id=?, our_contact=?,
			building_id=?, floor_id=?, room_id=?,
			loc_building_name=?, loc_floor_name=?, loc_room_name=?,
			install_location=?, location_detail=?, notes=?, project_id=?,
			updated_at=?
		WHERE asset_id=?`,
			a.CustomerID, a.ProductName, a.ProductType, a.ProductCategory, a.ModelName,
			a.Manufacturer, a.SerialNumber, a.InstallDate, a.RetireDate,
			a.InstallerType, a.OriginalInstaller, a.OperationStatus, a.ManagementType,
			boolToInt(a.IsManaged),
			a.MaintContractType, a.MaintCycle, a.MaintStartDate, a.MaintEndDate,
			a.MaintBillingParty, a.MaintBillingCycle,
			a.RequesterType, a.RequesterName,
			a.CustomerContactID, a.OurContact,
			nullStr(a.BuildingID), nullStr(a.FloorID), nullStr(a.RoomID),
			a.LocBuildingName, a.LocFloorName, a.LocRoomName,
			a.InstallLocation, a.LocationDetail, a.Notes, nullStr(a.ProjectID), now, a.AssetID,
		)
		return err
	})
}

func (r *AssetRepo) UpdateOperationStatus(id, status string) error {
	status = model.AssignAssetKanbanColumn(status)
	id = strings.TrimSpace(id)
	if id == "" {
		return sql.ErrNoRows
	}
	return touchUpdate(r.db, "assets", "asset_id", id, "운영상태", func() error {
		res, err := r.db.Exec(
			`UPDATE assets SET operation_status=?, updated_at=? WHERE asset_id=?`,
			status, time.Now().Format("2006-01-02 15:04:05"), id)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return sql.ErrNoRows
		}
		return nil
	})
}

func (r *AssetRepo) Delete(id string) error {
	return touchUpdate(r.db, "assets", "asset_id", id, "설치자산", func() error {
		_, err := r.db.Exec(
			`UPDATE assets SET operation_status='disposed', updated_at=? WHERE asset_id=?`,
			time.Now().Format("2006-01-02 15:04:05"), id)
		return err
	})
}

// ListForTab 고객 상세 탭 전용 — 화면 표시에 필요한 컬럼 포함
func (r *AssetRepo) ListForTab(customerID string) ([]model.Asset, error) {
	rows, err := r.db.Query(`
		SELECT asset_id, COALESCE(product_name,''), COALESCE(product_type,''),
		       COALESCE(product_category,''),
		       COALESCE(model_name,''), COALESCE(serial_number,''),
		       COALESCE(install_date,''), COALESCE(operation_status,'operating'),
		       COALESCE(maint_contract_type,''), COALESCE(maint_cycle,''),
		       COALESCE(install_location,''),
		       TRIM(COALESCE(loc_building_name,'') || ' ' || COALESCE(loc_floor_name,'') || ' ' ||
		            COALESCE(loc_room_name,'') || ' ' || COALESCE(location_detail,''))
		FROM assets
		WHERE customer_id = ?
		ORDER BY install_date DESC, product_name`, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.Asset
	for rows.Next() {
		var a model.Asset
		if err := rows.Scan(
			&a.AssetID, &a.ProductName, &a.ProductType,
			&a.ProductCategory,
			&a.ModelName, &a.SerialNumber,
			&a.InstallDate, &a.OperationStatus,
			&a.MaintContractType, &a.MaintCycle,
			&a.InstallLocation,
			&a.LocationDetail,
		); err != nil {
			return nil, err
		}
		items = append(items, a)
	}
	return items, rows.Err()
}

func (r *AssetRepo) ListByCustomer(customerID string) ([]model.Asset, error) {
	rows, err := r.db.Query(
		`SELECT a.asset_id, a.product_name, COALESCE(a.product_type,''), COALESCE(a.product_category,''), COALESCE(a.model_name,''), COALESCE(a.serial_number,''),
		        COALESCE(a.operation_status,'operating'),
		        COALESCE(NULLIF(TRIM(a.loc_building_name),''), b.building_name,''),
		        COALESCE(NULLIF(TRIM(a.loc_floor_name),''), f.floor_name,''),
		        COALESCE(NULLIF(TRIM(a.loc_room_name),''), rm.room_name,''),
		        COALESCE(a.install_location,'')
		 FROM assets a
		 LEFT JOIN customer_buildings b ON b.building_id=a.building_id
		 LEFT JOIN customer_floors f ON f.floor_id=a.floor_id
		 LEFT JOIN customer_rooms rm ON rm.room_id=a.room_id
		 WHERE a.customer_id=? AND a.operation_status!='disposed' ORDER BY a.product_name, a.install_location, a.asset_id`,
		customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.Asset
	for rows.Next() {
		var a model.Asset
		if err := rows.Scan(&a.AssetID, &a.ProductName, &a.ProductType, &a.ProductCategory, &a.ModelName, &a.SerialNumber, &a.OperationStatus,
			&a.BuildingName, &a.FloorName, &a.RoomName, &a.InstallLocation); err != nil {
			return nil, err
		}
		items = append(items, a)
	}
	return items, rows.Err()
}

// CountOperating 운영 중(폐기·철수 제외) 자산 수
func (r *AssetRepo) CountOperating(count *int) {
	r.db.QueryRow(`
		SELECT COUNT(*) FROM assets
		WHERE operation_status NOT IN ('disposed','retired')`).Scan(count)
}

// ListMaintExport 고객현황 엑셀용 — 기관별 설치자산 유지보수 계약 행
func (r *AssetRepo) ListMaintExport(customerIDs []string) ([]model.Asset, error) {
	if len(customerIDs) == 0 {
		return nil, nil
	}
	allow := make(map[string]struct{}, len(customerIDs))
	for _, id := range customerIDs {
		allow[id] = struct{}{}
	}
	rows, err := r.db.Query(`
		SELECT a.asset_id, a.customer_id, COALESCE(c.org_name,''),
		       COALESCE(a.product_name,''), COALESCE(a.product_type,''), COALESCE(a.product_category,''),
		       COALESCE(a.model_name,''), COALESCE(a.serial_number,''), COALESCE(a.install_date,''),
		       COALESCE(a.operation_status,'operating'),
		       COALESCE(a.maint_contract_type,''), COALESCE(a.maint_cycle,''),
		       COALESCE(a.maint_start_date,''), COALESCE(a.maint_end_date,''),
		       COALESCE(a.maint_billing_party,''), COALESCE(a.maint_billing_cycle,''),
		       COALESCE(a.install_location,''),
		       COALESCE(NULLIF(TRIM(a.loc_building_name),''), b.building_name,''),
		       COALESCE(NULLIF(TRIM(a.loc_floor_name),''), f.floor_name,''),
		       COALESCE(NULLIF(TRIM(a.loc_room_name),''), rm.room_name,''),
		       COALESCE(a.notes,'')
		FROM assets a
		JOIN customers c ON c.customer_id = a.customer_id
		LEFT JOIN customer_buildings b ON b.building_id = a.building_id
		LEFT JOIN customer_floors f ON f.floor_id = a.floor_id
		LEFT JOIN customer_rooms rm ON rm.room_id = a.room_id
		WHERE a.operation_status NOT IN ('disposed','retired')
		ORDER BY c.org_name, a.product_name, a.install_location, a.asset_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.Asset
	for rows.Next() {
		var a model.Asset
		if err := rows.Scan(
			&a.AssetID, &a.CustomerID, &a.OrgName,
			&a.ProductName, &a.ProductType, &a.ProductCategory,
			&a.ModelName, &a.SerialNumber, &a.InstallDate,
			&a.OperationStatus,
			&a.MaintContractType, &a.MaintCycle,
			&a.MaintStartDate, &a.MaintEndDate,
			&a.MaintBillingParty, &a.MaintBillingCycle,
			&a.InstallLocation,
			&a.BuildingName, &a.FloorName, &a.RoomName,
			&a.Notes,
		); err != nil {
			return nil, err
		}
		if _, ok := allow[a.CustomerID]; !ok {
			continue
		}
		items = append(items, a)
	}
	return items, rows.Err()
}

func (r *AssetRepo) ListBySalesOrder(orderID string) ([]model.Asset, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT asset_id, customer_id, COALESCE(product_name,''), COALESCE(product_category,''),
			COALESCE(model_name,''), COALESCE(manufacturer,''), COALESCE(serial_number,''),
			COALESCE(install_date,''), COALESCE(installer_type,''), COALESCE(install_location,''),
			COALESCE(maint_contract_type,''), COALESCE(maint_cycle,''),
			COALESCE(maint_start_date,''), COALESCE(maint_end_date,''),
			COALESCE(maint_billing_party,''), COALESCE(maint_billing_cycle,''),
			COALESCE(sales_order_id,'')
		FROM assets WHERE sales_order_id=? ORDER BY asset_id`, orderID)
	if err != nil {
		if strings.Contains(err.Error(), "no such column") || strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var items []model.Asset
	for rows.Next() {
		var a model.Asset
		if err := rows.Scan(&a.AssetID, &a.CustomerID, &a.ProductName, &a.ProductCategory,
			&a.ModelName, &a.Manufacturer, &a.SerialNumber, &a.InstallDate, &a.InstallerType, &a.InstallLocation,
			&a.MaintContractType, &a.MaintCycle, &a.MaintStartDate, &a.MaintEndDate,
			&a.MaintBillingParty, &a.MaintBillingCycle, &a.SalesOrderID); err != nil {
			return nil, err
		}
		items = append(items, a)
	}
	return items, rows.Err()
}
