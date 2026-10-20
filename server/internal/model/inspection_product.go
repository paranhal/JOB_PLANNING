package model

import (
	"regexp"
	"strings"
	"unicode"
)

// InspectionProduct 조직별 정기점검 제품 마스터.
type InspectionProduct struct {
	ProductID string
	OrgID     string
	Code      string
	Name      string
	SortOrder int
	IsActive  bool
	CreatedAt string
	UpdatedAt string
	InUse     bool
}

var inspectionProductCodeRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,30}$`)

func ValidInspectionProductCode(code string) bool {
	return inspectionProductCodeRe.MatchString(strings.TrimSpace(code))
}

func SuggestInspectionProductCode(name string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(name) {
		switch {
		case unicode.IsLetter(r) && r < unicode.MaxASCII:
			b.WriteRune(unicode.ToUpper(r))
		case unicode.IsDigit(r):
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			if b.Len() > 0 {
				b.WriteByte('_')
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	for strings.Contains(out, "__") {
		out = strings.ReplaceAll(out, "__", "_")
	}
	if len(out) > 30 {
		out = out[:30]
	}
	return out
}

func CompactInspectionToken(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	return s
}

func UseLegacyInspectionSlots(orgID string, orgMasterCount int) bool {
	if orgMasterCount > 0 {
		return false
	}
	orgID = strings.TrimSpace(orgID)
	return orgID == "" || orgID == OrgIDLibrary
}

func VisitProductSlotKeyFor(product string, catalog []InspectionProduct) string {
	if catalog == nil {
		return VisitProductSlotKey(product)
	}
	raw := strings.TrimSpace(product)
	if raw == "" {
		return "generic"
	}
	compact := CompactInspectionToken(raw)
	for _, p := range catalog {
		if !p.IsActive {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(p.Code), raw) || CompactInspectionToken(p.Code) == compact {
			return p.Code
		}
		if strings.EqualFold(strings.TrimSpace(p.Name), raw) || CompactInspectionToken(p.Name) == compact {
			return p.Code
		}
	}
	return "generic"
}

func (c MaintenanceSiteConfig) ProductLabels() []string {
	if c.Products == nil {
		var out []string
		if c.HasKlas {
			out = append(out, "KLAS")
		}
		if c.HasRfid {
			out = append(out, "앤로보틱스")
		}
		return out
	}
	var out []string
	for _, p := range c.Products {
		if p.IsActive {
			out = append(out, p.Name)
		}
	}
	return out
}

func (c MaintenanceSiteConfig) HasProductID(id string) bool {
	id = strings.TrimSpace(id)
	for _, p := range c.Products {
		if p.ProductID == id {
			return true
		}
	}
	return false
}
