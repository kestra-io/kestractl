package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// runPluginsCommand runs "kestractl plugins <args>" through the root command, so flag parsing and the
// environment fallback are exercised as a user would hit them.
func runPluginsCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	t.Setenv("KESTRACTL_TELEMETRY_DISABLED", "true")
	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"plugins"}, args...))
	err := root.Execute()
	return out.String(), err
}

func TestPluginsGet_MavenRepositoriesFromEnv(t *testing.T) {
	central := newFakeMavenRepository(t, "central", "1.0.0", "2.0.0", "3.0.0")
	kestra := newFakeMavenRepository(t, "kestra", "4.0.0", "5.0.0")
	t.Setenv("KESTRACTL_MAVEN_REPOSITORY", central.server.URL+", "+kestra.server.URL)
	dir := t.TempDir()

	for _, version := range []string{"2.0.0", "5.0.0"} {
		if out, err := runPluginsCommand(t, "get", splitPlugin(version), "--plugins-dir", dir); err != nil {
			t.Fatalf("plugin-split %s: unexpected error: %v\n%s", version, err, out)
		}
	}

	if got := readSplitJAR(t, dir, "2.0.0"); got != "central" {
		t.Errorf("plugin-split 2.0.0: expected the JAR from central, got it from %s", got)
	}
	if got := readSplitJAR(t, dir, "5.0.0"); got != "kestra" {
		t.Errorf("plugin-split 5.0.0: expected the JAR from kestra, got it from %s", got)
	}
}

func TestPluginsGet_MavenCredentialsFromEnv(t *testing.T) {
	var user, password string
	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, _ = r.BasicAuth()
		if !strings.HasSuffix(r.URL.Path, ".jar") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte("private"))
	}))
	t.Cleanup(private.Close)
	t.Setenv("KESTRACTL_MAVEN_REPOSITORY", private.URL)
	t.Setenv("KESTRACTL_MAVEN_USERNAME", "alice")
	t.Setenv("KESTRACTL_MAVEN_PASSWORD", "s3cr3t")

	if out, err := runPluginsCommand(t, "get", splitPlugin("4.0.0"), "--plugins-dir", t.TempDir()); err != nil {
		t.Fatalf("unexpected error: %v\n%s", err, out)
	}

	if user != "alice" || password != "s3cr3t" {
		t.Errorf("expected the credentials from the environment, got %q:%q", user, password)
	}
}

func TestPluginsGet_FlagWinsOverEnv(t *testing.T) {
	fromEnv := newFakeMavenRepository(t, "env", "4.0.0")
	fromFlag := newFakeMavenRepository(t, "flag", "4.0.0")
	t.Setenv("KESTRACTL_MAVEN_REPOSITORY", fromEnv.server.URL)
	dir := t.TempDir()

	if out, err := runPluginsCommand(t, "get", splitPlugin("4.0.0"), "--plugins-dir", dir, "--maven-repository", fromFlag.server.URL); err != nil {
		t.Fatalf("unexpected error: %v\n%s", err, out)
	}

	if got := readSplitJAR(t, dir, "4.0.0"); got != "flag" {
		t.Errorf("expected the JAR from the --maven-repository flag, got it from %s", got)
	}
	if fromEnv.jarRequested("4.0.0") {
		t.Error("expected KESTRACTL_MAVEN_REPOSITORY to be ignored when the flag is given")
	}
}

func TestPluginsDownload_MavenRepositoriesFromEnv(t *testing.T) {
	kestra := newFakeMavenRepository(t, "kestra", "5.0.0")
	t.Setenv("KESTRACTL_MAVEN_REPOSITORY", kestra.server.URL)
	dir := t.TempDir()

	if out, err := runPluginsCommand(t, "download", "--plugins", splitPlugin("5.0.0"), "--plugins-dir", dir); err != nil {
		t.Fatalf("unexpected error: %v\n%s", err, out)
	}

	if got := readSplitJAR(t, dir, "5.0.0"); got != "kestra" {
		t.Errorf("expected the JAR from the repository in KESTRACTL_MAVEN_REPOSITORY, got it from %s", got)
	}
}
