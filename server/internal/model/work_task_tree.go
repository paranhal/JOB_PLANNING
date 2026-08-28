package model

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// MaxWorkTaskDepth 상위 → 하위 → 하위의 하위 (§33.3.2).
const MaxWorkTaskDepth = 3

var (
	ErrSubtaskDepth  = errors.New("하위 업무는 3단계까지만 만들 수 있습니다")
	ErrSubtaskCycle  = errors.New("자기 자신이나 하위 업무는 상위가 될 수 없습니다")
	ErrSubtaskParent = errors.New("상위 업무를 확인할 수 없습니다")
	ErrSubtaskOccur  = errors.New("실행 작업에는 하위 업무를 달 수 없습니다")
)

// ErrHasSubtasks 하위가 있는 상위 삭제 거부 (§33.3.2).
type ErrHasSubtasks struct {
	N int
}

func (e ErrHasSubtasks) Error() string {
	return fmt.Sprintf("하위 업무 %d건을 먼저 처리하세요", e.N)
}

func HasSubtasksN(err error) (int, bool) {
	var e ErrHasSubtasks
	if errors.As(err, &e) {
		return e.N, true
	}
	return 0, false
}

// NextChildSeq 같은 부모의 형제 ID에서 다음 순번. 빈 자리(삭제분)는 재사용하지 않는다.
func NextChildSeq(parentID string, siblingIDs []string) int {
	parentID = strings.TrimSpace(parentID)
	max := 0
	for _, id := range siblingIDs {
		if n := ChildSeqOf(parentID, id); n > max {
			max = n
		}
	}
	return max + 1
}

func ChildTaskID(parentID string, seq int) string {
	return strings.TrimSpace(parentID) + "-" + strconv.Itoa(seq)
}

var childSeqSuffix = regexp.MustCompile(`-(\d+)$`)

// ChildSeqOf parent의 직계 자식 ID(WT-113-2)에서 순번. 손자(WT-113-2-1)는 0.
func ChildSeqOf(parentID, childID string) int {
	parentID = strings.TrimSpace(parentID)
	childID = strings.TrimSpace(childID)
	if parentID == "" || childID == "" {
		return 0
	}
	prefix := parentID + "-"
	if !strings.HasPrefix(childID, prefix) {
		return 0
	}
	rest := childID[len(prefix):]
	if rest == "" || strings.Contains(rest, "-") {
		return 0
	}
	n, err := strconv.Atoi(rest)
	if err != nil || n <= 0 {
		return 0
	}
	if !childSeqSuffix.MatchString(childID) {
		return 0
	}
	return n
}

func IsSubtaskRole(role string) bool {
	return strings.TrimSpace(role) == ""
}

func CanAddSubtaskUnder(parent WorkTask, parentDepth int) error {
	if strings.TrimSpace(parent.TaskID) == "" {
		return ErrSubtaskParent
	}
	if parent.RecurrenceRole == RecurrenceRoleOccurrence {
		return ErrSubtaskOccur
	}
	if parentDepth < 1 {
		parentDepth = 1
	}
	if parentDepth >= MaxWorkTaskDepth {
		return ErrSubtaskDepth
	}
	return nil
}
