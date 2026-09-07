-- 021 롤백. 키워드 사전·연결·불용어만 지운다. 접수·조치는 그대로 둔다.

DROP TABLE IF EXISTS as_keyword_links;
DROP TABLE IF EXISTS as_keyword_stopwords;
DROP TABLE IF EXISTS as_keywords;
DELETE FROM id_sequences WHERE seq_key='as_keyword';
