# 고객지원시스템 — Go 웹앱

기획서 `기획_업무일지_프로그램_요구사항.md` 기준으로 구현한 Go + HTMX 웹앱

## 기술 스택

- **Backend**: Go 1.22 + Echo v4
- **Frontend**: HTMX + Alpine.js + Tailwind CSS (CDN)
- **DB**: SQLite (1단계) → PostgreSQL (2단계)
- **배포**: Docker + docker-compose

---

## 맥북에어 개발 시작

### 1. 사전 준비 (최초 1회)

```bash
# Go 설치
brew install go

# 핫리로드 도구 설치
go install github.com/air-verse/air@latest

# 의존성 설치
cd server
go mod tidy
```

### 2. 개발 서버 실행 (핫리로드)

```bash
cd server
cp .env.example .env
air
# → http://localhost:8080
```

### 3. 일반 실행

```bash
go run ./cmd/server
```

---

## Mac Mini 서버 배포 (Docker)

```bash
cd server
docker-compose up -d

# 로그 확인
docker-compose logs -f

# 업데이트 배포
docker-compose down && docker-compose up -d --build
```

맥북에어에서 `http://맥미니IP:8080` 으로 접근

---

## 폴더 구조

```
server/
├── cmd/server/main.go          # 진입점, Echo 라우트
├── internal/
│   ├── config/config.go        # 환경설정
│   ├── model/                  # 도메인 구조체 (§12 기준)
│   ├── repository/             # SQLite CRUD
│   └── handler/                # HTTP 핸들러 (§13 화면 매핑)
├── web/templates/              # Go html/template (HTMX)
│   ├── layout/base.html        # 공통 레이아웃 + 사이드바
│   ├── dashboard.html
│   ├── customer/               # 고객 관리
│   └── as/                     # AS 접수·처리
├── migrations/001_init.sql     # DB 스키마 참조
├── data/app.db                 # SQLite 파일 (자동 생성)
├── Dockerfile
└── docker-compose.yml
```

---

## 구현 현황

| 기능 | 상태 |
|------|------|
| 대시보드 | ✅ |
| 고객 목록/등록/수정/상세 | ✅ |
| AS 접수/목록/처리 | ✅ |
| 공간 관리 (건물/층/실) | 🔲 구현 예정 |
| 담당자 관리 | 🔲 구현 예정 |
| 설치자산 관리 | 🔲 구현 예정 |
| 교체대상 분석 | 🔲 구현 예정 |
| 엑셀 출력 | 🔲 구현 예정 |
| 사용자 인증/권한 | 🔲 구현 예정 |
| PostgreSQL 전환 | 🔲 2단계 |

---

## PostgreSQL 전환 방법

1. `docker-compose.yml`에서 `db` 서비스 주석 해제
2. `.env`에서 `DB_TYPE=postgres`, `DB_DSN=...` 설정
3. `internal/repository/db.go`에서 드라이버 교체:
   - `modernc.org/sqlite` → `github.com/jackc/pgx/v5/stdlib`
4. `go mod tidy && docker-compose up -d --build`
