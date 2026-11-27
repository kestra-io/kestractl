package configgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func gen(t *testing.T, c Choices) (app, compose, env string) {
	t.Helper()
	dir := t.TempDir()
	if _, err := Generate(c, dir, false); err != nil {
		t.Fatal(err)
	}
	read := func(n string) string {
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	return read("application.yml"), read("docker-compose.yml"), read(".env")
}

func eeBase() Choices {
	return Choices{Edition: "ee", ProjectName: "x", Queue: "h2", LicenseID: "i", LicenseKey: "k", LicenseFingerprint: "f", TmpDirPath: "/data/tmp"}
}

func TestListsIgnoredWhenFlagUnset(t *testing.T) {
	c := eeBase()
	c.OAuthClients = []OAuthClient{{Name: "a | b", ClientID: "i", ClientSecret: "s", IssuerURL: "u"}}
	c.AIConfigs = []AIConfig{{Provider: "openai", Model: "m"}}
	c.PluginIncludes = "a,b"
	app, _, env := gen(t, c)
	for _, bad := range []string{"oauth2", "providers", "includes"} {
		if strings.Contains(app, bad) {
			t.Errorf("application.yml contains %q with flags unset", bad)
		}
	}
	if strings.Contains(env, "OIDC_") {
		t.Error(".env has OIDC secret with oidc unset")
	}
}

func TestOSSRejectsEEOnlyLists(t *testing.T) {
	c := Choices{Edition: "oss", ProjectName: "x", Queue: "h2", PluginIncludes: "a"}
	if err := Validate(&c); err == nil || !strings.Contains(err.Error(), "plugin_includes") {
		t.Errorf("got %v", err)
	}
}

func TestEmptyOIDCClaimPathLeavesNoPlaceholder(t *testing.T) {
	c := eeBase()
	c.OIDCEmailVerification = true
	app, _, _ := gen(t, c)
	if strings.Contains(app, "__") {
		t.Errorf("placeholder left in output:\n%s", app)
	}
}

func TestSuperAdminHasTenantAccess(t *testing.T) {
	c := eeBase()
	c.SuperAdmin, c.SuperAdminEmail, c.SuperAdminPass = true, "a@b.c", "p"
	app, _, _ := gen(t, c)
	if !strings.Contains(app, "tenant-admin-access") {
		t.Error("missing tenant-admin-access")
	}
}

func TestDatabaseValidation(t *testing.T) {
	mixed := Choices{Edition: "oss", ProjectName: "x", Queue: "jdbc", Repository: "jdbc", QueueDatabase: "postgres", RepositoryDatabase: "mysql"}
	if err := Validate(&mixed); err == nil || !strings.Contains(err.Error(), "must match") {
		t.Errorf("mixed dbs: %v", err)
	}
	c := Choices{Edition: "ee", ProjectName: "x", Queue: "kafka", Repository: "elasticsearch", QueueDatabase: "postgres", LicenseID: "i", LicenseKey: "k", LicenseFingerprint: "f"}
	if err := Validate(&c); err != nil {
		t.Fatal(err)
	}
	if c.QueueDatabase != "" || c.NeedsPostgres() {
		t.Error("queue_database kept for non-jdbc queue")
	}
}

func TestComposeTmpDirAndH2Volume(t *testing.T) {
	_, compose, _ := gen(t, eeBase())
	if !strings.Contains(compose, "/data/tmp:/data/tmp") {
		t.Error("tmp dir mount does not follow tmp_dir_path")
	}
	if !strings.Contains(compose, "kestra-db:/app/data") {
		t.Error("h2 data volume missing")
	}
	app, _, _ := gen(t, eeBase())
	if strings.Contains(app, "h2:mem") {
		t.Error("h2 must be file backed")
	}
}
