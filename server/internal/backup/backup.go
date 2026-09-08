package backup

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"customer-support/internal/audit"
	"customer-support/internal/repository"
)

const (
	BackupHour = 1  // 매일 01:00 (Asia/Seoul)
	KeepDays   = 14 // 최근 14일 유지
	KindAuto       = "auto"
	KindManual     = "manual"
	KindLogArchive = "log_archive"
)

// Config 일일 데이터 백업 설정.
type Config struct {
	DataDir string
	DB      *sql.DB
	Now     func() time.Time
	Loc     *time.Location
}

func (c Config) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c Config) loc() *time.Location {
	if c.Loc != nil {
		return c.Loc
	}
	return SeoulLocation()
}

// SeoulLocation Asia/Seoul. tzdata가 없으면 UTC+9.
func SeoulLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		return time.FixedZone("KST", 9*3600)
	}
	return loc
}

// NextBackupTime 다음 01:00. now가 01:00 이후이면 다음날.
func NextBackupTime(now time.Time, loc *time.Location) time.Time {
	now = now.In(loc)
	next := time.Date(now.Year(), now.Month(), now.Day(), BackupHour, 0, 0, 0, loc)
	if !now.Before(next) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

var runMu sync.Mutex

// Start 백그라운드에서 매일 01:00에 백업. 당일 폴더가 없고 01:00이 지났으면 즉시 보정.
func Start(cfg Config) {
	go func() {
		loc := cfg.loc()
		now := cfg.now().In(loc)
		_ = migrateLegacyNames(filepath.Join(cfg.DataDir, "backups"))
		if n, err := SyncIndex(cfg); err != nil {
			log.Printf("백업 목록 보정 실패: %v", err)
		} else if n > 0 {
			log.Printf("백업 목록 보정: %d건", n)
		}
		dest := filepath.Join(cfg.DataDir, "backups", snapshotDirName(KindAuto, now))
		if now.Hour() >= BackupHour && !backupComplete(dest) {
			log.Printf("데이터 백업(보정) 시작: %s", snapshotDirName(KindAuto, now))
			if err := Run(cfg); err != nil {
				log.Printf("데이터 백업 실패: %v", err)
			}
		}
		for {
			next := NextBackupTime(cfg.now(), loc)
			d := time.Until(next)
			if d < 0 {
				d = 0
			}
			timer := time.NewTimer(d)
			<-timer.C
			log.Printf("데이터 백업 시작: %s", next.Format("2006-01-02"))
			if err := Run(cfg); err != nil {
				log.Printf("데이터 백업 실패: %v", err)
			}
		}
	}()
}

// Run 오늘 날짜 폴더에 자동 백업을 만든다.
func Run(cfg Config) error {
	if cfg.DB != nil {
		repository.RunWorkListHousekeeping(cfg.DB)
	}
	_, err := Snapshot(cfg, KindAuto)
	return err
}

// Snapshot DB·uploads 복사본을 data/backups 아래 폴더로 만든다.
// 자동: 자동백업_YYYY-MM-DD, 수동: 수동저장_YYYY-MM-DD_HH시MM분SS초
func Snapshot(cfg Config, kind string) (string, error) {
	runMu.Lock()
	defer runMu.Unlock()

	if strings.TrimSpace(cfg.DataDir) == "" {
		return "", fmt.Errorf("data 폴더가 비어 있습니다")
	}
	if cfg.DB == nil {
		return "", fmt.Errorf("DB 연결이 없습니다")
	}
	if kind != KindManual {
		kind = KindAuto
	}

	loc := cfg.loc()
	now := cfg.now().In(loc)
	name := snapshotDirName(kind, now)
	root := filepath.Join(cfg.DataDir, "backups")
	_ = migrateLegacyNames(root)
	final := filepath.Join(root, name)
	if kind == KindManual {
		base := name
		for i := 2; dirExists(final); i++ {
			name = fmt.Sprintf("%s_%d", base, i)
			final = filepath.Join(root, name)
		}
	}
	tmp := final + ".tmp"

	_ = os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0755); err != nil {
		return "", err
	}

	if err := vacuumInto(cfg.DB, filepath.Join(tmp, "app.db")); err != nil {
		_ = os.RemoveAll(tmp)
		return "", fmt.Errorf("DB 스냅샷: %w", err)
	}

	uploadsSrc := filepath.Join(cfg.DataDir, "uploads")
	if info, err := os.Stat(uploadsSrc); err == nil && info.IsDir() {
		if err := copyDir(uploadsSrc, filepath.Join(tmp, "uploads")); err != nil {
			_ = os.RemoveAll(tmp)
			return "", fmt.Errorf("uploads 복사: %w", err)
		}
	}

	manifest := fmt.Sprintf("backup_at=%s\nkind=%s\ndb=app.db\n", now.Format(time.RFC3339), kind)
	if err := os.WriteFile(filepath.Join(tmp, "backup.txt"), []byte(manifest), 0644); err != nil {
		_ = os.RemoveAll(tmp)
		return "", err
	}

	if err := promoteDir(tmp, final); err != nil {
		_ = os.RemoveAll(tmp)
		return "", fmt.Errorf("백업 폴더 확정: %w", err)
	}
	log.Printf("데이터 백업 완료: %s", final)
	audit.Use(cfg.DB)
	audit.RecordBackup(kind, name, "")

	if err := prune(root, KeepDays, now, loc); err != nil {
		log.Printf("오래된 백업 정리 실패: %v", err)
	}
	return name, nil
}

// SyncIndex 백업 폴더를 스캔해 data_backups에 없는 행을 보정한다. 추가한 건수를 반환한다.
func SyncIndex(cfg Config) (int, error) {
	audit.Use(cfg.DB)
	items, err := List(cfg.DataDir)
	if err != nil {
		return 0, err
	}
	added := 0
	for _, it := range items {
		if audit.HasBackupFolder(it.Name) {
			continue
		}
		who := it.CreatedByName
		if who == "" && it.Kind == KindAuto {
			who = "시스템"
		}
		if err := audit.EnsureBackupRow(it.Kind, it.Name, it.SavedAt, who, "folder-scan"); err != nil {
			return added, err
		}
		added++
	}
	return added, nil
}

func snapshotDirName(kind string, now time.Time) string {
	day := now.Format("2006-01-02")
	if kind == KindManual {
		return fmt.Sprintf("수동저장_%s_%02d시%02d분%02d초", day, now.Hour(), now.Minute(), now.Second())
	}
	return "자동백업_" + day
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// Info 저장된 백업 한 건.
type Info struct {
	Name          string
	Kind          string
	KindLabel     string
	SavedAt       string
	CreatedByName string
}

// List backups 폴더 목록. 최신순.
func List(dataDir string) ([]Info, error) {
	root := filepath.Join(dataDir, "backups")
	_ = migrateLegacyNames(root)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	loc := SeoulLocation()
	var out []Info
	for _, e := range entries {
		if !e.IsDir() || strings.HasSuffix(e.Name(), ".tmp") {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if !backupComplete(dir) {
			continue
		}
		kind, at := readManifest(dir)
		if kind == "" {
			switch {
			case strings.HasPrefix(e.Name(), "관리로그_"):
				kind = KindLogArchive
			case strings.HasPrefix(e.Name(), "수동저장_") || strings.Contains(e.Name(), "수동"):
				kind = KindManual
			default:
				kind = KindAuto
			}
		}
		kind, label := classifyKind(kind, e.Name())
		if at == "" {
			if t, ok := folderTime(e.Name(), loc); ok {
				at = t.Format("2006-01-02 15:04:05")
			}
		}
		out = append(out, Info{Name: e.Name(), Kind: kind, KindLabel: label, SavedAt: at})
	}
	actors := audit.BackupActors()
	for i := range out {
		if who := actors[out[i].Name]; who != "" {
			out[i].CreatedByName = who
		} else if out[i].Kind == KindAuto {
			out[i].CreatedByName = "시스템"
		}
	}
	sort.Slice(out, func(i, j int) bool {
		ti, oki := folderTime(out[i].Name, loc)
		tj, okj := folderTime(out[j].Name, loc)
		if oki && okj && !ti.Equal(tj) {
			return ti.After(tj)
		}
		return out[i].Name > out[j].Name
	})
	return out, nil
}

func readManifest(dir string) (kind, at string) {
	b, err := os.ReadFile(filepath.Join(dir, "backup.txt"))
	if err != nil {
		return "", ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "kind="); ok {
			kind = strings.TrimSpace(v)
		}
		if v, ok := strings.CutPrefix(line, "backup_at="); ok {
			if t, err := time.Parse(time.RFC3339, strings.TrimSpace(v)); err == nil {
				at = t.In(SeoulLocation()).Format("2006-01-02 15:04:05")
			} else {
				at = strings.TrimSpace(v)
			}
		}
	}
	return kind, at
}

func vacuumInto(db *sql.DB, dest string) error {
	abs, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return err
	}
	_ = os.Remove(abs)
	q := "VACUUM INTO '" + strings.ReplaceAll(filepath.ToSlash(abs), "'", "''") + "'"
	_, err = db.Exec(q)
	return err
}

func backupComplete(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "backup.txt")); err == nil {
		return true
	}
	// 옛 백업은 backup.txt 없이 app.db만 있는 경우가 있다(§25.1 목록 보정).
	if _, err := os.Stat(filepath.Join(dir, "app.db")); err == nil {
		return true
	}
	return false
}

// promoteDir tmp를 dest로 옮긴다. Docker Desktop 볼륨에서는 Rename이 거절될 수 있어 복사로 대체한다.
func promoteDir(tmp, dest string) error {
	if err := os.Rename(tmp, dest); err == nil {
		return nil
	}
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}
	if err := copyDir(tmp, dest); err != nil {
		return err
	}
	return os.RemoveAll(tmp)
}

func prune(root string, keepDays int, now time.Time, loc *time.Location) error {
	if keepDays < 1 {
		keepDays = KeepDays
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	oldest := today.AddDate(0, 0, -(keepDays - 1))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".tmp") {
			_ = os.RemoveAll(filepath.Join(root, name))
			continue
		}
		if strings.HasPrefix(name, "관리로그_") {
			continue
		}
		day, ok := folderDate(name, loc)
		if !ok {
			continue
		}
		if day.Before(oldest) {
			if err := os.RemoveAll(filepath.Join(root, name)); err != nil {
				return err
			}
			log.Printf("오래된 백업 삭제: %s", name)
		}
	}
	return nil
}

func classifyKind(kind, name string) (string, string) {
	if kind == KindLogArchive || strings.HasPrefix(name, "관리로그_") {
		return KindLogArchive, "관리 로그"
	}
	if kind == KindManual {
		return KindManual, "사용자 백업"
	}
	return KindAuto, "정기 백업"
}

func folderDate(name string, loc *time.Location) (time.Time, bool) {
	m := reYMD.FindString(name)
	if m == "" {
		return time.Time{}, false
	}
	day, err := time.ParseInLocation("2006-01-02", m, loc)
	if err != nil {
		return time.Time{}, false
	}
	return day, true
}

var (
	reYMD        = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)
	reLegacyAuto = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	reLegacyMan  = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})_(\d{6})(?:_(\d+))?$`)
	reManualHMS  = regexp.MustCompile(`(\d{4}-\d{2}-\d{2})_(\d{2})시(\d{2})분(?:(\d{2})초)?`)
	reLegacyHMS  = regexp.MustCompile(`(\d{4}-\d{2}-\d{2})_(\d{6})`)
)

func folderTime(name string, loc *time.Location) (time.Time, bool) {
	if m := reManualHMS.FindStringSubmatch(name); len(m) >= 4 {
		h, _ := strconv.Atoi(m[2])
		min, _ := strconv.Atoi(m[3])
		sec := 0
		if m[4] != "" {
			sec, _ = strconv.Atoi(m[4])
		}
		day, err := time.ParseInLocation("2006-01-02", m[1], loc)
		if err != nil {
			return time.Time{}, false
		}
		return time.Date(day.Year(), day.Month(), day.Day(), h, min, sec, 0, loc), true
	}
	if m := reLegacyHMS.FindStringSubmatch(name); len(m) >= 3 {
		t := m[2]
		if len(t) != 6 {
			return time.Time{}, false
		}
		h, _ := strconv.Atoi(t[0:2])
		min, _ := strconv.Atoi(t[2:4])
		sec, _ := strconv.Atoi(t[4:6])
		day, err := time.ParseInLocation("2006-01-02", m[1], loc)
		if err != nil {
			return time.Time{}, false
		}
		return time.Date(day.Year(), day.Month(), day.Day(), h, min, sec, 0, loc), true
	}
	return folderDate(name, loc)
}

func legacyToNew(name string) (string, bool) {
	if strings.HasPrefix(name, "자동백업_") || strings.HasPrefix(name, "수동저장_") {
		return "", false
	}
	if reLegacyAuto.MatchString(name) {
		return "자동백업_" + name, true
	}
	m := reLegacyMan.FindStringSubmatch(name)
	if m == nil {
		return "", false
	}
	t := m[2]
	newName := fmt.Sprintf("수동저장_%s_%s시%s분%s초", m[1], t[0:2], t[2:4], t[4:6])
	if m[3] != "" {
		newName += "_" + m[3]
	}
	return newName, true
}

func migrateLegacyNames(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasSuffix(e.Name(), ".tmp") {
			continue
		}
		newName, ok := legacyToNew(e.Name())
		if !ok {
			continue
		}
		src := filepath.Join(root, e.Name())
		dst := filepath.Join(root, newName)
		if dirExists(dst) {
			continue
		}
		if err := os.Rename(src, dst); err != nil {
			if err2 := promoteDir(src, dst); err2 != nil {
				log.Printf("백업 폴더명 변경 실패 %s → %s: %v", e.Name(), newName, err2)
			}
		}
	}
	return nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}
