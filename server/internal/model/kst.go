package model

import (
	"sync"
	"time"
)

var (
	kstOnce sync.Once
	kstLoc  *time.Location
)

// KST 앱이 시각 문자열을 해석하는 기준 시간대(Asia/Seoul).
// tzdata가 없는 환경에서는 UTC+9 고정으로 대신한다. 95-133.
// 서버 프로세스의 TZ(time.Local)와 무관하게 같은 값을 돌려준다.
func KST() *time.Location {
	kstOnce.Do(func() {
		loc, err := time.LoadLocation("Asia/Seoul")
		if err != nil {
			loc = time.FixedZone("KST", 9*3600)
		}
		kstLoc = loc
	})
	return kstLoc
}
