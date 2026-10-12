package network

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type Mode string

const (
	ModeDirect Mode = "direct"
	ModeProxy  Mode = "proxy"
)

type Scheme string

const (
	SchemeHTTP   Scheme = "http"
	SchemeHTTPS  Scheme = "https"
	SchemeSOCKS5 Scheme = "socks5"
)

var (
	ErrInvalidConfig       = errors.New("invalid network configuration")
	ErrConfigMissing       = errors.New("network configuration is missing; explicitly configure network")
	ErrSecretRequired      = errors.New("proxy password is required")
	ErrSecretUnavailable   = errors.New("proxy secret is unavailable")
	ErrConnectionFailed    = errors.New("network connection test failed")
	ErrInvalidLocality     = errors.New("invalid network locality")
	ErrLocalityUnavailable = errors.New("network locality is unavailable")
)

var (
	countryCodePattern = regexp.MustCompile(`^[A-Z]{2}$`)
	localePattern      = regexp.MustCompile(`^[a-z]{2,3}(?:-[A-Z][a-z]{3})?(?:-(?:[A-Z]{2}|[0-9]{3}))?$`)
)

// Locality is non-secret, confirmed proxy-exit locality persisted in network.json.
type Locality struct {
	ExitIP      string `json:"exit_ip"`
	CountryCode string `json:"country_code"`
	City        string `json:"city,omitempty"`
	Timezone    string `json:"timezone"`
	Locale      string `json:"locale"`
}

func (l Locality) Validate() error {
	if net.ParseIP(l.ExitIP) == nil {
		return fmt.Errorf("%w: exit IP is invalid", ErrInvalidLocality)
	}
	if !countryCodePattern.MatchString(l.CountryCode) {
		return fmt.Errorf("%w: country code is invalid", ErrInvalidLocality)
	}
	if strings.TrimSpace(l.City) != l.City || containsControl(l.City) {
		return fmt.Errorf("%w: city is invalid", ErrInvalidLocality)
	}
	if strings.TrimSpace(l.Timezone) != l.Timezone || l.Timezone == "" || containsControl(l.Timezone) {
		return fmt.Errorf("%w: timezone is invalid", ErrInvalidLocality)
	}
	if _, err := time.LoadLocation(l.Timezone); err != nil {
		return fmt.Errorf("%w: timezone is unknown", ErrInvalidLocality)
	}
	if !localePattern.MatchString(l.Locale) {
		return fmt.Errorf("%w: locale is invalid", ErrInvalidLocality)
	}
	return nil
}

var countryDefaultLocales = map[string]string{
	"AU": "en-AU",
	"CA": "en-CA",
	"CN": "zh-CN",
	"DE": "de-DE",
	"FR": "fr-FR",
	"GB": "en-GB",
	"JP": "ja-JP",
	"KR": "ko-KR",
	"TW": "zh-TW",
	"US": "en-US",
}

// DefaultLocaleForCountry is the deliberately small V0.1 fallback map.
func DefaultLocaleForCountry(countryCode string) (string, bool) {
	locale, ok := countryDefaultLocales[strings.ToUpper(strings.TrimSpace(countryCode))]
	return locale, ok
}

// Config is the non-secret profile network configuration stored in network.json.
type Config struct {
	Mode     Mode      `json:"mode"`
	Scheme   Scheme    `json:"scheme,omitempty"`
	Host     string    `json:"host,omitempty"`
	Port     uint16    `json:"port,omitempty"`
	Username string    `json:"username,omitempty"`
	Locality *Locality `json:"locality,omitempty"`
}

// Validate rejects configurations that cannot be applied without guessing.
func (c Config) Validate() error {
	switch c.Mode {
	case ModeDirect:
		if c.Scheme != "" || c.Host != "" || c.Port != 0 || c.Username != "" || c.Locality != nil {
			return fmt.Errorf("%w: direct mode contains proxy fields", ErrInvalidConfig)
		}
		return nil
	case ModeProxy:
		if c.Scheme != SchemeHTTP && c.Scheme != SchemeHTTPS && c.Scheme != SchemeSOCKS5 {
			return fmt.Errorf("%w: unsupported proxy scheme", ErrInvalidConfig)
		}
		if !validHost(c.Host) {
			return fmt.Errorf("%w: proxy host is invalid", ErrInvalidConfig)
		}
		if c.Port == 0 {
			return fmt.Errorf("%w: proxy port is invalid", ErrInvalidConfig)
		}
		if containsControl(c.Username) {
			return fmt.Errorf("%w: proxy username contains control characters", ErrInvalidConfig)
		}
		if c.Locality != nil {
			if err := c.Locality.Validate(); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("%w: unsupported network mode", ErrInvalidConfig)
	}
}

func validHost(host string) bool {
	if host == "" || strings.TrimSpace(host) != host || containsControl(host) {
		return false
	}
	if strings.ContainsAny(host, "/\\@") || strings.Contains(host, "://") {
		return false
	}
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}
	if net.ParseIP(host) != nil {
		return true
	}
	if len(host) > 253 || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' {
				return false
			}
		}
	}
	return true
}

func containsControl(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

// ResolvedProxy exists only in memory and includes the DPAPI-unprotected password.
// It deliberately does not implement fmt.Stringer.
type ResolvedProxy struct {
	Scheme   Scheme
	Host     string
	Port     uint16
	Username string
	Password string
}

func (p ResolvedProxy) Validate() error {
	config := Config{Mode: ModeProxy, Scheme: p.Scheme, Host: p.Host, Port: p.Port, Username: p.Username}
	if err := config.Validate(); err != nil {
		return err
	}
	if p.Username != "" && p.Password == "" {
		return ErrSecretRequired
	}
	if p.Username == "" && p.Password != "" {
		return fmt.Errorf("%w: password requires a username", ErrInvalidConfig)
	}
	return nil
}

func (p ResolvedProxy) address() string {
	return net.JoinHostPort(p.Host, strconv.Itoa(int(p.Port)))
}

// Redacted returns an endpoint label with all authentication fields omitted.
func (p ResolvedProxy) Redacted() string {
	endpoint := Config{Mode: ModeProxy, Scheme: p.Scheme, Host: p.Host, Port: p.Port}
	if endpoint.Validate() != nil {
		return "[invalid proxy endpoint]"
	}
	return string(p.Scheme) + "://" + p.address()
}

// String and GoString also protect accidental fmt diagnostics, including %+v/%#v.
// They do not change JSON persistence or the normal configuration UI.
func (p ResolvedProxy) String() string   { return p.Redacted() }
func (p ResolvedProxy) GoString() string { return p.Redacted() }

func (c Config) String() string {
	if c.Mode == ModeDirect {
		return "direct"
	}
	return (ResolvedProxy{Scheme: c.Scheme, Host: c.Host, Port: c.Port}).Redacted()
}

func (c Config) GoString() string { return c.String() }

// EngineProxy is a credential-free proxy endpoint suitable for a native browser
// command line. Authenticated upstreams must be exposed through a loopback bridge.
type EngineProxy struct {
	Mode     Mode
	Scheme   Scheme
	Host     string
	Port     uint16
	Locality *Locality
}

func (p EngineProxy) Validate() error {
	return Config{Mode: p.Mode, Scheme: p.Scheme, Host: p.Host, Port: p.Port, Locality: p.Locality}.Validate()
}

// ValidateForLaunch requires resolved locality for every proxy-mode browser launch.
func (p EngineProxy) ValidateForLaunch() error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.Mode == ModeProxy && p.Locality == nil {
		return ErrLocalityUnavailable
	}
	return nil
}

func (p EngineProxy) ServerURL() string {
	if p.Mode != ModeProxy {
		return ""
	}
	return string(p.Scheme) + "://" + net.JoinHostPort(p.Host, strconv.Itoa(int(p.Port)))
}

// ConnectionResult is the minimal V0.1 connection evidence.
type ConnectionResult struct {
	OK        bool   `json:"ok"`
	ExitIP    string `json:"exit_ip"`
	LatencyMS int64  `json:"latency_ms"`
}
