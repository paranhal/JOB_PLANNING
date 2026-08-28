-- 024 수집함(inbox) → 할 일(waiting). §33.5.1
-- 개발 DB는 0건이어도 운영 DB에 남을 수 있다. 실제 적용은 InitDB → applyInboxToWaiting.

UPDATE work_tasks SET status='waiting' WHERE status='inbox';
