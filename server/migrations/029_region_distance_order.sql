-- 기획서 §34.4.3 지역 거리 순서표
-- 실제 앱은 InitDB 시 repository.applyRegionDistanceOrder 가 동일 내용을 실행한다.

CREATE TABLE IF NOT EXISTS region_distance_order (
    sido       TEXT NOT NULL,
    sigungu    TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (sido, sigungu)
);
