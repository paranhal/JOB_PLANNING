package auditlog

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

const timeLayout = "2006-01-02 15:04:05.000"

func nowSeoul() string {
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		loc = time.FixedZone("KST", 9*3600)
	}
	return time.Now().In(loc).Format(timeLayout)
}

// SessionFingerprint JWT 원문을 남기지 않고 세션 식별자만 만든다.
func SessionFingerprint(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:16])
}

func rowHash(rec Record) string {
	parts := []string{
		rec.PrevHash,
		rec.LogID,
		rec.UserID,
		rec.Username,
		rec.FullName,
		rec.Role,
		rec.AccessAt,
		rec.ClientIP,
		rec.ForwardedFor,
		rec.UserAgent,
		rec.SessionID,
		rec.SubjectType,
		rec.SubjectID,
		rec.SubjectName,
		rec.Action,
		rec.TargetTable,
		rec.TargetID,
		rec.Detail,
		rec.BeforeJSON,
		rec.AfterJSON,
		rec.Reason,
		rec.Result,
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}
