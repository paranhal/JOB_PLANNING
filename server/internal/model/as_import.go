package model

import "strings"

const (
	ImportStatusReported   = "reported"
	ImportStatusApplied    = "applied"
	ImportStatusCancelled  = "cancelled"

	ImportIssueMissing   = "missing"
	ImportIssueDate      = "date"
	ImportIssueCustomer  = "customer"
	ImportIssueAsset     = "asset"
	ImportIssueDuplicate = "duplicate"

	ImportNoDateReason = "이관 반입(예정일 없음)"
)

// ASImportBatch 반입 배치. 업로드 직후 status=reported. 바로 반영하지 않는다. §12.11.7
type ASImportBatch struct {
	BatchID      string
	Filename     string
	Status       string
	CreatedAt    string
	CreatedBy    string
	AppliedAt    string
	CancelledAt  string
	TotalRows    int
	ErrorRows    int
	WarningRows  int
	AppliedRows  int
	Note         string
	ReadyRows    int
	ExcludedRows int
}

func (b ASImportBatch) StatusLabel() string {
	switch b.Status {
	case ImportStatusApplied:
		return "반영됨"
	case ImportStatusCancelled:
		return "취소됨"
	default:
		return "검증 대기"
	}
}

func (b ASImportBatch) CanApply() bool {
	return b.Status == ImportStatusReported && b.ErrorRows == 0 && b.ReadyRows > 0
}

func (b ASImportBatch) CanCancel() bool {
	return b.Status == ImportStatusApplied && b.AppliedRows > 0
}

// ASImportRow 스테이징 행. 오류면 제외하거나 고친 뒤에만 반영한다.
type ASImportRow struct {
	BatchID     string
	RowNo       int
	Excluded    bool
	Issues      []string
	CustomerRaw string
	CustomerID  string
	AssetRaw    string
	AssetID     string
	ReceiptRaw  string
	ReceiptDate string
	ProcessRaw  string
	ProcessDate string
	VisitDate   string
	Symptom     string
	Action      string
	Channel     string
	Requester   string
	Worker      string
	WorkerUID   string
	Category    string
	Model       string
	AppliedASID string
}

func (r ASImportRow) IssueText() string {
	return strings.Join(r.IssueLabels(), ", ")
}

func (r ASImportRow) IssueLabels() []string {
	var out []string
	for _, c := range r.Issues {
		out = append(out, ImportIssueLabel(c))
	}
	return out
}

func (r ASImportRow) HasBlocker() bool {
	if r.Excluded {
		return false
	}
	for _, c := range r.Issues {
		if c != ImportIssueAsset {
			return true
		}
	}
	return false
}

func (r ASImportRow) HasWarning() bool {
	if r.Excluded {
		return false
	}
	for _, c := range r.Issues {
		if c == ImportIssueAsset {
			return true
		}
	}
	return false
}

func ImportIssueLabel(code string) string {
	switch code {
	case ImportIssueMissing:
		return "필수값 누락"
	case ImportIssueDate:
		return "날짜 범위(2000~2100)"
	case ImportIssueCustomer:
		return "고객 매칭 실패"
	case ImportIssueAsset:
		return "자산 매칭 실패"
	case ImportIssueDuplicate:
		return "중복 의심"
	default:
		return code
	}
}

func SplitIssueCodes(s string) []string {
	return SplitCommaList(s)
}

func JoinIssueCodes(codes []string) string {
	return strings.Join(codes, ",")
}
