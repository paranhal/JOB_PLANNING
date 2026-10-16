package repository

import "strings"

// appendSalesPersonSQL 담당·등록·활동 참여 중 하나라도 맞으면 포함한다.
func appendSalesPersonSQL(q string, args []interface{}, projectAlias, name, userID string) (string, []interface{}) {
	name = strings.TrimSpace(name)
	userID = strings.TrimSpace(userID)
	if name == "" && userID == "" {
		return q, args
	}
	p := projectAlias
	if p == "" {
		p = "s"
	}
	q += ` AND (`
	var parts []string
	if name != "" {
		parts = append(parts,
			p+`.sales_owner=?`,
			`EXISTS (SELECT 1 FROM sales_activities _sa JOIN sales_activity_members _sm ON _sm.activity_id=_sa.activity_id WHERE _sa.sales_id=`+p+`.sales_id AND _sm.member=?)`,
			`EXISTS (SELECT 1 FROM sales_activities _sa2 WHERE _sa2.sales_id=`+p+`.sales_id AND (_sa2.created_by=? OR COALESCE(_sa2.created_by_name,'')=?))`,
		)
		args = append(args, name, name, name, name)
	}
	if userID != "" {
		parts = append(parts, p+`.sales_owner_id=?`, p+`.created_by_user_id=?`)
		args = append(args, userID, userID)
	}
	q += strings.Join(parts, " OR ") + `)`
	return q, args
}

func appendActivityPersonSQL(q string, args []interface{}, name, userID string) (string, []interface{}) {
	name = strings.TrimSpace(name)
	userID = strings.TrimSpace(userID)
	if name == "" && userID == "" {
		return q, args
	}
	q += ` AND (`
	var parts []string
	if name != "" {
		parts = append(parts,
			`a.created_by=?`,
			`COALESCE(a.created_by_name,'')=?`,
			`EXISTS (SELECT 1 FROM sales_activity_members _sm WHERE _sm.activity_id=a.activity_id AND _sm.member=?)`,
			`s.sales_owner=?`,
		)
		args = append(args, name, name, name, name)
	}
	if userID != "" {
		parts = append(parts, `COALESCE(a.created_by_user_id,'')=?`, `s.sales_owner_id=?`, `s.created_by_user_id=?`)
		args = append(args, userID, userID, userID)
	}
	q += strings.Join(parts, " OR ") + `)`
	return q, args
}

func appendQuotePersonSQL(q string, args []interface{}, name, userID string) (string, []interface{}) {
	name = strings.TrimSpace(name)
	userID = strings.TrimSpace(userID)
	if name == "" && userID == "" {
		return q, args
	}
	q += ` AND (`
	var parts []string
	if name != "" {
		parts = append(parts, `COALESCE(owner_name,'')=?`, `COALESCE(created_by_name,'')=?`)
		args = append(args, name, name)
	}
	if userID != "" {
		parts = append(parts, `COALESCE(owner_user_id,'')=?`, `COALESCE(created_by_user_id,'')=?`)
		args = append(args, userID, userID)
	}
	q += strings.Join(parts, " OR ")
	q += ` OR EXISTS (SELECT 1 FROM sales_projects s WHERE s.sales_id=sales_quotes.sales_id`
	inner, innerArgs := appendSalesPersonSQL("", nil, "s", name, userID)
	q += inner + `))`
	args = append(args, innerArgs...)
	return q, args
}

func appendOrderOrgSQL(q string, args []interface{}, orgID string) (string, []interface{}) {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" || orgID == OrgAll {
		return q, args
	}
	q += ` AND EXISTS (SELECT 1 FROM sales_projects s WHERE s.sales_id=sales_orders.sales_id AND s.org_id=?)`
	args = append(args, orgID)
	return q, args
}

func appendOrderPersonSQL(q string, args []interface{}, name, userID string) (string, []interface{}) {
	name = strings.TrimSpace(name)
	userID = strings.TrimSpace(userID)
	if name == "" && userID == "" {
		return q, args
	}
	q += ` AND EXISTS (SELECT 1 FROM sales_projects s WHERE s.sales_id=sales_orders.sales_id`
	inner, innerArgs := appendSalesPersonSQL("", nil, "s", name, userID)
	q += inner + `)`
	args = append(args, innerArgs...)
	return q, args
}
