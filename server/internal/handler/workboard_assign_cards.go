package handler

import (
	"net/url"
	"strings"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-225: 수락한 배정을 수락자의 일정표·칸반에 카드로 보인다. 부모 업무는 다른 조직에 있을 수 있어(95-215) 업무 자체가 아니라
// 배정 단위 가상 카드를 합성하고, 카드를 누르면 배정 범위만 보여 주는 화면(/workboard/assignments/:id)으로 간다.

const assignCardPrefix = "ASG:"

func isAssignCardID(id string) bool { return strings.HasPrefix(id, assignCardPrefix) }

func assignCardHref(assignmentID string) string {
	return "/workboard/assignments/" + url.PathEscape(assignmentID)
}

// assignCardTasks 일정표용 가상 업무. 부모 업무의 날짜·시간을 따르고 담당은 수락자다.
func (h *WorkboardHandler) assignCardTasks(from, to, orgID string) []model.WorkTask {
	if h == nil || h.assign == nil {
		return nil
	}
	var out []model.WorkTask
	for _, a := range h.assign.ListAcceptedCards(from, to, orgID) {
		out = append(out, model.WorkTask{
			TaskID: assignCardPrefix + a.AssignmentID, WorkType: model.WBWorkProjectWork,
			Title:    "[배정] " + a.Heading + " · " + a.TargetLabel,
			WorkDate: a.WorkDate, DueDate: a.DueDate, StartTime: a.StartTime, EndTime: a.EndTime, DurationMin: a.DurationMin,
			Status: a.Status, Assignee: a.ToName, AssigneeUserID: a.ToUserID, OrgID: a.ToOrgID,
			Description: "배정: " + a.FromName,
		})
	}
	return out
}

// assignKanbanItems 칸반용 가상 카드(읽기 전용). 부모 업무의 날짜가 기준 날짜에 걸리는 수락 배정이다.
func (h *WorkboardHandler) assignKanbanItems(date, orgID string) []model.WorkListItem {
	if h == nil || h.assign == nil {
		return nil
	}
	var out []model.WorkListItem
	for _, a := range h.assign.ListAcceptedCards(date, date, orgID) {
		out = append(out, model.WorkListItem{
			Prefix: model.WorkPrefixProjectWork, RefID: assignCardPrefix + a.AssignmentID, RefNumber: a.AssignmentID,
			Title: "[배정] " + a.Heading + " · " + a.TargetLabel, Assignee: a.ToName, OrgID: a.ToOrgID,
			ScheduledDate: a.WorkDate, WorkDate: a.WorkDate, DueDate: a.DueDate, Status: a.Status,
			Href: assignCardHref(a.AssignmentID), EditLocked: true,
		})
	}
	return out
}

var _ = repository.AssignAccepted
