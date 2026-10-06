============================================
  고객지원시스템 (업무일지) - 배포 가이드
============================================

■ 사전 요구사항
  - Ubuntu: Docker Engine + Compose plugin
  - Windows: Docker Desktop

■ 폴더 구성
  server-app.tar     : Docker 이미지 (지금 59-G)
  docker-compose.yml : 호스트 8888 → 컨테이너 8080
  data/              : SQLite DB·업로드  (운영 서버에 이미 있으면 올리지 말 것)
  start.sh / stop.sh : Linux
  update.sh          : 앱만 교체
  rollback.sh        : 직전 이미지로 앱만 되돌림
  backup-db.sh       : 운영 DB 파일 백업
  start.bat / stop.bat : Windows

■ Ubuntu 배포 (앱만 교체 — 운영 DB가 이미 있을 때)
  0. (권장) 서버에서 DB 백업
       ssh -p 11800 sys2@112.216.128.139
       cd ~/deploy
       chmod +x backup-db.sh
       ./backup-db.sh 배포전
  1. WinSCP로 아래만 ~/deploy 에 덮어쓰기 (data/ 는 올리지 말 것)
       server-app.tar
       update.sh · rollback.sh · backup-db.sh
       docker-compose.yml 이 서버에 없거나 오래됐으면 같이
     (이미지는 linux/amd64. 맥에서 빌드해도 우분투 x86_64 에서 돈다)
  2. SSH (포트 11800. 주소에 :11800 을 붙이지 말 것):
       ssh -p 11800 sys2@112.216.128.139
       cd ~/deploy
       sed -i 's/\r$//' update.sh rollback.sh backup-db.sh
       chmod +x update.sh rollback.sh backup-db.sh
       ./update.sh
       curl -s localhost:8888/version
  3. version 이 59-G 이고 built·started 가 방금이어야 성공
  4. http://공인IP:8888

■ 앱만 원복 (이번 배포가 이상할 때)
  ./update.sh 가 직전 이미지를 server-app:prev 와 server-app.tar.prev 로 남긴다.
       cd ~/deploy
       ./rollback.sh
       curl -s localhost:8888/version
  data/ 는 그대로다. 새 버전에서 만든 표(컬럼)는 DB에 남아도 예전 앱은 보통 무시한다.
  WinSCP로 tar 를 덮어쓴 뒤에야 처음 update.sh 를 돌리면, 그 시점의 「직전」은
  이미 새 tar 이므로 이번 한 번은 이미지 원복이 안 된다.
  그래서 덮기 전에 서버에서 한 번:
       cp -f server-app.tar server-app.tar.prev
  를 해 두면, 이번 배포도 ./rollback.sh 로 돌아갈 수 있다.

■ DB까지 예전 날짜로 돌리기 (입력 분실)
  앱을 멈춘 뒤 backup-db.sh 가 만든 tar.gz 를 푼다. 그 시각 이후 입력은 사라진다.
       cd ~/deploy && docker compose down
       tar xzf ~/backup/app-날짜-배포전.tar.gz -C data
       docker compose up -d

■ Ubuntu 최초 배포
  1. WinSCP로 deploy 폴더 전체를 서버에 복사
  2. SSH (포트 11800):
       ssh -p 11800 sys2@112.216.128.139
       cd /home/sys2/deploy
       sed -i 's/\r$//' start.sh stop.sh
       chmod +x start.sh stop.sh
       ./start.sh
  3. http://공인IP:8888

■ 관리
  중지: ./stop.sh
  로그: docker logs -f server-app-1
============================================
