package repository

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"customer-support/internal/model"
)

type ProjectEquipRepo struct {
	db   *sql.DB
	proj *ProjectRepo
}

func NewProjectEquipRepo(db *sql.DB) *ProjectEquipRepo {
	return &ProjectEquipRepo{db: db, proj: NewProjectRepo(db)}
}

func (r *ProjectEquipRepo) ProjectOrg(projectID string) (string, error) {
	var org string
	err := r.db.QueryRow(`SELECT TRIM(COALESCE(org_id,'')) FROM work_projects WHERE project_id=?`, projectID).Scan(&org)
	if err != nil {
		return "", err
	}
	return org, nil
}

func (r *ProjectEquipRepo) bindOrg(projectID, want string) (string, error) {
	org, err := r.ProjectOrg(projectID)
	if err != nil {
		return "", err
	}
	if org == "" || org == OrgAll {
		if want == "" || want == OrgAll {
			return "", fmt.Errorf("등록용 조직이 사업과 다릅니다")
		}
		return want, nil
	}
	if want != "" && want != org {
		return "", fmt.Errorf("등록용 조직이 사업과 다릅니다")
	}
	return org, nil
}

func (r *ProjectEquipRepo) GetProject(id string) (*model.WorkProject, error) {
	return r.proj.Get(id)
}

func (r *ProjectEquipRepo) nextID(key, prefix string) (string, error) {
	n, err := NextSeq(r.db, key)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%04d", prefix, n), nil
}

func (r *ProjectEquipRepo) ListYears(projectID string) ([]model.ContractYear, error) {
	rows, err := r.db.Query(`SELECT id, project_id, year_no, COALESCE(label,''), start_date, end_date, amount, status, COALESCE(note,''), org_id, COALESCE(created_by,'')
		FROM contract_years WHERE project_id=? ORDER BY year_no`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	today := time.Now().Format("2006-01-02")
	var out []model.ContractYear
	for rows.Next() {
		var y model.ContractYear
		if err := rows.Scan(&y.ID, &y.ProjectID, &y.YearNo, &y.Label, &y.StartDate, &y.EndDate, &y.Amount, &y.Status, &y.Note, &y.OrgID, &y.CreatedBy); err != nil {
			return nil, err
		}
		y.IsCurrent = y.Status == model.YearActive || (y.Status != model.YearCancelled && y.StartDate <= today && today <= y.EndDate)
		out = append(out, y)
	}
	return out, rows.Err()
}

func (r *ProjectEquipRepo) SaveYear(y *model.ContractYear, userID string) error {
	if y.YearNo < 1 {
		return fmt.Errorf("연차 순번은 1부터입니다")
	}
	if y.StartDate == "" || y.EndDate == "" || y.EndDate < y.StartDate {
		return fmt.Errorf("기간을 확인하세요")
	}
	switch y.Status {
	case model.YearPlanned, model.YearActive, model.YearDone, model.YearCancelled:
	default:
		return fmt.Errorf("상태를 확인하세요")
	}
	org, err := r.bindOrg(y.ProjectID, y.OrgID)
	if err != nil {
		return err
	}
	y.OrgID = org
	if y.ID == "" {
		id, err := r.nextID("contract_year", "CY")
		if err != nil {
			return err
		}
		y.ID = id
		y.CreatedBy = userID
		_, err = r.db.Exec(`INSERT INTO contract_years(id,project_id,year_no,label,start_date,end_date,amount,status,note,org_id,created_by)
			VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
			y.ID, y.ProjectID, y.YearNo, y.Label, y.StartDate, y.EndDate, y.Amount, y.Status, y.Note, y.OrgID, userID)
		if err != nil {
			return mapYearErr(err)
		}
		logCreate(r.db, "contract_years", "id", y.ID, y.Label)
		return nil
	}
	_, err = r.db.Exec(`UPDATE contract_years SET year_no=?, label=?, start_date=?, end_date=?, amount=?, status=?, note=? WHERE id=? AND project_id=?`,
		y.YearNo, y.Label, y.StartDate, y.EndDate, y.Amount, y.Status, y.Note, y.ID, y.ProjectID)
	if err != nil {
		return mapYearErr(err)
	}
	logUpdate(r.db, "contract_years", "id", y.ID, y.Label, "")
	return nil
}

func mapYearErr(err error) error {
	if err == nil {
		return nil
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "unique") || strings.Contains(s, "constraint") && strings.Contains(s, "year_no") {
		return fmt.Errorf("연차 순번이 겹칩니다")
	}
	if strings.Contains(s, "overlap") {
		return fmt.Errorf("연차 기간이 겹칩니다")
	}
	return err
}

func (r *ProjectEquipRepo) YearSumWarn(projectID string, contractAmount int) (int, bool) {
	var sum int
	_ = r.db.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM contract_years WHERE project_id=? AND status!='cancelled'`, projectID).Scan(&sum)
	if contractAmount == 0 {
		return sum, false
	}
	return sum, sum != contractAmount
}

func (r *ProjectEquipRepo) ListSites(projectID, q, region, status string, page, size int) ([]model.ProjectSite, int, error) {
	if size != 100 {
		size = 50
	}
	if page < 1 {
		page = 1
	}
	where := `project_id=? AND is_active=1`
	args := []interface{}{projectID}
	if q = strings.TrimSpace(q); q != "" {
		where += ` AND (site_name LIKE ? OR region LIKE ? OR address LIKE ?)`
		like := "%" + q + "%"
		args = append(args, like, like, like)
	}
	if region = strings.TrimSpace(region); region != "" {
		where += ` AND region=?`
		args = append(args, region)
	}
	if status = strings.TrimSpace(status); status != "" {
		where += ` AND status=?`
		args = append(args, status)
	}
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM project_sites WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, size, (page-1)*size)
	rows, err := r.db.Query(`SELECT id, project_id, COALESCE(customer_id,''), site_name, COALESCE(region,''), COALESCE(address,''), COALESCE(contact_note,''), status, sort_order, is_active, COALESCE(note,''), org_id,
		(SELECT COUNT(*) FROM project_equipment e WHERE e.site_id=s.id AND e.is_active=1)
		FROM project_sites s WHERE `+where+` ORDER BY sort_order, site_name LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []model.ProjectSite
	for rows.Next() {
		var s model.ProjectSite
		var active int
		if err := rows.Scan(&s.ID, &s.ProjectID, &s.CustomerID, &s.SiteName, &s.Region, &s.Address, &s.ContactNote, &s.Status, &s.SortOrder, &active, &s.Note, &s.OrgID, &s.EquipN); err != nil {
			return nil, 0, err
		}
		s.IsActive = active == 1
		out = append(out, s)
	}
	return out, total, rows.Err()
}

func (r *ProjectEquipRepo) GetSite(id string) (*model.ProjectSite, error) {
	var s model.ProjectSite
	var active int
	err := r.db.QueryRow(`SELECT id, project_id, COALESCE(customer_id,''), site_name, COALESCE(region,''), COALESCE(address,''), COALESCE(contact_note,''), status, sort_order, is_active, COALESCE(note,''), org_id
		FROM project_sites WHERE id=?`, id).Scan(&s.ID, &s.ProjectID, &s.CustomerID, &s.SiteName, &s.Region, &s.Address, &s.ContactNote, &s.Status, &s.SortOrder, &active, &s.Note, &s.OrgID)
	if err != nil {
		return nil, err
	}
	s.IsActive = active == 1
	return &s, nil
}

func (r *ProjectEquipRepo) SaveSite(s *model.ProjectSite) error {
	s.SiteName = strings.TrimSpace(s.SiteName)
	if s.SiteName == "" {
		return fmt.Errorf("사이트명을 입력하세요")
	}
	s.Status = model.NormalizeSiteStatus(s.Status)
	if s.Status == "" {
		return fmt.Errorf("상태를 확인하세요")
	}
	org, err := r.bindOrg(s.ProjectID, s.OrgID)
	if err != nil {
		return err
	}
	s.OrgID = org
	cust := nullIfEmpty(s.CustomerID)
	if s.ID == "" {
		id, err := r.nextID("project_site", "PS")
		if err != nil {
			return err
		}
		s.ID = id
		_, err = r.db.Exec(`INSERT INTO project_sites(id,project_id,customer_id,site_name,region,address,contact_note,status,sort_order,is_active,note,org_id)
			VALUES(?,?,?,?,?,?,?,?,?,1,?,?)`,
			s.ID, s.ProjectID, cust, s.SiteName, s.Region, s.Address, s.ContactNote, s.Status, s.SortOrder, s.Note, s.OrgID)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				if s.CustomerID != "" {
					return fmt.Errorf("이미 대상입니다")
				}
				return fmt.Errorf("같은 사이트명이 있습니다")
			}
			return err
		}
		logCreate(r.db, "project_sites", "id", s.ID, s.SiteName)
		return nil
	}
	_, err = r.db.Exec(`UPDATE project_sites SET customer_id=?, site_name=?, region=?, address=?, contact_note=?, status=?, sort_order=?, note=? WHERE id=? AND project_id=?`,
		cust, s.SiteName, s.Region, s.Address, s.ContactNote, s.Status, s.SortOrder, s.Note, s.ID, s.ProjectID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return fmt.Errorf("같은 사이트명이 있습니다")
		}
		return err
	}
	logUpdate(r.db, "project_sites", "id", s.ID, s.SiteName, "")
	return nil
}

func (r *ProjectEquipRepo) DeactivateSite(id, projectID string) error {
	_, err := r.db.Exec(`UPDATE project_sites SET is_active=0 WHERE id=? AND project_id=?`, id, projectID)
	if err == nil {
		logUpdate(r.db, "project_sites", "id", id, id, "")
	}
	return err
}

func (r *ProjectEquipRepo) FindSiteByName(projectID, name string) (*model.ProjectSite, error) {
	var s model.ProjectSite
	var active int
	err := r.db.QueryRow(`SELECT id, project_id, COALESCE(customer_id,''), site_name, COALESCE(region,''), COALESCE(address,''), COALESCE(contact_note,''), status, sort_order, is_active, COALESCE(note,''), org_id
		FROM project_sites WHERE project_id=? AND site_name=?`, projectID, name).Scan(&s.ID, &s.ProjectID, &s.CustomerID, &s.SiteName, &s.Region, &s.Address, &s.ContactNote, &s.Status, &s.SortOrder, &active, &s.Note, &s.OrgID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.IsActive = active == 1
	return &s, nil
}

func (r *ProjectEquipRepo) ListCategories(orgID string, includeInactive bool) ([]model.EquipmentCategory, error) {
	q := `SELECT id, org_id, COALESCE(parent_id,''), code, name, sort_order, is_active FROM equipment_categories WHERE org_id=?`
	if !includeInactive {
		q += ` AND is_active=1`
	}
	q += ` ORDER BY sort_order, code`
	rows, err := r.db.Query(q, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.EquipmentCategory
	for rows.Next() {
		var c model.EquipmentCategory
		var active int
		if err := rows.Scan(&c.ID, &c.OrgID, &c.ParentID, &c.Code, &c.Name, &c.SortOrder, &active); err != nil {
			return nil, err
		}
		c.IsActive = active == 1
		var n int
		_ = r.db.QueryRow(`SELECT COUNT(*) FROM project_equipment WHERE category_id=? AND is_active=1`, c.ID).Scan(&n)
		c.InUse = n > 0
		fs, _ := r.ListFields(c.ID, includeInactive)
		c.Fields = fs
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *ProjectEquipRepo) CategoryByCode(orgID, code string) (*model.EquipmentCategory, error) {
	var c model.EquipmentCategory
	var active int
	err := r.db.QueryRow(`SELECT id, org_id, COALESCE(parent_id,''), code, name, sort_order, is_active FROM equipment_categories WHERE org_id=? AND code=?`, orgID, code).
		Scan(&c.ID, &c.OrgID, &c.ParentID, &c.Code, &c.Name, &c.SortOrder, &active)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.IsActive = active == 1
	return &c, nil
}

func (r *ProjectEquipRepo) SaveCategory(c *model.EquipmentCategory) error {
	c.Code = strings.TrimSpace(c.Code)
	c.Name = strings.TrimSpace(c.Name)
	if c.OrgID == "" || c.OrgID == OrgAll {
		return fmt.Errorf("등록용 조직을 고르세요")
	}
	if c.Code == "" || c.Name == "" {
		return fmt.Errorf("분류코드와 분류명은 필수입니다")
	}
	if c.ParentID != "" {
		p, err := r.categoryByID(c.ParentID)
		if err != nil || p == nil {
			return fmt.Errorf("상위 분류가 없습니다")
		}
		if p.OrgID != c.OrgID {
			return fmt.Errorf("상위 분류 조직이 다릅니다")
		}
		if p.ParentID != "" {
			return fmt.Errorf("분류는 2단계까지입니다")
		}
		if p.ID == c.ID {
			return fmt.Errorf("자기 자신을 상위로 둘 수 없습니다")
		}
	}
	active := 1
	if !c.IsActive {
		active = 0
	}
	if c.ID == "" {
		id, err := r.nextID("equip_cat", "EC")
		if err != nil {
			return err
		}
		c.ID = id
		_, err = r.db.Exec(`INSERT INTO equipment_categories(id,org_id,parent_id,code,name,sort_order,is_active) VALUES(?,?,?,?,?,?,?)`,
			c.ID, c.OrgID, nullIfEmpty(c.ParentID), c.Code, c.Name, c.SortOrder, active)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				return fmt.Errorf("같은 분류코드가 있습니다")
			}
			return err
		}
		logCreate(r.db, "equipment_categories", "id", c.ID, c.Name)
		return nil
	}
	_, err := r.db.Exec(`UPDATE equipment_categories SET parent_id=?, code=?, name=?, sort_order=?, is_active=? WHERE id=? AND org_id=?`,
		nullIfEmpty(c.ParentID), c.Code, c.Name, c.SortOrder, active, c.ID, c.OrgID)
	if err != nil {
		return err
	}
	logUpdate(r.db, "equipment_categories", "id", c.ID, c.Name, "")
	return nil
}

func (r *ProjectEquipRepo) categoryByID(id string) (*model.EquipmentCategory, error) {
	var c model.EquipmentCategory
	var active int
	err := r.db.QueryRow(`SELECT id, org_id, COALESCE(parent_id,''), code, name, sort_order, is_active FROM equipment_categories WHERE id=?`, id).
		Scan(&c.ID, &c.OrgID, &c.ParentID, &c.Code, &c.Name, &c.SortOrder, &active)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.IsActive = active == 1
	return &c, nil
}

func (r *ProjectEquipRepo) DeleteCategory(id, orgID string) error {
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM project_equipment WHERE category_id=? AND is_active=1`, id).Scan(&n)
	if n > 0 {
		return fmt.Errorf("사용 중인 분류는 삭제할 수 없습니다")
	}
	_, err := r.db.Exec(`UPDATE equipment_categories SET is_active=0 WHERE id=? AND org_id=?`, id, orgID)
	return err
}

func (r *ProjectEquipRepo) ListFields(categoryID string, includeInactive bool) ([]model.EquipmentField, error) {
	q := `SELECT id, category_id, field_key, label, field_type, COALESCE(options,''), required, sort_order, is_active FROM equipment_category_fields WHERE category_id=?`
	if !includeInactive {
		q += ` AND is_active=1`
	}
	q += ` ORDER BY sort_order, field_key`
	rows, err := r.db.Query(q, categoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.EquipmentField
	for rows.Next() {
		var f model.EquipmentField
		var req, active int
		if err := rows.Scan(&f.ID, &f.CategoryID, &f.FieldKey, &f.Label, &f.FieldType, &f.Options, &req, &f.SortOrder, &active); err != nil {
			return nil, err
		}
		f.Required = req == 1
		f.IsActive = active == 1
		var n int
		_ = r.db.QueryRow(`SELECT COUNT(*) FROM project_equipment_values WHERE field_id=?`, f.ID).Scan(&n)
		f.InUse = n > 0
		out = append(out, f)
	}
	return out, rows.Err()
}

func (r *ProjectEquipRepo) SaveField(f *model.EquipmentField) error {
	f.FieldKey = strings.TrimSpace(f.FieldKey)
	f.Label = strings.TrimSpace(f.Label)
	switch f.FieldType {
	case model.EquipFieldText, model.EquipFieldNumber, model.EquipFieldDate, model.EquipFieldSelect:
	default:
		return fmt.Errorf("형식을 확인하세요")
	}
	if f.FieldKey == "" || f.Label == "" {
		return fmt.Errorf("필드키와 항목명은 필수입니다")
	}
	req, active := 0, 1
	if f.Required {
		req = 1
	}
	if !f.IsActive {
		active = 0
	}
	if f.ID == "" {
		id, err := r.nextID("equip_field", "EF")
		if err != nil {
			return err
		}
		f.ID = id
		_, err = r.db.Exec(`INSERT INTO equipment_category_fields(id,category_id,field_key,label,field_type,options,required,sort_order,is_active)
			VALUES(?,?,?,?,?,?,?,?,?)`, f.ID, f.CategoryID, f.FieldKey, f.Label, f.FieldType, f.Options, req, f.SortOrder, active)
		return err
	}
	_, err := r.db.Exec(`UPDATE equipment_category_fields SET label=?, field_type=?, options=?, required=?, sort_order=?, is_active=? WHERE id=?`,
		f.Label, f.FieldType, f.Options, req, f.SortOrder, active, f.ID)
	return err
}

func (r *ProjectEquipRepo) DeleteField(id string) error {
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM project_equipment_values WHERE field_id=?`, id).Scan(&n)
	if n > 0 {
		return fmt.Errorf("사용 중인 항목은 삭제할 수 없습니다")
	}
	_, err := r.db.Exec(`UPDATE equipment_category_fields SET is_active=0 WHERE id=?`, id)
	return err
}

func (r *ProjectEquipRepo) ListEquipment(projectID, siteID, status, q string, page, size int) ([]model.ProjectEquipment, int, error) {
	if size != 100 {
		size = 50
	}
	if page < 1 {
		page = 1
	}
	where := `e.project_id=? AND e.is_active=1`
	args := []interface{}{projectID}
	if siteID != "" {
		where += ` AND e.site_id=?`
		args = append(args, siteID)
	}
	if status != "" {
		where += ` AND e.status=?`
		args = append(args, status)
	}
	if q = strings.TrimSpace(q); q != "" {
		where += ` AND (e.product_name LIKE ? OR e.model LIKE ? OR COALESCE(e.serial_no,'') LIKE ?)`
		like := "%" + q + "%"
		args = append(args, like, like, like)
	}
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM project_equipment e WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, size, (page-1)*size)
	rows, err := r.db.Query(`SELECT e.id, e.project_id, e.site_id, s.site_name, COALESCE(e.year_id,''), COALESCE(y.year_no,0), COALESCE(e.category_id,''), COALESCE(c.code,''),
		e.product_name, COALESCE(e.model,''), COALESCE(e.manufacturer,''), e.qty, COALESCE(e.serial_no,''), COALESCE(e.install_location,''), COALESCE(e.install_date,''),
		e.status, COALESCE(e.asset_id,''), COALESCE(e.note,''), e.org_id, e.is_active
		FROM project_equipment e
		JOIN project_sites s ON s.id=e.site_id
		LEFT JOIN contract_years y ON y.id=e.year_id
		LEFT JOIN equipment_categories c ON c.id=e.category_id
		WHERE `+where+` ORDER BY s.site_name, e.product_name LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []model.ProjectEquipment
	for rows.Next() {
		var e model.ProjectEquipment
		var active int
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.SiteID, &e.SiteName, &e.YearID, &e.YearNo, &e.CategoryID, &e.CategoryCode, &e.ProductName, &e.Model, &e.Manufacturer, &e.Qty, &e.SerialNo, &e.InstallLocation, &e.InstallDate, &e.Status, &e.AssetID, &e.Note, &e.OrgID, &active); err != nil {
			return nil, 0, err
		}
		e.IsActive = active == 1
		out = append(out, e)
	}
	return out, total, rows.Err()
}

func (r *ProjectEquipRepo) GetEquipment(id string) (*model.ProjectEquipment, error) {
	var e model.ProjectEquipment
	var active int
	err := r.db.QueryRow(`SELECT e.id, e.project_id, e.site_id, s.site_name, COALESCE(e.year_id,''), COALESCE(y.year_no,0), COALESCE(e.category_id,''), COALESCE(c.code,''),
		e.product_name, COALESCE(e.model,''), COALESCE(e.manufacturer,''), e.qty, COALESCE(e.serial_no,''), COALESCE(e.install_location,''), COALESCE(e.install_date,''),
		e.status, COALESCE(e.asset_id,''), COALESCE(e.note,''), e.org_id, e.is_active
		FROM project_equipment e
		JOIN project_sites s ON s.id=e.site_id
		LEFT JOIN contract_years y ON y.id=e.year_id
		LEFT JOIN equipment_categories c ON c.id=e.category_id
		WHERE e.id=?`, id).Scan(&e.ID, &e.ProjectID, &e.SiteID, &e.SiteName, &e.YearID, &e.YearNo, &e.CategoryID, &e.CategoryCode, &e.ProductName, &e.Model, &e.Manufacturer, &e.Qty, &e.SerialNo, &e.InstallLocation, &e.InstallDate, &e.Status, &e.AssetID, &e.Note, &e.OrgID, &active)
	if err != nil {
		return nil, err
	}
	e.IsActive = active == 1
	e.Values = r.loadValues(e.ID)
	return &e, nil
}

func (r *ProjectEquipRepo) loadValues(id string) map[string]string {
	out := map[string]string{}
	rows, err := r.db.Query(`SELECT f.field_key, v.value FROM project_equipment_values v JOIN equipment_category_fields f ON f.id=v.field_id WHERE v.equipment_id=?`, id)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if rows.Scan(&k, &v) == nil {
			out[k] = v
		}
	}
	return out
}

func (r *ProjectEquipRepo) FindEquipBySerial(projectID, serial string) (*model.ProjectEquipment, error) {
	serial = strings.TrimSpace(serial)
	if serial == "" {
		return nil, nil
	}
	var id string
	err := r.db.QueryRow(`SELECT id FROM project_equipment WHERE project_id=? AND serial_no=? AND is_active=1`, projectID, serial).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r.GetEquipment(id)
}

func (r *ProjectEquipRepo) SerialDup(projectID, serial, exceptID string) bool {
	serial = strings.TrimSpace(serial)
	if serial == "" {
		return false
	}
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM project_equipment WHERE project_id=? AND serial_no=? AND is_active=1 AND id!=?`, projectID, serial, exceptID).Scan(&n)
	return n > 0
}

func (r *ProjectEquipRepo) SaveEquipment(e *model.ProjectEquipment) error {
	e.ProductName = strings.TrimSpace(e.ProductName)
	if e.ProductName == "" {
		return fmt.Errorf("제품명을 입력하세요")
	}
	if e.Qty < 1 {
		return fmt.Errorf("수량은 1 이상입니다")
	}
	if model.ParseEquipStatus(e.Status) == "" {
		return fmt.Errorf("상태를 확인하세요")
	}
	e.Status = model.ParseEquipStatus(e.Status)
	org, err := r.bindOrg(e.ProjectID, e.OrgID)
	if err != nil {
		return err
	}
	e.OrgID = org
	site, err := r.GetSite(e.SiteID)
	if err != nil || site == nil || site.ProjectID != e.ProjectID {
		return fmt.Errorf("사이트를 확인하세요")
	}
	e.SerialWarn = r.SerialDup(e.ProjectID, e.SerialNo, e.ID)
	if err := r.validateValues(e); err != nil {
		return err
	}
	serial := nullIfEmpty(e.SerialNo)
	year := nullIfEmpty(e.YearID)
	cat := nullIfEmpty(e.CategoryID)
	asset := nullIfEmpty(e.AssetID)
	if e.ID == "" {
		id, err := r.nextID("project_equip", "EQ")
		if err != nil {
			return err
		}
		e.ID = id
		_, err = r.db.Exec(`INSERT INTO project_equipment(id,project_id,site_id,year_id,category_id,product_name,model,manufacturer,qty,serial_no,install_location,install_date,status,asset_id,note,org_id,is_active)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,1)`,
			e.ID, e.ProjectID, e.SiteID, year, cat, e.ProductName, e.Model, e.Manufacturer, e.Qty, serial, e.InstallLocation, nullIfEmpty(e.InstallDate), e.Status, asset, e.Note, e.OrgID)
		if err != nil {
			return err
		}
		logCreate(r.db, "project_equipment", "id", e.ID, e.ProductName)
	} else {
		_, err = r.db.Exec(`UPDATE project_equipment SET site_id=?, year_id=?, category_id=?, product_name=?, model=?, manufacturer=?, qty=?, serial_no=?, install_location=?, install_date=?, status=?, asset_id=?, note=? WHERE id=? AND project_id=?`,
			e.SiteID, year, cat, e.ProductName, e.Model, e.Manufacturer, e.Qty, serial, e.InstallLocation, nullIfEmpty(e.InstallDate), e.Status, asset, e.Note, e.ID, e.ProjectID)
		if err != nil {
			return err
		}
		logUpdate(r.db, "project_equipment", "id", e.ID, e.ProductName, "")
	}
	return r.saveValues(e)
}

func (r *ProjectEquipRepo) validateValues(e *model.ProjectEquipment) error {
	if e.CategoryID == "" {
		return nil
	}
	fs, err := r.ListFields(e.CategoryID, false)
	if err != nil {
		return err
	}
	if e.Values == nil {
		e.Values = map[string]string{}
	}
	for _, f := range fs {
		v := strings.TrimSpace(e.Values[f.FieldKey])
		if f.Required && v == "" {
			return fmt.Errorf("%s 은(는) 필수입니다", f.Label)
		}
		if v == "" {
			continue
		}
		switch f.FieldType {
		case model.EquipFieldNumber:
			if _, err := strconv.ParseFloat(v, 64); err != nil {
				return fmt.Errorf("%s 숫자 형식이 아닙니다", f.Label)
			}
		case model.EquipFieldDate:
			if len(v) < 8 {
				return fmt.Errorf("%s 날짜 형식이 아닙니다", f.Label)
			}
		}
	}
	return nil
}

func (r *ProjectEquipRepo) saveValues(e *model.ProjectEquipment) error {
	if e.CategoryID == "" {
		return nil
	}
	fs, err := r.ListFields(e.CategoryID, true)
	if err != nil {
		return err
	}
	for _, f := range fs {
		v := ""
		if e.Values != nil {
			v = e.Values[f.FieldKey]
		}
		_, err := r.db.Exec(`INSERT INTO project_equipment_values(equipment_id, field_id, value) VALUES(?,?,?)
			ON CONFLICT(equipment_id, field_id) DO UPDATE SET value=excluded.value`, e.ID, f.ID, v)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *ProjectEquipRepo) DeactivateEquipment(id, projectID string) error {
	_, err := r.db.Exec(`UPDATE project_equipment SET is_active=0 WHERE id=? AND project_id=?`, id, projectID)
	if err == nil {
		logUpdate(r.db, "project_equipment", "id", id, id, "")
	}
	return err
}

func (r *ProjectEquipRepo) LinkAsset(id, projectID, assetID string) error {
	_, err := r.db.Exec(`UPDATE project_equipment SET asset_id=? WHERE id=? AND project_id=?`, nullIfEmpty(assetID), id, projectID)
	return err
}

func (r *ProjectEquipRepo) Summary(projectID string) (sites map[string]int, equip map[string]int, pending []string) {
	sites, equip = map[string]int{}, map[string]int{}
	rows, err := r.db.Query(`SELECT status, COUNT(*) FROM project_sites WHERE project_id=? AND is_active=1 GROUP BY status`, projectID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var st string
			var n int
			if rows.Scan(&st, &n) == nil {
				sites[st] = n
			}
		}
	}
	erows, err := r.db.Query(`SELECT status, SUM(qty) FROM project_equipment WHERE project_id=? AND is_active=1 GROUP BY status`, projectID)
	if err == nil {
		defer erows.Close()
		for erows.Next() {
			var st string
			var n int
			if erows.Scan(&st, &n) == nil {
				equip[st] = n
			}
		}
	}
	prows, err := r.db.Query(`SELECT DISTINCT s.site_name FROM project_sites s
		JOIN project_equipment e ON e.site_id=s.id AND e.is_active=1
		WHERE s.project_id=? AND s.is_active=1 AND e.status IN ('planned','delivered','installed')
		ORDER BY s.site_name LIMIT 20`, projectID)
	if err == nil {
		defer prows.Close()
		for prows.Next() {
			var n string
			if prows.Scan(&n) == nil {
				pending = append(pending, n)
			}
		}
	}
	return
}

func (r *ProjectEquipRepo) InstallRate(projectID string) (done, total int) {
	_ = r.db.QueryRow(`SELECT COALESCE(SUM(qty),0) FROM project_equipment WHERE project_id=? AND is_active=1`, projectID).Scan(&total)
	_ = r.db.QueryRow(`SELECT COALESCE(SUM(qty),0) FROM project_equipment WHERE project_id=? AND is_active=1 AND status IN ('installed','accepted')`, projectID).Scan(&done)
	return
}

func (r *ProjectEquipRepo) CurrentYearLabel(projectID string) string {
	ys, _ := r.ListYears(projectID)
	for _, y := range ys {
		if y.IsCurrent {
			if y.Label != "" {
				return y.Label
			}
			return fmt.Sprintf("%d차", y.YearNo)
		}
	}
	return ""
}

func (r *ProjectEquipRepo) AssignedUser(projectID, userID string) bool {
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM project_assignments WHERE project_id=? AND user_id=?`, projectID, userID).Scan(&n)
	return n > 0
}

func (r *ProjectEquipRepo) RecordBatch(b *model.EquipImportBatch) error {
	if b.ID == "" {
		id, err := r.nextID("equip_import", "IB")
		if err != nil {
			return err
		}
		b.ID = id
	}
	_, err := r.db.Exec(`INSERT INTO equipment_import_batches(id,project_id,file_name,rows_total,rows_ok,rows_error,status,created_by)
		VALUES(?,?,?,?,?,?,?,?)`, b.ID, b.ProjectID, b.FileName, b.RowsTotal, b.RowsOK, b.RowsError, b.Status, b.CreatedBy)
	return err
}
