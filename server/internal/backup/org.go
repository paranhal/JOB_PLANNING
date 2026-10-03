package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

const (
	KindOrgDelete = "org_delete"
	KindOrgSplit  = "org_split"
	PrefixOrgDelete = "조직삭제_"
	PrefixOrgSplit  = "조직분리_"
)

// IsOrgSnapshotDir 조직 삭제·분리 백업은 14일 정리 대상이 아니다.
func IsOrgSnapshotDir(name string) bool {
	return strings.HasPrefix(name, PrefixOrgDelete) || strings.HasPrefix(name, PrefixOrgSplit)
}

func sanitizeFolderPart(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "이름없음"
	}
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			b.WriteRune('_')
		default:
			if unicode.IsControl(r) {
				b.WriteRune('_')
			} else {
				b.WriteRune(r)
			}
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "이름없음"
	}
	return out
}

// OrgFolderName 조직삭제_{조직명}_{날짜} 형태. 같은 날이 있으면 접미를 붙인다.
func OrgFolderName(prefix, orgName string, now time.Time) string {
	day := now.In(SeoulLocation()).Format("2006-01-02")
	return prefix + sanitizeFolderPart(orgName) + "_" + day
}

func uniqueDir(root, name string) string {
	final := filepath.Join(root, name)
	if !dirExists(final) && !dirExists(final+".tmp") {
		return name
	}
	base := name
	for i := 2; i < 1000; i++ {
		name = fmt.Sprintf("%s_%d", base, i)
		final = filepath.Join(root, name)
		if !dirExists(final) && !dirExists(final+".tmp") {
			return name
		}
	}
	return fmt.Sprintf("%s_%d", base, nowUnix())
}

func nowUnix() int64 { return time.Now().Unix() }

// PrepareOrgDir data/backups 아래 tmp 와 최종 폴더 이름을 만든다.
func PrepareOrgDir(dataDir, prefix, orgName string, now time.Time) (name, tmp, final string, err error) {
	if strings.TrimSpace(dataDir) == "" {
		return "", "", "", fmt.Errorf("data 폴더가 비어 있습니다")
	}
	root := filepath.Join(dataDir, "backups")
	if err = os.MkdirAll(root, 0755); err != nil {
		return "", "", "", err
	}
	name = uniqueDir(root, OrgFolderName(prefix, orgName, now))
	final = filepath.Join(root, name)
	tmp = final + ".tmp"
	_ = os.RemoveAll(tmp)
	if err = os.MkdirAll(tmp, 0755); err != nil {
		return "", "", "", err
	}
	return name, tmp, final, nil
}

// OrgSnapshot 폴더 한 건.
type OrgSnapshot struct {
	Name string
	Kind string
}

// ListOrgSnapshots 조직 전용 백업 폴더. 최신순.
func ListOrgSnapshots(dataDir string) ([]OrgSnapshot, error) {
	root := filepath.Join(dataDir, "backups")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []OrgSnapshot
	for _, e := range entries {
		if !e.IsDir() || !IsOrgSnapshotDir(e.Name()) {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if _, err := os.Stat(filepath.Join(dir, "org.db")); err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, "backup.txt")); err != nil {
			continue
		}
		kind := KindOrgDelete
		if strings.HasPrefix(e.Name(), PrefixOrgSplit) {
			kind = KindOrgSplit
		}
		out = append(out, OrgSnapshot{Name: e.Name(), Kind: kind})
	}
	return out, nil
}
