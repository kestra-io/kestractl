package cli

import (
	"bytes"
	"crypto/sha1" // nosemgrep: go.lang.security.audit.crypto.use_of_weak_crypto.use-of-sha1
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeMavenRepository serves plugin-split JARs for the given versions, with their .sha1, and 404s
// anything else. Each JAR's content is the repository name, so a test can tell where it came from.
type fakeMavenRepository struct {
	name     string
	server   *httptest.Server
	mu       sync.Mutex
	requests []string
}

func newFakeMavenRepository(t *testing.T, name string, versions ...string) *fakeMavenRepository {
	t.Helper()
	repo := &fakeMavenRepository{name: name}
	jars := make(map[string]bool, len(versions))
	for _, v := range versions {
		jars[fmt.Sprintf("/io/kestra/plugin/plugin-split/%s/plugin-split-%s.jar", v, v)] = true
	}
	sum := sha1.Sum([]byte(name)) // nosemgrep: go.lang.security.audit.crypto.use_of_weak_crypto.use-of-sha1
	repo.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		repo.mu.Lock()
		repo.requests = append(repo.requests, r.URL.Path)
		repo.mu.Unlock()
		switch {
		case jars[r.URL.Path]:
			fmt.Fprint(w, name)
		case strings.HasSuffix(r.URL.Path, ".sha1") && jars[strings.TrimSuffix(r.URL.Path, ".sha1")]:
			fmt.Fprint(w, hex.EncodeToString(sum[:]))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(repo.server.Close)
	return repo
}

func (r *fakeMavenRepository) jarRequested(version string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, path := range r.requests {
		if strings.HasSuffix(path, "/plugin-split-"+version+".jar") {
			return true
		}
	}
	return false
}

func splitPlugin(version string) string {
	return "io.kestra.plugin:plugin-split:" + version
}

func readSplitJAR(t *testing.T, dir string, version string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(dir, pluginFileName(pluginArtifact{GroupID: "io.kestra.plugin", ArtifactID: "plugin-split", Version: version})))
	if err != nil {
		t.Fatalf("plugin-split %s was not downloaded: %v", version, err)
	}
	return string(content)
}

// A plugin whose older releases (1.0.0 to 3.0.0) are on Maven Central and whose newer ones (4.0.0, 5.0.0) are
// only on the Kestra repository, as happens once releases move off Central: every version must download,
// each from the repository holding it.
func TestRunPluginsGet_PluginSplitAcrossRepositories(t *testing.T) {
	central := newFakeMavenRepository(t, "central", "1.0.0", "2.0.0", "3.0.0")
	kestra := newFakeMavenRepository(t, "kestra", "4.0.0", "5.0.0")
	dir := t.TempDir()

	var out bytes.Buffer
	for _, version := range []string{"1.0.0", "2.0.0", "3.0.0", "4.0.0", "5.0.0"} {
		if err := runPluginsGet(&out, splitPlugin(version), "", dir, false, nil, []string{central.server.URL, kestra.server.URL}, "", "", 5*time.Minute); err != nil {
			t.Fatalf("plugin-split %s: unexpected error: %v\n%s", version, err, out.String())
		}
	}

	for version, want := range map[string]string{"1.0.0": "central", "2.0.0": "central", "3.0.0": "central", "4.0.0": "kestra", "5.0.0": "kestra"} {
		if got := readSplitJAR(t, dir, version); got != want {
			t.Errorf("plugin-split %s: expected the JAR from %s, got it from %s", version, want, got)
		}
	}
	for _, version := range []string{"1.0.0", "2.0.0", "3.0.0"} {
		if kestra.jarRequested(version) {
			t.Errorf("plugin-split %s was found on central, the kestra repository should not have been asked for it", version)
		}
	}
	// The JARs not on central have no .sha1 there either: that must not read as a missing checksum.
	if strings.Contains(out.String(), "[warn]") {
		t.Errorf("expected no warning, got:\n%s", out.String())
	}
}

// The same split, through "plugins download": the default repositories are used when no
// --maven-repository is given.
func TestRunPluginsInstall_DefaultRepositoriesAreTriedInOrder(t *testing.T) {
	central := newFakeMavenRepository(t, "central", "1.0.0", "2.0.0", "3.0.0")
	kestra := newFakeMavenRepository(t, "kestra", "4.0.0", "5.0.0")
	orig := pluginsMavenRepositories
	pluginsMavenRepositories = []string{central.server.URL, kestra.server.URL}
	t.Cleanup(func() { pluginsMavenRepositories = orig })
	dir := t.TempDir()

	explicit := []pluginArtifact{
		{GroupID: "io.kestra.plugin", ArtifactID: "plugin-split", Version: "3.0.0"},
		{GroupID: "io.kestra.plugin", ArtifactID: "plugin-split", Version: "5.0.0"},
	}
	var out bytes.Buffer
	if err := runPluginsInstall(&out, "", dir, 1, false, "", false, 5*time.Minute, nil, "", "", nil, explicit); err != nil {
		t.Fatalf("unexpected error: %v\n%s", err, out.String())
	}

	if got := readSplitJAR(t, dir, "3.0.0"); got != "central" {
		t.Errorf("plugin-split 3.0.0: expected the JAR from central, got it from %s", got)
	}
	if got := readSplitJAR(t, dir, "5.0.0"); got != "kestra" {
		t.Errorf("plugin-split 5.0.0: expected the JAR from kestra, got it from %s", got)
	}
}

func TestRunPluginsGet_MissingFromEveryRepositoryNamesThemAll(t *testing.T) {
	central := newFakeMavenRepository(t, "central", "1.0.0")
	kestra := newFakeMavenRepository(t, "kestra", "4.0.0")

	var out bytes.Buffer
	err := runPluginsGet(&out, splitPlugin("9.0.0"), "", t.TempDir(), false, nil, []string{central.server.URL, kestra.server.URL}, "", "", 5*time.Minute)

	if !errors.Is(err, errArtifactNotFound) {
		t.Fatalf("expected an artifact-not-found error, got: %v", err)
	}
	for _, repo := range []*fakeMavenRepository{central, kestra} {
		if !strings.Contains(err.Error(), repo.server.URL+"/io/kestra/plugin/plugin-split/9.0.0/plugin-split-9.0.0.jar") {
			t.Errorf("expected the error to name the %s URL, got: %v", repo.name, err)
		}
	}
}

// Only a 404 moves on to the next repository: a failing repository must not be silently skipped,
// or a broken mirror would go unnoticed.
func TestRunPluginsGet_ServerErrorDoesNotFallThrough(t *testing.T) {
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(broken.Close)
	kestra := newFakeMavenRepository(t, "kestra", "4.0.0")

	var out bytes.Buffer
	err := runPluginsGet(&out, splitPlugin("4.0.0"), "", t.TempDir(), false, nil, []string{broken.URL, kestra.server.URL}, "", "", 5*time.Minute)

	if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("expected the HTTP 500 error, got: %v", err)
	}
	if kestra.jarRequested("4.0.0") {
		t.Error("expected the second repository not to be asked after a server error")
	}
}

func TestResolveMavenRepositories(t *testing.T) {
	got, err := resolveMavenRepositories(nil, "", "", nil)
	if err != nil || len(got) != len(pluginsMavenRepositories) || got[0].url != pluginsMavenRepositories[0] {
		t.Errorf("expected the default repositories without --maven-repository, got %v (%v)", got, err)
	}

	got, err = resolveMavenRepositories([]string{"https://a.example/maven2/", " ", "https://alice:s%3Acr3t@b.example/maven"}, "", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0].url != "https://a.example/maven2" || got[1].url != "https://b.example/maven" {
		t.Errorf("expected blank entries dropped, trailing slashes and credentials stripped, got %+v", got)
	}
	if got[0].username != "" || got[1].username != "alice" || got[1].password != "s:cr3t" {
		t.Errorf("expected the URL credentials on their own repository only, got %+v", got)
	}
}

// Shared credentials sent to several repositories would leak a private repository's password
// to the public ones, Maven Central included.
func TestResolveMavenRepositories_SharedCredentialsNeedASingleRepository(t *testing.T) {
	two := []string{"https://repo1.maven.org/maven2", "https://nexus.example.com/repository/kestra"}
	for name, call := range map[string]func() error{
		"username": func() error { _, err := resolveMavenRepositories(two, "alice", "", nil); return err },
		"password": func() error { _, err := resolveMavenRepositories(two, "", "s3cr3t", nil); return err },
		"header": func() error {
			_, err := resolveMavenRepositories(two, "", "", map[string]string{"Authorization": "Bearer x"})
			return err
		},
	} {
		if err := call(); err == nil || !strings.Contains(err.Error(), "own URL") {
			t.Errorf("%s with two repositories: expected an error pointing at per-URL credentials, got %v", name, err)
		}
	}

	got, err := resolveMavenRepositories(two[1:], "alice", "s3cr3t", map[string]string{"Authorization": "Bearer x"})
	if err != nil || got[0].username != "alice" || got[0].headers["Authorization"] != "Bearer x" {
		t.Errorf("expected shared credentials to apply to a single repository, got %+v (%v)", got, err)
	}
}

func TestResolveMavenRepositories_InvalidURLDoesNotPrintIt(t *testing.T) {
	_, err := resolveMavenRepositories([]string{"https://repo1.maven.org/maven2", "alice:s3cr3t@nexus"}, "", "", nil)
	if err == nil || strings.Contains(err.Error(), "s3cr3t") {
		t.Errorf("expected an error that does not print the URL, got %v", err)
	}
}

func TestRunPluginsGet_URLCredentialsGoToTheirRepositoryOnly(t *testing.T) {
	var centralAuth []string
	central := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		centralAuth = append(centralAuth, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(central.Close)
	var privateUser, privatePassword string
	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		privateUser, privatePassword, _ = r.BasicAuth()
		if !strings.HasSuffix(r.URL.Path, ".jar") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Fprint(w, "private")
	}))
	t.Cleanup(private.Close)
	privateWithCredentials := strings.Replace(private.URL, "http://", "http://alice:s3cr3t@", 1)

	var out bytes.Buffer
	if err := runPluginsGet(&out, splitPlugin("4.0.0"), "", t.TempDir(), false, nil, []string{central.URL, privateWithCredentials}, "", "", 5*time.Minute); err != nil {
		t.Fatalf("unexpected error: %v\n%s", err, out.String())
	}

	for _, auth := range centralAuth {
		if auth != "" {
			t.Errorf("expected no credentials sent to the first repository, got %q", auth)
		}
	}
	if privateUser != "alice" || privatePassword != "s3cr3t" {
		t.Errorf("expected the URL credentials sent to their repository, got %q:%q", privateUser, privatePassword)
	}
	if strings.Contains(out.String(), "s3cr3t") {
		t.Errorf("expected the password never printed, got:\n%s", out.String())
	}
}

// A repository without the artifact costs one request (the JAR, not its .sha1 first), and a .sha1
// error there is not reported, since that repository does not serve the JAR anyway.
func TestRunPluginsGet_RepositoryMissCostsOneRequestAndNoWarning(t *testing.T) {
	var centralRequests []string
	central := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		centralRequests = append(centralRequests, r.URL.Path)
		if strings.HasSuffix(r.URL.Path, ".sha1") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(central.Close)
	kestra := newFakeMavenRepository(t, "kestra", "4.0.0")

	var out bytes.Buffer
	if err := runPluginsGet(&out, splitPlugin("4.0.0"), "", t.TempDir(), false, nil, []string{central.URL, kestra.server.URL}, "", "", 5*time.Minute); err != nil {
		t.Fatalf("unexpected error: %v\n%s", err, out.String())
	}

	if len(centralRequests) != 1 || !strings.HasSuffix(centralRequests[0], ".jar") {
		t.Errorf("expected a single JAR request to the repository without the artifact, got %v", centralRequests)
	}
	if strings.Contains(strings.ToLower(out.String()), "[warn]") {
		t.Errorf("expected no warning, got:\n%s", out.String())
	}
}
