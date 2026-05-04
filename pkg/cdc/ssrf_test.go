package cdc

import (
	"strings"
	"testing"
)

func TestValidateSubscriptionURLRejects(t *testing.T) {
	cases := []struct {
		url     string
		wantSub string // substring expected in error
	}{
		{"", "empty"},
		{"://no-scheme", "scheme"},
		{"ftp://example.com/", "scheme"},
		{"file:///etc/passwd", "scheme"},
		{"http://localhost/hook", "localhost"},
		{"http://LOCALHOST/hook", "localhost"},
		{"http://127.0.0.1/hook", "private/reserved"},
		{"http://10.0.0.5/hook", "private/reserved"},
		{"http://192.168.1.1/hook", "private/reserved"},
		{"http://172.16.0.1/hook", "private/reserved"},
		{"http://169.254.169.254/latest/meta-data/", "private/reserved"}, // AWS IMDS
		{"http://[::1]/hook", "private/reserved"},
		{"http://0.0.0.0/hook", "private/reserved"},
	}
	for _, tc := range cases {
		err := validateSubscriptionURL(tc.url)
		if err == nil {
			t.Errorf("%q was accepted; expected rejection (substring %q)", tc.url, tc.wantSub)
			continue
		}
		if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.wantSub)) {
			t.Errorf("%q rejected with %q, expected substring %q", tc.url, err, tc.wantSub)
		}
	}
}

func TestValidateSubscriptionURLAccepts(t *testing.T) {
	// Public-network URLs should be accepted. Use IP literals so the test
	// does not depend on DNS resolution being available in the sandbox.
	cases := []string{
		"http://93.184.216.34/hook",     // example.com (TEST IP)
		"https://8.8.8.8/v1/cdc",        // Google public DNS, public IP
		"http://198.51.100.1:8443/hook", // TEST-NET-2 (RFC 5737), public-routed
	}
	for _, u := range cases {
		if err := validateSubscriptionURL(u); err != nil {
			t.Errorf("%q was rejected: %v", u, err)
		}
	}
}

func TestDispatcherSubscribeRejectsSSRF(t *testing.T) {
	d := &Dispatcher{subscriptions: make(map[string][]WebhookSubscription)}

	if err := d.Subscribe("c", WebhookSubscription{URL: "http://127.0.0.1/x"}); err == nil {
		t.Error("Subscribe accepted loopback URL; expected SSRF rejection")
	}
	if got := len(d.Subscriptions("c")); got != 0 {
		t.Errorf("rejected subscription was still recorded: count=%d", got)
	}
	// Sanity: a public IP literal still works.
	if err := d.Subscribe("c", WebhookSubscription{URL: "https://8.8.8.8/x"}); err != nil {
		t.Errorf("Subscribe rejected legitimate URL: %v", err)
	}
}
