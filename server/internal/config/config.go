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

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
