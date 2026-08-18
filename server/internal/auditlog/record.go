package auditlog

// 법정 5개 항목 + 변경 전후·사유·결과·연쇄 해시. MAC 주소는 두지 않는다(§25.3.2).

const (
	ActionView       = "조회"
	ActionCreate     = "등록"
	ActionUpdate     = "수정"
	ActionDelete     = "삭제"
	ActionBulkDelete = "일괄삭제"
	ActionDownload   = "다운로드"
	ActionPrint      = "출력"
	ActionLogin      = "로그인"
	ActionLogout     = "로그아웃"
	ActionLoginFail  = "로그인실패"
	ActionPermChange = "권한변경"

	ResultOK   = "성공"
	ResultDeny = "거부"
)

// Record 접속기록 한 줄. 업무 서버는 이 구조체만 Append 한다.
type Record struct {
	LogID        string
	UserID       string
	Username     string
	FullName     string
	Role         string
	AccessAt     string
	ClientIP     string
	ForwardedFor string
	UserAgent    string
	SessionID    string
	SubjectType  string
	SubjectID    string
	SubjectName  string
	Action       string
	TargetTable  string
	TargetID     string
	Detail       string
	BeforeJSON   string
	AfterJSON    string
	Reason       string
	Result       string
	PrevHash     string
	RowHash      string
}
