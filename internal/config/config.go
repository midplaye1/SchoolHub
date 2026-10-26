package config

import (
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Addr, DatabaseURL, WebDir string
	DBTimeout                 time.Duration
}

func Load() (Config, error) {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		return Config{}, err
	}
	c := Config{Addr: env("HTTP_ADDR", ":8080"), DatabaseURL: env("DATABASE_URL", "postgres://libraryhub:libraryhub@localhost:5432/libraryhub?sslmode=disable"), WebDir: env("WEB_DIR", "web")}
	var err error
	c.DBTimeout, err = time.ParseDuration(env("DB_TIMEOUT", "5s"))
	if err != nil || c.DBTimeout <= 0 {
		return c, fmt.Errorf("invalid DB_TIMEOUT")
	}
	return c, nil
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
