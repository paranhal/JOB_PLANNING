package repository

import "customer-support/internal/model"

// holidaySeedRows kr_holidays.go 하드코딩을 최초 1회 테이블로 옮긴 목록. §23.13.2 임시 시드.
// 2026은 §23.13.9 정정 20건. 2027은 추석이 빠져 있어 손대지 않는다.
var holidaySeedRows = []model.Holiday{
	{Date: "2025-01-01", Name: "신정", Kind: model.HolidayKindPublic},
	{Date: "2025-01-28", Name: "설날 연휴", Kind: model.HolidayKindPublic},
	{Date: "2025-01-29", Name: "설날", Kind: model.HolidayKindPublic},
	{Date: "2025-01-30", Name: "설날 연휴", Kind: model.HolidayKindPublic},
	{Date: "2025-03-01", Name: "삼일절", Kind: model.HolidayKindPublic},
	{Date: "2025-03-03", Name: "삼일절 대체공휴일", Kind: model.HolidayKindSubstitute},
	{Date: "2025-05-05", Name: "어린이날", Kind: model.HolidayKindPublic},
	{Date: "2025-06-06", Name: "현충일", Kind: model.HolidayKindPublic},
	{Date: "2025-08-15", Name: "광복절", Kind: model.HolidayKindPublic},
	{Date: "2025-10-03", Name: "개천절", Kind: model.HolidayKindPublic},
	{Date: "2025-10-05", Name: "추석 연휴", Kind: model.HolidayKindPublic},
	{Date: "2025-10-06", Name: "추석", Kind: model.HolidayKindPublic},
	{Date: "2025-10-07", Name: "추석 연휴", Kind: model.HolidayKindPublic},
	{Date: "2025-10-08", Name: "추석 대체공휴일", Kind: model.HolidayKindSubstitute},
	{Date: "2025-12-25", Name: "성탄절", Kind: model.HolidayKindPublic},

	{Date: "2026-01-01", Name: "신정", Kind: model.HolidayKindPublic},
	{Date: "2026-02-16", Name: "설날 연휴", Kind: model.HolidayKindPublic},
	{Date: "2026-02-17", Name: "설날", Kind: model.HolidayKindPublic},
	{Date: "2026-02-18", Name: "설날 연휴", Kind: model.HolidayKindPublic},
	{Date: "2026-03-01", Name: "삼일절", Kind: model.HolidayKindPublic},
	{Date: "2026-03-02", Name: "대체공휴일(삼일절)", Kind: model.HolidayKindSubstitute},
	{Date: "2026-05-05", Name: "어린이날", Kind: model.HolidayKindPublic},
	{Date: "2026-05-24", Name: "부처님오신날", Kind: model.HolidayKindPublic},
	{Date: "2026-05-25", Name: "대체공휴일(부처님오신날)", Kind: model.HolidayKindSubstitute},
	{Date: "2026-06-03", Name: "제9회 전국동시지방선거", Kind: model.HolidayKindPublic},
	{Date: "2026-06-06", Name: "현충일", Kind: model.HolidayKindPublic},
	{Date: "2026-08-15", Name: "광복절", Kind: model.HolidayKindPublic},
	{Date: "2026-08-17", Name: "대체공휴일(광복절)", Kind: model.HolidayKindSubstitute},
	{Date: "2026-09-24", Name: "추석 연휴", Kind: model.HolidayKindPublic},
	{Date: "2026-09-25", Name: "추석", Kind: model.HolidayKindPublic},
	{Date: "2026-09-26", Name: "추석 연휴", Kind: model.HolidayKindPublic},
	{Date: "2026-10-03", Name: "개천절", Kind: model.HolidayKindPublic},
	{Date: "2026-10-05", Name: "대체공휴일(개천절)", Kind: model.HolidayKindSubstitute},
	{Date: "2026-10-09", Name: "한글날", Kind: model.HolidayKindPublic},
	{Date: "2026-12-25", Name: "기독탄신일", Kind: model.HolidayKindPublic},

	// 2027: 추석(9/14~9/16)이 빠져 있어 신뢰할 수 없다. API 동기화로 받는다. 지금은 손대지 않는다.
	{Date: "2027-01-01", Name: "신정", Kind: model.HolidayKindPublic},
	{Date: "2027-02-07", Name: "설날 연휴", Kind: model.HolidayKindPublic},
	{Date: "2027-02-08", Name: "설날", Kind: model.HolidayKindPublic},
	{Date: "2027-02-09", Name: "설날 연휴", Kind: model.HolidayKindPublic},
	{Date: "2027-03-01", Name: "삼일절", Kind: model.HolidayKindPublic},
	{Date: "2027-05-05", Name: "어린이날", Kind: model.HolidayKindPublic},
	{Date: "2027-06-06", Name: "현충일", Kind: model.HolidayKindPublic},
	{Date: "2027-08-16", Name: "광복절 대체공휴일", Kind: model.HolidayKindSubstitute},
	{Date: "2027-08-17", Name: "임시공휴일", Kind: model.HolidayKindPublic},
	{Date: "2027-10-03", Name: "개천절", Kind: model.HolidayKindPublic},
	{Date: "2027-10-04", Name: "개천절 대체공휴일", Kind: model.HolidayKindSubstitute},
	{Date: "2027-10-05", Name: "공휴일", Kind: model.HolidayKindPublic},
	{Date: "2027-10-06", Name: "공휴일", Kind: model.HolidayKindPublic},
	{Date: "2027-10-11", Name: "한글날 대체공휴일", Kind: model.HolidayKindSubstitute},
	{Date: "2027-12-25", Name: "성탄절", Kind: model.HolidayKindPublic},
	{Date: "2027-12-27", Name: "성탄절 대체공휴일", Kind: model.HolidayKindSubstitute},
}

// wrong2026CopiedChuseok 2025년 추석을 2026년에 복사해 넣은 날짜. 공휴일이 아니다. §23.13.1
var wrong2026CopiedChuseok = []string{"2026-10-06", "2026-10-07", "2026-10-08"}

func HolidaySeedItems() []model.Holiday {
	out := make([]model.Holiday, len(holidaySeedRows))
	copy(out, holidaySeedRows)
	for i := range out {
		out[i].Source = model.HolidaySourceAPI
	}
	return out
}
