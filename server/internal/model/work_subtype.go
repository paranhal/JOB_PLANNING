package model

import "strings"

func NormalizeWorkSubtype(s string) string {
	switch strings.TrimSpace(s) {
	case WBWorkSubtypeBuild, "구축":
		return WBWorkSubtypeBuild
	case WBWorkSubtypeConstruction, "공사":
		return WBWorkSubtypeConstruction
	case WBWorkSubtypeOther, "기타":
		return WBWorkSubtypeOther
	default:
		return ""
	}
}

func WorkSubtypeLabel(s string) string {
	switch NormalizeWorkSubtype(s) {
	case WBWorkSubtypeBuild:
		return "구축"
	case WBWorkSubtypeConstruction:
		return "공사"
	case WBWorkSubtypeOther:
		return "기타"
	default:
		return s
	}
}

func SuggestWorkSubtype(projectKind string) string {
	switch NormalizeProjectKind(projectKind) {
	case ProjectKindBuild:
		return WBWorkSubtypeBuild
	case ProjectKindConstruction:
		return WBWorkSubtypeConstruction
	default:
		return ""
	}
}
