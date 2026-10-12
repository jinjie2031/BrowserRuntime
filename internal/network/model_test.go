package network

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{name: "direct", config: Config{Mode: ModeDirect}},
		{name: "http", config: Config{Mode: ModeProxy, Scheme: SchemeHTTP, Host: "proxy.example", Port: 8080}},
		{name: "https", config: Config{Mode: ModeProxy, Scheme: SchemeHTTPS, Host: "proxy.example", Port: 8443}},
		{name: "socks5", config: Config{Mode: ModeProxy, Scheme: SchemeSOCKS5, Host: "127.0.0.1", Port: 1080}},
		{name: "unknown mode", config: Config{Mode: "automatic"}, wantErr: true},
		{name: "direct with proxy fields", config: Config{Mode: ModeDirect, Host: "proxy.example"}, wantErr: true},
		{name: "unknown scheme", config: Config{Mode: ModeProxy, Scheme: "ftp", Host: "proxy.example", Port: 21}, wantErr: true},
		{name: "empty host", config: Config{Mode: ModeProxy, Scheme: SchemeHTTP, Port: 8080}, wantErr: true},
		{name: "zero port", config: Config{Mode: ModeProxy, Scheme: SchemeHTTP, Host: "proxy.example"}, wantErr: true},
		{name: "host is URL", config: Config{Mode: ModeProxy, Scheme: SchemeHTTP, Host: "http://proxy.example", Port: 8080}, wantErr: true},
		{name: "control in username", config: Config{Mode: ModeProxy, Scheme: SchemeHTTP, Host: "proxy.example", Port: 8080, Username: "bad\nuser"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("Validate() error = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

func TestResolvedProxyRedaction(t *testing.T) {
	proxy := ResolvedProxy{
		Scheme:   SchemeHTTP,
		Host:     "proxy.example",
		Port:     8080,
		Username: "operator-user",
		Password: randomTestSecret(t),
	}
	redacted := proxy.Redacted()
	if strings.Contains(redacted, proxy.Username) || strings.Contains(redacted, proxy.Password) {
		t.Fatalf("Redacted() disclosed credentials")
	}
	if !strings.Contains(redacted, "proxy.example:8080") {
		t.Fatalf("Redacted() = %q, want non-secret endpoint", redacted)
	}
}

func TestLocalityValidation(t *testing.T) {
	valid := Locality{
		ExitIP:      "203.0.113.42",
		CountryCode: "US",
		City:        "Los Angeles",
		Timezone:    "America/Los_Angeles",
		Locale:      "en-US",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate(valid) error = %v", err)
	}
	tests := []Locality{
		{CountryCode: "US", Timezone: "America/Los_Angeles", Locale: "en-US"},
		{ExitIP: valid.ExitIP, CountryCode: "USA", Timezone: valid.Timezone, Locale: valid.Locale},
		{ExitIP: valid.ExitIP, CountryCode: valid.CountryCode, Timezone: "Missing/Timezone", Locale: valid.Locale},
		{ExitIP: valid.ExitIP, CountryCode: valid.CountryCode, Timezone: valid.Timezone},
		{ExitIP: valid.ExitIP, CountryCode: valid.CountryCode, Timezone: valid.Timezone, Locale: "not_a_locale"},
	}
	for _, locality := range tests {
		if err := locality.Validate(); !errors.Is(err, ErrInvalidLocality) {
			t.Fatalf("Validate(%#v) error = %v, want ErrInvalidLocality", locality, err)
		}
	}
}

func TestCountryDefaultLocale(t *testing.T) {
	locale, ok := DefaultLocaleForCountry("US")
	if !ok || locale != "en-US" {
		t.Fatalf("DefaultLocaleForCountry(US) = %q, %v", locale, ok)
	}
	if _, ok := DefaultLocaleForCountry("ZZ"); ok {
		t.Fatal("DefaultLocaleForCountry(ZZ) unexpectedly resolved")
	}
}

func randomTestSecret(t *testing.T) string {
	t.Helper()
	random := make([]byte, 24)
	if _, err := rand.Read(random); err != nil {
		t.Fatalf("generate test value: %v", err)
	}
	return hex.EncodeToString(random)
}
