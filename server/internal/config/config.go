package config

import "os"

type Config struct {
	Port    string
	DBType  string
	DBPath  string
	DBDSN   string
	JWTSecret string
	AppEnv  string
}

func Load() *Config {
	return &Config{
		Port:      getEnv("PORT", "8080"),
		DBType:    getEnv("DB_TYPE", "sqlite"),
		DBPath:    getEnv("DB_PATH", "data/app.db"),
		DBDSN:     getEnv("DB_DSN", ""),
		JWTSecret: getEnv("JWT_SECRET", "dev-secret-change-in-production"),
		AppEnv:    getEnv("APP_ENV", "development"),
	}
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
