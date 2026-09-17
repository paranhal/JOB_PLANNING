# ── 1단계: 빌드 ───────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /app

# 의존성 먼저 캐시 (go.sum으로 버전을 고정한다)
COPY go.mod go.sum ./
RUN go mod download

# 소스 복사 및 빌드. 버전·커밋·시각은 빌드 스크립트가 ARG 로 넣는다. §40.3
COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X 'main.buildVersion=${VERSION}' -X 'main.buildCommit=${COMMIT}' -X 'main.buildTime=${BUILD_TIME}'" \
    -o server ./cmd/server

# ── 2단계: 실행 이미지 (~20MB) ────────────────────────────────────
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata
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
