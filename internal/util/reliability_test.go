package util

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestReliabilityBoundedAmounts(t *testing.T) {
	for _, value := range []string{"0", "-1", "NaN", "Inf", "1e999999999", "1e-999999999", "1.0000001", "1000000000000", strings.Repeat("1", 1000), " 1", "+1"} {
		if _, err := ParseAmount(value); err == nil {
			t.Fatal("invalid amount accepted", value)
		}
	}
	for _, value := range []string{"0.01", "1.000001", "999999999999.999999"} {
		if _, err := ParseAmount(value); err != nil {
			t.Fatal(value, err)
		}
	}
}

func TestReliabilityPublicTransport(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); w.Write([]byte("success")) }))
	defer server.Close()
	client := NewPublicHTTPClient(time.Second)
	if response, err := client.Get(server.URL); err == nil {
		response.Body.Close()
		t.Fatal("private destination reached")
	}
	if hits.Load() != 0 {
		t.Fatal("private server received request")
	}
	if client.CheckRedirect(nil, nil) != http.ErrUseLastResponse {
		t.Fatal("redirects followed")
	}
	for _, value := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "::1", "::ffff:127.0.0.1", "224.0.0.1", "0.0.0.0", "100.64.0.1", "64:ff9b::7f00:1", "2001:db8::1"} {
		if !IsPrivateIP(net.ParseIP(value)) {
			t.Fatal("unsafe IP accepted", value)
		}
	}
	if IsPrivateIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("public IP rejected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if conn, err := DialPublicContext(ctx, "tcp", "127.0.0.1:80"); err == nil {
		conn.Close()
		t.Fatal("private dial allowed")
	}
}
