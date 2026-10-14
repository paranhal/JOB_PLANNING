package handler

type workPick struct {
	ID   string
	Name string
	Kind string
}

type workStage struct {
	ID        string
	Kind      string
	Stage     string
	PlanStart string
	PlanEnd   string
	Actual    string
	Owner     string
	Status    string
}

type workDoc struct {
	Title  string
	Date   string
	Status string
	Action string
}

type workDocGroup struct {
	Kind  string
	Label string
	Count string
	Docs  []workDoc
}

type workMgmtSeed struct {
	DateLabel string
	Picks     []workPick
	Stages    []workStage
	DocGroups []workDocGroup
}

func sampleWorkMgmt() workMgmtSeed {
	return workMgmtSeed{
		DateLabel: "2026.10.12",
		Picks: []workPick{
			{ID: "construction", Name: "A동 골조공사", Kind: "공사"},
			{ID: "network", Name: "데이터센터 네트워크 구축", Kind: "구축"},
			{ID: "dev", Name: "ERP 연동 플랫폼 개발", Kind: "개발"},
			{ID: "maintenance", Name: "충남세종 RFID 자동화 장비 유지보수", Kind: "유지보수"},
		},
		Stages: []workStage{
			{ID: "construction", Kind: "공사", Stage: "공정", PlanStart: "2026.10.01", PlanEnd: "2026.10.12", Actual: "2026.10.12", Owner: "김현우", Status: "진행"},
			{ID: "construction", Kind: "공사", Stage: "기성", PlanStart: "2026.10.13", PlanEnd: "2026.10.25", Actual: "—", Owner: "박준석", Status: "대기"},
			{ID: "construction", Kind: "공사", Stage: "준공", PlanStart: "2026.10.26", PlanEnd: "2026.11.10", Actual: "—", Owner: "이미진", Status: "대기"},
			{ID: "network", Kind: "구축", Stage: "구축", PlanStart: "2026.10.03", PlanEnd: "2026.10.19", Actual: "—", Owner: "김현우", Status: "진행"},
			{ID: "network", Kind: "구축", Stage: "개통", PlanStart: "2026.10.19", PlanEnd: "2026.10.31", Actual: "—", Owner: "박준석", Status: "대기"},
			{ID: "network", Kind: "구축", Stage: "검수", PlanStart: "2026.11.01", PlanEnd: "2026.11.15", Actual: "—", Owner: "이미진", Status: "대기"},
			{ID: "dev", Kind: "개발", Stage: "구축", PlanStart: "2026.10.05", PlanEnd: "2026.10.20", Actual: "—", Owner: "김현우", Status: "진행"},
			{ID: "dev", Kind: "개발", Stage: "개통", PlanStart: "2026.10.21", PlanEnd: "2026.11.05", Actual: "—", Owner: "박준석", Status: "대기"},
			{ID: "dev", Kind: "개발", Stage: "검수", PlanStart: "2026.11.06", PlanEnd: "2026.11.20", Actual: "—", Owner: "이미진", Status: "대기"},
			{ID: "maintenance", Kind: "유지보수", Stage: "접수", PlanStart: "2026.10.12", PlanEnd: "2026.10.12", Actual: "2026.10.12", Owner: "최은지", Status: "진행"},
			{ID: "maintenance", Kind: "유지보수", Stage: "조치", PlanStart: "2026.10.12", PlanEnd: "2026.10.13", Actual: "—", Owner: "최은지", Status: "진행"},
			{ID: "maintenance", Kind: "유지보수", Stage: "완료", PlanStart: "2026.10.13", PlanEnd: "2026.10.15", Actual: "—", Owner: "정우성", Status: "대기"},
		},
		DocGroups: []workDocGroup{
			{Kind: "construction", Label: "공사 · 공정", Count: "2건", Docs: []workDoc{
				{Title: "공정 착수 승인서", Date: "2026.10.12", Status: "작성 중", Action: "write"},
				{Title: "공정 착수 확인서", Date: "2026.10.12", Status: "완료", Action: "view"},
			}},
			{Kind: "construction", Label: "공사 · 기성", Count: "2건", Docs: []workDoc{
				{Title: "기성 확인 보고서", Date: "2026.10.25", Status: "대기", Action: "write"},
				{Title: "기성 점검 사진", Date: "2026.10.25", Status: "대기", Action: "write"},
			}},
			{Kind: "construction", Label: "공사 · 준공", Count: "2건", Docs: []workDoc{
				{Title: "준공 검토 보고서", Date: "2026.11.10", Status: "대기", Action: "write"},
				{Title: "준공 검수 사진", Date: "2026.11.10", Status: "대기", Action: "write"},
			}},
			{Kind: "network", Label: "구축 · 구축", Count: "2건", Docs: []workDoc{
				{Title: "구축 착수 승인서", Date: "2026.10.18", Status: "완료", Action: "view"},
				{Title: "구축 착수 확인서", Date: "2026.10.18", Status: "완료", Action: "view"},
			}},
			{Kind: "network", Label: "구축 · 개통", Count: "2건", Docs: []workDoc{
				{Title: "개통 준비 계획서", Date: "2026.10.31", Status: "대기", Action: "write"},
				{Title: "개통 점검 사진", Date: "2026.10.31", Status: "대기", Action: "write"},
			}},
			{Kind: "network", Label: "구축 · 검수", Count: "2건", Docs: []workDoc{
				{Title: "검수 계획서", Date: "2026.11.15", Status: "대기", Action: "write"},
				{Title: "검수 확인서", Date: "2026.11.15", Status: "대기", Action: "write"},
			}},
			{Kind: "dev", Label: "개발 · 구축", Count: "2건", Docs: []workDoc{
				{Title: "개발 착수 승인서", Date: "2026.10.20", Status: "대기", Action: "write"},
				{Title: "개발 착수 확인서", Date: "2026.10.20", Status: "대기", Action: "write"},
			}},
			{Kind: "dev", Label: "개발 · 개통", Count: "2건", Docs: []workDoc{
				{Title: "개통 테스트 계획서", Date: "2026.11.05", Status: "대기", Action: "write"},
				{Title: "개통 테스트 결과", Date: "2026.11.05", Status: "대기", Action: "write"},
			}},
			{Kind: "dev", Label: "개발 · 검수", Count: "2건", Docs: []workDoc{
				{Title: "검수 보고서", Date: "2026.11.20", Status: "대기", Action: "write"},
				{Title: "검수 확인서", Date: "2026.11.20", Status: "대기", Action: "write"},
			}},
			{Kind: "maintenance", Label: "유지보수 · 접수", Count: "2건", Docs: []workDoc{
				{Title: "장애 접수 확인서", Date: "2026.10.12", Status: "완료", Action: "view"},
				{Title: "현장 방문 계획서", Date: "2026.10.12", Status: "작성 중", Action: "write"},
			}},
			{Kind: "maintenance", Label: "유지보수 · 조치", Count: "2건", Docs: []workDoc{
				{Title: "AS 조치 보고서", Date: "2026.10.13", Status: "대기", Action: "write"},
				{Title: "부품 교체 기록", Date: "2026.10.13", Status: "대기", Action: "write"},
			}},
			{Kind: "maintenance", Label: "유지보수 · 완료", Count: "2건", Docs: []workDoc{
				{Title: "조치 완료 확인서", Date: "2026.10.15", Status: "대기", Action: "write"},
				{Title: "정기점검 결과서", Date: "2026.10.15", Status: "대기", Action: "write"},
			}},
		},
	}
}
