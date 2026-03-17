package model

import "time"

// Customer 고객 마스터 (기획서 §5.1)
type Customer struct {
	CustomerID       string    `json:"customer_id"`
	OrgName          string    `json:"org_name"`           // 기관명 (실무 명칭)
	OfficialName     string    `json:"official_name"`      // 공식명칭 (계약서 기준)
	OrgEmail         string    `json:"org_email"`
	MainPhone        string    `json:"main_phone"`
	Website          string    `json:"website"`
	BusinessNumber   string    `json:"business_number"`    // 사업자번호
	Representative   string    `json:"representative"`     // 대표자
	Industry         string    `json:"industry"`           // 업종 코드
	HasParent        bool      `json:"has_parent"`
	ParentCustomerID string    `json:"parent_customer_id"` // 상위기관 ID
	Address          string    `json:"address"`
	AddressDetail    string    `json:"address_detail"`
	IsActive         bool      `json:"is_active"`
	Notes            string    `json:"notes"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`

	// JOIN용 (DB에 저장 안 함)
	ParentOrgName string `json:"parent_org_name,omitempty"`
	AsCount       int    `json:"as_count,omitempty"`
	AssetCount    int    `json:"asset_count,omitempty"`
}

// CustomerBuilding 고객 건물 (기획서 §5.2)
type CustomerBuilding struct {
	BuildingID   string    `json:"building_id"`
	CustomerID   string    `json:"customer_id"`
	BuildingName string    `json:"building_name"`
	BuildingType string    `json:"building_type"`
	Address      string    `json:"address"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`

	Floors []CustomerFloor `json:"floors,omitempty"`
}

// CustomerFloor 고객 층
type CustomerFloor struct {
	FloorID    string    `json:"floor_id"`
	BuildingID string    `json:"building_id"`
	FloorName  string    `json:"floor_name"`
	SortOrder  int       `json:"sort_order"`
	CreatedAt  time.Time `json:"created_at"`

	Rooms []CustomerRoom `json:"rooms,omitempty"`
}

// CustomerRoom 고객 실
type CustomerRoom struct {
	RoomID    string    `json:"room_id"`
	FloorID   string    `json:"floor_id"`
	RoomName  string    `json:"room_name"`
	RoomNumber string   `json:"room_number"`
	Purpose   string    `json:"purpose"`
	CreatedAt time.Time `json:"created_at"`
}

// Contact 고객 담당자 (기획서 §5.3)
type Contact struct {
	ContactID  string    `json:"contact_id"`
	CustomerID string    `json:"customer_id"`
	FullName   string    `json:"full_name"`
	JobRole    string    `json:"job_role"`
	Title      string    `json:"title"`
	Phone      string    `json:"phone"`
	Mobile     string    `json:"mobile"`
	Email      string    `json:"email"`
	StartDate  string    `json:"start_date"`
	EndDate    string    `json:"end_date"`
	Status     string    `json:"status"`    // active, transferred, resigned
	IsPrimary  bool      `json:"is_primary"`
	Notes      string    `json:"notes"`
	CreatedAt  time.Time `json:"created_at"`

	// JOIN용
	OrgName string `json:"org_name,omitempty"`
}

// ContactHistory 담당자 이력 (기획서 §5.4)
type ContactHistory struct {
	HistoryID    string    `json:"history_id"`
	ContactID    string    `json:"contact_id"`
	CustomerID   string    `json:"customer_id"`
	StartDate    string    `json:"start_date"`
	EndDate      string    `json:"end_date"`
	Department   string    `json:"department"`
	JobRole      string    `json:"job_role"`
	Title        string    `json:"title"`
	Phone        string    `json:"phone"`
	Email        string    `json:"email"`
	Status       string    `json:"status"`
	ChangeReason string    `json:"change_reason"`
	CreatedAt    time.Time `json:"created_at"`
}

// CustomerListItem 목록 표시용 (집계 포함)
type CustomerListItem struct {
	CustomerID   string `json:"customer_id"`
	OrgName      string `json:"org_name"`
	OfficialName string `json:"official_name"`
	Industry     string `json:"industry"`
	MainPhone    string `json:"main_phone"`
	IsActive     bool   `json:"is_active"`
	AssetCount   int    `json:"asset_count"`
	AsCount      int    `json:"as_count"`
}
