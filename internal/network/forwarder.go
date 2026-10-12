package network

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const bridgeDialTimeout = 15 * time.Second

// LoopbackBridge keeps authenticated upstream proxy material out of native
// browser arguments. It accepts credential-free HTTP CONNECT on loopback and
// injects authentication only into the upstream connection in memory.
type LoopbackBridge struct {
	listener net.Listener
	upstream ResolvedProxy
	done     chan struct{}
	once     sync.Once
	mu       sync.Mutex
	clients  map[net.Conn]struct{}
	closed   bool
}

func StartLoopbackBridge(upstream *ResolvedProxy) (*LoopbackBridge, error) {
	return StartLoopbackBridgeContext(context.Background(), upstream)
}

func StartLoopbackBridgeContext(ctx context.Context, upstream *ResolvedProxy) (*LoopbackBridge, error) {
	if upstream == nil || upstream.Validate() != nil {
		return nil, ErrInvalidConfig
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, errors.New("start loopback proxy bridge")
	}
	bridge := &LoopbackBridge{
		listener: listener,
		upstream: *upstream,
		done:     make(chan struct{}),
		clients:  make(map[net.Conn]struct{}),
	}
	go bridge.serve()
	go func() {
		select {
		case <-ctx.Done():
			_ = bridge.Close()
		case <-bridge.done:
		}
	}()
	return bridge, nil
}

func (b *LoopbackBridge) EngineProxy() EngineProxy {
	_, portText, err := net.SplitHostPort(b.listener.Addr().String())
	if err != nil {
		return EngineProxy{Mode: ModeProxy}
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil || port == 0 {
		return EngineProxy{Mode: ModeProxy}
	}
	return EngineProxy{Mode: ModeProxy, Scheme: SchemeHTTP, Host: "127.0.0.1", Port: uint16(port)}
}

func (b *LoopbackBridge) Done() <-chan struct{} {
	return b.done
}

func (b *LoopbackBridge) Close() error {
	var closeErr error
	b.once.Do(func() {
		closeErr = b.listener.Close()
		b.mu.Lock()
		b.closed = true
		for client := range b.clients {
			_ = client.Close()
		}
		b.mu.Unlock()
	})
	if errors.Is(closeErr, net.ErrClosed) {
		return nil
	}
	return closeErr
}

func (b *LoopbackBridge) serve() {
	defer close(b.done)
	for {
		connection, err := b.listener.Accept()
		if err != nil {
			return
		}
		go b.handle(connection)
	}
}

func (b *LoopbackBridge) handle(client net.Conn) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		_ = client.Close()
		return
	}
	b.clients[client] = struct{}{}
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.clients, client)
		b.mu.Unlock()
	}()
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(bridgeDialTimeout))
	reader := bufio.NewReader(client)
	request, err := http.ReadRequest(reader)
	if err != nil || request.Method != http.MethodConnect || request.Host == "" {
		writeBridgeError(client, http.StatusBadRequest)
		return
	}
	upstream, err := b.connectTarget(request.Context(), request.Host)
	if err != nil {
		writeBridgeError(client, http.StatusBadGateway)
		return
	}
	defer upstream.Close()
	_ = client.SetDeadline(time.Time{})
	_ = upstream.SetDeadline(time.Time{})
	if _, err := io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	clientSource := io.Reader(client)
	if reader.Buffered() > 0 {
		clientSource = io.MultiReader(reader, client)
	}
	done := make(chan struct{}, 1)
	go func() {
		_, _ = io.Copy(upstream, clientSource)
		if tcp, ok := upstream.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
		done <- struct{}{}
	}()
	_, _ = io.Copy(client, upstream)
	<-done
}

func (b *LoopbackBridge) connectTarget(ctx context.Context, target string) (net.Conn, error) {
	switch b.upstream.Scheme {
	case SchemeHTTP, SchemeHTTPS:
		connection, err := dialHTTPProxy(ctx, &b.upstream, bridgeDialTimeout)
		if err != nil {
			return nil, ErrConnectionFailed
		}
		if err := connection.SetDeadline(time.Now().Add(bridgeDialTimeout)); err != nil {
			_ = connection.Close()
			return nil, ErrConnectionFailed
		}
		request := "CONNECT " + target + " HTTP/1.1\r\nHost: " + target + "\r\n"
		if authorization := proxyAuthorization(&b.upstream); authorization != "" {
			request += "Proxy-Authorization: " + authorization + "\r\n"
		}
		request += "Proxy-Connection: Keep-Alive\r\n\r\n"
		if _, err := io.WriteString(connection, request); err != nil {
			_ = connection.Close()
			return nil, ErrConnectionFailed
		}
		reader := bufio.NewReader(connection)
		response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodConnect})
		if err != nil {
			_ = connection.Close()
			return nil, ErrConnectionFailed
		}
		_ = response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			_ = connection.Close()
			return nil, ErrConnectionFailed
		}
		if reader.Buffered() > 0 {
			return &readerConn{Conn: connection, reader: reader}, nil
		}
		return connection, nil
	case SchemeSOCKS5:
		return socks5DialContext(&b.upstream, bridgeDialTimeout)(ctx, "tcp", target)
	default:
		return nil, ErrConnectionFailed
	}
}

type readerConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *readerConn) Read(buffer []byte) (int, error) {
	return c.reader.Read(buffer)
}

func writeBridgeError(connection net.Conn, status int) {
	text := http.StatusText(status)
	_, _ = fmt.Fprintf(connection, "HTTP/1.1 %d %s\r\nContent-Length: 0\r\nConnection: close\r\n\r\n", status, text)
}
