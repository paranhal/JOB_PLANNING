package handler

type docsPick struct {
	ID   string
	Name string
	Kind string
}

type docsItem struct {
	ID         string
	Kind       string
	KindLabel  string
	Stage      string
	Title      string
	Sub        string
	Owner      string
	Due        string
	Status     string
	DocNo      string
	Project    string
	BodyLead   string
	BodyNote   string
	Milestones []docsMilestone
	Writer     string
	Reviewer   string
	Approver   string
}

type docsMilestone struct {
	Label string
	When  string
}

type docsSeed struct {
	DateLabel string
	Weekday   string
	Weather   string
	Picks     []docsPick
	Items     []docsItem
}

func sampleProjectDocs() docsSeed {
	return docsSeed{
		DateLabel: "2026.10.12",
		Weekday:   "월",
		Weather:   "맑음 · 18~24℃",
		Picks: []docsPick{
			{ID: "construction", Name: "A동 골조공사", Kind: "공사"},
			{ID: "network", Name: "데이터센터 네트워크 구축", Kind: "구축"},
			{ID: "dev", Name: "ERP 연동 플랫폼 개발", Kind: "개발"},
			{ID: "maintenance", Name: "충남세종 RFID 자동화 장비 유지보수", Kind: "유지보수"},
		},
		Items: []docsItem{
			{
				ID: "d1", Kind: "construction", KindLabel: "공사", Stage: "공정",
				Title: "공정 착수 승인서", Sub: "공사 · 공정 단계", Owner: "김현우", Due: "2026.10.12",
				Status: "작성 중", DocNo: "D-2026-104", Project: "A동 골조공사",
				BodyLead: "공정 착수 승인서는 A동 8층 슬래브 타설 공정 착수 전 확인을 위해 작성됩니다.",
				BodyNote: "하자 점검 결과 기초부 설치/철거 대기 / 앙카 콘크리트가 파손되어있음. 공정 착수 전 잔여 상태는 확인 여부입니다.(확인)",
				Milestones: []docsMilestone{
					{Label: "공정 착수", When: "2026.10.12 09:00"},
					{Label: "기성 확인", When: "2026.10.15 18:00"},
					{Label: "준공 검토", When: "2026.10.22 17:00"},
				},
				Writer: "김현우", Reviewer: "박준석", Approver: "이정민",
			},
			{
				ID: "d2", Kind: "construction", KindLabel: "공사", Stage: "공정",
				Title: "공정 착수 확인서", Sub: "공사 · 공정 단계", Owner: "박준석", Due: "2026.10.12",
				Status: "완료", DocNo: "D-2026-105", Project: "A동 골조공사",
				BodyLead: "공정 착수 확인서는 착수 승인 이후 현장 확인 결과를 기록합니다.",
				BodyNote: "현장 확인 완료. 잔여 지적 사항은 후속 공정 진행 보고서에 반영합니다.",
				Milestones: []docsMilestone{
					{Label: "공정 착수", When: "2026.10.12 09:00"},
					{Label: "기성 확인", When: "2026.10.15 18:00"},
					{Label: "준공 검토", When: "2026.10.22 17:00"},
				},
				Writer: "박준석", Reviewer: "김현우", Approver: "이정민",
			},
			{
				ID: "d3", Kind: "construction", KindLabel: "공사", Stage: "공정",
				Title: "공정 진행 보고서", Sub: "공사 · 공정 단계", Owner: "이미진", Due: "2026.10.13",
				Status: "대기", DocNo: "D-2026-106", Project: "A동 골조공사",
				BodyLead: "공정 진행 보고서는 당일 공정률과 이슈를 정리합니다.",
				BodyNote: "작성 대기. 착수 승인서 제출 후 이어서 작성합니다.",
				Milestones: []docsMilestone{
					{Label: "공정 착수", When: "2026.10.12 09:00"},
					{Label: "기성 확인", When: "2026.10.15 18:00"},
					{Label: "준공 검토", When: "2026.10.22 17:00"},
				},
				Writer: "이미진", Reviewer: "박준석", Approver: "이정민",
			},
			{
				ID: "n1", Kind: "network", KindLabel: "구축", Stage: "구축",
				Title: "구축 착수 승인서", Sub: "구축 · 구축 단계", Owner: "김현우", Due: "2026.10.18",
				Status: "완료", DocNo: "D-2026-201", Project: "데이터센터 네트워크 구축",
				BodyLead: "구축 착수 승인서는 장비 반입·설치 전 확인을 위해 작성됩니다.",
				BodyNote: "착수 승인 완료.",
				Milestones: []docsMilestone{
					{Label: "구축 착수", When: "2026.10.18 09:00"},
					{Label: "개통 준비", When: "2026.10.31 18:00"},
					{Label: "검수", When: "2026.11.15 17:00"},
				},
				Writer: "김현우", Reviewer: "박준석", Approver: "이정민",
			},
			{
				ID: "v1", Kind: "dev", KindLabel: "개발", Stage: "구축",
				Title: "개발 착수 승인서", Sub: "개발 · 구축 단계", Owner: "김현우", Due: "2026.10.20",
				Status: "대기", DocNo: "D-2026-301", Project: "ERP 연동 플랫폼 개발",
				BodyLead: "개발 착수 승인서는 API 연동 착수 전 범위를 확정합니다.",
				BodyNote: "작성 대기.",
				Milestones: []docsMilestone{
					{Label: "개발 착수", When: "2026.10.20 09:00"},
					{Label: "개통 테스트", When: "2026.11.05 18:00"},
					{Label: "검수", When: "2026.11.20 17:00"},
				},
				Writer: "김현우", Reviewer: "박준석", Approver: "이정민",
			},
			{
				ID: "m1", Kind: "maintenance", KindLabel: "유지보수", Stage: "조치",
				Title: "AS 조치 보고서", Sub: "유지보수 · 조치 단계", Owner: "최은지", Due: "2026.10.13",
				Status: "작성 중", DocNo: "D-2026-401", Project: "충남세종 RFID 자동화 장비 유지보수",
				BodyLead: "AS 조치 보고서는 현장 장애 조치 내용과 교체 부품을 기록합니다.",
				BodyNote: "RFID 리더 오류 현장 조치. 예비 모듈 교체 후 정상 동작 확인.",
				Milestones: []docsMilestone{
					{Label: "접수", When: "2026.10.12 09:00"},
					{Label: "조치", When: "2026.10.12 16:00"},
					{Label: "완료", When: "2026.10.15 17:00"},
				},
				Writer: "최은지", Reviewer: "정우성", Approver: "이정민",
			},
		},
	}
}
