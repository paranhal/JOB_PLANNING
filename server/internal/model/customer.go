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
	PostalCode       string    `json:"postal_code"`        // 우편번호
	AddrSido         string    `json:"addr_sido"`          // 시도
	AddrSigungu      string    `json:"addr_sigungu"`       // 시군구(군구)
	AddrDong         string    `json:"addr_dong"`          // 동읍면
	Address          string    `json:"address"`            // 표시·검색용 조합주소(시도~동)
	AddressDetail    string    `json:"address_detail"`     // 상세주소
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
	ContactID   string    `json:"contact_id"`
	CustomerID  string    `json:"customer_id"`
	FullName    string    `json:"full_name"`
	Affiliation string    `json:"affiliation"` // institution/integrator/partner/other
	JobRole     string    `json:"job_role"`
	Title       string    `json:"title"`
	JobGrade    string    `json:"job_grade"` // librarian/it/other 또는 직접입력 문구
	Phone       string    `json:"phone"`
	Mobile      string    `json:"mobile"`
	Email       string    `json:"email"`
	StartDate   string    `json:"start_date"`
	EndDate     string    `json:"end_date"`
	Status      string    `json:"status"`       // active, transferred, resigned
	ContactRole string    `json:"contact_role"` // primary, secondary, regular
	IsPrimary   bool      `json:"is_primary"`   // contact_role==primary 와 동기
	Notes       string    `json:"notes"`
	CreatedAt   time.Time `json:"created_at"`

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
	Mobile       string    `json:"mobile"`
	Email        string    `json:"email"`
	Status       string    `json:"status"`
	Affiliation  string    `json:"affiliation"`
	ContactRole  string    `json:"contact_role"`
	ChangeReason string    `json:"change_reason"` // transfer, resign, role_adjust
	CreatedAt    time.Time `json:"created_at"`

	// JOIN용
	ContactName string `json:"contact_name,omitempty"`
}

// ContactHistoryListItem 담당자 이력 화면 행 (§5.4 표시용)
type ContactHistoryListItem struct {
	ContactID    string `json:"contact_id"`
	Name         string `json:"name"`
	Affiliation  string `json:"affiliation"`
	StartDate    string `json:"start_date"`
	EndDate      string `json:"end_date"`
	ContactRole  string `json:"contact_role"`
	ChangeReason string `json:"change_reason"`
	Phone        string `json:"phone"`
	Mobile       string `json:"mobile"`
	Email        string `json:"email"`
	IsCurrent    bool   `json:"is_current"` // 변경 이력 없을 때 현행 담당자 행
}

// CustomerListItem 목록 표시용 (집계 포함)
type CustomerListItem struct {
	CustomerID       string `json:"customer_id"`
	OrgName          string `json:"org_name"`
	OfficialName     string `json:"official_name"`
	Industry         string `json:"industry"`
	MainPhone        string `json:"main_phone"`
	IsActive         bool   `json:"is_active"`
	AssetCount       int    `json:"asset_count"`
	AsCount          int    `json:"as_count"`
	ParentCustomerID string `json:"parent_customer_id"`
	ParentOrgName    string `json:"parent_org_name"`
	HasParent        bool   `json:"has_parent"`
	Address          string `json:"address,omitempty"`      // 엑셀·지역필터용(조합 또는 구주소)
	AddrSido         string `json:"addr_sido,omitempty"`    // 시도(구조화)
	SiteRegion       string `json:"site_region,omitempty"` // 점검사이트 지역
}

// CustomerCategory 고객현황 상위기관 카테고리(콤보)
type CustomerCategory struct {
	ParentID   string `json:"parent_id"`   // 빈값=전체, "none"=상위기관 없음
	Name       string `json:"name"`
	ChildCount int    `json:"child_count"`
}

// CustomerIndustryStat 업종별 건수
type CustomerIndustryStat struct {
	Industry string `json:"industry"`
	Count    int    `json:"count"`
}

