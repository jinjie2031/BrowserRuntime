package network

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

const diagnosticUsername = "BR_USERNAME_MUST_NOT_LEAK"
const diagnosticPassword = "BR_PASSWORD_MUST_NOT_LEAK"

func assertSafeDiagnostic(t *testing.T, output string) {
	t.Helper()
	if strings.Contains(output, diagnosticUsername) || strings.Contains(output, diagnosticPassword) || strings.Contains(output, "@") {
		t.Fatal("diagnostic disclosed authentication data")
	}
}

func TestEndpointOnlyDiagnostics(t *testing.T) {
	p := ResolvedProxy{Scheme: SchemeHTTP, Host: "proxy.example.com", Port: 8080, Username: diagnosticUsername, Password: diagnosticPassword}
	c := Config{Mode: ModeProxy, Scheme: p.Scheme, Host: p.Host, Port: p.Port, Username: p.Username}
	for _, output := range []string{p.Redacted(), fmt.Sprint(p), fmt.Sprintf("%+v", &p), fmt.Sprintf("%#v", p), fmt.Sprint(c), fmt.Sprintf("%#v", &c)} {
		assertSafeDiagnostic(t, output)
		if output != "http://proxy.example.com:8080" {
			t.Fatal("diagnostic must contain only scheme, host and port")
		}
	}
	p.Host = diagnosticUsername + ":" + diagnosticPassword + "@proxy.example.com"
	assertSafeDiagnostic(t, p.Redacted())
	c.Port = 0
	err := fmt.Errorf("validation for %v: %w", c, c.Validate())
	assertSafeDiagnostic(t, err.Error())
}

func TestConnectionErrorDiagnostic(t *testing.T) {
	// A local proxy rejects the request; no public network is used.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusProxyAuthRequired) }))
	defer server.Close()
	endpoint, _ := url.Parse(server.URL)
	port, _ := strconv.ParseUint(endpoint.Port(), 10, 16)
	p := ResolvedProxy{Scheme: SchemeHTTP, Host: endpoint.Hostname(), Port: uint16(port), Username: diagnosticUsername, Password: diagnosticPassword}
	tester, err := NewTester("http://target.invalid/ip")
	if err != nil {
		t.Fatal("test setup failed")
	}
	_, err = tester.TestConnection(context.Background(), &p)
	if err == nil {
		t.Fatal("expected proxy connection error")
	}
	assertSafeDiagnostic(t, fmt.Sprintf("connection for %v: %v", p, err))
}
