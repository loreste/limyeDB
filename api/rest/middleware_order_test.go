package rest

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/limyedb/limyedb/pkg/collection"
	"github.com/limyedb/limyedb/pkg/config"
)

// newAuthTestServer builds a server with authentication enabled.
func newAuthTestServer(t *testing.T, token string) *Server {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "limyedb-mworder")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	mgr, err := collection.NewManager(&collection.ManagerConfig{
		DataDir:        tmpDir,
		MaxCollections: 10,
	})
	if err != nil {
		t.Fatalf("failed to create collection manager: %v", err)
	}

	return NewServerWithOptions(&config.ServerConfig{
		RESTAddress:    ":0",
		ReadTimeout:    config.Duration(5 * time.Second),
		WriteTimeout:   config.Duration(5 * time.Second),
		MaxRequestSize: 1 << 20,
	}, mgr, &ServerOptions{
		AuthToken:      token,
		AllowedOrigins: []string{"https://allowed.example"},
	})
}

// TestAuthMiddlewareAppliesToRegisteredRoutes guards against a regression where
// setupRoutes ran before setupMiddleware. Gin merges group middleware into a
// route's handler chain when the route is registered, so routes registered
// first bypassed the entire chain -- including authentication. The only path
// that returned 401 was one that did not exist, because gin rebuilds the
// NoRoute chain on Use().
func TestAuthMiddlewareAppliesToRegisteredRoutes(t *testing.T) {
	srv := newAuthTestServer(t, "super-secret-token")

	protected := []struct{ method, path string }{
		{http.MethodGet, "/collections"},
		{http.MethodPost, "/collections"},
		{http.MethodGet, "/collections/foo"},
		{http.MethodDelete, "/collections/foo"},
		{http.MethodPatch, "/collections/foo"},
		{http.MethodGet, "/aliases"},
		{http.MethodGet, "/snapshots"},
		{http.MethodGet, "/metrics"},
		{http.MethodGet, "/definitely-not-a-route"},
	}

	for _, tc := range protected {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rr := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rr, req)

			if rr.Code != http.StatusUnauthorized {
				t.Errorf("%s %s without a token = %d, want %d",
					tc.method, tc.path, rr.Code, http.StatusUnauthorized)
			}
		})
	}
}

// TestHealthEndpointsBypassAuth documents that liveness and readiness stay
// reachable without credentials, so orchestrators can probe them.
func TestHealthEndpointsBypassAuth(t *testing.T) {
	srv := newAuthTestServer(t, "super-secret-token")

	for _, path := range []string{"/health", "/readiness"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rr := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rr, req)

			if rr.Code == http.StatusUnauthorized {
				t.Errorf("GET %s = 401, want it to bypass auth", path)
			}
		})
	}
}

// TestValidTokenIsAccepted confirms the fix rejects only unauthenticated calls.
func TestValidTokenIsAccepted(t *testing.T) {
	const token = "super-secret-token"
	srv := newAuthTestServer(t, token)

	req := httptest.NewRequest(http.MethodGet, "/collections", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)

	if rr.Code == http.StatusUnauthorized {
		t.Errorf("GET /collections with a valid token = 401, want it accepted")
	}
}

// TestCORSPreflightPrecedesAuth checks that a browser preflight is answered
// rather than rejected. Preflight requests carry no Authorization header, so if
// authentication runs before CORS every cross-origin browser client breaks.
func TestCORSPreflightPrecedesAuth(t *testing.T) {
	srv := newAuthTestServer(t, "super-secret-token")

	req := httptest.NewRequest(http.MethodOptions, "/collections", nil)
	req.Header.Set("Origin", "https://allowed.example")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)

	if rr.Code == http.StatusUnauthorized {
		t.Fatalf("OPTIONS preflight = 401, want CORS to answer before auth")
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://allowed.example" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "https://allowed.example")
	}
}

// TestNoAuthTokenLeavesAPIOpen pins the documented behavior that omitting a
// token disables authentication, so the fail-closed change in checkPermission
// does not break unauthenticated deployments.
func TestNoAuthTokenLeavesAPIOpen(t *testing.T) {
	srv := newAuthTestServer(t, "")

	req := httptest.NewRequest(http.MethodGet, "/collections", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET /collections with auth disabled = %d, want %d", rr.Code, http.StatusOK)
	}
}

// TestPanicRecoveryAppliesToRoutes verifies gin.Recovery() actually wraps
// registered routes. Before the ordering fix it did not, so a panic in any
// handler would have taken the process down instead of returning a 500.
func TestPanicRecoveryAppliesToRoutes(t *testing.T) {
	srv := newAuthTestServer(t, "")
	srv.router.GET("/boom", func(_ *gin.Context) {
		panic("intentional panic for recovery test")
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	rr := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic escaped gin.Recovery(): %v", r)
		}
	}()
	srv.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("panicking handler = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}
