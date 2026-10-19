package leaveapp

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service/approval"
	"customer-support/internal/service/leavecalc"
)

func hwpxSection0(t *testing.T, doc []byte) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(doc), int64(len(doc)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if filepath.ToSlash(f.Name) != "Contents/section0.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	t.Fatal("section0 없음")
	return ""
}

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 24, 24))
	for y := 4; y < 20; y++ {
		for x := 4; x < 20; x++ {
			img.Set(x, y, color.RGBA{A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestFillHWPXSignatureOrName(t *testing.T) {
	svc, head, staff := testSvc(t)
	staffAct := approval.Actor{UserID: staff, OrgID: "O01", Role: model.RoleTech, Name: "직원"}
	headAct := approval.Actor{UserID: head, OrgID: "O01", Role: model.RoleOrgAdmin, Name: "팀장"}

	id, err := svc.Submit(staffAct, Input{
		TypeKey: leavecalc.TypeAnnual, PeriodMode: "days",
		StartDate: "2026-08-20", EndDate: "2026-08-20", Reason: "서명대체대기",
	})
	if err != nil {
		t.Fatal(err)
	}
	doc, _, err := svc.FillHWPX(id)
	if err != nil {
		t.Fatal(err)
	}
	sec := hwpxSection0(t, doc)
	if strings.Contains(sec, "{{") {
		t.Fatal("자리표시자")
	}
	if strings.Contains(sec, `binaryItemIDRef="sig`) {
		t.Fatal("결재 중 그림")
	}
	if !strings.Contains(sec, "직원") {
		t.Fatal("작성란 이름")
	}
	cells := svc.StampCells(mustLeave(t, svc, id))
	if len(cells) != 3 || !cells[1].Empty || !cells[2].Empty || cells[0].Name != "직원" {
		t.Fatalf("화면 칸 %+v", cells)
	}

	inbox, err := svc.Eng.ListInbox("O01", head, "2026-08-07")
	if err != nil || len(inbox) == 0 {
		t.Fatalf("inbox %v", err)
	}
	if err := svc.Eng.Approve(headAct, inbox[0].RequestID, "", ""); err != nil {
		t.Fatal(err)
	}
	doc, _, err = svc.FillHWPX(id)
	if err != nil {
		t.Fatal(err)
	}
	sec = hwpxSection0(t, doc)
	if strings.Contains(sec, "{{") || strings.Contains(sec, `(사인)`) {
		t.Fatal("승인 후 잔여")
	}
	if !strings.Contains(sec, "팀장") {
		t.Fatal("승인란 이름")
	}
	cells = svc.StampCells(mustLeave(t, svc, id))
	if cells[2].Empty || cells[2].Name != "팀장" {
		t.Fatalf("승인 화면 %+v", cells[2])
	}

	selfID, err := svc.Submit(headAct, Input{
		TypeKey: leavecalc.TypeAnnual, PeriodMode: "days",
		StartDate: "2026-08-21", EndDate: "2026-08-21", Reason: "자기결재이름",
	})
	if err != nil {
		t.Fatal(err)
	}
	doc, _, err = svc.FillHWPX(selfID)
	if err != nil {
		t.Fatal(err)
	}
	sec = hwpxSection0(t, doc)
	if strings.Count(sec, "팀장") < 2 {
		t.Fatal("자기 결재는 작성·승인 모두 본인")
	}
	selfCells := svc.StampCells(mustLeave(t, svc, selfID))
	if selfCells[0].Name != "팀장" || selfCells[2].Name != "팀장" {
		t.Fatalf("자기 결재 화면 %+v", selfCells)
	}
}

func TestFillHWPXCorruptAndMissingNameUsesUsername(t *testing.T) {
	svc, head, staff := testSvc(t)
	users := repository.NewUserRepo(svc.DB)
	bad := filepath.Join(t.TempDir(), "bad.png")
	if err := os.WriteFile(bad, []byte("not-png"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := users.UpdateSignaturePath(staff, bad); err != nil {
		t.Fatal(err)
	}
	_, _ = svc.DB.Exec(`UPDATE users SET full_name='' WHERE user_id=?`, staff)
	id, err := svc.Submit(approval.Actor{UserID: staff, OrgID: "O01", Role: model.RoleTech, Name: "staff1"}, Input{
		TypeKey: leavecalc.TypeAnnual, PeriodMode: "days",
		StartDate: "2026-08-24", EndDate: "2026-08-24", Reason: "아이디대체",
	})
	if err != nil {
		t.Fatal(err)
	}
	doc, _, err := svc.FillHWPX(id)
	if err != nil {
		t.Fatal(err)
	}
	sec := hwpxSection0(t, doc)
	if strings.Contains(sec, `binaryItemIDRef="sig`) {
		t.Fatal("손상 PNG")
	}
	if !strings.Contains(sec, "staff1") {
		t.Fatal("성명 없으면 아이디")
	}

	good := filepath.Join(t.TempDir(), "ok.png")
	if err := os.WriteFile(good, tinyPNG(t), 0644); err != nil {
		t.Fatal(err)
	}
	if err := users.UpdateSignaturePath(head, good); err != nil {
		t.Fatal(err)
	}
	inbox, _ := svc.Eng.ListInbox("O01", head, "2026-08-07")
	if len(inbox) == 0 {
		t.Fatal("inbox")
	}
	if err := svc.Eng.Approve(approval.Actor{UserID: head, OrgID: "O01", Role: model.RoleOrgAdmin, Name: "팀장"}, inbox[0].RequestID, "", ""); err != nil {
		t.Fatal(err)
	}
	doc, _, err = svc.FillHWPX(id)
	if err != nil {
		t.Fatal(err)
	}
	sec = hwpxSection0(t, doc)
	if !strings.Contains(sec, `binaryItemIDRef="sig`) {
		t.Fatal("등록 서명은 그림")
	}
}

func TestFillHWPXDelegateNameMark(t *testing.T) {
	svc, head, staff := testSvc(t)
	users := repository.NewUserRepo(svc.DB)
	dep := &model.User{Username: "dep1", PasswordHash: "x", FullName: "대리인", Role: model.RoleTech, IsActive: true, OrgID: "O01"}
	if err := users.Create(dep); err != nil {
		t.Fatal(err)
	}
	headAct := approval.Actor{UserID: head, OrgID: "O01", Role: model.RoleOrgAdmin, Name: "팀장"}
	if _, err := svc.Eng.CreateDelegation(headAct, "O01", head, dep.UserID, "2026-08-01", "2026-08-31", approval.SrcHead, ""); err != nil {
		t.Fatal(err)
	}
	id, err := svc.Submit(approval.Actor{UserID: staff, OrgID: "O01", Role: model.RoleTech, Name: "직원"}, Input{
		TypeKey: leavecalc.TypeAnnual, PeriodMode: "days",
		StartDate: "2026-08-25", EndDate: "2026-08-25", Reason: "대결서명",
	})
	if err != nil {
		t.Fatal(err)
	}
	inbox, err := svc.Eng.ListInbox("O01", dep.UserID, "2026-08-07")
	if err != nil || len(inbox) == 0 {
		t.Fatalf("대결 inbox %d %v", len(inbox), err)
	}
	if err := svc.Eng.Approve(approval.Actor{UserID: dep.UserID, OrgID: "O01", Role: model.RoleTech, Name: "대리인"}, inbox[0].RequestID, "", ""); err != nil {
		t.Fatal(err)
	}
	doc, _, err := svc.FillHWPX(id)
	if err != nil {
		t.Fatal(err)
	}
	sec := hwpxSection0(t, doc)
	if !strings.Contains(sec, "대리인") || !strings.Contains(sec, "(대결)") {
		t.Fatalf("대결 표기 없음 %s", sec[0:min(400, len(sec))])
	}
	cells := svc.StampCells(mustLeave(t, svc, id))
	if cells[2].Name != "대리인" || cells[2].Mark != "(대결)" {
		t.Fatalf("%+v", cells[2])
	}
}

func TestStampNameLongClass(t *testing.T) {
	c := stampCell("작성", nil, "Alexandria", "")
	if c.NameClass != "text-ui-xs" || c.Name != "Alexandria" || c.HasImage {
		t.Fatalf("%+v", c)
	}
}

func mustLeave(t *testing.T, svc *Service, id string) *Request {
	t.Helper()
	got, err := svc.Get(id)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	return got
}
