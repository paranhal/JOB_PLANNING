package audit

import (
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// Actor 변경을 수행한 사용자.
type Actor struct {
	UserID   string
	Username string
	Name     string
}

var actors sync.Map // goroutine id → Actor

// Push 현재 고루틴의 행위자를 기록한다. 반환 함수로 해제.
func Push(a Actor) func() {
	id := goroutineID()
	actors.Store(id, a)
	return func() { actors.Delete(id) }
}

// Current 현재 고루틴의 행위자. 없으면 빈 값.
func Current() Actor {
	v, ok := actors.Load(goroutineID())
	if !ok {
		return Actor{}
	}
	a, _ := v.(Actor)
	return a
}

func goroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	s := strings.TrimPrefix(string(buf[:n]), "goroutine ")
	i := strings.IndexByte(s, ' ')
	if i <= 0 {
		return 0
	}
	id, _ := strconv.ParseUint(s[:i], 10, 64)
	return id
}
