============================================
  고객지원시스템 (업무일지) - 배포 가이드
============================================

■ 사전 요구사항
  - Ubuntu 서버에 Docker Engine + Compose plugin 설치
  - 그 외 개발 도구(Go 등)는 필요 없음

■ 폴더 구성
  server-app.tar     : Docker 이미지 (최신 앱)
  docker-compose.yml : 실행 설정 (호스트 8888 → 컨테이너 8080)
  data/              : SQLite DB·업로드 파일 (배포 시점 데이터)
  start.sh / stop.sh : Linux 시작·중지

■ Ubuntu 배포 (앱만 교체 — 운영 DB가 이미 있을 때)
  1. WinSCP로 server-app.tar 만 ~/deploy 에 덮어쓰기 (data/ 는 올리지 말 것)
     docker-compose.yml · update.sh 가 서버에 없거나 오래됐으면 같이 덮어쓴다
  2. SSH:
       cd ~/deploy
       sed -i 's/\r$//' update.sh
       chmod +x update.sh
       ./update.sh
       curl -s localhost:8888/version
  3. version 이 44-G 이고 built·started 가 방금이어야 성공
  4. http://공인IP:8888

■ 데이터
  - data/app.db 가 실제 업무 데이터입니다.
  - 배포 전 개발 PC의 server/data 를 checkpoint 후 복사한 상태여야
    최신 데이터가 반영됩니다.

■ 관리
  중지: ./stop.sh
  로그: docker logs -f server-app-1
  재시작: docker compose up -d
============================================
