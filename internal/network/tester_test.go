package network

import (
	"bufio"
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
)

func TestTesterDirectUsesInjectedEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ip":"203.0.113.8"}`))
	}))
	defer server.Close()

	tester, err := NewTester(server.URL)
	if err != nil {
		t.Fatalf("NewTester() error = %v", err)
	}
	result, err := tester.TestConnection(context.Background(), nil)
	if err != nil {
		t.Fatalf("TestConnection() error = %v", err)
	}
	if !result.OK || result.ExitIP != "203.0.113.8" || result.LatencyMS < 0 {
		t.Fatalf("TestConnection() = %#v", result)
	}
}

func TestTesterHTTPAndHTTPSProxySchemes(t *testing.T) {
	for _, scheme := range []Scheme{SchemeHTTP, SchemeHTTPS} {
		t.Run(string(scheme), func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"ip":"203.0.113.21"}`))
			})
			var proxyServer *httptest.Server
			if scheme == SchemeHTTPS {
				proxyServer = httptest.NewTLSServer(handler)
			} else {
				proxyServer = httptest.NewServer(handler)
			}
			defer proxyServer.Close()
			proxyURL, _ := url.Parse(proxyServer.URL)
			port, _ := strconv.ParseUint(proxyURL.Port(), 10, 16)
			tester, err := NewTester("http://connection.test/ip")
			if err != nil {
				t.Fatalf("NewTester() error = %v", err)
			}
			if scheme == SchemeHTTPS {
				tester.tlsConfig = proxyServer.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
			}
			result, err := tester.TestConnection(context.Background(), &ResolvedProxy{Scheme: scheme, Host: proxyURL.Hostname(), Port: uint16(port)})
			if err != nil || !result.OK || result.ExitIP != "203.0.113.21" {
				t.Fatalf("TestConnection() = %#v, %v", result, err)
			}
		})
	}
}

func TestTesterSOCKS5Proxy(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ip":"203.0.113.22"}`))
	}))
	defer target.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()
	go serveOneSOCKS5(listener)
	host, port := splitAddress(t, listener.Addr().String())
	tester, err := NewTester(target.URL)
	if err != nil {
		t.Fatalf("NewTester() error = %v", err)
	}
	result, err := tester.TestConnection(context.Background(), &ResolvedProxy{Scheme: SchemeSOCKS5, Host: host, Port: port})
	if err != nil || !result.OK || result.ExitIP != "203.0.113.22" {
		t.Fatalf("TestConnection() = %#v, %v", result, err)
	}
}

func serveOneSOCKS5(listener net.Listener) {
	client, err := listener.Accept()
	if err != nil {
		return
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	reader := bufio.NewReader(client)
	header := make([]byte, 2)
	if _, err := io.ReadFull(reader, header); err != nil {
		return
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(reader, methods); err != nil {
		return
	}
	_, _ = client.Write([]byte{0x05, 0x00})
	request := make([]byte, 4)
	if _, err := io.ReadFull(reader, request); err != nil {
		return
	}
	var host string
	switch request[3] {
	case 0x01:
		address := make([]byte, 4)
		_, _ = io.ReadFull(reader, address)
		host = net.IP(address).String()
	case 0x04:
		address := make([]byte, 16)
		_, _ = io.ReadFull(reader, address)
		host = net.IP(address).String()
	case 0x03:
		length, err := reader.ReadByte()
		if err != nil {
			return
		}
		address := make([]byte, int(length))
		_, _ = io.ReadFull(reader, address)
		host = string(address)
	default:
		return
	}
	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(reader, portBytes); err != nil {
		return
	}
	target, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(portBytes)))), 2*time.Second)
	if err != nil {
		return
	}
	defer target.Close()
	_, _ = client.Write([]byte{0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1, 0, 0})
	done := make(chan struct{}, 1)
	go func() {
		_, _ = io.Copy(target, reader)
		done <- struct{}{}
	}()
	_, _ = io.Copy(client, target)
	<-done
}

func TestTesterUnavailableProxyFailsClosed(t *testing.T) {
	directHits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		directHits++
		_, _ = w.Write([]byte(`{"ip":"203.0.113.9"}`))
	}))
	defer server.Close()

	tester, err := NewTester(server.URL)
	if err != nil {
		t.Fatalf("NewTester() error = %v", err)
	}
	proxy := &ResolvedProxy{Scheme: SchemeHTTP, Host: "127.0.0.1", Port: 1}
	if _, err := tester.TestConnection(context.Background(), proxy); err == nil {
		t.Fatal("TestConnection() error = nil, want unavailable proxy failure")
	}
	if directHits != 0 {
		t.Fatalf("direct endpoint hits = %d, want 0", directHits)
	}
}
