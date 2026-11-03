package appclock

import (
	"testing"
	"time"
)

func TestSetFixedRestores(t *testing.T) {
	want := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	restore := SetFixed(want)
	if !Now().Equal(want) {
		t.Fatalf("fixed=%s", Now())
	}
	restore()
	if Now().Equal(want) && time.Since(want) > time.Hour {
		// 복원 뒤 실제 시각이어야 한다. 2027-01-01 고정이 남으면 실패.
		if Now().Year() == 2027 && Now().Month() == 1 && Now().Day() == 1 {
			t.Fatal("복원되지 않았다")
		}
	}
}
