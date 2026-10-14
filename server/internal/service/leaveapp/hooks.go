package leaveapp

import (
	"database/sql"
	"fmt"
	"strings"

	"customer-support/internal/service/approval"
)

func (s *Service) Snapshot(tx *sql.Tx, docType, docID string) (string, string, error) {
	if docType != approval.DocLeave {
		return docType + " " + docID, `{"doc_id":"` + docID + `"}`, nil
	}
	return s.SnapshotJSON(docID)
}

func (s *Service) AfterApproved(tx *sql.Tx, req approval.Request) error {
	if req.DocType != approval.DocLeave {
		return nil
	}
	if tx == nil {
		return fmt.Errorf("tx")
	}
	leave, err := s.Get(req.DocID)
	if err != nil || leave == nil {
		return fmt.Errorf("신청서가 없다")
	}
	if _, err := tx.Exec(`UPDATE leave_requests SET status='approved' WHERE leave_req_id=?`, leave.LeaveReqID); err != nil {
		return err
	}
	kind, ok := staffKind(leave.TypeKey, leave.HalfPart)
	if ok {
		rows, err := tx.Query(`SELECT leave_date FROM leave_request_days WHERE leave_req_id=?`, leave.LeaveReqID)
		if err != nil {
			return err
		}
		var dates []string
		for rows.Next() {
			var d string
			if err := rows.Scan(&d); err != nil {
				rows.Close()
				return err
			}
			dates = append(dates, d)
		}
		rows.Close()
		for _, d := range dates {
			lid := "LV-" + strings.ReplaceAll(fmt.Sprintf("%s%s", leave.LeaveReqID, d), "-", "")
			if len(lid) > 64 {
				lid = "LV-" + d + "-" + leave.LeaveReqID[len(leave.LeaveReqID)-8:]
			}
			if _, err := tx.Exec(`INSERT INTO staff_leaves(leave_id, user_id, leave_date, leave_kind, note, created_by, source_request_id)
				VALUES (?,?,?,?,?,?,?)`, lid, leave.UserID, d, kind, leave.DocNo+" "+typeLabel(leave.TypeKey), leave.UserID, leave.LeaveReqID); err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "unique") {
					return fmt.Errorf("%s 에 이미 연차가 있습니다", d)
				}
				return err
			}
		}
	}
	if strings.TrimSpace(leave.ApproverDeputyUserID) == "" {
		return nil
	}
	p, err := s.Eng.GetPolicy(leave.OrgID)
	if err != nil {
		return err
	}
	if !p.DelegWithLeave {
		return nil
	}
	head, err := s.Eng.OrgHead(leave.OrgID)
	if err != nil || head == "" || head != leave.UserID {
		return nil
	}
	id := "AD" + leave.LeaveReqID
	start, end := leave.StartDate, leave.EndDate
	if leave.PeriodMode == "swap" && leave.SwapDate != "" {
		start, end = leave.SwapDate, leave.SwapDate
	}
	_, err = tx.Exec(`INSERT INTO approval_delegations(
		delegation_id, org_id, from_user_id, to_user_id, start_date, end_date, source, leave_req_id, created_by)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		id, leave.OrgID, head, leave.ApproverDeputyUserID, start, end, approval.SrcLeave, leave.LeaveReqID, leave.UserID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "overlap") {
			return fmt.Errorf("기간이 겹치는 지정이 있다")
		}
		return err
	}
	return nil
}
