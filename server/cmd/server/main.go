package main

import (
	"log"
	"net/http"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"customer-support/internal/config"
	"customer-support/internal/handler"
	"customer-support/internal/repository"
)

func main() {
	// .env 로드 (없어도 무시)
	_ = godotenv.Load()

	cfg := config.Load()

	// DB 초기화
	db, err := repository.InitDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("DB 초기화 실패: %v", err)
	}
	defer db.Close()

	e := echo.New()
	e.HideBanner = true

	// 에러 핸들러 — 500 오류 원인을 로그에 출력
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		log.Printf("❌ 오류 [%s %s]: %v", c.Request().Method, c.Request().URL.Path, err)
		e.DefaultHTTPErrorHandler(err, c)
	}

	// 미들웨어
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: "[${time_rfc3339}] ${method} ${uri} → ${status} (${latency_human})\n",
	}))
	e.Use(middleware.Recover())

	// 템플릿 렌더러
	e.Renderer = handler.NewRenderer()

	// 정적 파일
	e.Static("/static", "web/static")

	// 핸들러 초기화
	h := handler.New(db)

	// ── 라우트 ──────────────────────────────────────
	e.GET("/", h.Dashboard)

	// 고객 관리
	cust := e.Group("/customers")
	cust.GET("", h.Customer.List)
	cust.GET("/new", h.Customer.New)
	cust.POST("", h.Customer.Create)
	cust.GET("/:id", h.Customer.Show)
	cust.GET("/:id/edit", h.Customer.Edit)
	cust.POST("/:id/update", h.Customer.Update)
	cust.POST("/:id/delete", h.Customer.Delete)

	// 공간 관리 (건물/층/실)
	space := e.Group("/spaces")
	space.GET("", h.Space.List)

	// 담당자 관리
	contact := e.Group("/contacts")
	contact.GET("", h.Contact.List)
	contact.GET("/new", h.Contact.New)
	contact.POST("", h.Contact.Create)
	contact.GET("/:id", h.Contact.Show)

	// 설치자산 관리
	asset := e.Group("/assets")
	asset.GET("", h.Asset.List)
	asset.GET("/new", h.Asset.New)
	asset.POST("", h.Asset.Create)
	asset.GET("/:id", h.Asset.Show)

	// AS 관리
	as := e.Group("/as")
	as.GET("", h.AS.List)
	as.GET("/new", h.AS.New)
	as.POST("", h.AS.Create)
	as.GET("/:id", h.AS.Show)
	as.POST("/:id/update", h.AS.Update)

	// 분석/영업
	e.GET("/analysis", h.Analysis.Dashboard)

	// 코드 관리
	e.GET("/codes", h.Code.List)

	// ── 서버 시작 ─────────────────────────────────
	log.Printf("🚀 고객지원시스템 서버 시작: http://localhost:%s", cfg.Port)
	if err := e.Start(":" + cfg.Port); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
