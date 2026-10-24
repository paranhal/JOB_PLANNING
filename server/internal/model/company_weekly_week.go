package model

import "time"

// WeekInfo 월~일 한 주의 표시 월·주차. 목요일이 속한 달, 그 달의 N번째 목요일.
type WeekInfo struct {
	Monday time.Time
	Sunday time.Time
	Year   int
	Month  int
	WeekN  int
}

func (w WeekInfo) YYMM() string {
	return w.Monday.Format("0601")
}
