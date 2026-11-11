package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"customer-support/internal/model"
)

// ErrNoStructureToCopy 원본에 활성 단계가 없다. 오류 화면이 아니라 안내로 쓴다.
var ErrNoStructureToCopy = errors.New("복사할 구조가 없습니다")

// structureWriteBeforeCommit 시험이 커밋 직전에 실패를 넣는다. 운영 경로는 nil.
var structureWriteBeforeCommit func() error

// StructureCopyStat 추가·건너뛴 개수.
type StructureCopyStat struct {
	PhasesAdded     int
	PackagesAdded   int
	PhasesSkipped   int
	PackagesSkipped int
}

// StructureCopyLine 미리보기에 보여 줄 한 줄.
type StructureCopyLine struct {
	Phase   string
	Package string
	Track   string
	Skip    bool
}

// StructureCopyPlan 저장 없이 계산한 복사 계획.
type StructureCopyPlan struct {
	Stat StructureCopyStat
	Add  []StructureCopyLine
	Skip []StructureCopyLine
	rows []phaseWrite
}

type pkgWrite struct {
	Name  string
	Track string
	Seq   int
}

type phaseWrite struct {
	Name       string
	ExistingID string
	Seq        int
	Pkgs       []pkgWrite
}

func (r *ProjectRepo) PreviewStructureCopy(toID, fromID string) (StructureCopyPlan, error) {
	return r.planStructureCopy(toID, fromID)
}

func (r *ProjectRepo) CopyStructure(toID, fromID, orgID string) (StructureCopyStat, error) {
	plan, err := r.planStructureCopy(toID, fromID)
	if err != nil {
		return StructureCopyStat{}, err
	}
	if err := r.writeNewStructure(toID, orgID, plan.rows); err != nil {
		return StructureCopyStat{}, err
	}
	return plan.Stat, nil
}

func (r *ProjectRepo) planStructureCopy(toID, fromID string) (StructureCopyPlan, error) {
	var plan StructureCopyPlan
	srcPhases, err := r.ListPhases(fromID)
	if err != nil {
		return plan, err
	}
	srcPkgs, err := r.listPackages(fromID)
	if err != nil {
		return plan, err
	}
	dstPhases, err := r.ListPhases(toID)
	if err != nil {
		return plan, err
	}
	dstPkgs, err := r.listPackages(toID)
	if err != nil {
		return plan, err
	}
	byPhase := map[string][]pkgRow{}
	for _, pk := range srcPkgs {
		byPhase[pk.phase] = append(byPhase[pk.phase], pk)
	}
	havePhase := map[string]string{}
	phaseActive := map[string]bool{}
	maxPhaseSeq := -1
	for _, p := range dstPhases {
		if p.Seq > maxPhaseSeq {
			maxPhaseSeq = p.Seq
		}
		key := model.NormalizePhaseMergeName(p.Name)
		if phaseActive[key] {
			continue
		}
		havePhase[key] = p.PhaseID
		if p.IsActive {
			phaseActive[key] = true
		}
	}
	maxPkgSeq := map[string]int{}
	havePkg := map[string]bool{}
	for _, pk := range dstPkgs {
		if cur, ok := maxPkgSeq[pk.phase]; !ok || pk.seq > cur {
			maxPkgSeq[pk.phase] = pk.seq
		}
		havePkg[pk.phase+"\t"+model.NormalizePlanMergeName(pk.name)] = true
	}
	r.noteInactivePackages(toID, havePkg)
	active := 0
	for _, p := range srcPhases {
		if !p.IsActive {
			continue
		}
		active++
		key := model.NormalizePhaseMergeName(p.Name)
		row := phaseWrite{Name: p.Name}
		phaseKey := havePhase[key]
		if phaseKey != "" {
			row.ExistingID = phaseKey
			plan.Stat.PhasesSkipped++
			plan.Skip = append(plan.Skip, StructureCopyLine{Phase: p.Name, Skip: true})
		} else {
			maxPhaseSeq++
			row.Seq = maxPhaseSeq
			plan.Stat.PhasesAdded++
			plan.Add = append(plan.Add, StructureCopyLine{Phase: p.Name})
			phaseKey = "new:" + key
			havePhase[key] = phaseKey
		}
		next, seen := maxPkgSeq[phaseKey]
		if !seen && row.ExistingID == "" {
			next = -1
		}
		if row.ExistingID != "" {
			if n, ok := maxPkgSeq[row.ExistingID]; ok {
				next = n
			} else {
				next = -1
			}
		}
		for _, pk := range byPhase[p.PhaseID] {
			nk := model.NormalizePlanMergeName(pk.name)
			bag := phaseKey
			if row.ExistingID != "" {
				bag = row.ExistingID
			}
			line := StructureCopyLine{Phase: p.Name, Package: pk.name, Track: pk.track}
			if havePkg[bag+"\t"+nk] {
				plan.Stat.PackagesSkipped++
				line.Skip = true
				plan.Skip = append(plan.Skip, line)
				continue
			}
			havePkg[bag+"\t"+nk] = true
			next++
			track := pk.track
			if !model.ValidTrackMode(track) {
				track = model.TrackModeSite
			}
			row.Pkgs = append(row.Pkgs, pkgWrite{Name: pk.name, Track: track, Seq: next})
			plan.Stat.PackagesAdded++
			line.Track = track
			plan.Add = append(plan.Add, line)
		}
		if row.ExistingID != "" {
			maxPkgSeq[row.ExistingID] = next
		}
		if row.ExistingID == "" || len(row.Pkgs) > 0 {
			plan.rows = append(plan.rows, row)
		}
	}
	if active == 0 {
		return StructureCopyPlan{}, ErrNoStructureToCopy
	}
	return plan, nil
}

func (r *ProjectRepo) writeNewStructure(projectID, orgID string, rows []phaseWrite) error {
	if len(rows) == 0 {
		return nil
	}
	orgID = strings.TrimSpace(orgID)
	if orgID == "" || orgID == OrgAll {
		return fmt.Errorf("소속 조직을 선택하세요")
	}
	nPhase, nPkg := 0, 0
	for _, row := range rows {
		if row.ExistingID == "" {
			nPhase++
		}
		nPkg += len(row.Pkgs)
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	phaseStart, err := reserveSeqTx(tx, phaseSeqKey, nPhase)
	if err != nil {
		return err
	}
	pkgStart, err := reserveSeqTx(tx, pkgSeqKey, nPkg)
	if err != nil {
		return err
	}
	msStart, err := reserveSeqTx(tx, "package_milestone", nPkg*4)
	if err != nil {
		return err
	}
	now := nowStamp()
	var phaseRows, pkgRows, msRows [][]any
	pi, ki, mi := 0, 0, 0
	for _, row := range rows {
		phaseID := row.ExistingID
		if phaseID == "" {
			phaseID = fmt.Sprintf("PH-%03d", phaseStart+pi)
			pi++
			phaseRows = append(phaseRows, []any{
				phaseID, projectID, orgID, row.Seq, row.Name,
				"", "", "", "", model.PhasePlanned, "", 1, now, now,
			})
		}
		for _, pk := range row.Pkgs {
			id := fmt.Sprintf("PK-%03d", pkgStart+ki)
			ki++
			pkgRows = append(pkgRows, []any{
				id, phaseID, pk.Seq, pk.Name, pk.Track, 1, "auto", 1, now, now,
			})
			for _, pct := range []int{25, 50, 75, 100} {
				msRows = append(msRows, []any{
					fmt.Sprintf("MS-%03d", msStart+mi), id, "", pct, fmt.Sprintf("%d%%", pct),
				})
				mi++
			}
		}
	}
	if err := insertChunks(tx, `INSERT INTO project_phases(
		phase_id, project_id, org_id, seq, name,
		plan_start, plan_end, actual_start, actual_end, status, note, is_active, created_at, updated_at) VALUES `,
		phaseRows); err != nil {
		return err
	}
	if err := insertChunks(tx, `INSERT INTO phase_packages(
		package_id, phase_id, seq, name, track_mode, weight, status, is_active, created_at, updated_at) VALUES `,
		pkgRows); err != nil {
		return err
	}
	if err := insertChunks(tx, `INSERT INTO package_milestones(
		milestone_id, package_id, plan_id, threshold_pct, label) VALUES `, msRows); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			err = nil
		}
		if err != nil {
			return err
		}
	}
	if structureWriteBeforeCommit != nil {
		if err := structureWriteBeforeCommit(); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *ProjectRepo) noteInactivePackages(projectID string, have map[string]bool) {
	if r == nil || r.db == nil || have == nil {
		return
	}
	rows, err := r.db.Query(`
		SELECT pk.phase_id, pk.name FROM phase_packages pk
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE ph.project_id=? AND pk.is_active=0`, projectID)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var phase, name string
		if err := rows.Scan(&phase, &name); err != nil {
			return
		}
		have[phase+"\t"+model.NormalizePlanMergeName(name)] = true
	}
}

func reserveSeqTx(tx *sql.Tx, key string, n int) (int, error) {
	if n <= 0 {
		return 1, nil
	}
	var last int
	err := tx.QueryRow(`SELECT last_no FROM id_sequences WHERE seq_key=?`, key).Scan(&last)
	if err == sql.ErrNoRows {
		if _, err := tx.Exec(`INSERT INTO id_sequences(seq_key, last_no) VALUES(?,?)`, key, n); err != nil {
			return 0, err
		}
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`UPDATE id_sequences SET last_no=? WHERE seq_key=?`, last+n, key); err != nil {
		return 0, err
	}
	return last + 1, nil
}

func insertChunks(tx *sql.Tx, prefix string, rows [][]any) error {
	if len(rows) == 0 {
		return nil
	}
	width := len(rows[0])
	chunk := 400 / width
	if chunk < 1 {
		chunk = 1
	}
	for i := 0; i < len(rows); i += chunk {
		end := i + chunk
		if end > len(rows) {
			end = len(rows)
		}
		part := rows[i:end]
		var b strings.Builder
		b.WriteString(prefix)
		args := make([]any, 0, len(part)*width)
		for ri, row := range part {
			if ri > 0 {
				b.WriteByte(',')
			}
			b.WriteByte('(')
			for c := range row {
				if c > 0 {
					b.WriteByte(',')
				}
				b.WriteByte('?')
			}
			b.WriteByte(')')
			args = append(args, row...)
		}
		if _, err := tx.Exec(b.String(), args...); err != nil {
			return err
		}
	}
	return nil
}
