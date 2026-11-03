package model

import "strings"

const ItemUnitDefault = "건"

type ItemTemplate struct {
	TemplateID  string
	OrgID       string
	Kind        string
	Name        string
	DefaultQty  int
	Unit        string
	IsActive    bool
	SortOrder   int
	Note        string
	CreatedBy   string
	KindLabel   string
	OrgLabel    string
	InUse       bool
	CanEdit     bool
	Links       []ItemTemplateLink
	LinkSummary string
}

type ItemTemplateLink struct {
	TemplateID      string
	ChildTemplateID string
	ChildName       string
	ChildKind       string
	ChildUnit       string
	KindLabel       string
	DefaultQty      int
	DefaultChecked  bool
	SortOrder       int
}

func NormalizeItemUnit(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ItemUnitDefault
	}
	return s
}

func ValidItemKindCode(s string) bool {
	switch strings.TrimSpace(s) {
	case ItemKindInstall, ItemKindWork, ItemKindAS, ItemKindEtc:
		return true
	default:
		return false
	}
}

func ValidItemUnitCode(s string, allowed []Code) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	for _, c := range allowed {
		if strings.TrimSpace(c.CodeValue) == s {
			return true
		}
	}
	switch s {
	case "건", "대", "m", "식":
		return true
	default:
		return false
	}
}

func ItemTemplateNameKey(orgID, kind, name string) string {
	return strings.TrimSpace(orgID) + "\x1e" + strings.TrimSpace(kind) + "\x1e" + strings.ToLower(strings.TrimSpace(name))
}
