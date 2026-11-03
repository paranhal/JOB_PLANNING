package model

import "strings"

const AssetImportMaxBytes = 2 << 20
const AssetImportMaxRows = 1000

const (
	AssetImportRegister = "register"
	AssetImportSkip     = "skip"
	AssetImportError    = "error"

	AssetImportErrOtherOrg = "other_org"
	AssetImportErrMissing  = "missing"
	AssetImportErrNoOrg    = "no_org"
	AssetImportErrDup      = "dup"
)

const AssetImportEmptySheet = "「자산입력」시트 2행부터 장비 한 줄씩 넣으세요. 머리글만 있으면 미리보기할 내용이 없습니다."

type AssetImportRow struct {
	Line         int
	Action       string
	Msgs         []string
	SiteName     string
	CustomerID   string
	FileDup      bool
	LocationWarn bool
	MatchKind    string
	Asset        Asset
}

type AssetImportPreview struct {
	FileName      string
	CreateSpaces  bool
	Rows          []AssetImportRow
	OKN           int
	SkipN         int
	ErrN          int
	WarnN         int
	ErrOtherOrgN  int
	ErrMissingN   int
	ErrNoOrgN     int
	ErrDupN       int
}

func TallyAssetImport(p *AssetImportPreview) {
	if p == nil {
		return
	}
	p.OKN, p.SkipN, p.ErrN, p.WarnN = 0, 0, 0, 0
	p.ErrOtherOrgN, p.ErrMissingN, p.ErrNoOrgN, p.ErrDupN = 0, 0, 0, 0
	for i := range p.Rows {
		switch p.Rows[i].Action {
		case AssetImportRegister:
			p.OKN++
		case AssetImportSkip:
			p.SkipN++
		default:
			p.ErrN++
			switch p.Rows[i].MatchKind {
			case AssetImportErrOtherOrg:
				p.ErrOtherOrgN++
			case AssetImportErrNoOrg:
				p.ErrNoOrgN++
			case AssetImportErrDup:
				p.ErrDupN++
			default:
				p.ErrMissingN++
			}
		}
		if len(p.Rows[i].Msgs) > 0 && p.Rows[i].Action != AssetImportError {
			p.WarnN++
		}
	}
}

func SplitSlashLocation(s string) (building, floor, room string, ok bool) {
	parts := strings.Split(s, "/")
	if len(parts) != 3 {
		return "", "", "", false
	}
	building = strings.TrimSpace(parts[0])
	floor = strings.TrimSpace(parts[1])
	room = strings.TrimSpace(parts[2])
	if building == "" || floor == "" || room == "" {
		return "", "", "", false
	}
	return building, floor, room, true
}

func JoinAssetInstallLocation(building, floor, room string) string {
	var parts []string
	for _, p := range []string{strings.TrimSpace(building), strings.TrimSpace(floor), strings.TrimSpace(room)} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " ")
}
