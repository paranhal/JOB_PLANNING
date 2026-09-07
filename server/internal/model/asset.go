package model

import (
	"path/filepath"
	"strings"
	"time"
)

// Asset 설치 기본정보 (기획서 §6.1)
type Asset struct {
	AssetID           string `json:"asset_id"`
	CustomerID        string `json:"customer_id"`
	ProductName       string `json:"product_name"`
	ProductType       string `json:"product_type"`     // 제품구분 코드: SW, HW, 서버 등
	ProductCategory   string `json:"product_category"` // 제품분류: 홈페이지, 전자도서관, 모바일, RFID자동화, 자료관리, 기타
	ModelName         string `json:"model_name"`
	Manufacturer      string `json:"manufacturer"`
	SerialNumber      string `json:"serial_number"`
	InstallDate       string `json:"install_date"`
	RetireDate        string `json:"retire_date"`
	InstallerType     string `json:"installer_type"` // 자사, 타사, 제조사, 협력사, 미상
	OriginalInstaller string `json:"original_installer"`
	OperationStatus   string `json:"operation_status"` // 운영중, 점검중, 장애, 철수, 폐기
	ManagementType    string `json:"management_type"`  // 직접유지보수, 장애대응, 타사장비 등
	IsManaged         bool   `json:"is_managed"`
	// 유지보수 계약 (기획서 §6.1) — 관리유형과 별개
	MaintContractType string    `json:"maint_contract_type"` // paid/free/call/none
	MaintCycle        string    `json:"maint_cycle"`         // monthly/quarterly/semi/odd_bimonthly/even_bimonthly 또는 직접입력 문구
	MaintStartDate    string    `json:"maint_start_date"`
	MaintEndDate      string    `json:"maint_end_date"`
	MaintBillingParty string    `json:"maint_billing_party"`
	MaintBillingCycle string    `json:"maint_billing_cycle"`
	RequesterType     string    `json:"requester_type"` // 고객직접, 제조사, 협력사 등
	RequesterName     string    `json:"requester_name"`
	CustomerContactID string    `json:"customer_contact_id"`
	OurContact        string    `json:"our_contact"`
	BuildingID        string    `json:"building_id"`
	FloorID           string    `json:"floor_id"`
	RoomID            string    `json:"room_id"`
	LocBuildingName   string    `json:"loc_building_name"` // 건물명 (전용 필드)
	LocFloorName      string    `json:"loc_floor_name"`    // 층 (전용 필드)
	LocRoomName       string    `json:"loc_room_name"`     // 호·실명 (전용 필드)
	InstallLocation   string    `json:"install_location"`  // 설치위치 — 동일층·동일실·동일모델 구분 (AS 접수 중요)
	LocationDetail    string    `json:"location_detail"`   // 상세위치(보조 서술)
	Notes             string    `json:"notes"`
	ProjectID         string    `json:"project_id"` // 사업(프로젝트) work_projects
	SalesOrderID      string    `json:"sales_order_id"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`

	// JOIN용
	OrgName      string `json:"org_name,omitempty"`
	BuildingName string `json:"building_name,omitempty"`
	FloorName    string `json:"floor_name,omitempty"`
	RoomName     string `json:"room_name,omitempty"`
	ProjectName  string `json:"project_name,omitempty"`
	AsCount      int    `json:"as_count,omitempty"`
	InstallYears int    `json:"install_years,omitempty"` // 설치연수 (영업활용)
}

// 운영상태 코드 (codes.operation_status) §35.2
const (
	AssetOpOperating   = "operating"
	AssetOpMaintenance = "maintenance"
	AssetOpFault       = "fault"
	AssetOpRetired     = "retired"
	AssetOpDisposed    = "disposed"
)

// 관리유형 코드값 (codes.management_type)
const ManagementTypeThirdParty = "third_party"
const InstallerTypeSelf = "self"

func (a *Asset) MaintFieldsEmpty() bool {
	if a == nil {
		return true
	}
	return strings.TrimSpace(a.MaintContractType) == "" &&
		strings.TrimSpace(a.MaintCycle) == "" &&
		strings.TrimSpace(a.MaintStartDate) == "" &&
		strings.TrimSpace(a.MaintEndDate) == "" &&
		strings.TrimSpace(a.MaintBillingParty) == "" &&
		strings.TrimSpace(a.MaintBillingCycle) == ""
}

// IsThirdPartyEquipment 당사가 장비를 유지보수하지 않는 타사 장비(연동·환경 참고용).
func (a *Asset) IsThirdPartyEquipment() bool {
	if a == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(a.ManagementType), ManagementTypeThirdParty)
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
	RelationID   string    `json:"relation_id"`
	CustomerID   string    `json:"customer_id"`
	AssetID      string    `json:"asset_id"`
	RelationType string    `json:"relation_type"` // 제조사 요청 수행, 협력사 요청 수행 등
	CompanyType  string    `json:"company_type"`  // 제조사, 협력사, 원청사, 고객기관
	CompanyName  string    `json:"company_name"`
	ContactName  string    `json:"contact_name"`
	ContactPhone string    `json:"contact_phone"`
	ContactEmail string    `json:"contact_email"`
	StartDate    string    `json:"start_date"`
	EndDate      string    `json:"end_date"`
	IsActive     bool      `json:"is_active"`
	Notes        string    `json:"notes"`
	CreatedAt    time.Time `json:"created_at"`
}

// Attachment 첨부파일 (기획서 §12)
type Attachment struct {
	AttachmentID string    `json:"attachment_id"`
	RefType      string    `json:"ref_type"` // asset, as, as_receipt, as_action_photo, as_report
	RefID        string    `json:"ref_id"`
	FileName     string    `json:"file_name"`
	FilePath     string    `json:"file_path"`
	FileSize     int64     `json:"file_size"`
	MIMEType     string    `json:"mime_type"`
	Keywords     string    `json:"keywords"` // 설치사진 등 검색·매뉴얼용 키워드
	SlotNo       int       `json:"slot_no"`  // 자산 이미지 슬롯 1~3
	UploadedAt   time.Time `json:"uploaded_at"`
}

// DisplayName 화면에 보여줄 원본 파일명. 경로·타임스탬프 접두사는 숨긴다.
func (a Attachment) DisplayName() string {
	name := strings.TrimSpace(a.FileName)
	if name == "" {
		name = filepath.Base(strings.ReplaceAll(a.FilePath, "\\", "/"))
	}
	name = filepath.Base(name)
	if i := strings.Index(name, "_"); i > 0 {
		prefix := name[:i]
		allDigits := true
		for _, r := range prefix {
			if r < '0' || r > '9' {
				allDigits = false
				break
			}
		}
		if allDigits && len(prefix) >= 13 && i+1 < len(name) {
			return name[i+1:]
		}
	}
	return name
}

const (
	RefTypeAsset         = "asset"
	RefTypeAS            = "as"
	RefTypeASReceipt     = "as_receipt"
	RefTypeASActionPhoto = "as_action_photo" // §12.9.8 조치 사진
	RefTypeASReport      = "as_report"
	RefTypeWorkActivity  = "work_activity"
	RefTypeSalesOrder    = "sales_order"
	RefTypeSalesDelivery = "sales_delivery"
)

const (
	MaxReceiptPhotos = 10
	MaxActionPhotos  = 3
	MaxReceiptBytes  = 20 << 20 // 20MB
)

// IsImage 접수 사진 썸네일 대상. HEIC는 업로드 시 JPEG로 바뀌지만 원본명으로도 판별한다.
func (a Attachment) IsImage() bool {
	mime := strings.ToLower(strings.TrimSpace(a.MIMEType))
	if strings.HasPrefix(mime, "image/") {
		return true
	}
	name := strings.ToLower(a.FileName)
	if name == "" {
		name = strings.ToLower(a.FilePath)
	}
	ext := filepath.Ext(name)
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".heic", ".heif", ".bmp":
		return true
	}
	return false
}

func publicUploadURL(filePath string) string {
	p := strings.ReplaceAll(filePath, "\\", "/")
	const prefix = "data/uploads/"
	if i := strings.Index(p, prefix); i >= 0 {
		return "/uploads/" + p[i+len(prefix):]
	}
	if strings.HasPrefix(p, "uploads/") {
		return "/" + p
	}
	return ""
}

// PublicURL 원본(축소본) 공개 경로.
func (a Attachment) PublicURL() string {
	return publicUploadURL(a.FilePath)
}

// ThumbURL 썸네일 공개 경로. 없으면 원본 URL.
func (a Attachment) ThumbURL() string {
	if !a.IsImage() {
		return ""
	}
	p := strings.ReplaceAll(a.FilePath, "\\", "/")
	ext := filepath.Ext(p)
	thumb := strings.TrimSuffix(p, ext) + "_thumb.jpg"
	if u := publicUploadURL(thumb); u != "" {
		return u
	}
	return a.PublicURL()
}

// AssetImageSlot 자산 상세/수정 화면용 사진 슬롯(최대 3)
type AssetImageSlot struct {
	Slot     int    `json:"slot"`
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	URL      string `json:"url,omitempty"`
	Keywords string `json:"keywords,omitempty"`
}

// AccessInfoReference 접속정보참조 (기획서 §6.3)
type AccessInfoReference struct {
	AccessInfoID  string    `json:"access_info_id"`
	AssetID       string    `json:"asset_id"`
	FilePath      string    `json:"file_path"`
	StorageMethod string    `json:"storage_method"` // 암호화파일, 금고시스템, 별도저장소
	LastVerified  string    `json:"last_verified"`
	ManagedBy     string    `json:"managed_by"`
	Notes         string    `json:"notes"`
	CreatedAt     time.Time `json:"created_at"`
}

// Code 코드관리 (기획서 §11)
type Code struct {
	CodeID    string `json:"code_id"`
	CodeGroup string `json:"code_group"`
	CodeValue string `json:"code_value"`
	CodeName  string `json:"code_name"`
	SortOrder int    `json:"sort_order"`
	IsActive  bool   `json:"is_active"`
}

// User 사용자 (기획서 §10)
type User struct {
	UserID       string    `json:"user_id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"`
	FullName     string    `json:"full_name"`
	Role         string    `json:"role"`        // admin, tech, sales, office, observer
	Permissions  string    `json:"permissions"` // CSV 권한 키
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
}

// PermList EffectivePermissions 편의
func (u *User) PermList() []string {
	if u == nil {
		return nil
	}
	return EffectivePermissions(u.Role, u.Permissions)
}

// HasPerm 권한 체크
func (u *User) HasPerm(key string) bool {
	if u == nil {
		return false
	}
	if NormalizeRole(u.Role) == RoleAdmin {
		return true
	}
	return HasPermission(u.PermList(), key)
}
