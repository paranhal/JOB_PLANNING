package repository

import "customer-support/internal/model"

// MeetingTodaySets 업무 회의 Show 와 대시보드가 같이 쓰는 오늘 목록.
func (r *WorkBoardRepo) MeetingTodaySets(date, mineUserID string, mineKeys []string, limit int) (done, sched, progress []model.WorkListItem, err error) {
	done, err = r.ListCompletedOn(date, mineUserID, mineKeys, limit)
	if err != nil {
		return nil, nil, nil, err
	}
	sched, err = r.ListScheduledOn(date, mineUserID, mineKeys, limit)
	if err != nil {
		return nil, nil, nil, err
	}
	progress, err = r.ListBucketOn(model.WorkBucketInProgress, date, mineUserID, mineKeys, limit)
	if err != nil {
		return nil, nil, nil, err
	}
	progress = model.MeetingProgressExcludingScheduled(progress, date)
	return done, sched, progress, nil
}
