# ── 1단계: 빌드 ───────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /app

# 의존성 먼저 캐시 (go.sum이 없어도 빌드 가능)
COPY go.mod ./
RUN go mod download

# 소스 복사 및 빌드
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o server ./cmd/server

# ── 2단계: 실행 이미지 (~20MB) ────────────────────────────────────
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata
ENV TZ=Asia/Seoul

WORKDIR /app

# 빌드 결과물 복사
COPY --from=builder /app/server .
COPY web/ web/
COPY migrations/ migrations/

# 데이터 디렉토리 (SQLite 파일 저장용)
RUN mkdir -p data

EXPOSE 8080

CMD ["./server"]
