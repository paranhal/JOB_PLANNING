============================================
  고객지원시스템 (업무일지) - 배포 가이드
============================================

■ 사전 요구사항
  - Docker Desktop 설치 필요 (https://www.docker.com/products/docker-desktop)
  - 그 외 개발 도구(Go, Node 등)는 필요 없음

■ 폴더 구성
  server-app.tar    : Docker 이미지 파일 (9MB)
  docker-compose.yml: 실행 설정
  start.bat         : Windows 실행 스크립트
  start.sh          : Mac/Linux 실행 스크립트
  stop.bat          : Windows 중지 스크립트
  stop.sh           : Mac/Linux 중지 스크립트

■ Windows에서 실행
  1. deploy 폴더를 원하는 위치에 복사
  2. start.bat 더블클릭
  3. 브라우저에서 http://localhost:8080 접속

■ Mac/Linux에서 실행
  1. deploy 폴더를 원하는 위치에 복사
  2. 터미널에서:
     cd deploy
     chmod +x start.sh stop.sh
     ./start.sh
  3. 브라우저에서 http://localhost:8080 접속

■ 로그인 정보
  관리자 계정: admin / admin
  (첫 로그인 후 비밀번호 변경 권장)

■ 주의사항
  - data/ 폴더에 SQLite DB가 저장됨 (백업 시 이 폴더 보관)
  - Mac에서 Apple Silicon(M1/M2/M3)인 경우
    이미지가 linux/amd64로 빌드되어 있어
    에뮬레이션으로 동작함 (정상 작동, 약간 느릴 수 있음)
  - ARM 네이티브 이미지가 필요하면 Mac에서 직접 빌드:
    git clone https://github.com/paranhal/JOB_PLANNING.git
    cd JOB_PLANNING/server
    docker compose build
    docker compose up -d

■ 서버 관리
  중지: stop.bat (Windows) 또는 ./stop.sh (Mac)
  로그: docker logs server-app-1
  재시작: docker compose restart
============================================
