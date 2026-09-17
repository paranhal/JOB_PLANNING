============================================
  고객지원시스템 (업무일지) - 배포 가이드
============================================

■ 사전 요구사항
  - Ubuntu: Docker Engine + Compose plugin
  - Windows: Docker Desktop

■ 폴더 구성
  server-app.tar     : Docker 이미지
  docker-compose.yml : 호스트 8888 → 컨테이너 8080
  data/              : SQLite DB·업로드
  start.sh / stop.sh : Linux
  start.bat / stop.bat : Windows

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

■ Ubuntu 최초 배포
  1. WinSCP로 deploy 폴더 전체를 서버에 복사
  2. SSH:
       cd /home/sys2/deploy
       sed -i 's/\r$//' start.sh stop.sh
       chmod +x start.sh stop.sh
       ./start.sh
  3. http://공인IP:8888

■ 관리
  중지: ./stop.sh
  로그: docker logs -f server-app-1
============================================
