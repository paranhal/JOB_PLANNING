package model

import (
	"os"
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

// NowKST 한국 시각의 지금. 「오늘」을 정하는 곳은 time.Now().In(time.Local) 대신 이것을 쓴다.
// SetNowForTest 로 고정할 수 있다. 95-133 후속(time.Local 정리).
func NowKST() time.Time { return nowTime().In(KST()) }

// PinKST 프로세스 시간대를 한국으로 고정한다. 서버가 시작하자마자(DB·시각을 쓰기 전에) 한 번 부른다.
// time.Local 을 바꾸고 TZ 환경변수도 맞춘다. SQLite 의 'localtime' 은 TZ 를 처음 쓸 때 읽으므로
// DB 를 열기 전에 불러야 한다. 배포(Docker TZ=Asia/Seoul)와 같지만 TZ 설정이 빠진 환경에서도 같게 동작한다.
func PinKST() {
	_ = os.Setenv("TZ", "Asia/Seoul")
	time.Local = KST()
}
