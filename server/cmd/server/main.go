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
	_ = godotenv.Load()
	cfg := config.Load()

	db, err := repository.InitDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("DB 초기화 실패: %v", err)
	}
	defer db.Close()

	// 기본 관리자 계정 확보
	userRepo := repository.NewUserRepo(db)
	userRepo.EnsureAdmin(handler.HashPassword("admin"))

	e := echo.New()
	e.HideBanner = true

	e.HTTPErrorHandler = func(err error, c echo.Context) {
		log.Printf("오류 [%s %s]: %v", c.Request().Method, c.Request().URL.Path, err)
		e.DefaultHTTPErrorHandler(err, c)
	}

	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: "[${time_rfc3339}] ${method} ${uri} → ${status} (${latency_human})\n",
	}))
	e.Use(middleware.Recover())

	e.Renderer = handler.NewRenderer()
	e.Static("/static", "web/static")
	e.Static("/uploads", "data/uploads")

	h := handler.New(db)

	// 인증
	e.GET("/login", h.Auth.LoginPage)
	e.POST("/login", h.Auth.Login)
	e.GET("/logout", h.Auth.Logout)

	// 인증 미들웨어 적용 그룹
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)

	g.GET("/", h.Dashboard)

	// 고객 관리
	cust := g.Group("/customers")
	cust.GET("", h.Customer.List)
	cust.GET("/new", h.Customer.New)
	cust.POST("", h.Customer.Create)
	cust.GET("/:id", h.Customer.Show)
	cust.GET("/:id/edit", h.Customer.Edit)
	cust.POST("/:id/update", h.Customer.Update)
	cust.POST("/:id/delete", h.Customer.Delete)

	// 공간 관리 (건물/층/실)
	space := g.Group("/spaces")
	space.GET("", h.Space.List)
	space.POST("/buildings", h.Space.CreateBuilding)
	space.POST("/buildings/:id/update", h.Space.UpdateBuilding)
	space.POST("/buildings/:id/delete", h.Space.DeleteBuilding)
	space.POST("/floors", h.Space.CreateFloor)
	space.POST("/floors/:id/update", h.Space.UpdateFloor)
	space.POST("/floors/:id/delete", h.Space.DeleteFloor)
	space.POST("/rooms", h.Space.CreateRoom)
	space.POST("/rooms/:id/update", h.Space.UpdateRoom)
	space.POST("/rooms/:id/delete", h.Space.DeleteRoom)

	// API: 위치 cascade (HTMX/JSON)
	api := g.Group("/api")
	api.GET("/buildings/:customer_id", h.Space.APIBuildings)
	api.GET("/floors/:building_id", h.Space.APIFloors)
	api.GET("/rooms/:floor_id", h.Space.APIRooms)
	api.GET("/assets/:customer_id", h.Asset.APIAssetsByCustomer)

	// 담당자 관리
	contact := g.Group("/contacts")
	contact.GET("", h.Contact.List)
	contact.GET("/new", h.Contact.New)
	contact.POST("", h.Contact.Create)
	contact.GET("/:id", h.Contact.Show)
	contact.GET("/:id/edit", h.Contact.Edit)
	contact.POST("/:id/update", h.Contact.Update)

	// 담당자 이력
	g.GET("/contact-history", h.ContactHistory.List)
	g.POST("/contact-history", h.ContactHistory.Create)

	// 설치자산 관리
	asset := g.Group("/assets")
	asset.GET("", h.Asset.List)
	asset.GET("/new", h.Asset.New)
	asset.POST("", h.Asset.Create)
	asset.GET("/:id", h.Asset.Show)
	asset.GET("/:id/edit", h.Asset.Edit)
	asset.POST("/:id/update", h.Asset.Update)
	asset.POST("/:id/delete", h.Asset.Delete)

	// SW 상세 관리
	asset.GET("/:asset_id/sw", h.SWDetail.ListByAsset)
	asset.POST("/:asset_id/sw", h.SWDetail.Create)
	g.GET("/sw/:id/edit", h.SWDetail.Edit)
	g.POST("/sw/:id/update", h.SWDetail.Update)
	g.POST("/sw/:id/delete", h.SWDetail.Delete)

	// 수행관계 관리
	rel := g.Group("/relations")
	rel.GET("", h.Relation.List)
	rel.GET("/new", h.Relation.New)
	rel.POST("", h.Relation.Create)
	rel.GET("/:id/edit", h.Relation.Edit)
	rel.POST("/:id/update", h.Relation.Update)
	rel.POST("/:id/delete", h.Relation.Delete)

	// AS 관리
	as := g.Group("/as")
	as.GET("", h.AS.List)
	as.GET("/new", h.AS.New)
	as.POST("", h.AS.Create)
	as.GET("/stats", h.AS.StatsDashboard)
	as.GET("/:id", h.AS.Show)
	as.POST("/:id/update", h.AS.Update)
	as.POST("/:id/process", h.AS.AddProcess)

	// 정기 점검 (기획서 §17) — /maintenance/sites 를 :id 보다 먼저 등록
	ms := g.Group("/maintenance/sites")
	ms.GET("", h.Maintenance.ListSiteConfigs)
	ms.GET("/new", h.Maintenance.NewSiteConfigPage)
	ms.POST("", h.Maintenance.SaveSiteConfig)
	ms.GET("/:customer_id/edit", h.Maintenance.EditSiteConfigPage)
	ms.POST("/:customer_id/delete", h.Maintenance.DeleteSiteConfig)

	maint := g.Group("/maintenance")
	maint.GET("", h.Maintenance.ListPlans)
	maint.GET("/new", h.Maintenance.NewPlanPage)
	maint.POST("", h.Maintenance.CreatePlan)
	maint.POST("/visits/:visit_id/delete", h.Maintenance.DeleteVisit)
	maint.GET("/:id/export.xlsx", h.Maintenance.ExportExcel)
	maint.POST("/:id/generate", h.Maintenance.GenerateAuto)
	maint.POST("/:id/approve", h.Maintenance.ApprovePlan)
	maint.POST("/:id/unapprove", h.Maintenance.UnapprovePlan)
	maint.POST("/:id/delete", h.Maintenance.DeletePlan)
	maint.POST("/:id/visits", h.Maintenance.AddVisit)
	maint.GET("/:id", h.Maintenance.ShowPlan)

	// 분석/영업
	g.GET("/analysis", h.Analysis.Dashboard)

	// 코드 관리
	g.GET("/codes", h.Code.List)
	g.POST("/codes", h.Code.Create)
	g.POST("/codes/:id/update", h.Code.Update)
	g.POST("/codes/:id/delete", h.Code.Delete)

	// 첨부파일
	g.POST("/attachments", h.Attachment.Upload)
	g.GET("/attachments/:id", h.Attachment.Download)
	g.POST("/attachments/:id/delete", h.Attachment.Delete)

	// 사용자 관리
	g.GET("/users", h.Auth.UserList)
	g.POST("/users", h.Auth.UserCreate)
	g.POST("/users/:id/update", h.Auth.UserUpdate)

	log.Printf("고객지원시스템 서버 시작: http://localhost:%s", cfg.Port)
	if err := e.Start(":" + cfg.Port); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
