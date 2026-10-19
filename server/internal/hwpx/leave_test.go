package hwpx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readOfficialLeaveForm(t *testing.T) []byte {
	t.Helper()
	path, err := OfficialLeaveFormPath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func buildLeaveTpl(t *testing.T) []byte {
	t.Helper()
	tpl, err := BuildLeavePermitTemplate(readOfficialLeaveForm(t))
	if err != nil {
		t.Fatal(err)
	}
	return tpl
}

func leaveSigBox() SignatureBox {
	return SignatureBox{Size: 4600, RightGap: 900, NudgeY: 0}
}

func TestLeavePlaceholdersStayInOneRun(t *testing.T) {
	tpl := buildLeaveTpl(t)
	sec := string(mustZipFile(t, tpl, "Contents/section0.xml"))
	for _, key := range LeavePermitKeys {
		if !placeholderInSingleT(sec, key) {
			t.Fatalf("%s 가 한 <hp:t> 안에 없다", key)
		}
	}
	split := `<hp:t>{{문</hp:t></hp:run><hp:run><hp:t>서번호}}</hp:t>`
	got := normalizePlaceholders(split)
	if !strings.Contains(got, "{{문서번호}}") {
		t.Fatalf("쪼갠 자리표시자를 못 모음: %s", got)
	}
	if !placeholderInSingleT(got, "문서번호") {
		t.Fatalf("모은 뒤에도 한 <hp:t>가 아니다: %s", got)
	}
}

func TestLeaveTemplateStripsSamplePIIAndPreview(t *testing.T) {
	orig := readOfficialLeaveForm(t)
	needles, err := officialLeaveSampleStrings(orig)
	if err != nil {
		t.Fatal(err)
	}
	tpl, err := BuildLeavePermitTemplate(orig)
	if err != nil {
		t.Fatal(err)
	}
	if hit, ok := zipContainsAny(tpl, needles); ok {
		t.Fatalf("작업 템플릿에 샘플 문자열이 남았다: %s", hit)
	}
	prv := mustZipFile(t, tpl, "Preview/PrvText.txt")
	if !strings.Contains(decodeUTF16LE(prv), "{{문서번호}}") {
		t.Fatal("미리보기 텍스트에 자리표시자가 없다")
	}
	img := mustZipFile(t, tpl, "Preview/PrvImage.png")
	origImg := mustZipFile(t, orig, "Preview/PrvImage.png")
	if len(img) == 0 || len(img) >= len(origImg) {
		t.Fatalf("미리보기 그림을 비우지 않았다 orig=%d new=%d", len(origImg), len(img))
	}
	if _, err := Replace(tpl, map[string]string{"문서번호": "x"}); err == nil {
		t.Fatal("키를 덜 넣었는데 {{ 잔여를 막지 않았다")
	}
}

func TestLeaveFillGoldenPeriodModesAndStamps(t *testing.T) {
	tpl := buildLeaveTpl(t)
	png := inkPNG(t)
	origNeedles, err := officialLeaveSampleStrings(readOfficialLeaveForm(t))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		in     LeavePermitInput
		stamps map[string][]byte
		want   []string
		not    []string
		pics   int
	}{
		{
			name: "연차_일단위",
			in: LeavePermitInput{
				DocNo: "VI-근태-260101-001", WrittenOn: "2026년 01월 01일", Author: "신청자",
				TypeKey: "annual", PeriodMode: LeavePeriodDays,
				DayStart: "2026년 01월 02일", DayEnd: "01월 02일", DayCount: "1",
				Reason: "연차 사용", Deputy: "",
				AuthorStampDate: "01/01", ApproveStampDate: "01/01",
			},
			stamps: map[string][]byte{"작성사인": png, "승인사인": png},
			want:   []string{"VI-근태-260101-001", "☑연차", "☐반차", "☑ 2026년 01월 02일", "01월 02일", "(1 일간)", "연차 사용", "01/01"},
			not:    []string{"(대결)", "☑반차", "☑대체"},
			pics:   2,
		},
		{
			name: "반차_시간",
			in: LeavePermitInput{
				DocNo: "VI-근태-260102-001", WrittenOn: "2026년 01월 02일", Author: "신청자",
				TypeKey: "half", PeriodMode: LeavePeriodHours,
				HourYear: "2026", HourMonth: "01", HourDay: "02",
				HourStart: "09:00", HourEnd: "13:00", HourCount: "4",
				Reason: "반차", AuthorStampDate: "01/02", ApproveStampDate: "01/02",
				ApproveMark: "(대결)",
			},
			stamps: map[string][]byte{"작성사인": png, "승인사인": png},
			want:   []string{"☑반차", "☐연차", "☑ 2026년 01월 02일", "09:00시 ~ 13:00시", "(4시간)", "(대결)"},
			not:    []string{"☑연차", "☑일"},
			pics:   2,
		},
		{
			name: "대체근무",
			in: LeavePermitInput{
				DocNo: "VI-근태-260103-001", WrittenOn: "2026년 01월 03일", Author: "신청자",
				TypeKey: "vacation", PeriodMode: LeavePeriodSwap,
				SwapDate: "2026년 01월 04일", SwapWorked: true,
				Reason: "대체근무", AuthorStampDate: "01/03", ApproveStampDate: "01/03",
			},
			stamps: map[string][]byte{"작성사인": png, "승인사인": png},
			want:   []string{"☑휴가", "☑ 대체근무", "2026년 01월 04일", "☑旣근무", "☐근무예정"},
			not:    []string{"(대결)", "☑기근무"},
			pics:   2,
		},
		{
			name: "자기결재",
			in: LeavePermitInput{
				DocNo: "VI-근태-260104-001", WrittenOn: "2026년 01월 04일", Author: "신청자",
				TypeKey: "annual", PeriodMode: LeavePeriodDays,
				DayStart: "2026년 01월 05일", DayEnd: "01월 05일", DayCount: "1",
				Reason: "자기 결재", AuthorStampDate: "01/04", ApproveStampDate: "01/04",
			},
			stamps: map[string][]byte{"작성사인": png, "승인사인": png},
			want:   []string{"☑연차", "자기 결재", "binaryItemIDRef=\"sig1\"", "binaryItemIDRef=\"sig2\""},
			not:    []string{"(대결)"},
			pics:   2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vals := LeavePermitValues(tc.in)
			if vals["검토사인"] != "" || vals["검토결재일"] != "" {
				t.Fatal("검토란은 항상 공백이어야 한다")
			}
			if vals["업무대행자"] != tc.in.Deputy {
				t.Fatal("업무대행자 공백이 유지되지 않는다")
			}
			out, err := FillLeavePermit(tpl, vals, tc.stamps, nil, leaveSigBox())
			if err != nil {
				t.Fatal(err)
			}
			sec := string(mustZipFile(t, out, "Contents/section0.xml"))
			assertXMLWellFormed(t, []byte(sec))
			if strings.Contains(sec, "{{") {
				t.Fatal("자리표시자가 남았다")
			}
			if hit, ok := zipContainsAny(out, origNeedles); ok {
				t.Fatalf("출력에 샘플 문자열이 있다: %s", hit)
			}
			if n := strings.Count(sec, `binaryItemIDRef="sig`); n != tc.pics {
				t.Fatalf("사인 그림 수=%d want %d", n, tc.pics)
			}
			if strings.Contains(sec, "{{검토사인}}") || strings.Contains(sec, "binaryItemIDRef=\"sig3\"") {
				t.Fatal("검토란에 서명이 들어갔다")
			}
			for _, w := range tc.want {
				if !strings.Contains(sec, w) {
					t.Fatalf("없음 %q", w)
				}
			}
			for _, n := range tc.not {
				if strings.Contains(sec, n) {
					t.Fatalf("있으면 안 됨 %q", n)
				}
			}
		})
	}
}

func TestLeaveFillNameFallbackPendingAndCorrupt(t *testing.T) {
	tpl := buildLeaveTpl(t)
	base := LeavePermitInput{
		DocNo: "VI-근태-260107-001", WrittenOn: "2026년 01월 07일", Author: "신청자",
		TypeKey: "annual", PeriodMode: LeavePeriodDays,
		DayStart: "2026년 01월 08일", DayEnd: "01월 08일", DayCount: "1",
		Reason: "이름 대체", AuthorStampDate: "01/07",
	}
	vals := LeavePermitValues(base)
	pending, err := FillLeavePermit(tpl, vals, nil, map[string]string{"작성사인": "신청자"}, leaveSigBox())
	if err != nil {
		t.Fatal(err)
	}
	sec := string(mustZipFile(t, pending, "Contents/section0.xml"))
	if strings.Contains(sec, "{{") {
		t.Fatal("자리표시자가 남았다")
	}
	if !strings.Contains(sec, "신청자") {
		t.Fatal("작성란 이름이 없다")
	}
	if strings.Contains(sec, `binaryItemIDRef="sig`) {
		t.Fatal("결재 중인데 그림이 들어갔다")
	}
	if vals["검토사인"] != "" || strings.Contains(sec, "binaryItemIDRef=\"sig3\"") {
		t.Fatal("검토란")
	}

	out, err := FillLeavePermit(tpl, LeavePermitValues(LeavePermitInput{
		DocNo: "VI-근태-260108-001", WrittenOn: "2026년 01월 08일", Author: "신청자",
		TypeKey: "annual", PeriodMode: LeavePeriodDays,
		DayStart: "2026년 01월 09일", DayEnd: "01월 09일", DayCount: "1",
		Reason: "손상", AuthorStampDate: "01/08", ApproveStampDate: "01/08",
		ApproveMark: "(대결)",
	}), map[string][]byte{"작성사인": []byte("not-png"), "승인사인": []byte("not-png")},
		map[string]string{"작성사인": "신청자", "승인사인": "대리인"}, leaveSigBox())
	if err != nil {
		t.Fatal(err)
	}
	sec = string(mustZipFile(t, out, "Contents/section0.xml"))
	if strings.Contains(sec, "{{") || strings.Contains(sec, `binaryItemIDRef="sig`) {
		t.Fatal("손상 서명은 이름이어야 한다")
	}
	if !strings.Contains(sec, "신청자") || !strings.Contains(sec, "대리인") || !strings.Contains(sec, "(대결)") {
		t.Fatal("이름·대결 표기")
	}
	long, err := FillLeavePermit(tpl, LeavePermitValues(base), nil, map[string]string{
		"작성사인": "Alexandria", "승인사인": "남궁민수",
	}, leaveSigBox())
	if err != nil {
		t.Fatal(err)
	}
	sec = string(mustZipFile(t, long, "Contents/section0.xml"))
	if !strings.Contains(sec, "Alexandria") || !strings.Contains(sec, "남궁민수") {
		t.Fatal("긴 이름")
	}
}

func TestLeaveTypeCheckboxesExclusive(t *testing.T) {
	vals := LeavePermitValues(LeavePermitInput{TypeKey: "outing", PeriodMode: LeavePeriodHours})
	if vals["ck외출"] != "☑" || vals["ck연차"] != "☐" || vals["ck반차"] != "☐" {
		t.Fatalf("구분 체크 오류: %+v", vals)
	}
	empty := LeavePermitValues(LeavePermitInput{})
	if empty["업무대행자"] != "" {
		t.Fatal("업무대행자 기본은 빈 칸")
	}
}

func TestWriteLeavePermitArtifacts(t *testing.T) {
	root, err := leaveRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	tplPath := filepath.Join(root, "server", "templates", "report", "근태허가원.hwpx")
	if err := WriteLeavePermitTemplate(tplPath); err != nil {
		t.Fatal(err)
	}
	tpl, err := os.ReadFile(tplPath)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "server", "testdata", "leave-permit")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	png := inkPNG(t)
	samples := []struct {
		file   string
		in     LeavePermitInput
		stamps map[string][]byte
		names  map[string]string
	}{
		{"01_연차_일단위.hwpx", LeavePermitInput{
			DocNo: "VI-근태-260101-001", WrittenOn: "2026년 01월 01일", Author: "신청자",
			TypeKey: "annual", PeriodMode: LeavePeriodDays,
			DayStart: "2026년 01월 02일", DayEnd: "01월 02일", DayCount: "1",
			Reason: "연차 사용", AuthorStampDate: "01/01", ApproveStampDate: "01/01",
		}, map[string][]byte{"작성사인": png, "승인사인": png}, nil},
		{"02_반차_시간.hwpx", LeavePermitInput{
			DocNo: "VI-근태-260102-001", WrittenOn: "2026년 01월 02일", Author: "신청자",
			TypeKey: "half", PeriodMode: LeavePeriodHours,
			HourYear: "2026", HourMonth: "01", HourDay: "02",
			HourStart: "09:00", HourEnd: "13:00", HourCount: "4",
			Reason: "반차", AuthorStampDate: "01/02", ApproveStampDate: "01/02",
			ApproveMark: "(대결)",
		}, map[string][]byte{"작성사인": png, "승인사인": png}, nil},
		{"03_대체근무.hwpx", LeavePermitInput{
			DocNo: "VI-근태-260103-001", WrittenOn: "2026년 01월 03일", Author: "신청자",
			TypeKey: "vacation", PeriodMode: LeavePeriodSwap,
			SwapDate: "2026년 01월 04일", SwapWorked: true,
			Reason: "대체근무", AuthorStampDate: "01/03", ApproveStampDate: "01/03",
		}, map[string][]byte{"작성사인": png, "승인사인": png}, nil},
		{"04_자기결재.hwpx", LeavePermitInput{
			DocNo: "VI-근태-260104-001", WrittenOn: "2026년 01월 04일", Author: "신청자",
			TypeKey: "annual", PeriodMode: LeavePeriodDays,
			DayStart: "2026년 01월 05일", DayEnd: "01월 05일", DayCount: "1",
			Reason: "자기 결재", AuthorStampDate: "01/04", ApproveStampDate: "01/04",
		}, map[string][]byte{"작성사인": png, "승인사인": png}, nil},
		{"05_서명없음_이름.hwpx", LeavePermitInput{
			DocNo: "VI-근태-260105-001", WrittenOn: "2026년 01월 05일", Author: "신청자",
			TypeKey: "annual", PeriodMode: LeavePeriodDays,
			DayStart: "2026년 01월 06일", DayEnd: "01월 06일", DayCount: "1",
			Reason: "서명 없음", AuthorStampDate: "01/05", ApproveStampDate: "01/05",
		}, nil, map[string]string{"작성사인": "신청자", "승인사인": "팀장"}},
		{"06_자기결재_이름.hwpx", LeavePermitInput{
			DocNo: "VI-근태-260106-001", WrittenOn: "2026년 01월 06일", Author: "신청자",
			TypeKey: "annual", PeriodMode: LeavePeriodDays,
			DayStart: "2026년 01월 07일", DayEnd: "01월 07일", DayCount: "1",
			Reason: "자기 결재 이름", AuthorStampDate: "01/06", ApproveStampDate: "01/06",
		}, nil, map[string]string{"작성사인": "신청자", "승인사인": "신청자"}},
	}
	for _, s := range samples {
		out, err := FillLeavePermit(tpl, LeavePermitValues(s.in), s.stamps, s.names, leaveSigBox())
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, s.file), out, 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func leaveRepoRoot() (string, error) {
	path, err := OfficialLeaveFormPath()
	if err != nil {
		return "", err
	}
	// .../docs/서식/file.hwpx → repo root
	return filepath.Clean(filepath.Join(filepath.Dir(path), "..", "..")), nil
}
