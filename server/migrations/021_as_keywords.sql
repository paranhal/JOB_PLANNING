-- 021 AS 키워드 사전·연결·불용어 (§12.11.6)
-- 초기 키워드는 이 표가 출처다. 코드에 목록을 박지 않는다. 담당자가 화면에서 늘린다.
-- 자동 확정하지 않는다. 후보는 제안만 하고 사람이 체크한다.

CREATE TABLE IF NOT EXISTS as_keywords (
  keyword_id TEXT PRIMARY KEY,
  keyword    TEXT NOT NULL,
  kw_group   TEXT NOT NULL,
  synonyms   TEXT NOT NULL DEFAULT '',
  is_active  INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  note       TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS as_keyword_links (
  as_id      TEXT NOT NULL,
  keyword_id TEXT NOT NULL,
  source     TEXT NOT NULL,
  field      TEXT NOT NULL,
  PRIMARY KEY (as_id, keyword_id, field),
  FOREIGN KEY (as_id) REFERENCES as_receipts(as_id),
  FOREIGN KEY (keyword_id) REFERENCES as_keywords(keyword_id)
);

CREATE TABLE IF NOT EXISTS as_keyword_stopwords (
  word TEXT PRIMARY KEY,
  note TEXT NOT NULL DEFAULT ''
);

INSERT OR IGNORE INTO as_keywords(keyword_id, keyword, kw_group, synonyms, is_active, sort_order, note) VALUES
('KW001','홈페이지','대상','누리집',1,10,''),
('KW002','전자도서관','대상','',1,20,''),
('KW003','자료관리시스템','대상','KLAS',1,30,''),
('KW004','상호대차','대상','책이음',1,40,''),
('KW005','무인예약','대상','',1,50,''),
('KW006','자가대출반납기','대상','',1,60,''),
('KW007','키오스크','대상','',1,70,''),
('KW008','서버','대상','',1,80,''),
('KW009','WAS','대상','',1,90,''),
('KW010','DB','대상','',1,100,''),
('KW011','방화벽','대상','',1,110,''),
('KW012','오류','증상','',1,10,''),
('KW013','장애','증상','',1,20,''),
('KW014','접속불가','증상','',1,30,''),
('KW015','팝업','증상','',1,40,''),
('KW016','검색안됨','증상','',1,50,''),
('KW017','로그인','증상','',1,60,''),
('KW018','속도지연','증상','',1,70,''),
('KW019','출력','증상','',1,80,''),
('KW020','인식불가','증상','',1,90,''),
('KW021','패치누락','원인','',1,10,''),
('KW022','설정오류','원인','',1,20,''),
('KW023','네트워크','원인','',1,30,''),
('KW024','전원','원인','',1,40,''),
('KW025','소모품','원인','',1,50,''),
('KW026','사용자오류','원인','',1,60,''),
('KW027','데이터오류','원인','',1,70,''),
('KW028','재기동','조치','재시작',1,10,''),
('KW029','패치','조치','',1,20,''),
('KW030','설정변경','조치','',1,30,''),
('KW031','교체','조치','',1,40,''),
('KW032','원격지원','조치','',1,50,''),
('KW033','방문','조치','',1,60,''),
('KW034','자료송부','조치','',1,70,''),
('KW035','안내','조치','',1,80,''),
('KW036','이관','조치','',1,90,'');

INSERT OR IGNORE INTO as_keyword_stopwords(word, note) VALUES
('안녕하세요','인사말'),
('안녕하십니까','인사말'),
('감사합니다','인사말'),
('고맙습니다','인사말'),
('부탁드립니다','인사말'),
('부탁드리겠습니다','인사말'),
('수고하세요','인사말'),
('수고하셨습니다','인사말'),
('확인부탁드립니다','인사말'),
('드립니다','인사말'),
('입니다','조사·어미'),
('습니다','조사·어미'),
('합니다','조사·어미'),
('됩니다','조사·어미'),
('해주세요','조사·어미'),
('그리고','접속'),
('또는','접속'),
('kr','URL 조각'),
('go','URL 조각'),
('com','URL 조각'),
('www','URL 조각'),
('http','URL 조각'),
('https','URL 조각'),
('co','URL 조각'),
('ne','URL 조각'),
('ac','URL 조각'),
('041','전화번호 조각'),
('044','전화번호 조각'),
('02','전화번호 조각'),
('010','전화번호 조각');

INSERT INTO id_sequences(seq_key, last_no) VALUES ('as_keyword', 36)
ON CONFLICT(seq_key) DO UPDATE SET last_no=excluded.last_no
WHERE id_sequences.last_no < excluded.last_no;
