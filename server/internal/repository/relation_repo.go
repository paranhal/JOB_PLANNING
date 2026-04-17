package repository

import (
	"database/sql"
	"time"

	"customer-support/internal/model"
)

type RelationRepo struct{ db *sql.DB }

func NewRelationRepo(db *sql.DB) *RelationRepo { return &RelationRepo{db: db} }

func (r *RelationRepo) List(customerID, assetID string, page, pageSize int) ([]model.PerformanceRelation, int, error) {
	offset := (page - 1) * pageSize
	base := `SELECT relation_id, COALESCE(customer_id,''), COALESCE(asset_id,''),
	                relation_type, COALESCE(company_type,''), COALESCE(company_name,''),
	                COALESCE(contact_name,''), COALESCE(contact_phone,''), COALESCE(contact_email,''),
	                COALESCE(start_date,''), COALESCE(end_date,''), is_active, COALESCE(notes,'')
	         FROM performance_relations WHERE 1=1`
	cnt := `SELECT COUNT(*) FROM performance_relations WHERE 1=1`
	args, cntArgs := []interface{}{}, []interface{}{}

	if customerID != "" {
		base += ` AND customer_id=?`
		cnt += ` AND customer_id=?`
		args = append(args, customerID)
		cntArgs = append(cntArgs, customerID)
	}
	if assetID != "" {
		base += ` AND asset_id=?`
		cnt += ` AND asset_id=?`
		args = append(args, assetID)
		cntArgs = append(cntArgs, assetID)
	}

	var total int
	r.db.QueryRow(cnt, cntArgs...).Scan(&total)

	base += ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, pageSize, offset)

	rows, err := r.db.Query(base, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []model.PerformanceRelation
	for rows.Next() {
		var p model.PerformanceRelation
		var active int
		if err := rows.Scan(&p.RelationID, &p.CustomerID, &p.AssetID,
			&p.RelationType, &p.CompanyType, &p.CompanyName,
			&p.ContactName, &p.ContactPhone, &p.ContactEmail,
			&p.StartDate, &p.EndDate, &active, &p.Notes); err != nil {
			return nil, 0, err
		}
		p.IsActive = active == 1
		items = append(items, p)
	}
	return items, total, rows.Err()
}

func (r *RelationRepo) GetByID(id string) (*model.PerformanceRelation, error) {
	var p model.PerformanceRelation
	var active int
	err := r.db.QueryRow(
		`SELECT relation_id, COALESCE(customer_id,''), COALESCE(asset_id,''),
		        relation_type, COALESCE(company_type,''), COALESCE(company_name,''),
		        COALESCE(contact_name,''), COALESCE(contact_phone,''), COALESCE(contact_email,''),
		        COALESCE(start_date,''), COALESCE(end_date,''), is_active, COALESCE(notes,'')
		 FROM performance_relations WHERE relation_id=?`, id).
		Scan(&p.RelationID, &p.CustomerID, &p.AssetID,
			&p.RelationType, &p.CompanyType, &p.CompanyName,
			&p.ContactName, &p.ContactPhone, &p.ContactEmail,
			&p.StartDate, &p.EndDate, &active, &p.Notes)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.IsActive = active == 1
	return &p, nil
}

func (r *RelationRepo) Create(p *model.PerformanceRelation) error {
	p.RelationID = newID("REL")
	_, err := r.db.Exec(`
		INSERT INTO performance_relations
		(relation_id,customer_id,asset_id,relation_type,company_type,company_name,
		 contact_name,contact_phone,contact_email,start_date,end_date,is_active,notes,created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.RelationID, nullStr(p.CustomerID), nullStr(p.AssetID),
		p.RelationType, p.CompanyType, p.CompanyName,
		p.ContactName, p.ContactPhone, p.ContactEmail,
		p.StartDate, p.EndDate, boolToInt(p.IsActive), p.Notes,
		time.Now().Format("2006-01-02 15:04:05"))
	return err
}

func (r *RelationRepo) Update(p *model.PerformanceRelation) error {
	_, err := r.db.Exec(`
		UPDATE performance_relations SET
		customer_id=?,asset_id=?,relation_type=?,company_type=?,company_name=?,
		contact_name=?,contact_phone=?,contact_email=?,start_date=?,end_date=?,is_active=?,notes=?
		WHERE relation_id=?`,
		nullStr(p.CustomerID), nullStr(p.AssetID),
		p.RelationType, p.CompanyType, p.CompanyName,
		p.ContactName, p.ContactPhone, p.ContactEmail,
		p.StartDate, p.EndDate, boolToInt(p.IsActive), p.Notes, p.RelationID)
	return err
}

func (r *RelationRepo) Delete(id string) error {
	_, err := r.db.Exec(`DELETE FROM performance_relations WHERE relation_id=?`, id)
	return err
}
