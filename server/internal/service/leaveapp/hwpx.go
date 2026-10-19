package leaveapp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"customer-support/internal/hwpx"
	"customer-support/internal/service/approval"
)

func (s *Service) FillHWPX(leaveID string) ([]byte, string, error) {
	req, err := s.Get(leaveID)
	if err != nil || req == nil {
		return nil, "", fmt.Errorf("신청서가 없다")
	}
	tpl, err := loadLeaveTemplate()
	if err != nil {
		return nil, "", err
	}
	sn := snapshotFrom(req, s.now())
	approveStamp, approveMark := "", ""
	if req.ApprovalRequestID != "" {
		ar, err := s.Eng.GetRequest(req.ApprovalRequestID)
		if err == nil && ar.SnapshotJSON != "" {
			if parsed, err := ParseSnapshot(ar.SnapshotJSON); err == nil && parsed.DocNo != "" {
				sn = parsed
			}
		}
		if err == nil && ar.Status == approval.StatusApproved {
			if ar.CompletedAt != "" {
				if t, e := time.Parse("2006-01-02 15:04:05", ar.CompletedAt); e == nil {
					approveStamp = fmt.Sprintf("%02d/%02d", int(t.Month()), t.Day())
				}
			}
			if approveStamp == "" {
				approveStamp = sn.AuthorStampDate
			}
		}
	}
	stamps, names, stampMark := s.leaveStamps(req)
	if stampMark != "" {
		approveMark = stampMark
	}
	vals := hwpx.LeavePermitValues(PermitInput(sn, approveStamp, approveMark))
	out, err := hwpx.FillLeavePermit(tpl, vals, stamps, names, hwpx.SignatureBox{Size: 4600, RightGap: 900})
	if err != nil {
		return nil, "", err
	}
	name := strings.ReplaceAll(req.DocNo, "/", "-") + ".hwpx"
	return out, name, nil
}

func loadLeaveTemplate() ([]byte, error) {
	candidates := []string{
		hwpx.DefaultLeaveTemplatePath(),
		filepath.Join("templates", "report", "근태허가원.hwpx"),
		filepath.Join("..", "..", "templates", "report", "근태허가원.hwpx"),
	}
	for _, p := range candidates {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return b, nil
		}
	}
	src, err := hwpx.OfficialLeaveFormPath()
	if err != nil {
		return nil, fmt.Errorf("근태허가원 템플릿이 없습니다")
	}
	orig, err := os.ReadFile(src)
	if err != nil {
		return nil, err
	}
	return hwpx.BuildLeavePermitTemplate(orig)
}
