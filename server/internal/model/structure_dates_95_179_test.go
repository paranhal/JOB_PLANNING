package model

import "testing"

func TestEndChangeKind(t *testing.T) {
	for _, c := range []struct{ old, new, want string }{
		{"", "", EndChangeNone},
		{"2026-12-01", "2026-12-01", EndChangeNone},
		{"", "2026-12-01", EndChangeSet},
		{"2026-12-01", "", EndChangeClear},
		{"2026-12-01", "2026-12-15", EndChangeExtend},
		{"2026-12-01", "2026-11-20", EndChangeShorten},
		{" 2026-12-01 ", "2026-12-01", EndChangeNone},
	} {
		if got := EndChangeKind(c.old, c.new); got != c.want {
			t.Fatalf("%q→%q = %q (기대 %q)", c.old, c.new, got, c.want)
		}
	}
}

func TestBadgeOK(t *testing.T) {
	for _, in := range []string{"", "  ", "사전GB", "사전FW", "12345678", "가나다라마바사아"} {
		if _, err := BadgeOK(in); err != nil {
			t.Fatalf("%q 는 허용: %v", in, err)
		}
	}
	if got, _ := BadgeOK("  사전GB "); got != "사전GB" {
		t.Fatalf("다듬기 %q", got)
	}
	for _, in := range []string{"123456789", "가나다라마바사아자", "a<b", "a\nb"} {
		if _, err := BadgeOK(in); err == nil {
			t.Fatalf("%q 는 거절", in)
		}
	}
}

// 배지 색: 같은 이름은 항상 같은 색, 고정 팔레트 안에서만 고른다(badge-danger 제외).
func TestBadgeTone(t *testing.T) {
	allowed := map[string]bool{"badge-info": true, "badge-success": true, "badge-warning": true, "badge-neutral": true}
	seen := map[string]bool{}
	for _, l := range []string{"사전GB", "사전FW", "사전L2", "사전AP", "설치", "검수", "환경", "ABC"} {
		tone := BadgeTone(l)
		if !allowed[tone] {
			t.Fatalf("%s → %s", l, tone)
		}
		if BadgeTone(l) != tone || BadgeTone(" "+l+" ") != tone {
			t.Fatalf("%s 색이 일정하지 않다", l)
		}
		seen[tone] = true
	}
	if len(seen) < 3 {
		t.Fatalf("색이 거의 한 가지로 몰린다: %v", seen)
	}
	if BadgeTone("") != "badge-neutral" {
		t.Fatal("빈 이름")
	}
}

func TestExtendReasonOK(t *testing.T) {
	if _, err := ExtendReasonOK(string(make([]rune, 201))); err == nil {
		t.Fatal("201자 거절")
	}
	long := ""
	for i := 0; i < 200; i++ {
		long += "가"
	}
	if got, err := ExtendReasonOK(" " + long + " "); err != nil || got != long {
		t.Fatalf("200자 허용 %v", err)
	}
}
