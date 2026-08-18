package service

import (
	"fmt"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// SyncYear 관리자 버튼으로만 호출한다. 화면 렌더 시 API 를 부르지 않는다. §23.13.2
// 실패하면 테이블을 비우지 않고 error 만 반환한다. source='manual' 행은 건드리지 않는다.
func SyncYear(repo *repository.HolidayRepo, api HolidayAPI, year int) (int, error) {
	if repo == nil {
		return 0, fmt.Errorf("휴무일 저장소가 없습니다")
	}
	if api == nil {
		return 0, fmt.Errorf("HOLIDAY_API_KEY 가 없습니다")
	}
	if year < 2000 || year > 2100 {
		return 0, fmt.Errorf("연도가 올바르지 않습니다")
	}
	var all []model.Holiday
	for m := 1; m <= 12; m++ {
		items, err := api.FetchRestDe(year, m)
		if err != nil {
			return 0, err
		}
		all = append(all, items...)
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	return repo.ReplaceAPIYear(year, all, now)
}
