package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"customer-support/internal/model"
)

// ErrTemplateNameTaken 같은 이름의 전사 템플릿이 이미 있다.
var ErrTemplateNameTaken = errors.New("같은 이름의 템플릿이 있습니다")

// ErrNoStructureToSave 저장할 활성 단계가 없다.
var ErrNoStructureToSave = errors.New("저장할 구조가 없습니다")

// StructureTemplate 전사 공통 수행 구조 템플릿.
type StructureTemplate struct {
	ID, OrgID, Name, Kind, Note, CreatedBy string
	Active                                 bool
	Phases                                 []StructureTemplatePhase
}

// StructureTemplatePhase 템플릿 단계.
type StructureTemplatePhase struct {
	Name     string
	Seq      int
	Packages []StructureTemplatePackage
}

// StructureTemplatePackage 템플릿 하부 업무. 이름·진행 방식·순서만 담는다.
type StructureTemplatePackage struct {
	Name  string
	Track string
	Seq   int
}

// StructureSourceHit 구조가 있는 사업 검색 결과.
type StructureSourceHit struct {
	ProjectID, Name, ContractNo, OrgName string
	Phases, Packages                     int
}

func (r *ProjectRepo) SearchStructureSources(excludeID, q string) ([]StructureSourceHit, error) {
	q = strings.TrimSpace(q)
	if r == nil || r.db == nil || q == "" {
		return nil, nil
	}
	like := "%" + escapeLike(q) + "%"
	rows, err := r.db.Query(`
		SELECT p.project_id, p.name, COALESCE(p.contract_no,''), COALESCE(o.org_name,''),
			(SELECT COUNT(*) FROM project_phases ph WHERE ph.project_id=p.project_id AND ph.is_active=1),
			(SELECT COUNT(*) FROM phase_packages pk
				JOIN project_phases ph ON ph.phase_id=pk.phase_id
				WHERE ph.project_id=p.project_id AND pk.is_active=1 AND ph.is_active=1)
		FROM work_projects p
		LEFT JOIN orgs o ON o.org_id=p.org_id
		WHERE p.project_id<>?
		  AND (p.name LIKE ? ESCAPE '\' OR COALESCE(p.short_name,'') LIKE ? ESCAPE '\'
		    OR COALESCE(p.contract_no,'') LIKE ? ESCAPE '\' OR p.project_id LIKE ? ESCAPE '\')
		  AND EXISTS (
		    SELECT 1 FROM phase_packages pk
		    JOIN project_phases ph ON ph.phase_id=pk.phase_id
		    WHERE ph.project_id=p.project_id AND pk.is_active=1 AND ph.is_active=1)
		ORDER BY p.name
		LIMIT 30`, excludeID, like, like, like, like)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StructureSourceHit
	for rows.Next() {
		var h StructureSourceHit
		if err := rows.Scan(&h.ProjectID, &h.Name, &h.ContractNo, &h.OrgName, &h.Phases, &h.Packages); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (r *ProjectRepo) ListStructureTemplates(activeOnly bool) ([]StructureTemplate, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	q := `SELECT template_id, org_id, name, project_kind, note, is_active, created_by
		FROM structure_templates`
	if activeOnly {
		q += ` WHERE is_active=1`
	}
	q += ` ORDER BY name`
	rows, err := r.db.Query(q)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []StructureTemplate
	for rows.Next() {
		var t StructureTemplate
		var active int
		if err := rows.Scan(&t.ID, &t.OrgID, &t.Name, &t.Kind, &t.Note, &active, &t.CreatedBy); err != nil {
			return nil, err
		}
		t.Active = active != 0
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *ProjectRepo) GetStructureTemplate(id string) (*StructureTemplate, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, sql.ErrNoRows
	}
	var t StructureTemplate
	var active int
	err := r.db.QueryRow(`
		SELECT template_id, org_id, name, project_kind, note, is_active, created_by
		FROM structure_templates WHERE template_id=?`, id).Scan(
		&t.ID, &t.OrgID, &t.Name, &t.Kind, &t.Note, &active, &t.CreatedBy)
	if err != nil {
		return nil, err
	}
	t.Active = active != 0
	rows, err := r.db.Query(`
		SELECT ph.name, ph.seq, COALESCE(pk.name,''), COALESCE(pk.track_mode,'site'), COALESCE(pk.seq,0)
		FROM structure_template_phases ph
		LEFT JOIN structure_template_packages pk ON pk.tphase_id=ph.tphase_id
		WHERE ph.template_id=?
		ORDER BY ph.seq, ph.tphase_id, pk.seq, pk.tpackage_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	index := map[string]int{}
	for rows.Next() {
		var phase, pkg, track string
		var pseq, kseq int
		if err := rows.Scan(&phase, &pseq, &pkg, &track, &kseq); err != nil {
			return nil, err
		}
		i, ok := index[phase+"\t"+fmt.Sprint(pseq)]
		if !ok {
			t.Phases = append(t.Phases, StructureTemplatePhase{Name: phase, Seq: pseq})
			i = len(t.Phases) - 1
			index[phase+"\t"+fmt.Sprint(pseq)] = i
		}
		if strings.TrimSpace(pkg) != "" {
			t.Phases[i].Packages = append(t.Phases[i].Packages, StructureTemplatePackage{
				Name: pkg, Track: track, Seq: kseq,
			})
		}
	}
	return &t, rows.Err()
}

func (r *ProjectRepo) SaveStructureTemplate(projectID, orgID, name, kind, note, actor string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("템플릿 이름을 입력하세요")
	}
	if len([]rune(name)) > 100 {
		return "", fmt.Errorf("템플릿 이름은 100자까지입니다")
	}
	srcPhases, err := r.ListPhases(projectID)
	if err != nil {
		return "", err
	}
	srcPkgs, err := r.listPackages(projectID)
	if err != nil {
		return "", err
	}
	by := map[string][]pkgRow{}
	for _, pk := range srcPkgs {
		by[pk.phase] = append(by[pk.phase], pk)
	}
	var phases []StructureTemplatePhase
	for _, p := range srcPhases {
		if !p.IsActive {
			continue
		}
		ph := StructureTemplatePhase{Name: p.Name, Seq: p.Seq}
		for _, pk := range by[p.PhaseID] {
			track := pk.track
			if !model.ValidTrackMode(track) {
				track = model.TrackModeSite
			}
			ph.Packages = append(ph.Packages, StructureTemplatePackage{Name: pk.name, Track: track, Seq: pk.seq})
		}
		phases = append(phases, ph)
	}
	if len(phases) == 0 {
		return "", ErrNoStructureToSave
	}
	var n int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM structure_templates WHERE name=?`, name).Scan(&n); err != nil {
		return "", err
	}
	if n > 0 {
		return "", ErrTemplateNameTaken
	}
	tx, err := r.db.Begin()
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	idNo, err := reserveSeqTx(tx, "structure_template", 1)
	if err != nil {
		return "", err
	}
	phNo, err := reserveSeqTx(tx, "structure_template_phase", len(phases))
	if err != nil {
		return "", err
	}
	nPkg := 0
	for _, ph := range phases {
		nPkg += len(ph.Packages)
	}
	pkNo, err := reserveSeqTx(tx, "structure_template_package", nPkg)
	if err != nil {
		return "", err
	}
	now := nowStamp()
	id := fmt.Sprintf("ST-%03d", idNo)
	if _, err := tx.Exec(`
		INSERT INTO structure_templates(template_id, org_id, name, project_kind, note, is_active, created_by, created_at, updated_at)
		VALUES (?,?,?,?,?,1,?,?,?)`, id, strings.TrimSpace(orgID), name, strings.TrimSpace(kind), strings.TrimSpace(note), strings.TrimSpace(actor), now, now); err != nil {
		return "", err
	}
	var phaseRows, pkgRows [][]any
	ki := 0
	for i, ph := range phases {
		pid := fmt.Sprintf("TP-%03d", phNo+i)
		phaseRows = append(phaseRows, []any{pid, id, i, ph.Name})
		for _, pk := range ph.Packages {
			pkgRows = append(pkgRows, []any{fmt.Sprintf("TK-%03d", pkNo+ki), pid, ki, pk.Name, pk.Track})
			ki++
		}
	}
	if err := insertChunks(tx, `INSERT INTO structure_template_phases(tphase_id, template_id, seq, name) VALUES `, phaseRows); err != nil {
		return "", err
	}
	if err := insertChunks(tx, `INSERT INTO structure_template_packages(tpackage_id, tphase_id, seq, name, track_mode) VALUES `, pkgRows); err != nil {
		return "", err
	}
	if structureWriteBeforeCommit != nil {
		if err := structureWriteBeforeCommit(); err != nil {
			return "", err
		}
	}
	return id, tx.Commit()
}

func (r *ProjectRepo) RenameStructureTemplate(id, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("템플릿 이름을 입력하세요")
	}
	var n int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM structure_templates WHERE name=? AND template_id<>?`, name, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrTemplateNameTaken
	}
	res, err := r.db.Exec(`UPDATE structure_templates SET name=?, updated_at=? WHERE template_id=?`, name, nowStamp(), id)
	if err != nil {
		return err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *ProjectRepo) DeactivateStructureTemplate(id string) error {
	res, err := r.db.Exec(`UPDATE structure_templates SET is_active=0, updated_at=? WHERE template_id=? AND is_active=1`, nowStamp(), id)
	if err != nil {
		return err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *ProjectRepo) PreviewTemplateApply(projectID, templateID string) (StructureCopyPlan, error) {
	t, err := r.GetStructureTemplate(templateID)
	if err != nil {
		return StructureCopyPlan{}, err
	}
	if !t.Active {
		return StructureCopyPlan{}, sql.ErrNoRows
	}
	return r.planTemplateApply(projectID, t)
}

func (r *ProjectRepo) ApplyStructureTemplate(projectID, orgID, templateID string) (StructureCopyStat, error) {
	t, err := r.GetStructureTemplate(templateID)
	if err != nil {
		return StructureCopyStat{}, err
	}
	if t == nil || !t.Active {
		return StructureCopyStat{}, sql.ErrNoRows
	}
	plan, err := r.planTemplateApply(projectID, t)
	if err != nil {
		return StructureCopyStat{}, err
	}
	if err := r.writeNewStructure(projectID, orgID, plan.rows); err != nil {
		return StructureCopyStat{}, err
	}
	return plan.Stat, nil
}

func (r *ProjectRepo) planTemplateApply(projectID string, t *StructureTemplate) (StructureCopyPlan, error) {
	var plan StructureCopyPlan
	if t == nil || len(t.Phases) == 0 {
		return plan, ErrNoStructureToCopy
	}
	dstPhases, err := r.ListPhases(projectID)
	if err != nil {
		return plan, err
	}
	dstPkgs, err := r.listPackages(projectID)
	if err != nil {
		return plan, err
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
	r.noteInactivePackages(projectID, havePkg)
	for _, p := range t.Phases {
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
		next := -1
		bag := phaseKey
		if row.ExistingID != "" {
			bag = row.ExistingID
			if n, ok := maxPkgSeq[bag]; ok {
				next = n
			}
		}
		for _, pk := range p.Packages {
			nk := model.NormalizePlanMergeName(pk.Name)
			line := StructureCopyLine{Phase: p.Name, Package: pk.Name, Track: pk.Track}
			if havePkg[bag+"\t"+nk] {
				plan.Stat.PackagesSkipped++
				line.Skip = true
				plan.Skip = append(plan.Skip, line)
				continue
			}
			havePkg[bag+"\t"+nk] = true
			next++
			track := pk.Track
			if !model.ValidTrackMode(track) {
				track = model.TrackModeSite
			}
			row.Pkgs = append(row.Pkgs, pkgWrite{Name: pk.Name, Track: track, Seq: next})
			plan.Stat.PackagesAdded++
			line.Track = track
			plan.Add = append(plan.Add, line)
		}
		if row.ExistingID == "" || len(row.Pkgs) > 0 {
			plan.rows = append(plan.rows, row)
		}
	}
	return plan, nil
}
