package repository

import "sync/atomic"

// holidayDataEpoch holidays 테이블이 바뀌면 증가한다. 연도 캐시가 이 값을 본다. §23.13.7
var holidayDataEpoch atomic.Int64

func HolidayDataEpoch() int64 {
	return holidayDataEpoch.Load()
}

func BumpHolidayData() {
	holidayDataEpoch.Add(1)
}
