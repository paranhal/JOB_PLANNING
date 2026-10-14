package handler

type diaryPick struct {
	ID   string
	Name string
	Kind string
}

type diaryWork struct {
	ID        string
	Kind      string
	KindLabel string
	Name      string
	Today     string
	Next      string
}

type diaryPerson struct {
	ID      string
	Kind    string
	Name    string
	Role    string
	Project string
	Plan    string
	Prev    string
	Today   string
}

type diaryRes struct {
	ID   string
	Kind string
	Name string
	Sub  string
	Spec string
	Qty  string
	When string
}

type diaryFile struct {
	ID        string
	Kind      string
	KindLabel string
	Title     string
	File      string
	Meta      string
}

type diarySeed struct {
	DateLabel string
	Weekday   string
	DocNo     string
	SavedAt   string
	Picks     []diaryPick
	Works     []diaryWork
	People    []diaryPerson
	Resources []diaryRes
	Files     []diaryFile
}

func sampleProjectDiary() diarySeed {
	return diarySeed{
		DateLabel: "2026.10.12",
		Weekday:   "월",
		DocNo:     "I-2026-1012",
		SavedAt:   "16:42",
		Picks: []diaryPick{
			{ID: "construction", Name: "A동 골조공사", Kind: "공사"},
			{ID: "network", Name: "데이터센터 네트워크 구축", Kind: "구축"},
			{ID: "dev", Name: "ERP 연동 플랫폼 개발", Kind: "개발"},
			{ID: "maintenance", Name: "충남세종 RFID 자동화 장비 유지보수", Kind: "유지보수"},
		},
		Works: []diaryWork{
			{ID: "construction", Kind: "construction", KindLabel: "공사", Name: "A동 골조공사",
				Today: "A동 8층 슬래브 타설 120㎡ 완료. 양생 상태 및 안전 통로 점검, 이상 없음.",
				Next:  "10.13 - 슬래브 양생 상태 확인 및 9층 거푸집 설치 구간 사전 점검."},
			{ID: "network", Kind: "network", KindLabel: "구축", Name: "데이터센터 네트워크 구축",
				Today: "코어 스위치 2대 설치 및 VLAN 설정 완료. 포트 연결·이중화 전환 시험 정상 확인.",
				Next:  "10.13 - 방화벽 정책 적용 후 통신 시험. 서비스 구간별 설정 상태 재점검."},
			{ID: "dev", Kind: "dev", KindLabel: "개발", Name: "ERP 연동 플랫폼 개발",
				Today: "발주 API 연동 및 결재 화면 구현. 통합 테스트 24건 수행, 22건 통과 2건 보류.",
				Next:  "10.13 - 인증 오류 2건 수정 및 재시험. QA 회귀 테스트와 배포 전 코드 검토."},
			{ID: "maintenance", Kind: "maintenance", KindLabel: "유지보수", Name: "충남세종 RFID 자동화 장비 유지보수",
				Today: "RFID 리더 오류 현장 조치. 예비 모듈 교체 후 정상 동작 확인.",
				Next:  "10.13 - 조치 보고서 제출 및 세종시 10월 정기점검 일정 확정."},
		},
		People: []diaryPerson{
			{ID: "p1", Kind: "construction", Name: "김현우", Role: "현장소장", Project: "A동 골조공사", Plan: "8", Prev: "8", Today: "8"},
			{ID: "p2", Kind: "network", Name: "박준석", Role: "네트워크 팀장", Project: "데이터센터 구축", Plan: "6", Prev: "5", Today: "6"},
			{ID: "p3", Kind: "dev", Name: "이미진", Role: "개발 PM", Project: "ERP 플랫폼 개발", Plan: "8", Prev: "7", Today: "7"},
			{ID: "p4", Kind: "dev", Name: "전상호", Role: "QA 엔지니어", Project: "ERP 플랫폼 개발", Plan: "6", Prev: "4", Today: "5"},
			{ID: "p5", Kind: "construction", Name: "박성우", Role: "보조 인력", Project: "A동 골조공사", Plan: "4", Prev: "4", Today: "4"},
			{ID: "p6", Kind: "maintenance", Name: "최은지", Role: "AS 엔지니어", Project: "RFID 유지보수", Plan: "6", Prev: "5", Today: "6"},
		},
		Resources: []diaryRes{
			{ID: "r1", Kind: "construction", Name: "레미콘", Sub: "공사 · 자재", Spec: "25-24-150", Qty: "120m³", When: "09:20"},
			{ID: "r2", Kind: "network", Name: "코어 스위치", Sub: "구축 · 장비", Spec: "48포트 / L3", Qty: "2대", When: "10:30"},
			{ID: "r3", Kind: "dev", Name: "테스트 VM", Sub: "개발 · 시스템", Spec: "Ubuntu 24.04", Qty: "2대", When: "13:00"},
			{ID: "r4", Kind: "maintenance", Name: "예비 RFID 리더", Sub: "유지보수 · 부품", Spec: "UHF / 교체 모듈", Qty: "1대", When: "16:00"},
		},
		Files: []diaryFile{
			{ID: "f1", Kind: "construction", KindLabel: "공사", Title: "슬래브 타설 120㎡ 완료 · 현장 점검 이상 없음",
				File: "A동_8층_타설기록_1012.jpg", Meta: "JPG · 2.8 MB · 김현우 · 16:10"},
			{ID: "f2", Kind: "network", KindLabel: "구축", Title: "코어 스위치 2대 설치 · 이중화 전환 시험 정상",
				File: "네트워크_연결기록_1012.pdf", Meta: "PDF · 840 KB · 박준석 · 16:20"},
			{ID: "f3", Kind: "dev", KindLabel: "개발", Title: "API 테스트 24건 · 22건 통과, 2건 보류 배정",
				File: "ERP_API_테스트결과_1012.xlsx", Meta: "XLSX · 156 KB · 전상호 · 16:35"},
			{ID: "f4", Kind: "maintenance", KindLabel: "유지보수", Title: "RFID 리더 오류 현장 조치 · 모듈 교체 정상",
				File: "RFID_AS조치보고서_1012.pdf", Meta: "PDF · 420 KB · 최은지 · 16:40"},
		},
	}
}
