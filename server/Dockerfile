# ── 1단계: 빌드 ───────────────────────────────────────────────────
# 맥(arm64)에서 우분투(x86_64)용으로 뽑는다. --platform 은 build.sh 가 넣는다.
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

WORKDIR /app

# 의존성 먼저 캐시 (go.sum으로 버전을 고정한다)
COPY go.mod go.sum ./
RUN go mod download

# 소스 복사 및 빌드. 버전·커밋·시각은 빌드 스크립트가 ARG 로 넣는다. §40.3
COPY . .
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -buildmode=exe \
    -ldflags="-s -w -extldflags '-static' -X 'main.buildVersion=${VERSION}' -X 'main.buildCommit=${COMMIT}' -X 'main.buildTime=${BUILD_TIME}'" \
    -o server ./cmd/server

# ── 2단계: 실행 이미지 ────────────────────────────────────────────
# 맥에서 amd64 로 뽑은 Go 바이너리가 glibc 를 찾는다. Alpine(musl) 에서는
# exec ./server: no such file or directory 가 난다.
FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*
ENV TZ=Asia/Seoul

WORKDIR /app

# 빌드 결과물 복사
COPY --from=builder /app/server .
# web/static/vendor 포함: 고정 버전 Tailwind CSS · Alpine · htmx (CDN 없음)
COPY web/ web/
COPY migrations/ migrations/
COPY templates/ templates/

# 데이터 디렉토리 (SQLite 파일 저장용)
RUN mkdir -p data

EXPOSE 8080

CMD ["./server"]
