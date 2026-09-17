-- 050 롤백. SQLite 는 열 삭제가 제한적이므로 스키마 번호만 되돌린다.
-- 이름 열은 남겨 두었으므로 읽기 코드만 되돌리면 된다. §44.10

DELETE FROM schema_migrations WHERE version = 50;
