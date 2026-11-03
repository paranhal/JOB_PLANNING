package handler

import (
	"strings"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func orgNameMap(orgs *repository.OrgRepo) map[string]string {
	out := map[string]string{}
	if orgs == nil {
		return out
	}
	rows, err := orgs.ListActive()
	if err != nil {
		return out
	}
	for _, o := range rows {
		name := strings.TrimSpace(o.ShortName)
		if name == "" {
			name = strings.TrimSpace(o.OrgName)
		}
		if name == "" {
			name = o.OrgID
		}
		out[o.OrgID] = name
	}
	return out
}

func pickerOrgGroups(users []model.User, names map[string]string) []model.PickerOrgGroup {
	order := make([]string, 0)
	idx := map[string]int{}
	var groups []model.PickerOrgGroup
	for _, u := range users {
		if strings.TrimSpace(u.UserID) == "" {
			continue
		}
		org := strings.TrimSpace(u.OrgID)
		if org == "" {
			org = "_"
		}
		i, ok := idx[org]
		if !ok {
			label := names[org]
			if org == "_" || label == "" {
				if org == "_" {
					label = "조직 미지정"
				} else {
					label = org
				}
			}
			groups = append(groups, model.PickerOrgGroup{ID: org, Name: label})
			idx[org] = len(groups) - 1
			order = append(order, org)
			i = len(groups) - 1
		}
		groups[i].Members = append(groups[i].Members, model.PickerOrgMember{
			UserID: u.UserID,
			Name:   pickerDisplayName(u),
		})
	}
	_ = order
	return groups
}

func dutyAssigneeUsers(users *repository.UserRepo, p *model.WorkProject) []model.User {
	if users == nil || p == nil {
		return nil
	}
	orgs := []string{strings.TrimSpace(p.OrgID)}
	orgs = append(orgs, p.RelatedOrgIDs...)
	list, _ := users.ListAssignableInOrgs(orgs)
	return list
}

func accountingAssigneeUsers(users *repository.UserRepo) []model.User {
	if users == nil {
		return nil
	}
	list, _ := users.LookupAccountingAssignees()
	return list
}

func assignSelectedMap(items []model.ProjectAssignment) map[string][]string {
	out := map[string][]string{}
	for _, a := range items {
		out[a.Duty] = append(out[a.Duty], a.UserID)
	}
	return out
}
