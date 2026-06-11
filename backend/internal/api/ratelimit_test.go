package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandlerRL(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

// withRawIP injects a raw IP into the context the same way PreserveRawIP does,
// so unit tests can exercise the trustProxy=false path without wiring the full
// global middleware chain.
func withRawIP(r *http.Request, ip string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), rawIPKey{}, ip))
}

// TestBurstExhaustion verifies that burst tokens are consumed and the (burst+1)th
// request receives 429 with a Retry-After header.
func TestBurstExhaustion(t *testing.T) {
	mw := IPRateLimit(600, 2, false) // burst=2
	handler := mw(http.HandlerFunc(okHandlerRL))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req = withRawIP(req, "10.0.0.1")
		req.RemoteAddr = "10.0.0.1:9999"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, rr.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req = withRawIP(req, "10.0.0.1")
	req.RemoteAddr = "10.0.0.1:9999"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after burst exhausted, got %d", rr.Code)
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header on 429 response")
	}
}

// TestDifferentIPsSeparateBuckets verifies that distinct IPs each get their
// own bucket and do not consume from each other's quota.
func TestDifferentIPsSeparateBuckets(t *testing.T) {
	mw := IPRateLimit(600, 1, false) // burst=1 per IP
	handler := mw(http.HandlerFunc(okHandlerRL))

	for _, ip := range []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"} {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req = withRawIP(req, ip)
		req.RemoteAddr = ip + ":9999"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("IP %s: expected 200, got %d", ip, rr.Code)
		}
	}
}

// TestTrustProxy_XRealIP verifies that when trustProxy=true, X-Real-IP is used
// as the rate-limit key, so two requests from the same real IP share a bucket
// even when sent from the same proxy address.
func TestTrustProxy_XRealIP(t *testing.T) {
	mw := IPRateLimit(600, 1, true) // burst=1, trust proxy
	handler := mw(http.HandlerFunc(okHandlerRL))

	makeReq := func(realIP string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.RemoteAddr = "10.0.0.100:9999" // proxy IP — same for all requests
		req.Header.Set("X-Real-IP", realIP)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}

	if rr := makeReq("1.2.3.4"); rr.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", rr.Code)
	}
	// Same real IP, same proxy → bucket exhausted
	if rr := makeReq("1.2.3.4"); rr.Code != http.StatusTooManyRequests {
		t.Fatalf("second request same IP: expected 429, got %d", rr.Code)
	}
	// Different real IP → own fresh bucket
	if rr := makeReq("5.6.7.8"); rr.Code != http.StatusOK {
		t.Fatalf("different real IP: expected 200, got %d", rr.Code)
	}
}

// TestNoTrustProxy_IgnoresXRealIP verifies that when trustProxy=false, X-Real-IP
// is completely ignored, so a client cannot bypass per-IP limits by rotating
// the header value.
func TestNoTrustProxy_IgnoresXRealIP(t *testing.T) {
	mw := IPRateLimit(600, 1, false) // burst=1, do not trust proxy headers
	handler := mw(http.HandlerFunc(okHandlerRL))

	makeReq := func(remoteAddr, xRealIP string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req = withRawIP(req, remoteAddr) // simulate what PreserveRawIP saves
		req.RemoteAddr = remoteAddr + ":9999"
		if xRealIP != "" {
			req.Header.Set("X-Real-IP", xRealIP)
		}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}

	// First request exhausts the single-token burst for 10.0.0.100
	if rr := makeReq("10.0.0.100", "9.9.9.1"); rr.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", rr.Code)
	}
	// Second request from same TCP address with a *different* X-Real-IP must still
	// be rate-limited (the header is ignored when trustProxy=false).
	if rr := makeReq("10.0.0.100", "9.9.9.2"); rr.Code != http.StatusTooManyRequests {
		t.Fatalf("spoofed X-Real-IP must not bypass rate limit, got %d", rr.Code)
	}
}

// TestTrustProxy_XForwardedFor verifies that X-Forwarded-For leftmost IP is
// used as the rate-limit key when X-Real-IP is absent and trustProxy=true.
func TestTrustProxy_XForwardedFor(t *testing.T) {
	mw := IPRateLimit(600, 1, true)
	handler := mw(http.HandlerFunc(okHandlerRL))

	makeReq := func(xff string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.RemoteAddr = "10.0.0.100:9999"
		req.Header.Set("X-Forwarded-For", xff)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}

	if rr := makeReq("2.3.4.5, 10.0.0.100"); rr.Code != http.StatusOK {
		t.Fatalf("first: expected 200, got %d", rr.Code)
	}
	// Same leftmost IP → same bucket → 429
	if rr := makeReq("2.3.4.5, 10.0.0.100"); rr.Code != http.StatusTooManyRequests {
		t.Fatalf("second same IP: expected 429, got %d", rr.Code)
	}
}
