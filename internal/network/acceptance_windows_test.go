//go:build windows

package network

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"browserruntime/internal/profile"
)

func TestRealProxyPositiveAcceptance(t *testing.T) {
	if os.Getenv("BR_RUN_NETWORK_ACCEPTANCE") != "1" {
		t.Skip("set BR_RUN_NETWORK_ACCEPTANCE=1 to run the authorized real-proxy acceptance")
	}
	port, err := strconv.ParseUint(os.Getenv("BR_WEBSHARE_PROXY_PORT"), 10, 16)
	if err != nil || port == 0 {
		t.Fatal("proxy acceptance environment is incomplete")
	}
	proxy := &ResolvedProxy{
		Scheme:   SchemeHTTP,
		Host:     os.Getenv("BR_WEBSHARE_PROXY_HOST"),
		Port:     uint16(port),
		Username: os.Getenv("BR_WEBSHARE_PROXY_USERNAME"),
		Password: os.Getenv("BR_WEBSHARE_PROXY_PASSWORD"),
	}
	if err := proxy.Validate(); err != nil {
		t.Fatal("proxy acceptance environment is invalid")
	}
	tester, err := NewTester("https://api.ipify.org?format=json")
	if err != nil {
		t.Fatal("create connection tester")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := tester.TestConnection(ctx, proxy)
	if err != nil || !result.OK || result.ExitIP == "" {
		t.Fatal("positive proxy acceptance failed")
	}
}

func TestRealProxyLocalityAcceptance(t *testing.T) {
	if os.Getenv("BR_RUN_LOCALITY_ACCEPTANCE") != "1" {
		t.Skip("set BR_RUN_LOCALITY_ACCEPTANCE=1 to run the authorized locality acceptance")
	}
	port, err := strconv.ParseUint(os.Getenv("BR_WEBSHARE_PROXY_PORT"), 10, 16)
	if err != nil || port == 0 {
		t.Fatal("locality acceptance environment is incomplete")
	}
	root := filepath.Join(t.TempDir(), "profiles")
	profiles, err := profile.NewStore(root)
	if err != nil {
		t.Fatal("create locality acceptance profile store")
	}
	p, err := profiles.Create("Task 4 locality acceptance")
	if err != nil {
		t.Fatal("create locality acceptance profile")
	}
	store, err := NewStore(root)
	if err != nil {
		t.Fatal("create locality acceptance network store")
	}
	config := Config{
		Mode:     ModeProxy,
		Scheme:   SchemeHTTP,
		Host:     os.Getenv("BR_WEBSHARE_PROXY_HOST"),
		Port:     uint16(port),
		Username: os.Getenv("BR_WEBSHARE_PROXY_USERNAME"),
	}
	if err := store.SetProxy(p.ID, config, os.Getenv("BR_WEBSHARE_PROXY_PASSWORD")); err != nil {
		t.Fatal("persist locality acceptance proxy")
	}
	tester, err := NewTester("https://ipwho.is/")
	if err != nil {
		t.Fatal("create locality acceptance connection tester")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	connection, err := store.TestConnection(ctx, p.ID, tester)
	if err != nil || !connection.OK || connection.ExitIP == "" {
		t.Fatal("locality acceptance proxy connection failed")
	}
	resolver, err := NewHTTPJSONLocalityResolver("https://ipwho.is")
	if err != nil {
		t.Fatal("create locality resolver")
	}
	locality, err := store.ResolveLocality(ctx, p.ID, connection.ExitIP, resolver)
	if err != nil || locality.ExitIP == "" || locality.CountryCode != "US" || locality.City == "" || locality.Timezone == "" || locality.Locale == "" {
		t.Fatal("locality acceptance resolution failed")
	}
	persisted, err := store.Get(p.ID)
	if err != nil || persisted.Locality == nil || *persisted.Locality != *locality {
		t.Fatal("locality acceptance persistence failed")
	}
}
