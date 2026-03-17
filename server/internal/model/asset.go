package model

import "time"

// Asset 설치 기본정보 (기획서 §6.1)
type Asset struct {
	AssetID          string    `json:"asset_id"`
	CustomerID       string    `json:"customer_id"`
	ProductName      string    `json:"product_name"`
	ProductType      string    `json:"product_type"`      // 코드: SW, HW, 서버, 네트워크장비 등
	ModelName        string    `json:"model_name"`
	Manufacturer     string    `json:"manufacturer"`
	SerialNumber     string    `json:"serial_number"`
	InstallDate      string    `json:"install_date"`
	RetireDate       string    `json:"retire_date"`
	InstallerType    string    `json:"installer_type"`    // 자사, 타사, 제조사, 협력사, 미상
	OriginalInstaller string   `json:"original_installer"`
	OperationStatus  string    `json:"operation_status"`  // 운영중, 점검중, 장애, 철수, 폐기
	ManagementType   string    `json:"management_type"`   // 직접유지보수, 장애대응 등
	IsManaged        bool      `json:"is_managed"`
	RequesterType    string    `json:"requester_type"`    // 고객직접, 제조사, 협력사 등
	RequesterName    string    `json:"requester_name"`
	CustomerContactID string   `json:"customer_contact_id"`
	OurContact       string    `json:"our_contact"`
	BuildingID       string    `json:"building_id"`
	FloorID          string    `json:"floor_id"`
	RoomID           string    `json:"room_id"`
	LocationDetail   string    `json:"location_detail"`
	Notes            string    `json:"notes"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`

	// JOIN용
	OrgName      string `json:"org_name,omitempty"`
	BuildingName string `json:"building_name,omitempty"`
	FloorName    string `json:"floor_name,omitempty"`
	RoomName     string `json:"room_name,omitempty"`
	AsCount      int    `json:"as_count,omitempty"`
	InstallYears int    `json:"install_years,omitempty"` // 설치연수 (영업활용)
}

// AssetSWDetail SW 설치 상세정보 (기획서 §6.2)
type AssetSWDetail struct {
	SWDetailID   string    `json:"sw_detail_id"`
	AssetID      string    `json:"asset_id"`
	SoftwareName string    `json:"software_name"`
	Version      string    `json:"version"`
	InstallType  string    `json:"install_type"` // PC설치형, 서버형, 웹형
	HWInfo       string    `json:"hw_info"`
	OS           string    `json:"os"`
	OSVersion    string    `json:"os_version"`
	DBMS         string    `json:"dbms"`
	DBVersion    string    `json:"db_version"`
	AccessMethod string    `json:"access_method"` // RDP, 웹 등
	AccessURL    string    `json:"access_url"`
	InstallPath  string    `json:"install_path"`
	BackupPath   string    `json:"backup_path"`
	ConfigPath   string    `json:"config_path"`
	CreatedAt    time.Time `json:"created_at"`
}

// PerformanceRelation 수행관계 (기획서 §7)
type PerformanceRelation struct {
	RelationID    string    `json:"relation_id"`
	CustomerID    string    `json:"customer_id"`
	AssetID       string    `json:"asset_id"`
	RelationType  string    `json:"relation_type"` // 제조사 요청 수행, 협력사 요청 수행 등
	CompanyType   string    `json:"company_type"`  // 제조사, 협력사, 원청사, 고객기관
	CompanyName   string    `json:"company_name"`
	ContactName   string    `json:"contact_name"`
	ContactPhone  string    `json:"contact_phone"`
	ContactEmail  string    `json:"contact_email"`
	StartDate     string    `json:"start_date"`
	EndDate       string    `json:"end_date"`
	IsActive      bool      `json:"is_active"`
	Notes         string    `json:"notes"`
	CreatedAt     time.Time `json:"created_at"`
}

// Attachment 첨부파일 (기획서 §12)
type Attachment struct {
	AttachmentID string    `json:"attachment_id"`
	RefType      string    `json:"ref_type"` // customer, asset, as, relation
	RefID        string    `json:"ref_id"`
	FileName     string    `json:"file_name"`
	FilePath     string    `json:"file_path"`
	FileSize     int64     `json:"file_size"`
	MIMEType     string    `json:"mime_type"`
	UploadedAt   time.Time `json:"uploaded_at"`
}

// Code 코드관리 (기획서 §11)
type Code struct {
	CodeID     string `json:"code_id"`
	CodeGroup  string `json:"code_group"`
	CodeValue  string `json:"code_value"`
	CodeName   string `json:"code_name"`
	SortOrder  int    `json:"sort_order"`
	IsActive   bool   `json:"is_active"`
}
