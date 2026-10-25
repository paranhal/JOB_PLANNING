-- SQLite 는 DROP COLUMN 이 환경에 따라 다름. 열은 남겨 두고 값은 비운다.

UPDATE work_tasks SET acted_by_user_id='', acted_by_name='', on_behalf=0;
UPDATE sales_activities SET acted_by_user_id='', acted_by_name='', on_behalf=0;
UPDATE maintenance_visits SET acted_by_user_id='', acted_by_name='', on_behalf=0;
