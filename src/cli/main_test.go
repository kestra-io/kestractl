package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// TestMain fences the unit suite off from the network.
//
// Several commands talk to hosts we do not control — api.kestra.io for the
// PostHog telemetry config, api.github.com for the update notifier,
// repo1.maven.org for plugin downloads — and every test that exercises them is
// expected to point the corresponding package var at an httptest server. That
// convention is invisible until it is broken: a test that forgets an override
// still passes, quietly making a real request, and only shows up later as a
// flake on an offline machine or a rate-limited CI runner.
//
// So the dialer under the tests refuses anything that is not loopback. A missed
// override fails with a message naming the address instead of succeeding by
// accident.
func TestMain(m *testing.M) {
	blockExternalNetwork()
	os.Exit(m.Run())
}

// errExternalNetwork is returned for any non-loopback dial attempt.
type errExternalNetwork struct {
	network string
	address string
}

func (e *errExternalNetwork) Error() string {
	return fmt.Sprintf(
		"unit tests must not reach the network: blocked %s dial to %s. "+
			"Point the relevant package var (telemetryConfigURL, newVersionCheckLatestReleaseURL, pluginsAPIBase, pluginsMavenBase, ...) at an httptest server",
		e.network, e.address,
	)
}

func blockExternalNetwork() {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		panic("http.DefaultTransport is not *http.Transport; cannot install the network guard")
	}

	// Mutated in place on purpose: newCompatHTTPClient captures
	// http.DefaultTransport as its base at construction time, so replacing the
	// package var would leave the SDK path unguarded.
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if !isLoopbackAddress(address) {
			return nil, &errExternalNetwork{network: network, address: address}
		}
		return dialer.DialContext(ctx, network, address)
	}
	// A proxy in the developer's or runner's environment would otherwise turn
	// every external request into a loopback dial and slip past the guard.
	transport.Proxy = nil
}

func isLoopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}

	if host == "localhost" {
		return true
	}

	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// TestNetworkGuard_BlocksExternalDialsAndAllowsLoopback keeps TestMain honest.
// Without it the guard could become a silent no-op — a changed transport type, a
// client that builds its own — and nothing else in the suite would notice.
//
// The external case is itself hermetic: http.Transport hands the dialer the
// hostname, so the guard rejects it before any DNS lookup happens.
func TestNetworkGuard_BlocksExternalDialsAndAllowsLoopback(t *testing.T) {
	if _, err := http.Get("https://api.github.com/repos/kestra-io/kestractl/releases/latest"); err == nil {
		t.Fatal("expected the external request to be blocked")
	} else if !strings.Contains(err.Error(), "unit tests must not reach the network") {
		t.Fatalf("expected the guard's error, got %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()

	res, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("loopback request must still work, got %v", err)
	}
	_ = res.Body.Close()
}
