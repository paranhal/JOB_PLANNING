package model

import (
	"fmt"
	"strings"
)

const WorkKindSupport = "support"

const (
	SupportFactoryInbound = "factory_inbound"
	SupportFactoryReturn  = "factory_return"
	SupportLoanerSend     = "loaner_send"
	SupportCourier        = "courier"
	SupportOther          = "other"
)

const (
	FactoryWaitNone       = "none"
	FactoryWaitInProgress = "in_progress"
	FactoryWaitDays       = "days"
)

type ASSupportDetail struct {
	WorkID      string `json:"work_id"`
	SupportType string `json:"support_type"`
	VendorName  string `json:"vendor_name"`
	TrackingNo  string `json:"tracking_no"`
	ActionDate  string `json:"action_date"`
	DueDate     string `json:"due_date"`
	AssetID     string `json:"asset_id"`
	Memo        string `json:"memo"`
}

type ASSupportItem struct {
	Work   ASWorkItem
	Detail ASSupportDetail
}

type SupportStepper struct {
	Show  bool
	Stage int // 0 입고예정 · 1 입고완료(수리중) · 2 회수예정 · 3 회수완료
}

type FactoryWaitDisplay struct {
	Mode  string // none | in_progress | days
	Days  int
	Label string
}

type FactoryWaitCard struct {
	Count        int
	InProgress   int
	AvgDays      float64
	HasAvg       bool
	DisplayLabel string
}

type SupportTypeOption struct {
	Value string
	Label string
}

func SupportTypeOptions() []SupportTypeOption {
	return []SupportTypeOption{
		{SupportFactoryInbound, "제조사 입고"},
		{SupportFactoryReturn, "제조사 회수"},
		{SupportLoanerSend, "대체장비 발송"},
		{SupportCourier, "택배 발송·수령"},
		{SupportOther, "기타 지원"},
	}
}

func IsSupportType(s string) bool {
	switch strings.TrimSpace(s) {
	case SupportFactoryInbound, SupportFactoryReturn, SupportLoanerSend, SupportCourier, SupportOther:
		return true
	default:
		return false
	}
}

func SupportTypeLabel(s string) string {
	for _, o := range SupportTypeOptions() {
		if o.Value == s {
			return o.Label
		}
	}
	if s == "" {
		return "지원"
	}
	return s
}

func ValidateSupportDetail(d ASSupportDetail) error {
	if !IsSupportType(d.SupportType) {
		return fmt.Errorf("지원 유형이 올바르지 않습니다")
	}
	switch d.SupportType {
	case SupportFactoryInbound, SupportFactoryReturn:
		if strings.TrimSpace(d.VendorName) == "" {
			return fmt.Errorf("제조사를 입력하세요")
		}
		if strings.TrimSpace(d.DueDate) == "" {
			return fmt.Errorf("예정일을 입력하세요")
		}
	case SupportCourier:
		if strings.TrimSpace(d.TrackingNo) == "" {
			return fmt.Errorf("송장번호를 입력하세요")
		}
	case SupportLoanerSend:
		if strings.TrimSpace(d.AssetID) == "" {
			return fmt.Errorf("대상 장비를 선택하세요")
		}
	case SupportOther:
		if strings.TrimSpace(d.Memo) == "" {
			return fmt.Errorf("기타 지원은 메모가 필요합니다")
		}
	}
	return nil
}

func supportComplete(status, actionDate string) bool {
	return status == "done" || strings.TrimSpace(actionDate) != ""
}

func DeriveSupportStepper(items []ASSupportItem) SupportStepper {
	var inbound, ret *ASSupportItem
	for i := range items {
		switch items[i].Detail.SupportType {
		case SupportFactoryInbound:
			inbound = &items[i]
		case SupportFactoryReturn:
			ret = &items[i]
		}
	}
	if inbound == nil || ret == nil {
		return SupportStepper{}
	}
	inDone := supportComplete(inbound.Work.Status, inbound.Detail.ActionDate)
	retDone := supportComplete(ret.Work.Status, ret.Detail.ActionDate)
	st := SupportStepper{Show: true, Stage: 0}
	if !inDone {
		return st
	}
	if retDone {
		st.Stage = 3
		return st
	}
	if strings.TrimSpace(ret.Detail.DueDate) != "" || strings.TrimSpace(ret.Work.ScheduledDate) != "" {
		st.Stage = 2
		return st
	}
	st.Stage = 1
	return st
}

func FactoryWaitFromDates(inboundAction, returnAction, today string, daysFn func(from, to string) (int, bool)) FactoryWaitDisplay {
	in := strings.TrimSpace(inboundAction)
	if in == "" {
		return FactoryWaitDisplay{Mode: FactoryWaitNone, Label: "—"}
	}
	end := strings.TrimSpace(returnAction)
	if end == "" {
		end = strings.TrimSpace(today)
		if end == "" {
			return FactoryWaitDisplay{Mode: FactoryWaitInProgress, Label: "진행 중"}
		}
		if daysFn == nil {
			return FactoryWaitDisplay{Mode: FactoryWaitInProgress, Label: "진행 중"}
		}
		d, ok := daysFn(in, end)
		if !ok {
			return FactoryWaitDisplay{Mode: FactoryWaitInProgress, Label: "진행 중"}
		}
		return FactoryWaitDisplay{Mode: FactoryWaitInProgress, Days: d, Label: "진행 중"}
	}
	if daysFn == nil {
		return FactoryWaitDisplay{Mode: FactoryWaitNone, Label: "—"}
	}
	d, ok := daysFn(in, end)
	if !ok {
		return FactoryWaitDisplay{Mode: FactoryWaitNone, Label: "—"}
	}
	return FactoryWaitDisplay{Mode: FactoryWaitDays, Days: d, Label: fmt.Sprintf("%d영업일", d)}
}

func OpenSupportWarnMessage(n int) string {
	return fmt.Sprintf("미완료 지원업무 %d건이 있습니다. 그대로 완료하시겠습니까?", n)
}
