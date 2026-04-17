# 맥 미니에서 Docker 설치 및 서버 실행 가이드

이 문서는 **Mac Mini**에 Docker를 설치하고, 고객지원시스템 Go 웹앱(`server/`)을 Docker로 실행하는 순서와 방법을 정리한 것입니다.

---

## 1. Docker 설치 (맥 미니, 최초 1회)

### 1-1. 설치 방법 선택

| 방법 | 설명 |
|------|------|
| **A. Docker Desktop (권장)** | GUI 제공, 가장 쉬움. Apple Silicon(M1/M2/M3) 및 Intel 맥 지원. |
| **B. Colima + Docker CLI** | 터미널만 사용, 리소스 적게 씀. |

---

### 1-2. 방법 A: Docker Desktop 설치

1. **다운로드**
   - https://www.docker.com/products/docker-desktop/
   - **Mac with Apple chip** 또는 **Mac with Intel chip** 중 본인 맥 미니에 맞게 선택 후 다운로드.

2. **설치**
   - 다운로드한 `.dmg` 열기 → **Docker 아이콘을 Applications 폴더로 드래그**.

3. **실행 및 권한**
   - **Applications**에서 **Docker** 실행.
   - 최초 실행 시 “Docker가 시스템 확장을 설치하려고 합니다” → **승인** 후 맥 비밀번호 입력.
   - 필요 시 **재시동** 안내가 나오면 재시동.

4. **동작 확인**
   - 상단 메뉴바에 고래 아이콘이 보이면 실행된 것.
   - 터미널에서:
     ```bash
     docker --version
     docker compose version
     ```
     버전이 나오면 설치 완료.

---

### 1-3. 방법 B: Colima + Docker CLI (선택)

Homebrew가 있을 때:

```bash
brew install docker colima
colima start
docker --version
docker compose version
```

---

## 2. 프로젝트 서버 실행 순서 (맥 미니)

Docker 설치가 끝난 뒤, **아래 순서**대로 진행하면 됩니다.

### 2-1. 프로젝트로 이동

```bash
cd /Users/zinilee/Projects/JOB_PLANNING/server
```

(맥 미니에서 실제 경로가 다르면 그 경로로 `cd` 하세요.)

### 2-2. 환경 설정 파일 생성 (최초 1회)

**반드시 `server` 폴더 안에서** 실행하세요.

```bash
# 현재 위치 확인 (server 폴더여야 함)
pwd
# 예: /Users/본인사용자명/Projects/JOB_PLANNING/server

# .env.example 있는지 확인
ls -la .env.example

# 없으면 "No such file" → 상위에서 server로 이동
cd /Users/본인사용자명/Projects/JOB_PLANNING/server

# 복사
cp .env.example .env
```

- `"No such file or directory"` 나오면 → **경로가 잘못된 것**입니다. `cd`로 `JOB_PLANNING/server` 폴더로 들어간 뒤 다시 실행하세요.
- 필요하면 `.env`에서 `PORT`, `DB_PATH` 등 수정. 기본값만 써도 동작합니다.

### 2-3. 데이터 디렉터리 확인

SQLite DB가 `./data`에 저장되므로 폴더가 있어야 합니다. 없으면:

```bash
mkdir -p data
```

### 2-4. Docker로 서버 실행

```bash
docker compose up -d
```

- `-d`: 백그라운드 실행.
- 최초 실행 시 이미지 빌드 후 컨테이너가 올라갑니다.
- 접속 주소: **http://맥미니IP:8080** 또는 **http://localhost:8080** (맥 미니에서 직접 접속 시).

### 2-5. 동작 확인

```bash
# 로그 보기
docker compose logs -f

# 컨테이너 상태
docker compose ps
```

브라우저에서 `http://localhost:8080` (또는 맥 미니 IP:8080) 접속해 대시보드가 보이면 성공입니다.

---

## 3. 자주 쓰는 명령어

| 목적 | 명령어 |
|------|--------|
| 서버 시작 (백그라운드) | `docker compose up -d` |
| 로그 보기 | `docker compose logs -f` |
| 서버 중지 | `docker compose down` |
| 코드/설정 변경 후 재배포 | `docker compose down && docker compose up -d --build` |
| 컨테이너 상태 확인 | `docker compose ps` |

---

## 4. 다른 PC에서 접속 (맥북에어 등)

- 맥 미니 IP 확인: 맥 미니 터미널에서 `ifconfig | grep "inet "` 또는 **시스템 설정 → 네트워크**.
- 같은 네트워크의 다른 PC 브라우저에서: **http://맥미니IP:8080**.

---

## 5. 문제 해결

| 현상 | 확인 사항 |
|------|------------|
| `docker: command not found` | Docker Desktop이 실행 중인지, 터미널을 재실행했는지 확인. |
| `port is already allocated` | 8080을 쓰는 다른 프로그램 종료하거나, `.env`에서 `PORT=8081` 등으로 변경 후 `docker compose` 재실행. |
| DB 오류 | `server/data` 폴더 권한 확인. `chmod 755 data` 후 `docker compose up -d --build`. |

---

## 요약: 맥 미니에서 할 일 순서

1. **Docker 설치** (Docker Desktop 또는 Colima).
2. `cd /Users/zinilee/Projects/JOB_PLANNING/server`
3. `cp .env.example .env` (최초 1회).
4. `mkdir -p data` (없을 때만).
5. `docker compose up -d`
6. 브라우저에서 **http://localhost:8080** 또는 **http://맥미니IP:8080** 접속.

이 순서대로 하면 이 프로젝트의 서버를 맥 미니에서 Docker로 실행할 수 있습니다.
