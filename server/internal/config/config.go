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

// ShowProjectsSector 사이드바 사업관리 메뉴 노출만 정한다.
// 운영 배포(deploy/docker-compose)는 SHOW_PROJECTS_SECTOR=0. URL /projects 계열은 이 플래그와 무관하게 연다.
func (c *Config) ShowProjectsSector() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SHOW_PROJECTS_SECTOR"))) {
	case "0", "false", "no", "off":
		return false
	}
	return true
}

// ShowProjectExampleData 설계용 가상 데이터. 기본은 끈다. 명시적으로 1일 때만 켠다.
func (c *Config) ShowProjectExampleData() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PROJECT_EXAMPLE_DATA"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
