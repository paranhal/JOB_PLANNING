package repository

import (
	"database/sql"
	"time"

	"customer-support/internal/model"
)

type SWDetailRepo struct{ db *sql.DB }

func NewSWDetailRepo(db *sql.DB) *SWDetailRepo { return &SWDetailRepo{db: db} }

func (r *SWDetailRepo) ListByAsset(assetID string) ([]model.AssetSWDetail, error) {
	rows, err := r.db.Query(`
		SELECT sw_detail_id, asset_id, COALESCE(software_name,''), COALESCE(version,''),
		       COALESCE(install_type,''), COALESCE(hw_info,''),
		       COALESCE(os,''), COALESCE(os_version,''),
		       COALESCE(dbms,''), COALESCE(db_version,''),
		       COALESCE(access_method,''), COALESCE(access_url,''),
		       COALESCE(install_path,''), COALESCE(backup_path,''), COALESCE(config_path,'')
		FROM asset_sw_details WHERE asset_id=? ORDER BY software_name`, assetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.AssetSWDetail
	for rows.Next() {
		var s model.AssetSWDetail
		if err := rows.Scan(&s.SWDetailID, &s.AssetID, &s.SoftwareName, &s.Version,
			&s.InstallType, &s.HWInfo, &s.OS, &s.OSVersion,
			&s.DBMS, &s.DBVersion, &s.AccessMethod, &s.AccessURL,
			&s.InstallPath, &s.BackupPath, &s.ConfigPath); err != nil {
			return nil, err
		}
		items = append(items, s)
	}
	return items, rows.Err()
}

func (r *SWDetailRepo) GetByID(id string) (*model.AssetSWDetail, error) {
	var s model.AssetSWDetail
	err := r.db.QueryRow(`
		SELECT sw_detail_id, asset_id, COALESCE(software_name,''), COALESCE(version,''),
		       COALESCE(install_type,''), COALESCE(hw_info,''),
		       COALESCE(os,''), COALESCE(os_version,''),
		       COALESCE(dbms,''), COALESCE(db_version,''),
		       COALESCE(access_method,''), COALESCE(access_url,''),
		       COALESCE(install_path,''), COALESCE(backup_path,''), COALESCE(config_path,'')
		FROM asset_sw_details WHERE sw_detail_id=?`, id).
		Scan(&s.SWDetailID, &s.AssetID, &s.SoftwareName, &s.Version,
			&s.InstallType, &s.HWInfo, &s.OS, &s.OSVersion,
			&s.DBMS, &s.DBVersion, &s.AccessMethod, &s.AccessURL,
			&s.InstallPath, &s.BackupPath, &s.ConfigPath)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SWDetailRepo) Create(s *model.AssetSWDetail) error {
	s.SWDetailID = newID("SWD")
	_, err := r.db.Exec(`
		INSERT INTO asset_sw_details
		(sw_detail_id,asset_id,software_name,version,install_type,hw_info,
		 os,os_version,dbms,db_version,access_method,access_url,
		 install_path,backup_path,config_path,created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		s.SWDetailID, s.AssetID, s.SoftwareName, s.Version, s.InstallType, s.HWInfo,
		s.OS, s.OSVersion, s.DBMS, s.DBVersion, s.AccessMethod, s.AccessURL,
		s.InstallPath, s.BackupPath, s.ConfigPath,
		time.Now().Format("2006-01-02 15:04:05"))
	return err
}

func (r *SWDetailRepo) Update(s *model.AssetSWDetail) error {
	_, err := r.db.Exec(`
		UPDATE asset_sw_details SET
		software_name=?,version=?,install_type=?,hw_info=?,
		os=?,os_version=?,dbms=?,db_version=?,
		access_method=?,access_url=?,install_path=?,backup_path=?,config_path=?
		WHERE sw_detail_id=?`,
		s.SoftwareName, s.Version, s.InstallType, s.HWInfo,
		s.OS, s.OSVersion, s.DBMS, s.DBVersion,
		s.AccessMethod, s.AccessURL, s.InstallPath, s.BackupPath, s.ConfigPath,
		s.SWDetailID)
	return err
}

func (r *SWDetailRepo) Delete(id string) error {
	_, err := r.db.Exec(`DELETE FROM asset_sw_details WHERE sw_detail_id=?`, id)
	return err
}
