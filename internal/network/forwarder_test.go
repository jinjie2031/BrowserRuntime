package network

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoopbackBridgeInjectsAuthenticationAndKeepsEngineSecretFree(t *testing.T) {
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ip":"203.0.113.10"}`))
	}))
	defer target.Close()

	username := "bridge-user"
	password := randomTestSecret(t)
	var authenticated atomic.Bool
	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen(proxy) error = %v", err)
	}
	defer proxyListener.Close()
	go serveSingleConnectProxy(proxyListener, username, password, &authenticated)

	host, port := splitAddress(t, proxyListener.Addr().String())
	bridge, err := StartLoopbackBridge(&ResolvedProxy{Scheme: SchemeHTTP, Host: host, Port: port, Username: username, Password: password})
	if err != nil {
		t.Fatalf("StartLoopbackBridge() error = %v", err)
	}
	defer bridge.Close()
	engineProxy := bridge.EngineProxy()
	if engineProxy.Host != "127.0.0.1" || engineProxy.Scheme != SchemeHTTP || engineProxy.Port == 0 {
		t.Fatalf("EngineProxy() = %#v", engineProxy)
	}
	if strings.Contains(engineProxy.ServerURL(), username) || strings.Contains(engineProxy.ServerURL(), password) {
		t.Fatal("EngineProxy() disclosed credentials")
	}

	proxyURL, _ := url.Parse(engineProxy.ServerURL())
	transport := target.Client().Transport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxyURL)
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	response, err := client.Get(target.URL)
	if err != nil {
		t.Fatalf("GET through bridge error = %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !authenticated.Load() {
		t.Fatalf("bridge status = %d, authenticated = %v", response.StatusCode, authenticated.Load())
	}
}

func TestLoopbackBridgeUnavailableUpstreamFailsClosed(t *testing.T) {
	bridge, err := StartLoopbackBridge(&ResolvedProxy{Scheme: SchemeHTTP, Host: "127.0.0.1", Port: 1})
	if err != nil {
		t.Fatalf("StartLoopbackBridge() error = %v", err)
	}
	defer bridge.Close()

	directHits := atomic.Int32{}
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		directHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	proxyURL, _ := url.Parse(bridge.EngineProxy().ServerURL())
	transport := target.Client().Transport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxyURL)
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	if _, err := client.Get(target.URL); err == nil {
		t.Fatal("GET through unavailable upstream succeeded")
	}
	if directHits.Load() != 0 {
		t.Fatalf("direct target hits = %d, want 0", directHits.Load())
	}
}

func serveSingleConnectProxy(listener net.Listener, username, password string, authenticated *atomic.Bool) {
	connection, err := listener.Accept()
	if err != nil {
		return
	}
	defer connection.Close()
	reader := bufio.NewReader(connection)
	request, err := http.ReadRequest(reader)
	if err != nil || request.Method != http.MethodConnect {
		return
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
	if request.Header.Get("Proxy-Authorization") != wantAuth {
		_, _ = io.WriteString(connection, "HTTP/1.1 407 Proxy Authentication Required\r\nContent-Length: 0\r\n\r\n")
		return
	}
	authenticated.Store(true)
	upstream, err := net.DialTimeout("tcp", request.Host, 2*time.Second)
	if err != nil {
		_, _ = io.WriteString(connection, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n")
		return
	}
	defer upstream.Close()
	_, _ = io.WriteString(connection, "HTTP/1.1 200 Connection Established\r\n\r\n")
	done := make(chan struct{}, 1)
	go func() {
		_, _ = io.Copy(upstream, reader)
		done <- struct{}{}
	}()
	_, _ = io.Copy(connection, upstream)
	<-done
}

func splitAddress(t *testing.T, address string) (string, uint16) {
	t.Helper()
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("SplitHostPort(%q) error = %v", address, err)
	}
	var port uint16
	if _, err := fmt.Sscanf(portText, "%d", &port); err != nil {
		t.Fatalf("parse port error = %v", err)
	}
	return host, port
}

func TestBridgeContextCancellationClosesListener(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	bridge, err := StartLoopbackBridgeContext(ctx, &ResolvedProxy{Scheme: SchemeHTTP, Host: "127.0.0.1", Port: 1})
	if err != nil {
		t.Fatalf("StartLoopbackBridgeContext() error = %v", err)
	}
	address := bridge.EngineProxy().ServerURL()
	cancel()
	select {
	case <-bridge.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("bridge did not stop after context cancellation")
	}
	if address == "" {
		t.Fatal("bridge returned empty engine endpoint")
	}
}
