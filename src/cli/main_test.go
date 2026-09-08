package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
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
// So the dialer under the tests refuses anything that is not loopback. Blocking
// the dial is not enough on its own: several of those fetches are best-effort
// and swallow the client error (fetchPosthogConfig returns "", ""; the update
// notifier just gives up), so the test that forgot its override would still
// pass. Every blocked address is therefore recorded, and the run fails here
// after m.Run() even if no assertion noticed.
func TestMain(m *testing.M) {
	blockExternalNetwork()
	code := m.Run()

	if blocked := blockedExternalDials(); len(blocked) > 0 {
		fmt.Fprintf(os.Stderr, "\nunit tests must not reach the network, but %d external dial(s) were attempted:\n", len(blocked))
		for _, address := range blocked {
			fmt.Fprintf(os.Stderr, "  - %s\n", address)
		}
		fmt.Fprint(os.Stderr, "Point the relevant package var (telemetryConfigURL, newVersionCheckLatestReleaseURL, pluginsAPIBase, pluginsMavenBase, ...) at an httptest server.\n")
		if code == 0 {
			code = 1
		}
	}

	os.Exit(code)
}

// guardSelfTestHost is the target of the guard's own self-test. It is blocked
// like any other external address but deliberately left out of the report, so
// the self-test does not fail the run it is verifying. `.invalid` is reserved by
// RFC 2606 and never resolves, which keeps the self-test offline-safe: if the
// guard ever regresses into a no-op, the request fails on DNS rather than
// actually reaching a host we do not control.
const guardSelfTestHost = "kestractl-network-guard.invalid"

var (
	blockedDialsMu sync.Mutex
	blockedDials   = map[string]struct{}{}
)

func recordBlockedDial(address string) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	if host == guardSelfTestHost {
		return
	}

	blockedDialsMu.Lock()
	defer blockedDialsMu.Unlock()
	blockedDials[address] = struct{}{}
}

func blockedExternalDials() []string {
	blockedDialsMu.Lock()
	defer blockedDialsMu.Unlock()

	addresses := make([]string, 0, len(blockedDials))
	for address := range blockedDials {
		addresses = append(addresses, address)
	}
	sort.Strings(addresses)
	return addresses
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
			recordBlockedDial(address)
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
// hostname, so the guard rejects it before any DNS lookup happens, and the
// hostname is an unresolvable `.invalid` one so a regressed guard fails offline
// rather than reaching a real host.
func TestNetworkGuard_BlocksExternalDialsAndAllowsLoopback(t *testing.T) {
	if _, err := http.Get("https://" + guardSelfTestHost + "/repos/kestra-io/kestractl/releases/latest"); err == nil {
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

// TestNetworkGuard_ReportsBlockedDialsExceptTheSelfTest covers the reporting
// half of the guard: a blocked address must land in the report TestMain reads,
// while the self-test's own target must not.
func TestNetworkGuard_ReportsBlockedDialsExceptTheSelfTest(t *testing.T) {
	recordBlockedDial(guardSelfTestHost + ":443")
	for _, address := range blockedExternalDials() {
		if strings.HasPrefix(address, guardSelfTestHost) {
			t.Fatalf("the self-test target must not be reported, got %q", address)
		}
	}

	const sentinel = "api.example.test:443"
	recordBlockedDial(sentinel)
	defer func() {
		blockedDialsMu.Lock()
		delete(blockedDials, sentinel)
		blockedDialsMu.Unlock()
	}()

	found := false
	for _, address := range blockedExternalDials() {
		if address == sentinel {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected %q in the blocked-dial report, got %v", sentinel, blockedExternalDials())
	}
}
