#!/bin/sh
# 이미지를 만들고 deploy/server-app.tar 로 저장한다. 버전은 VERSION 파일, 시각은 지금.
# git 이 없어도 된다 — commit 은 nogit. §40.3
set -e
cd "$(dirname "$0")/.."
VERSION=$(cat VERSION 2>/dev/null || echo dev)
VERSION=$(printf '%s' "$VERSION" | tr -d '\r\n')
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo nogit)
BUILD_TIME=$(date '+%Y-%m-%d %H:%M')

echo "빌드: VERSION=$VERSION COMMIT=$COMMIT BUILD_TIME=$BUILD_TIME platform=linux/amd64"

# 증명(attestation)을 빼지 않으면 tar 가 빈 이미지로 저장돼 서버에서
# exec ./server: no such file or directory 가 난다.
export BUILDX_NO_DEFAULT_ATTESTATIONS=1
docker build \
  --platform linux/amd64 \
  --provenance=false \
  --sbom=false \
  --build-arg VERSION="$VERSION" \
  --build-arg COMMIT="$COMMIT" \
  --build-arg BUILD_TIME="$BUILD_TIME" \
  -t server-app:latest \
  -f server/Dockerfile \
  server

echo "이미지 확인:"
docker image inspect server-app:latest --format 'os={{.Os}} arch={{.Architecture}}'
docker run --rm --platform linux/amd64 --entrypoint /bin/ls server-app:latest -l /app/server

docker save server-app:latest -o deploy/server-app.tar
echo "저장: deploy/server-app.tar"
