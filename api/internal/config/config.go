package config

import (
	"fmt"
	"net/url"
	"os"
	"time"
)

type Config struct {
	WebOrigin         string
	DatabaseURL       string
	FaceServiceURL    string
	CookieSecure      bool
	AdminUsername     string
	AdminPasswordHash string
	SessionTTL        time.Duration
	BusinessTimezone  *time.Location
}

func Load() (Config, error) {
	webOrigin := os.Getenv("WEB_ORIGIN")
	if webOrigin == "" {
		return Config{}, fmt.Errorf("WEB_ORIGIN is required")
	}
	if err := validateWebOrigin(webOrigin); err != nil {
		return Config{}, err
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	faceServiceURL := os.Getenv("FACE_SERVICE_URL")
	if faceServiceURL == "" {
		return Config{}, fmt.Errorf("FACE_SERVICE_URL is required")
	}

	adminUsername := os.Getenv("ADMIN_USERNAME")
	if adminUsername == "" {
		return Config{}, fmt.Errorf("ADMIN_USERNAME is required")
	}

	adminPasswordHash := os.Getenv("ADMIN_PASSWORD_HASH")
	if adminPasswordHash == "" {
		return Config{}, fmt.Errorf("ADMIN_PASSWORD_HASH is required")
	}

	cookieSecure := os.Getenv("COOKIE_SECURE") == "true"

	sessionTTL := 8 * time.Hour
	if raw := os.Getenv("SESSION_TTL"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("SESSION_TTL: %w", err)
		}
		sessionTTL = parsed
	}

	tzName := os.Getenv("BUSINESS_TIMEZONE")
	if tzName == "" {
		tzName = "Asia/Phnom_Penh"
	}
	businessTZ, err := time.LoadLocation(tzName)
	if err != nil {
		return Config{}, fmt.Errorf("BUSINESS_TIMEZONE: %w", err)
	}

	return Config{
		WebOrigin:         webOrigin,
		DatabaseURL:       databaseURL,
		FaceServiceURL:    faceServiceURL,
		CookieSecure:      cookieSecure,
		AdminUsername:     adminUsername,
		AdminPasswordHash: adminPasswordHash,
		SessionTTL:        sessionTTL,
		BusinessTimezone:  businessTZ,
	}, nil
}

func validateWebOrigin(origin string) error {
	u, err := url.Parse(origin)
	if err != nil {
		return fmt.Errorf("WEB_ORIGIN: invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("WEB_ORIGIN: scheme must be http or https")
	}
	if u.Hostname() != "localhost" {
		return fmt.Errorf("WEB_ORIGIN: development hostname must be localhost")
	}
	return nil
}
