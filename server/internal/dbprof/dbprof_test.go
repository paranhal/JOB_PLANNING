package dbprof

import "testing"

func TestMntProfileEnabledOnlyOnOne(t *testing.T) {
	t.Setenv(EnvKey, "")
	if Enabled() {
		t.Fatal("기본은 꺼져 있어야 한다")
	}
	t.Setenv(EnvKey, "0")
	if Enabled() {
		t.Fatal("0 은 꺼짐")
	}
	t.Setenv(EnvKey, "1")
	if !Enabled() {
		t.Fatal("1 은 켜짐")
	}
}
