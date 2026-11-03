package appclock

import "time"

var nowFn = time.Now

// Now 앱이 쓰는 현재시각. 테스트는 Set 으로 고정한다.
func Now() time.Time { return nowFn() }

// Set 현재시각 함수를 바꾼다. 복원 함수를 돌려준다.
func Set(fn func() time.Time) (restore func()) {
	if fn == nil {
		fn = time.Now
	}
	prev := nowFn
	nowFn = fn
	return func() { nowFn = prev }
}

// SetFixed Now 를 t 로 고정한다.
func SetFixed(t time.Time) func() {
	return Set(func() time.Time { return t })
}
