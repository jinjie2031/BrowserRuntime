package network

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPJSONLocalityResolverUsesInjectedEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/203.0.113.60" {
			t.Fatalf("resolver path = %q", request.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"ip":"203.0.113.60","country_code":"US","city":"Los Angeles","timezone":{"id":"America/Los_Angeles"}}`))
	}))
	defer server.Close()
	resolver, err := NewHTTPJSONLocalityResolver(server.URL)
	if err != nil {
		t.Fatalf("NewHTTPJSONLocalityResolver() error = %v", err)
	}
	locality, err := resolver.ResolveLocality(context.Background(), "203.0.113.60")
	if err != nil {
		t.Fatalf("ResolveLocality() error = %v", err)
	}
	want := Locality{ExitIP: "203.0.113.60", CountryCode: "US", City: "Los Angeles", Timezone: "America/Los_Angeles", Locale: "en-US"}
	if locality != want {
		t.Fatalf("ResolveLocality() = %#v, want %#v", locality, want)
	}
}
