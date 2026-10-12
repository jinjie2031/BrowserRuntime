package network

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const localityResolverTimeout = 15 * time.Second

type LocalityResolver interface {
	ResolveLocality(context.Context, string) (Locality, error)
}

// HTTPJSONLocalityResolver resolves one confirmed exit IP using an injected
// ipwho.is-compatible JSON endpoint.
type HTTPJSONLocalityResolver struct {
	baseURL string
	client  *http.Client
}

func NewHTTPJSONLocalityResolver(baseURL string) (*HTTPJSONLocalityResolver, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return nil, ErrLocalityUnavailable
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return &HTTPJSONLocalityResolver{
		baseURL: strings.TrimRight(parsed.String(), "/"),
		client: &http.Client{
			Timeout: localityResolverTimeout,
			Transport: &http.Transport{
				Proxy:                 nil,
				TLSHandshakeTimeout:   localityResolverTimeout,
				ResponseHeaderTimeout: localityResolverTimeout,
				DisableKeepAlives:     true,
			},
		},
	}, nil
}

func (r *HTTPJSONLocalityResolver) ResolveLocality(ctx context.Context, exitIP string) (Locality, error) {
	if net.ParseIP(exitIP) == nil {
		return Locality{}, ErrInvalidLocality
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL+"/"+url.PathEscape(exitIP), nil)
	if err != nil {
		return Locality{}, ErrLocalityUnavailable
	}
	response, err := r.client.Do(request)
	if err != nil {
		return Locality{}, ErrLocalityUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Locality{}, ErrLocalityUnavailable
	}
	var payload struct {
		Success     *bool  `json:"success"`
		IP          string `json:"ip"`
		CountryCode string `json:"country_code"`
		City        string `json:"city"`
		Locale      string `json:"locale"`
		Timezone    struct {
			ID string `json:"id"`
		} `json:"timezone"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 128<<10))
	if err := decoder.Decode(&payload); err != nil || (payload.Success != nil && !*payload.Success) {
		return Locality{}, ErrLocalityUnavailable
	}
	locality := Locality{
		ExitIP:      strings.TrimSpace(payload.IP),
		CountryCode: strings.ToUpper(strings.TrimSpace(payload.CountryCode)),
		City:        strings.TrimSpace(payload.City),
		Timezone:    strings.TrimSpace(payload.Timezone.ID),
		Locale:      strings.TrimSpace(payload.Locale),
	}
	if locality.Locale == "" {
		var ok bool
		locality.Locale, ok = DefaultLocaleForCountry(locality.CountryCode)
		if !ok {
			return Locality{}, ErrLocalityUnavailable
		}
	}
	if err := locality.Validate(); err != nil {
		return Locality{}, ErrLocalityUnavailable
	}
	if !sameExitIP(locality.ExitIP, exitIP) {
		return Locality{}, ErrInvalidLocality
	}
	return locality, nil
}

func normalizeResolvedLocality(locality Locality) (Locality, error) {
	locality.ExitIP = strings.TrimSpace(locality.ExitIP)
	locality.CountryCode = strings.ToUpper(strings.TrimSpace(locality.CountryCode))
	locality.City = strings.TrimSpace(locality.City)
	locality.Timezone = strings.TrimSpace(locality.Timezone)
	locality.Locale = strings.TrimSpace(locality.Locale)
	if locality.Locale == "" {
		var ok bool
		locality.Locale, ok = DefaultLocaleForCountry(locality.CountryCode)
		if !ok {
			return Locality{}, ErrLocalityUnavailable
		}
	}
	if err := locality.Validate(); err != nil {
		return Locality{}, err
	}
	return locality, nil
}

func sameExitIP(first, second string) bool {
	firstIP, secondIP := net.ParseIP(first), net.ParseIP(second)
	return firstIP != nil && secondIP != nil && firstIP.Equal(secondIP)
}
