package model

import "strings"

const (
	SalesActionNone    = ""
	SalesActionWin     = "win"
	SalesActionConvert = "convert"
	SalesActionPromote = "promote"
	SalesActionView    = "view"
)

// SalesNextAction 파이프라인 「처리」열·팝업 판정. 사업 상세 조건과 같은 함수를 쓴다(D-96).
type SalesNextAction struct {
	Kind     string
	Label    string
	Enabled  bool
	Reason   string
	Href     string
	ShowLost bool
}

func SalesNextActionOf(p *SalesProject, workProjectID string, canWrite bool) SalesNextAction {
	act := salesNextActionKind(p, workProjectID)
	if act.Kind == SalesActionNone {
		return act
	}
	if !canWrite {
		act.Enabled = false
		act.ShowLost = false
		act.Reason = "쓰기 권한이 없습니다"
		act.Href = ""
		return act
	}
	act.Enabled = true
	return act
}

func salesNextActionKind(p *SalesProject, workProjectID string) SalesNextAction {
	if p == nil {
		return SalesNextAction{}
	}
	if p.IsDormant() {
		return SalesNextAction{Reason: "휴면"}
	}
	stage := strings.TrimSpace(p.Stage)
	bid := strings.TrimSpace(p.BidStatus)
	close := strings.TrimSpace(p.CloseReason)
	id := strings.TrimSpace(p.SalesID)

	switch stage {
	case SalesStage4Discover, SalesStage4Propose:
		return SalesNextAction{Kind: SalesActionWin, Label: "수주"}
	case SalesStage4Bid:
		switch bid {
		case SalesBidWon, SalesBidNegotiating:
			return SalesNextAction{Kind: SalesActionConvert, Label: "계약으로 전환"}
		default:
			return SalesNextAction{Kind: SalesActionWin, Label: "수주", ShowLost: true}
		}
	case SalesStage4Closed:
		if close == SalesCloseLost || close == SalesCloseDropped {
			return SalesNextAction{Reason: "종료"}
		}
		if close == SalesCloseContracted {
			if NeedsWorkProject(p, workProjectID) {
				return SalesNextAction{
					Kind:  SalesActionPromote,
					Label: "계약으로 전환",
					Href:  "/sales/" + id + "/promote",
				}
			}
			if wp := strings.TrimSpace(workProjectID); wp != "" {
				return SalesNextAction{
					Kind:  SalesActionView,
					Label: "사업 보기",
					Href:  "/projects/" + wp,
				}
			}
		}
	}
	return SalesNextAction{}
}
