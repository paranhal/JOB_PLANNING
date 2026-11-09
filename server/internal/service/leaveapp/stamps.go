package leaveapp

import (
	"encoding/base64"
	"html/template"
	"os"
	"strings"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/service/approval"
)

// StampCell 화면 결재란 한 칸. 처리 전·검토란은 Empty.
type StampCell struct {
	Label     string
	Name      string
	Mark      string
	NameClass string
	ImageSrc  template.URL
	HasImage  bool
	Empty     bool
	Tip       string
	ActedAt   string
}

func (s *Service) StampCells(req *Request) []StampCell {
	stamps, names, mark, authorAt, approveAt := s.leaveStamps(req)
	return []StampCell{
		stampCell("작성", stamps["작성사인"], names["작성사인"], "", authorAt),
		{Label: "검토", Empty: true},
		stampCell("승인", stamps["승인사인"], names["승인사인"], mark, approveAt),
	}
}

func stampCell(label string, png []byte, name, mark, actedAt string) StampCell {
	name = strings.TrimSpace(name)
	c := StampCell{Label: label, Name: name, Mark: mark, NameClass: model.StampNameClass(name), ActedAt: FormatStampDateTime(actedAt)}
	if name == "" && len(png) == 0 {
		c.Empty = true
		return c
	}
	markImg := model.SignatureOrName(png, name, "", "")
	if len(markImg.PNG) > 0 {
		c.HasImage = true
		c.ImageSrc = template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(markImg.PNG))
		c.Name = markImg.Name
		return c
	}
	c.Name = markImg.Name
	if c.Name != "" {
		c.Tip = "서명 미등록"
	} else {
		c.Empty = true
	}
	return c
}

func (s *Service) leaveStamps(req *Request) (map[string][]byte, map[string]string, string, string, string) {
	stamps := map[string][]byte{}
	names := map[string]string{}
	if req == nil {
		return stamps, names, "", "", ""
	}
	st := strings.TrimSpace(req.Status)
	if st == "" || st == "withdrawn" || st == "draft" {
		return stamps, names, "", "", ""
	}
	png, name := s.userStamp(req.UserID)
	if name == "" {
		name = strings.TrimSpace(req.AuthorName)
	}
	stamps["작성사인"] = png
	names["작성사인"] = name
	authorAt := strings.TrimSpace(req.CreatedAt)

	if strings.TrimSpace(req.ApprovalRequestID) == "" || s.Eng == nil {
		return stamps, names, "", authorAt, ""
	}
	ar, err := s.Eng.GetRequest(req.ApprovalRequestID)
	if err != nil {
		return stamps, names, "", authorAt, ""
	}
	if t := strings.TrimSpace(ar.SubmittedAt); t != "" {
		authorAt = t
	}
	if ar.Status != approval.StatusApproved {
		return stamps, names, "", authorAt, ""
	}
	approveMark := ""
	actorID := ""
	approveAt := ""
	steps, _ := s.Eng.ListSteps(ar.RequestID)
	for _, step := range steps {
		if step.StepType != approval.TypeApprove || step.Status != approval.StepApproved {
			continue
		}
		actorID = strings.TrimSpace(step.ActedBy)
		if actorID == "" {
			actorID = strings.TrimSpace(step.ApproverID)
		}
		if t := strings.TrimSpace(step.ActedAt); t != "" {
			approveAt = t
		}
		if step.ActMode == approval.ModeDelegate || step.ActMode == approval.ModeAdminEmergency {
			approveMark = "(대결)"
		}
	}
	if actorID == "" {
		actorID = req.UserID
	}
	png, name = s.userStamp(actorID)
	stamps["승인사인"] = png
	names["승인사인"] = name
	return stamps, names, approveMark, authorAt, approveAt
}

// FormatStampDateTime 결재란 표시용. Asia/Seoul, 분 단위(초 없음).
func FormatStampDateTime(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		loc = time.FixedZone("KST", 9*3600)
	}
	layouts := []string{
		time.RFC3339Nano, time.RFC3339,
		"2006-01-02 15:04:05.000", "2006-01-02 15:04:05",
		"2006-01-02T15:04:05", "2006-01-02 15:04", "2006-01-02",
	}
	for _, ly := range layouts {
		if t, e := time.ParseInLocation(ly, raw, loc); e == nil {
			return t.In(loc).Format("2006-01-02 15:04")
		}
		if t, e := time.Parse(ly, raw); e == nil {
			return t.In(loc).Format("2006-01-02 15:04")
		}
	}
	return raw
}

func (s *Service) userStamp(userID string) ([]byte, string) {
	userID = strings.TrimSpace(userID)
	if s == nil || s.DB == nil || userID == "" {
		return nil, userID
	}
	var full, user, path string
	err := s.DB.QueryRow(`SELECT COALESCE(full_name,''), COALESCE(username,''), COALESCE(signature_path,'') FROM users WHERE user_id=?`, userID).Scan(&full, &user, &path)
	if err != nil {
		return nil, model.PersonDisplayName("", "", userID)
	}
	raw := readStampPNG(path)
	mark := model.SignatureOrName(raw, full, user, userID)
	return mark.PNG, mark.Name
}

func readStampPNG(path string) []byte {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		return nil
	}
	return b
}
