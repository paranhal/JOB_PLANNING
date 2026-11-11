-- 95-120P 되돌리기. 사업·업무 표는 건드리지 않는다.

DROP TABLE IF EXISTS project_phases;
DELETE FROM codes WHERE code_group IN ('phase_tpl_build','phase_tpl_develop','phase_tpl_construction');
