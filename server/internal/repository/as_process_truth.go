package repository

import (
	"database/sql"
	"log"
	"strings"

	"customer-support/internal/model"
)

// ActionProcessConflict 접수 조치 열과 as_processes 가 둘 다 채워져 있는데 내용이 다른 행. §44.8.4
// 사람이 판정하기 전에는 어느 쪽도 지우지 않는다.
type ActionProcessConflict struct {
	ASID               string
	ASNumber           string
	OrgName            string
	ReceiptAction      string
	ProcessAction      string
	ReceiptCause       string
	ProcessCause       string
	ReceiptParts       string
	ProcessParts       string
	ReceiptResult      string
	ProcessResult      string
	ReceiptProcessType string
	ProcessWorkType    string
	ReceiptTransfer    string
	ProcessTransfer    string
}

// applyASProcessTruth 마이그레이션 051. as_processes 에 cause_type 을 보태고 어긋난 건수를 남긴다.
// as_receipts 조치 열은 지우지 않는다. complete_datetime 도 건드리지 않는다. §44.8.4
func applyASProcessTruth(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`ALTER TABLE as_processes ADD COLUMN cause_type TEXT`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		log.Printf("051 as_processes.cause_type: %v", err)
	}
	n := len(ListActionConflicts(db))
	if n == 0 {
		log.Printf("051 process truth: 조치 어긋남 없음")
		return
	}
	log.Printf("051 process truth: 조치 어긋남 %d건 — xlsx 로 뽑아 사람이 보기 전에는 지우지 않는다", n)
}

// asLatestProcessColSQL 최신 비어 있지 않은 조치 행의 열. 없으면 접수 열. 40-E 읽기 정답은 이력.
func asLatestProcessColSQL(alias, processCol, receiptCol string) string {
	return `COALESCE(NULLIF(TRIM((
		SELECT p.` + processCol + ` FROM as_processes p
		 WHERE p.as_id = ` + alias + `.as_id
		   AND TRIM(COALESCE(p.` + processCol + `,'')) != ''
		 ORDER BY p.process_datetime DESC, p.process_id DESC LIMIT 1
	)), ''), ` + alias + `.` + receiptCol + `)`
}

func asActionTakenSQL(alias string) string {
	return asLatestProcessColSQL(alias, "work_content", "action_taken")
}

func asCauseTypeSQL(alias string) string {
	return asLatestProcessColSQL(alias, "cause_type", "cause_type")
}

func asPartsUsedSQL(alias string) string {
	return asLatestProcessColSQL(alias, "parts_used", "parts_used")
}

func asResultCodeSQL(alias string) string {
	return asLatestProcessColSQL(alias, "result_code", "result_code")
}

func asProcessTypeSQL(alias string) string {
	return asLatestProcessColSQL(alias, "work_type", "process_type")
}

func asTransferDetailSQL(alias string) string {
	return asLatestProcessColSQL(alias, "transfer_detail", "transfer_detail")
}

// ListActionConflicts 접수·이력이 둘 다 채워졌는데 조치 본문이 다른 행. 지시서 SQL 그대로.
func ListActionConflicts(db *sql.DB) []ActionProcessConflict {
	if db == nil {
		return nil
	}
	causeSel := `''`
	if tableHasColumn(db, "as_processes", "cause_type") {
		causeSel = `COALESCE(p.cause_type,'')`
	}
	q := `
		SELECT r.as_id, r.as_number, COALESCE(c.org_name,''),
		       COALESCE(r.action_taken,''), COALESCE(p.work_content,''),
		       COALESCE(r.cause_type,''), ` + causeSel + `,
		       COALESCE(r.parts_used,''), COALESCE(p.parts_used,''),
		       COALESCE(r.result_code,''), COALESCE(p.result_code,''),
		       COALESCE(r.process_type,''), COALESCE(p.work_type,''),
		       COALESCE(r.transfer_detail,''), COALESCE(p.transfer_detail,'')
		  FROM as_receipts r
		  JOIN as_processes p ON p.as_id = r.as_id
		  LEFT JOIN customers c ON c.customer_id = r.customer_id
		 WHERE IFNULL(r.action_taken,'') <> '' AND IFNULL(p.work_content,'') <> ''
		   AND TRIM(r.action_taken) <> TRIM(p.work_content)
		 ORDER BY r.as_number, p.process_datetime, p.process_id`
	rows, err := db.Query(q)
	if err != nil {
		log.Printf("051 ListActionConflicts: %v", err)
		return nil
	}
	defer rows.Close()
	var out []ActionProcessConflict
	for rows.Next() {
		var it ActionProcessConflict
		if err := rows.Scan(
			&it.ASID, &it.ASNumber, &it.OrgName,
			&it.ReceiptAction, &it.ProcessAction,
			&it.ReceiptCause, &it.ProcessCause,
			&it.ReceiptParts, &it.ProcessParts,
			&it.ReceiptResult, &it.ProcessResult,
			&it.ReceiptProcessType, &it.ProcessWorkType,
			&it.ReceiptTransfer, &it.ProcessTransfer,
		); err != nil {
			log.Printf("051 ListActionConflicts scan: %v", err)
			return out
		}
		out = append(out, it)
	}
	return out
}

// overlayProcessActionFields 단건 화면은 최신 비어 있지 않은 이력을 보여 준다. 접수 열은 비울 때만 남긴다.
func overlayProcessActionFields(as *model.ASReceipt, procs []model.ASProcess) {
	if as == nil || len(procs) == 0 {
		return
	}
	pick := func(get func(model.ASProcess) string) string {
		for i := len(procs) - 1; i >= 0; i-- {
			if s := strings.TrimSpace(get(procs[i])); s != "" {
				return s
			}
		}
		return ""
	}
	if s := pick(func(p model.ASProcess) string { return p.WorkContent }); s != "" {
		as.ActionTaken = s
	}
	if s := pick(func(p model.ASProcess) string { return p.CauseType }); s != "" {
		as.CauseType = s
	}
	if s := pick(func(p model.ASProcess) string { return p.PartsUsed }); s != "" {
		as.PartsUsed = s
	}
	if s := pick(func(p model.ASProcess) string { return p.ResultCode }); s != "" {
		as.ResultCode = s
	}
	if s := pick(func(p model.ASProcess) string { return p.WorkType }); s != "" {
		as.ProcessType = s
	}
	if s := pick(func(p model.ASProcess) string { return p.TransferDetail }); s != "" {
		as.TransferDetail = s
	}
}
