package limyedb_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/limyedb/limyedb/clients/go/limyedb"
)

// TestWithAuthTokenSetsHeader verifies the client sends the bearer token, which
// is required to reach a server started with -auth-token. The client previously
// set no Authorization header at all, so it could not authenticate.
func TestWithAuthTokenSetsHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := limyedb.NewClient(srv.URL, limyedb.WithAuthToken("secret-token"))
	// Any call exercises the shared request path.
	_ = client.DeleteCollection("whatever")

	if gotAuth != "Bearer secret-token" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer secret-token")
	}
}

// TestNoAuthTokenSendsNoHeader confirms the anonymous path stays header-free.
func TestNoAuthTokenSendsNoHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := limyedb.NewClient(srv.URL)
	_ = client.DeleteCollection("whatever")

	if gotAuth != "" {
		t.Errorf("Authorization header = %q, want it unset", gotAuth)
	}
}
