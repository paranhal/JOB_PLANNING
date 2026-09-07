-- 롤백 §34.3.3 시드 v2. 데이터는 지우지 않고 패치만 비활성화·라벨 원복.

UPDATE as_cause_categories SET is_active=0 WHERE code='server.patch';
UPDATE as_cause_categories SET label='HW고장' WHERE code='server.hw';
UPDATE as_cause_categories SET label='SW오류' WHERE code='server.sw';
UPDATE as_cause_categories SET label='네트워크' WHERE code='server.network';
UPDATE as_cause_categories SET label='게시물수정' WHERE code='web.post_edit';
UPDATE as_cause_categories SET label='메뉴수정' WHERE code='web.menu_edit';
UPDATE as_cause_categories SET label='팝업요청' WHERE code='web.popup';
