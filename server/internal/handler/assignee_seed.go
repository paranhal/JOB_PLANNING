package handler

import (
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// AssigneeSeed 95-123 담당자 초기값. 로그인 본인을 고르고, 조직관리자·지원은 바꿀 수 있다(필수).
type AssigneeSeed struct {
	Name     string
	OwnerID  string
	IDs      []string
	Required bool
}

func defaultAssigneeSeed(c echo.Context) AssigneeSeed {
	req := requiresAssigneeChoice(c)
	id := strings.TrimSpace(currentUserID(c))
	name := pickerPersonLabel(currentUserDisplayName(c))
	if name == "" {
		name = pickerPersonLabel(ctxString(c, "username"))
	}
	ids := []string{}
	if id != "" {
		ids = []string{id}
	}
	return AssigneeSeed{Name: name, OwnerID: id, IDs: ids, Required: req}
}

func attachNewWorkAssigneePicker(c echo.Context, users *repository.UserRepo, data map[string]interface{}) {
	if data == nil {
		return
	}
	seed := defaultAssigneeSeed(c)
	list, _ := data["Assignees"].([]model.User)
	list = ensurePickerUsers(users, list, seed.OwnerID, currentUserID(c))
	names := pickerNamesFor(users, list, seed.OwnerID, currentUserID(c))
	if seed.OwnerID != "" {
		if n := pickerPersonLabel(names[seed.OwnerID]); n == "" {
			if u := pickerPersonLabel(ctxString(c, "username")); u != "" {
				names[seed.OwnerID] = u
				if seed.Name == "" {
					seed.Name = u
				}
			}
		} else if seed.Name == "" {
			seed.Name = n
		}
	}
	data["Assignees"] = list
	data["PickerNames"] = names
	data["PickerOwnerID"] = seed.OwnerID
	data["PickerUserIDs"] = seed.IDs
	data["AssigneeRequired"] = seed.Required
	data["AssigneeSeed"] = seed
	data["DisplayAssignee"] = seed.Name
}
