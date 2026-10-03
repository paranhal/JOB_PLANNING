package repository

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"customer-support/internal/model"
)

type OrgRepo struct {
	db *sql.DB
}

func NewOrgRepo(db *sql.DB) *OrgRepo {
	return &OrgRepo{db: db}
}

type OrgListRow struct {
	model.Org
	UserCount     int
	CustomerCount int
	ASCount       int
	SalesCount    int
}

func (r *OrgRepo) List() ([]OrgListRow, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("db")
	}
	rows, err := r.db.Query(`
		SELECT o.org_id, COALESCE(o.org_no,''), o.org_name, COALESCE(o.short_name,''),
		       COALESCE(o.is_active,1), COALESCE(o.sort_order,0), COALESCE(o.note,''),
		       COALESCE(o.created_at,''), COALESCE(o.created_by,''),
		       (SELECT COUNT(*) FROM users u WHERE u.org_id=o.org_id AND COALESCE(u.role,'')<>'vision_admin'),
		       (SELECT COUNT(*) FROM customers c WHERE c.org_id=o.org_id),
		       (SELECT COUNT(*) FROM as_receipts ar WHERE ar.org_id=o.org_id),
		       (SELECT COUNT(*) FROM sales_projects s WHERE s.org_id=o.org_id)
		FROM orgs o
		ORDER BY o.sort_order, o.org_no, o.org_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OrgListRow
	for rows.Next() {
		var it OrgListRow
		var active int
		if err := rows.Scan(&it.OrgID, &it.OrgNo, &it.OrgName, &it.ShortName, &active,
			&it.SortOrder, &it.Note, &it.CreatedAt, &it.CreatedBy,
			&it.UserCount, &it.CustomerCount, &it.ASCount, &it.SalesCount); err != nil {
			return nil, err
		}
		it.IsActive = active != 0
		out = append(out, it)
	}
	return out, rows.Err()
}

func (r *OrgRepo) ListActive() ([]model.Org, error) {
	rows, err := r.List()
	if err != nil {
		return nil, err
	}
	out := make([]model.Org, 0, len(rows))
	for _, it := range rows {
		if it.IsActive {
			out = append(out, it.Org)
		}
	}
	return out, nil
}

func (r *OrgRepo) Get(orgID string) (*model.Org, error) {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" || orgID == OrgAll {
		return nil, ErrEmptyOrgID
	}
	var it model.Org
	var active int
	err := r.db.QueryRow(`
		SELECT org_id, COALESCE(org_no,''), org_name, COALESCE(short_name,''),
		       COALESCE(is_active,1), COALESCE(sort_order,0), COALESCE(note,''),
		       COALESCE(created_at,''), COALESCE(created_by,'')
		FROM orgs WHERE org_id=?`, orgID).Scan(
		&it.OrgID, &it.OrgNo, &it.OrgName, &it.ShortName, &active,
		&it.SortOrder, &it.Note, &it.CreatedAt, &it.CreatedBy)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	it.IsActive = active != 0
	return &it, nil
}

func (r *OrgRepo) nextOrgNo() (string, error) {
	rows, err := r.db.Query(`SELECT COALESCE(org_no,''), org_id FROM orgs`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	maxN := 0
	for rows.Next() {
		var no, id string
		if err := rows.Scan(&no, &id); err != nil {
			return "", err
		}
		maxN = maxOrgNum(maxN, no)
		maxN = maxOrgNum(maxN, id)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return fmt.Sprintf("O%02d", maxN+1), nil
}

func maxOrgNum(cur int, s string) int {
	s = strings.TrimSpace(strings.ToUpper(s))
	if !strings.HasPrefix(s, "O") {
		return cur
	}
	n, err := strconv.Atoi(strings.TrimPrefix(s, "O"))
	if err != nil || n <= cur {
		return cur
	}
	return n
}

func (r *OrgRepo) Create(name, shortName, note, createdBy string) (*model.Org, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("조직명을 입력하세요")
	}
	no, err := r.nextOrgNo()
	if err != nil {
		return nil, err
	}
	var maxSort int
	_ = r.db.QueryRow(`SELECT COALESCE(MAX(sort_order),0) FROM orgs`).Scan(&maxSort)
	now := time.Now().Format("2006-01-02 15:04:05")
	o := &model.Org{
		OrgID: no, OrgNo: no, OrgName: name,
		ShortName: strings.TrimSpace(shortName),
		IsActive:  true, SortOrder: maxSort + 1,
		Note: strings.TrimSpace(note), CreatedAt: now, CreatedBy: strings.TrimSpace(createdBy),
	}
	_, err = r.db.Exec(`INSERT INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order,note,created_at,created_by)
		VALUES(?,?,?,?,1,?,?,?,?)`,
		o.OrgID, o.OrgNo, o.OrgName, o.ShortName, o.SortOrder, o.Note, o.CreatedAt, o.CreatedBy)
	if err != nil {
		return nil, err
	}
	return o, nil
}

func (r *OrgRepo) Update(orgID, name, shortName, note string, active bool) error {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" || orgID == OrgAll {
		return ErrEmptyOrgID
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("조직명을 입력하세요")
	}
	res, err := r.db.Exec(`UPDATE orgs SET org_name=?, short_name=?, note=?, is_active=?
		WHERE org_id=?`, name, strings.TrimSpace(shortName), strings.TrimSpace(note), boolToInt(active), orgID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("조직을 찾지 못했습니다")
	}
	return nil
}
