package leaveapp

import "customer-support/internal/service/leavecalc"

type Input struct {
	TypeKey, TypeNote, PeriodMode          string
	StartDate, EndDate, StartTime, EndTime string
	HalfPart                               string
	SwapDate, SwapState                    string
	Reason, Destination, Phone             string
	DeputyUserID, ApproverDeputyUserID     string
}

type Request struct {
	LeaveReqID, OrgID, UserID, DocNo string
	TypeKey, TypeNote, PeriodMode    string
	StartDate, EndDate               string
	StartTime, EndTime, HalfPart     string
	WorkMinutes                      int
	SwapDate, SwapState              string
	Reason, Destination, Phone       string
	DeputyUserID, DeputyName         string
	ApproverDeputyUserID             string
	ApprovalRequestID, Status        string
	CreatedAt, AuthorName            string
}

type Snapshot struct {
	LeaveReqID, DocNo, WrittenOn, Author string
	TypeKey, TypeNote, PeriodMode        string
	HourYear, HourMonth, HourDay         string
	HourStart, HourEnd, HourCount        string
	DayStart, DayEnd, DayCount           string
	SwapDate                             string
	SwapWorked, SwapScheduled            bool
	Reason, Destination, Phone, Deputy   string
	AuthorStampDate                      string
	StartDate, EndDate, HalfPart         string
}

func typeLabel(key string) string {
	t, ok := leavecalc.TypeByKey(key)
	if !ok {
		return key
	}
	return t.Label
}
