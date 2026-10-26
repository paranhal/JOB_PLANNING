package model

// ProductClass 제품분류 (HW / SW / 보안장비)
type ProductClass struct {
	ClassID   string `json:"class_id"`
	ClassName string `json:"class_name"`
	SortOrder int    `json:"sort_order"`
}

// ProductGroup 세부 제품군 (PG-####)
type ProductGroup struct {
	GroupID      string `json:"group_id"`
	ClassID      string `json:"class_id"`
	ClassName    string `json:"class_name"`
	TypeName     string `json:"type_name"`
	DetailName   string `json:"detail_name"`
	SyncType     string `json:"sync_type"`
	SyncCategory string `json:"sync_category"`
	IsActive     bool   `json:"is_active"`
	SortOrder    int    `json:"sort_order"`
	UpdatedAt    string `json:"updated_at"`
	AssetCount   int    `json:"asset_count"`
}

func (g ProductGroup) DetailLabel() string {
	if g.DetailName == "" {
		return "미등록"
	}
	return g.DetailName
}

func (g ProductGroup) DateDot() string {
	s := g.UpdatedAt
	if len(s) >= 10 {
		s = s[:10]
	}
	if len(s) == 10 && s[4] == '-' && s[7] == '-' {
		return s[0:4] + ". " + s[5:7] + ". " + s[8:10]
	}
	return s
}

type ProductGroupKPI struct {
	Total     int
	Classes   int
	Types     int
	NoDetail  int
}
