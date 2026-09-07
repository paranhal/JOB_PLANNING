-- AS 원인분류 3단계 시드 v2 (출처: CRM AS 조치 원인분류종합20260831.xlsx · 2026-09-02 개정본)
-- 1차 4 · 2차 24 · 3차 27 = 총 55행
-- v1 대비 변경: 서버 2차에 '패치' 추가(4→5종), 라벨 정정(HW고장→HW 장애 등), 홈페이지 띄어쓰기
-- cause_type_map: 기존 as_receipts.cause_type 으로 자동 채울 값. 빈 문자열이면 매핑 없음.
-- is_fault=0: 고장이 아닌 요청·작업 유형(§17 장애빈도에서 제외)

CREATE TABLE IF NOT EXISTS as_cause_categories (
  code            TEXT PRIMARY KEY,
  level           INTEGER NOT NULL,
  parent_code     TEXT,
  label           TEXT NOT NULL,
  sort_order      INTEGER NOT NULL DEFAULT 0,
  cause_type_map  TEXT NOT NULL DEFAULT '',
  is_active       INTEGER NOT NULL DEFAULT 1,
  is_fault        INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_as_cause_parent ON as_cause_categories(parent_code, sort_order);


INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid',1,NULL,'RFID',10,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.hw',2,'rfid','HW',10,'hw',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.hw.part_check',3,'rfid.hw','부품 점검',10,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.hw.part_replace',3,'rfid.hw','부품/소모품 교체',20,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.hw.config',3,'rfid.hw','설정 변경',30,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.hw.transient',3,'rfid.hw','단발성 오류',40,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.hw.etc',3,'rfid.hw','기타',50,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.sw',2,'rfid','SW',20,'sw',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.sw.program_fix',3,'rfid.sw','프로그램 수정',10,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.sw.data_fix',3,'rfid.sw','데이터 변경',20,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.sw.program_config',3,'rfid.sw','프로그램 설정 변경',30,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.sw.transient',3,'rfid.sw','단발성 오류',40,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.sw.etc',3,'rfid.sw','기타',50,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.net',2,'rfid','서버/네트워크',30,'network',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.net.server_conn',3,'rfid.net','서버 접속 오류',10,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.net.network_conn',3,'rfid.net','네트워크 접속 오류',20,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.net.etc',3,'rfid.net','기타',30,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.env',2,'rfid','환경문제',40,'env',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.env.booth',3,'rfid.env','부스',10,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.env.facility',3,'rfid.env','기관 시설',20,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.user',2,'rfid','사용자/도서',50,'user',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.user.training',3,'rfid.user','교육',10,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.user.book_damage',3,'rfid.user','도서 파손/분실',20,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.user.book_feed',3,'rfid.user','도서 투입/배출',30,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.install',2,'rfid','설치',60,'',1,0);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.install.new',3,'rfid.install','신규 납품',10,'',1,0);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.install.move',3,'rfid.install','이동',20,'',1,0);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.install.dispose',3,'rfid.install','폐기',30,'',1,0);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.etc',2,'rfid','기타',70,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.etc.quote',3,'rfid.etc','견적서 요청',10,'',1,0);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.etc.contract',3,'rfid.etc','계약 관련',20,'',1,0);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('rfid.etc.etc',3,'rfid.etc','기타',30,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('klas',1,NULL,'KLAS',20,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('klas.book_stats',2,'klas','도서관련 및 통계',10,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('klas.ill',2,'klas','상호대차',20,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('klas.opac',2,'klas','자료검색대',30,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('klas.patron',2,'klas','이용자관련',40,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('klas.inventory',2,'klas','장서점검',50,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('klas.etc',2,'klas','기타',60,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('klas.etc.rfid',3,'klas.etc','RFID',10,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('klas.etc.seat',3,'klas.etc','좌석관리시스템',20,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('klas.etc.etc',3,'klas.etc','기타',30,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('web',1,NULL,'홈페이지',30,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('web.post_edit',2,'web','게시물 수정',10,'',1,0);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('web.menu_edit',2,'web','메뉴 수정',20,'',1,0);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('web.popup',2,'web','팝업 요청',30,'',1,0);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('web.service_error',2,'web','서비스 오류',40,'sw',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('web.inquiry',2,'web','단순문의',50,'',1,0);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('web.etc',2,'web','기타',60,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('server',1,NULL,'서버',40,'',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('server.hw',2,'server','HW 장애',10,'hw',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('server.sw',2,'server','SW 장애',20,'sw',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('server.network',2,'server','네트워크 장애',30,'network',1,1);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('server.patch',2,'server','패치',40,'sw',1,0);
INSERT OR REPLACE INTO as_cause_categories (code,level,parent_code,label,sort_order,cause_type_map,is_active,is_fault) VALUES ('server.etc',2,'server','기타',50,'',1,1);

-- v1 시드를 이미 넣었다면: 아래로 사라진 코드를 비활성화한다 (데이터는 지우지 않는다)
UPDATE as_cause_categories SET is_active=0 WHERE code NOT IN ('rfid','rfid.hw','rfid.hw.part_check','rfid.hw.part_replace','rfid.hw.config','rfid.hw.transient','rfid.hw.etc','rfid.sw','rfid.sw.program_fix','rfid.sw.data_fix','rfid.sw.program_config','rfid.sw.transient','rfid.sw.etc','rfid.net','rfid.net.server_conn','rfid.net.network_conn','rfid.net.etc','rfid.env','rfid.env.booth','rfid.env.facility','rfid.user','rfid.user.training','rfid.user.book_damage','rfid.user.book_feed','rfid.install','rfid.install.new','rfid.install.move','rfid.install.dispose','rfid.etc','rfid.etc.quote','rfid.etc.contract','rfid.etc.etc','klas','klas.book_stats','klas.ill','klas.opac','klas.patron','klas.inventory','klas.etc','klas.etc.rfid','klas.etc.seat','klas.etc.etc','web','web.post_edit','web.menu_edit','web.popup','web.service_error','web.inquiry','web.etc','server','server.hw','server.sw','server.network','server.patch','server.etc');
