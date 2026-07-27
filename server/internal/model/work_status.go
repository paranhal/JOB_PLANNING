package model

// WorkCalItem 캘린더 칸에 표시하는 업무 1건
type WorkCalItem struct {
	Category string // as | maintenance | other
	ID       string
	Title    string // 접수번호·기관명 등
	Subtitle string
	Link     string
	Date     string // YYYY-MM-DD
}

// WorkCalDay 달력 1칸
type WorkCalDay struct {
	Day     int    // 0=빈칸
	Date    string // YYYY-MM-DD
	Count   int
	Items   []WorkCalItem
	IsToday bool
	InMonth bool
}

// WorkCalSummary 상단 (접수건수/완료건수/전월이관건수)
type WorkCalSummary struct {
	ReceiptCount      int
	CompleteCount     int
	PrevTransferCount int
}
