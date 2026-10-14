package config

import (
	"os"
	"strings"
)

type Config struct {
	Port              string
	DBType            string
	DBPath            string
	DBDSN             string
	JWTSecret         string
	AppEnv            string
	CookieSecure      bool
	IntegrationAPIKey string
	MailEnabled       bool
	MailHost          string
	MailPort          string
	MailUser          string
	MailPass          string
	MailFromName      string
	MailAdminTo       string
	SMSEnabled        bool
	SMSAPIURL         string
	SMSAPIKey         string
	SMSFrom           string
	KakaoEnabled      bool
}

func Load() *Config {
	return &Config{
		Port:              getEnv("PORT", "8080"),
		DBType:            getEnv("DB_TYPE", "sqlite"),
		DBPath:            getEnv("DB_PATH", "data/app.db"),
		DBDSN:             getEnv("DB_DSN", ""),
		JWTSecret:         getEnv("JWT_SECRET", "dev-secret-change-in-production"),
		AppEnv:            getEnv("APP_ENV", "development"),
		CookieSecure:      envBool("COOKIE_SECURE"),
		IntegrationAPIKey: getEnv("INTEGRATION_API_KEY", ""),
		MailEnabled:       envBool("MAIL_ENABLED"),
		MailHost:          getEnv("MAIL_HOST", "smtp.gmail.com"),
		MailPort:          getEnv("MAIL_PORT", "587"),
		MailUser:          getEnv("MAIL_USER", ""),
		MailPass:          getEnv("MAIL_PASS", ""),
		MailFromName:      getEnv("MAIL_FROM_NAME", "비젼아이티 고객지원시스템"),
		MailAdminTo:       getEnv("MAIL_ADMIN_TO", ""),
		SMSEnabled:        envBool("SMS_ENABLED"),
		SMSAPIURL:         getEnv("SMS_API_URL", ""),
		SMSAPIKey:         getEnv("SMS_API_KEY", ""),
		SMSFrom:           getEnv("SMS_FROM", ""),
		KakaoEnabled:      envBool("KAKAO_ENABLED"),
	}
}

func envBool(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func (c *Config) IsDev() bool {
	return c.AppEnv == "development"
}

// ShowProjectsSector 사업관리 메뉴·초안 화면.
// 로컬(노트북·server/docker-compose)은 기본으로 연다. APP_ENV=production 이어도 끄지 않는다.
// 운영 서버 배포(deploy/docker-compose)에서만 SHOW_PROJECTS_SECTOR=0 으로 숨긴다.
func (c *Config) ShowProjectsSector() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SHOW_PROJECTS_SECTOR"))) {
	case "0", "false", "no", "off":
		return false
	}
	return true
}

// ShowProjectExampleData 사업관리 화면의 설계용 예시(가상 인명·공정).
// 노트북 개발에서만 기본으로 켠다. 운영(APP_ENV=production)이나 PROJECT_EXAMPLE_DATA=0 이면 빈 화면.
func (c *Config) ShowProjectExampleData() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PROJECT_EXAMPLE_DATA"))) {
	case "0", "false", "no", "off":
		return false
	case "1", "true", "yes", "on":
		return true
	}
	return !strings.EqualFold(strings.TrimSpace(c.AppEnv), "production")
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
