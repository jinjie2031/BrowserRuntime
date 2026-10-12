package network

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultConnectionTimeout = 15 * time.Second

// Tester checks one injected IP endpoint through direct or explicitly resolved proxy transport.
type Tester struct {
	endpoint  string
	timeout   time.Duration
	tlsConfig *tls.Config
}

func NewTester(endpoint string) (*Tester, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, errors.New("invalid connection test endpoint")
	}
	return &Tester{endpoint: parsed.String(), timeout: defaultConnectionTimeout}, nil
}

func (t *Tester) TestConnection(ctx context.Context, proxy *ResolvedProxy) (ConnectionResult, error) {
	transport := &http.Transport{
		Proxy:                 nil,
		TLSHandshakeTimeout:   t.timeout,
		ResponseHeaderTimeout: t.timeout,
		IdleConnTimeout:       t.timeout,
		DisableKeepAlives:     true,
		TLSClientConfig:       t.tlsConfig,
	}
	if proxy != nil {
		if err := proxy.Validate(); err != nil {
			return ConnectionResult{}, ErrConnectionFailed
		}
		switch proxy.Scheme {
		case SchemeHTTP, SchemeHTTPS:
			proxyURL := &url.URL{Scheme: string(proxy.Scheme), Host: proxy.address()}
			if proxy.Username != "" {
				proxyURL.User = url.UserPassword(proxy.Username, proxy.Password)
			}
			transport.Proxy = http.ProxyURL(proxyURL)
		case SchemeSOCKS5:
			transport.DialContext = socks5DialContext(proxy, t.timeout)
		default:
			return ConnectionResult{}, ErrConnectionFailed
		}
	}
	client := &http.Client{Transport: transport, Timeout: t.timeout}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, t.endpoint, nil)
	if err != nil {
		return ConnectionResult{}, ErrConnectionFailed
	}
	started := time.Now()
	response, err := client.Do(request)
	latency := time.Since(started).Milliseconds()
	if err != nil {
		return ConnectionResult{}, ErrConnectionFailed
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ConnectionResult{}, ErrConnectionFailed
	}
	var payload struct {
		IP string `json:"ip"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 64<<10))
	if err := decoder.Decode(&payload); err != nil || net.ParseIP(strings.TrimSpace(payload.IP)) == nil {
		return ConnectionResult{}, ErrConnectionFailed
	}
	return ConnectionResult{OK: true, ExitIP: strings.TrimSpace(payload.IP), LatencyMS: latency}, nil
}

func socks5DialContext(proxy *ResolvedProxy, timeout time.Duration) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" {
			return nil, ErrConnectionFailed
		}
		dialer := net.Dialer{Timeout: timeout}
		connection, err := dialer.DialContext(ctx, "tcp", proxy.address())
		if err != nil {
			return nil, ErrConnectionFailed
		}
		if deadline, ok := ctx.Deadline(); ok {
			_ = connection.SetDeadline(deadline)
		} else {
			_ = connection.SetDeadline(time.Now().Add(timeout))
		}
		if err := socks5Handshake(connection, proxy, address); err != nil {
			_ = connection.Close()
			return nil, ErrConnectionFailed
		}
		_ = connection.SetDeadline(time.Time{})
		return connection, nil
	}
}

func socks5Handshake(connection net.Conn, proxy *ResolvedProxy, target string) error {
	methods := []byte{0x00}
	if proxy.Username != "" {
		methods = []byte{0x02}
	}
	greeting := append([]byte{0x05, byte(len(methods))}, methods...)
	if _, err := connection.Write(greeting); err != nil {
		return err
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(connection, reply); err != nil || reply[0] != 0x05 || reply[1] == 0xff {
		return ErrConnectionFailed
	}
	if reply[1] == 0x02 {
		if len(proxy.Username) > 255 || len(proxy.Password) > 255 {
			return ErrConnectionFailed
		}
		auth := []byte{0x01, byte(len(proxy.Username))}
		auth = append(auth, proxy.Username...)
		auth = append(auth, byte(len(proxy.Password)))
		auth = append(auth, proxy.Password...)
		if _, err := connection.Write(auth); err != nil {
			return err
		}
		if _, err := io.ReadFull(connection, reply); err != nil || reply[1] != 0x00 {
			return ErrConnectionFailed
		}
	} else if reply[1] != 0x00 {
		return ErrConnectionFailed
	}

	host, portText, err := net.SplitHostPort(target)
	if err != nil {
		return ErrConnectionFailed
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil || port == 0 {
		return ErrConnectionFailed
	}
	request := []byte{0x05, 0x01, 0x00}
	if ip := net.ParseIP(host); ip != nil {
		if ipv4 := ip.To4(); ipv4 != nil {
			request = append(request, 0x01)
			request = append(request, ipv4...)
		} else {
			request = append(request, 0x04)
			request = append(request, ip.To16()...)
		}
	} else {
		if len(host) == 0 || len(host) > 255 {
			return ErrConnectionFailed
		}
		request = append(request, 0x03, byte(len(host)))
		request = append(request, host...)
	}
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(port))
	request = append(request, portBytes...)
	if _, err := connection.Write(request); err != nil {
		return err
	}
	reader := bufio.NewReader(connection)
	header := make([]byte, 4)
	if _, err := io.ReadFull(reader, header); err != nil || header[0] != 0x05 || header[1] != 0x00 {
		return ErrConnectionFailed
	}
	switch header[3] {
	case 0x01:
		_, err = io.CopyN(io.Discard, reader, 4)
	case 0x04:
		_, err = io.CopyN(io.Discard, reader, 16)
	case 0x03:
		length, readErr := reader.ReadByte()
		if readErr != nil {
			return readErr
		}
		_, err = io.CopyN(io.Discard, reader, int64(length))
	default:
		return ErrConnectionFailed
	}
	if err != nil {
		return err
	}
	_, err = io.CopyN(io.Discard, reader, 2)
	return err
}

// proxyAuthorization returns a header value for in-memory bridge use only.
func proxyAuthorization(proxy *ResolvedProxy) string {
	if proxy == nil || proxy.Username == "" {
		return ""
	}
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(proxy.Username+":"+proxy.Password))
}

func dialHTTPProxy(ctx context.Context, proxy *ResolvedProxy, timeout time.Duration) (net.Conn, error) {
	dialer := net.Dialer{Timeout: timeout}
	connection, err := dialer.DialContext(ctx, "tcp", proxy.address())
	if err != nil {
		return nil, ErrConnectionFailed
	}
	if proxy.Scheme == SchemeHTTPS {
		tlsConnection := tls.Client(connection, &tls.Config{ServerName: proxy.Host, MinVersion: tls.VersionTLS12})
		if err := tlsConnection.HandshakeContext(ctx); err != nil {
			_ = connection.Close()
			return nil, ErrConnectionFailed
		}
		return tlsConnection, nil
	}
	return connection, nil
}
