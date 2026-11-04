package model

import (
	"fmt"
	"strings"
	"time"
)

const (
	SiteCheckPending = "pending"
	SiteCheckDone    = "done"
	SiteCheckNA      = "na"
)

func ValidSiteCheckStatus(v string) bool {
	switch strings.TrimSpace(v) {
	case SiteCheckPending, SiteCheckDone, SiteCheckNA:
		return true
	default:
		return false
	}
}

// SiteCheckRateLabel 완료/전체. 분모 0이면 —.
func SiteCheckRateLabel(done, total int) string {
	if total <= 0 {
		return "—"
	}
	if done < 0 {
		done = 0
	}
	if done > total {
		done = total
	}
	return fmt.Sprintf("%d%%", done*100/total)
}

func DatesInclusive(from, to string) []string {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if from == "" {
		return nil
	}
	if to == "" || to < from {
		to = from
	}
	a, err1 := time.Parse("2006-01-02", from)
	b, err2 := time.Parse("2006-01-02", to)
	if err1 != nil || err2 != nil {
		return []string{from}
	}
	var out []string
	for d := a; !d.After(b); d = d.AddDate(0, 0, 1) {
		out = append(out, d.Format("2006-01-02"))
		if len(out) > 366 {
			break
		}
	}
	return out
}

func TaskTracksSiteProgress(workType, sourceType string) bool {
	wt := strings.TrimSpace(workType)
	st := strings.TrimSpace(sourceType)
	if wt == WBWorkAS || wt == WBWorkMaintenance {
		return false
	}
	if st == "as" || st == "maintenance" {
		return false
	}
	return true
}
