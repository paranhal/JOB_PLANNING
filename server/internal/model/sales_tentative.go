package model

import (
	"fmt"
	"strings"
)

func ApplySalesTentative(p *SalesProject) error {
	if p == nil {
		return fmt.Errorf("사업이 없습니다")
	}
	official := strings.TrimSpace(p.Name)
	tent := strings.TrimSpace(p.TentativeName)
	if p.IsTentativeName {
		if tent == "" && official == "" {
			return fmt.Errorf("가칭을 입력하세요")
		}
		if tent == "" {
			tent = official
		}
		p.TentativeName = tent
		if official == "" {
			p.Name = tent
		} else {
			p.Name = official
		}
	} else {
		p.TentativeName = ""
		p.Name = official
		if p.Name == "" {
			return fmt.Errorf("사업명을 입력하세요")
		}
	}
	return nil
}

func ApplySalesCustomerExclusive(p *SalesProject) {
	if p == nil {
		return
	}
	if strings.TrimSpace(p.CustomerID) != "" {
		p.ProspectName = ""
	}
}

func UniqueIDs(ids []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func (p *SalesProject) HasSalesPerson(userID, name string) bool {
	if p == nil {
		return false
	}
	userID = strings.TrimSpace(userID)
	name = strings.TrimSpace(name)
	if userID != "" {
		if userID == strings.TrimSpace(p.SalesOwnerID) {
			return true
		}
		for _, id := range p.SalesOwnerIDs {
			if strings.TrimSpace(id) == userID {
				return true
			}
		}
	}
	if name != "" && name == strings.TrimSpace(p.SalesOwner) {
		return true
	}
	return false
}

type PickerOrgMember struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	Role   string `json:"role,omitempty"`
}

type PickerOrgGroup struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Members []PickerOrgMember `json:"members"`
}
