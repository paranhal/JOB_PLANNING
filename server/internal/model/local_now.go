package model

import (
	"sync"
	"time"
)

const LocalNowLayout = "2006-01-02 15:04:05"

var (
	nowMu sync.Mutex
	nowFn = time.Now
)

// SetNowForTest 저장 시각 테스트용. 반환 함수로 되돌린다.
func SetNowForTest(fn func() time.Time) func() {
	nowMu.Lock()
	prev := nowFn
	if fn == nil {
		nowFn = time.Now
	} else {
		nowFn = fn
	}
	nowMu.Unlock()
	return func() {
		nowMu.Lock()
		nowFn = prev
		nowMu.Unlock()
	}
}

func nowTime() time.Time {
	nowMu.Lock()
	fn := nowFn
	nowMu.Unlock()
	if fn == nil {
		return time.Now()
	}
	return fn()
}

// LocalNowString 한국 시각(Asia/Seoul)의 지금. SQLite CURRENT_TIMESTAMP(UTC) 대신 INSERT에 넣는다.
// 서버 프로세스의 TZ 설정과 무관하게 한국 시각이다. 95-133.
func LocalNowString() string {
	return nowTime().In(KST()).Format(LocalNowLayout)
}
