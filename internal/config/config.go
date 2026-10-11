package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv             string
	AppPort            string
	DBConnectionString string
	JWTSecret          string
}

func Load() Config {
	_ = godotenv.Load()

	return Config{
		AppEnv:             getEnv("APP_ENV", "development"),
		AppPort:            getEnv("APP_PORT", "8080"),
		DBConnectionString: getEnv("DB_CONNECTION_STRING", "postgres://postgres:postgres@localhost:5432/reading_journal?sslmode=disable"),
		JWTSecret:          getEnv("JWT_SECRET", ""),
	}
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
