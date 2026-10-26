package model

// ActionDurationMins 작업 조치 소요시간 선택값.
// 95-47 정기점검 칩(30·60·90·120·180·240)에 지시서 480분을 더한 목록이다.
var ActionDurationMins = []int{30, 60, 90, 120, 180, 240, 480}

func IsActionDurationOption(min int) bool {
	for _, v := range ActionDurationMins {
		if v == min {
			return true
		}
	}
	return false
}
