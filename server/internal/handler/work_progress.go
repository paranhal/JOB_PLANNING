package handler

import (
	"strconv"
	"strings"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func ParentProgress(repo *repository.WBRepo, parentID string) (model.ParentProgress, error) {
	var out model.ParentProgress
	if repo == nil || strings.TrimSpace(parentID) == "" {
		return out, nil
	}
	children, err := repo.ListChildren(parentID)
	if err != nil {
		return out, err
	}
	var occ []model.WorkTask
	ids := []string{parentID}
	for _, ch := range children {
		if ch.RecurrenceRole != model.RecurrenceRoleOccurrence {
			continue
		}
		occ = append(occ, ch)
		ids = append(ids, ch.TaskID)
	}
	bySeg, err := repo.ListSegmentsByTaskIDs(ids)
	if err != nil {
		return out, err
	}
	denom := 0
	completeEst := 0
	for i := range occ {
		_ = repo.LoadTaskLiveFields(&occ[i])
		st := model.NormalizeOccurrenceStatus(occ[i].OccurrenceStatus)
		if st == model.OccurrenceSkipped {
			continue
		}
		denom++
		est := occ[i].EstMinutes
		out.EstMin += est
		actual := sumClosedActual(bySeg[occ[i].TaskID])
		out.ActualMin += actual
		out.TravelMin += sumTravelBefore(bySeg[occ[i].TaskID])
		switch st {
		case model.OccurrenceComplete:
			out.Done++
			completeEst += est
		case model.OccurrenceInProgress:
			out.InProgress++
			out.RemainingMin += remainingEst(est, actual, occ[i])
		case model.OccurrenceHold:
			out.Held++
			out.RemainingMin += remainingEst(est, actual, occ[i])
		case model.OccurrenceDeferred:
			out.Deferred++
			out.RemainingMin += remainingEst(est, actual, occ[i])
		default:
			out.Planned++
			out.RemainingMin += remainingEst(est, actual, occ[i])
		}
		ic, im := interruptStats(bySeg[occ[i].TaskID], repo)
		out.InterruptCount += ic
		out.InterruptMin += im
	}
	out.Total = denom
	out.OccupyMin = out.ActualMin + out.TravelMin
	if denom > 0 {
		p := out.Done * 100 / denom
		out.ByCountPct = &p
	}
	if out.EstMin > 0 {
		p := completeEst * 100 / out.EstMin
		out.ByTimePct = &p
	}
	return out, nil
}

func remainingEst(est, actual int, t model.WorkTask) int {
	if t.EstMinutes > 0 && actual == 0 && t.OccurrenceStatus == model.OccurrenceHold {
		// 나중에 재개 때 남은 예상은 별도 저장이 있으면 우선. next_action 경로의 remaining 은 departure.
	}
	n := est - actual
	if n < 0 {
		return 0
	}
	return n
}

func sumClosedActual(segs []model.WorkTaskSegment) int {
	n := 0
	for _, s := range segs {
		if model.NormalizeSegmentKind(s.Kind) != model.SegKindActual {
			continue
		}
		if strings.TrimSpace(s.EndTime) == "" {
			continue
		}
		n += s.DurationMin
	}
	return n
}

func sumTravelBefore(segs []model.WorkTaskSegment) int {
	n := 0
	for _, s := range segs {
		if model.NormalizeSegmentKind(s.Kind) != model.SegKindActual {
			continue
		}
		n += model.ClampTravelMin(s.TravelBeforeMin)
	}
	return n
}

func interruptStats(segs []model.WorkTaskSegment, repo *repository.WBRepo) (count, min int) {
	seen := map[string]bool{}
	for _, s := range segs {
		id := strings.TrimSpace(s.InterruptTaskID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		count++
		if repo != nil {
			min += repo.SumActualMin(id)
		}
	}
	return count, min
}

func TaskTimeSummary(repo *repository.WBRepo, taskID string) (model.TaskTimeSummary, error) {
	var out model.TaskTimeSummary
	if repo == nil || strings.TrimSpace(taskID) == "" {
		return out, nil
	}
	segs, err := repo.ListActualSegments(taskID)
	if err != nil {
		return out, err
	}
	first := -1
	last := -1
	for _, s := range segs {
		out.ActualMin += s.DurationMin
		st := model.ParseHHMMMinutes(s.StartTime)
		en := model.ParseHHMMMinutes(s.EndTime)
		if st >= 0 && (first < 0 || st < first) {
			first = st
		}
		if en >= 0 && en > last {
			last = en
		}
	}
	out.TravelMin = repo.SumTravel(taskID)
	out.OccupyMin = out.ActualMin + out.TravelMin
	if first >= 0 && last > first {
		out.ElapsedMin = last - first
		out.PausedMin = out.ElapsedMin - out.ActualMin
		if out.PausedMin < 0 {
			out.PausedMin = 0
		}
	}
	return out, nil
}

func progressPctDash(p *int) string {
	if p == nil {
		return "—"
	}
	return strconv.Itoa(*p) + "%"
}
