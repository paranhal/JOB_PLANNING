package handler

import (
	"strings"
	"time"

	"customer-support/internal/model"
)

type TravelEstimator interface {
	Estimate(origin, dest string, at time.Time) (minutes int, ok bool)
}

type ManualEstimator struct{}

func (ManualEstimator) Estimate(origin, dest string, at time.Time) (int, bool) {
	return 0, false
}

func FitCheck(workerID string, now time.Time, interruptMin, travelBefore, travelAfter int, nextFixed *time.Time, nextTitle string) model.FitCheckResult {
	need := interruptMin + model.ClampTravelMin(travelBefore) + model.ClampTravelMin(travelAfter)
	out := model.FitCheckResult{NeedMin: need, NextFixedTitle: strings.TrimSpace(nextTitle), Choices: model.FitCheckChoices()}
	if nextFixed == nil {
		out.AvailMin = 24 * 60
		out.OK = true
		return out
	}
	avail := int(nextFixed.Sub(now).Minutes())
	if avail < 0 {
		avail = 0
	}
	out.AvailMin = avail
	if avail >= need {
		out.OK = true
		return out
	}
	out.ShortMin = need - avail
	return out
}

func travelRequiredForInterrupt(ad LiveAdapter, t *model.WorkTask, fieldChecked bool) bool {
	if fieldChecked {
		return true
	}
	if t != nil && ad.IsFieldWork(t) {
		return true
	}
	return false
}
