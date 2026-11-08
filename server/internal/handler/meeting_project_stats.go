package handler

import (
	"fmt"
	"strings"

	"customer-support/internal/model"
)

type meetingStatItem struct {
	model.WorkListItem
	Date string
	Kind string
	Site string
	Done bool
}

type meetingStatBox struct {
	Key   string
	Label string
	Line  string
	Done  int
	Total int
	Rate  string
	Items []meetingStatItem
}

var meetingStatBoxMeta = []struct {
	Key, Label string
}{
	{"mnt", "정기점검 방문"},
	{"as", "AS 방문/완료"},
	{"work", "작업 방문"},
	{"admin", "행정/지원 업무"},
	{"sales", "영업"},
}

func meetingStatBoxKey(prefix string) string {
	switch strings.TrimSpace(prefix) {
	case model.WorkPrefixMaintenance:
		return "mnt"
	case model.WorkPrefixAS:
		return "as"
	case model.WorkPrefixProjectWork:
		return "work"
	case model.WorkPrefixSales:
		return "sales"
	default:
		return "admin"
	}
}

func workListItemDone(it model.WorkListItem) bool {
	st := strings.TrimSpace(it.MappedStatus)
	if st == "" {
		st = model.MapASStatusToWB(it.Status)
	}
	return st == model.WBTaskComplete
}

func meetingStatLine(key string, done, total int) string {
	switch key {
	case "mnt":
		return fmt.Sprintf("완료 %d / 총 %d건", done, total)
	case "as":
		return fmt.Sprintf("방문 %d · 완료 %d", total, done)
	default:
		return fmt.Sprintf("%d건 · 완료 %d", total, done)
	}
}

func buildMeetingProjectStatBoxes(items []model.WorkListItem) []meetingStatBox {
	out := make([]meetingStatBox, len(meetingStatBoxMeta))
	idx := map[string]int{}
	for i, m := range meetingStatBoxMeta {
		out[i] = meetingStatBox{Key: m.Key, Label: m.Label, Rate: model.SiteCheckRateLabel(0, 0)}
		idx[m.Key] = i
	}
	for _, it := range items {
		k := meetingStatBoxKey(it.Prefix)
		i := idx[k]
		done := workListItemDone(it)
		site := strings.TrimSpace(it.OrgName)
		if site == "" {
			site = strings.TrimSpace(it.Title)
		}
		out[i].Items = append(out[i].Items, meetingStatItem{
			WorkListItem: it,
			Date:         model.WorkItemScheduledDate(it),
			Kind:         model.WorkPrefixLabel(it.Prefix),
			Site:         site,
			Done:         done,
		})
		out[i].Total++
		if done {
			out[i].Done++
		}
	}
	for i := range out {
		out[i].Rate = model.SiteCheckRateLabel(out[i].Done, out[i].Total)
		out[i].Line = meetingStatLine(out[i].Key, out[i].Done, out[i].Total)
	}
	return out
}

func attachMeetingProjectStatBoxes(groups []meetingProjectGroup) {
	for i := range groups {
		groups[i].StatBoxes = buildMeetingProjectStatBoxes(groups[i].Items)
	}
}

func meetingKindSummary(items []model.WorkListItem) string {
	var mnt, as, work, admin, sales int
	for _, it := range items {
		switch meetingStatBoxKey(it.Prefix) {
		case "mnt":
			mnt++
		case "as":
			as++
		case "work":
			work++
		case "sales":
			sales++
		default:
			admin++
		}
	}
	return fmt.Sprintf("정기점검 %d · AS %d · 작업 %d · 행정/지원 %d · 영업 %d", mnt, as, work, admin, sales)
}
