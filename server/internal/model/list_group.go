package model

import "strings"

// NamedGroup 상태 축이 없는 목록의 접고 펼 그룹. §35.2
type NamedGroup[T any] struct {
	Key   string
	Label string
	Items []T
}

func GroupCustomersByParent(items []CustomerListItem) []NamedGroup[CustomerListItem] {
	idx := map[string]int{}
	var groups []NamedGroup[CustomerListItem]
	for _, c := range items {
		key := strings.TrimSpace(c.ParentCustomerID)
		label := strings.TrimSpace(c.ParentOrgName)
		if key == "" {
			key = "_"
			label = "상위기관 없음"
		}
		if label == "" {
			label = key
		}
		i, ok := idx[key]
		if !ok {
			idx[key] = len(groups)
			groups = append(groups, NamedGroup[CustomerListItem]{Key: key, Label: label})
			i = len(groups) - 1
		}
		groups[i].Items = append(groups[i].Items, c)
	}
	return groups
}

func GroupUsersByRole(users []User) []NamedGroup[User] {
	order := []string{RoleTech, RoleSales, RoleOffice, RoleAdmin, RoleObserver}
	idx := map[string]int{}
	groups := make([]NamedGroup[User], 0, len(order)+1)
	for _, role := range order {
		idx[role] = len(groups)
		groups = append(groups, NamedGroup[User]{Key: role, Label: RoleLabel(role) + " 소속"})
	}
	other := len(groups)
	groups = append(groups, NamedGroup[User]{Key: "_", Label: "소속 미지정"})

	for _, u := range users {
		role := NormalizeRole(u.Role)
		i, ok := idx[role]
		if !ok {
			i = other
		}
		groups[i].Items = append(groups[i].Items, u)
	}
	out := make([]NamedGroup[User], 0, len(groups))
	for _, g := range groups {
		if len(g.Items) == 0 {
			continue
		}
		out = append(out, g)
	}
	return out
}
