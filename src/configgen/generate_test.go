package configgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGenerate_KafkaElasticsearch(t *testing.T) {
	dir := t.TempDir()
	projName := filepath.Join(dir, "test-kafka-es")

	c := Choices{
		Edition:     "ee",
		ProjectName: projName,
		Queue:       "kafka",
		Repository:  "elasticsearch",
		Storage:     "local",
		LicenseID:   "test-license-id",
		LicenseKey:  "test-license-key",
		TmpDirPath:  "/tmp/kestra-wd/tmp",
		KestraURL:   "http://localhost:8080/",
	}

	if _, err := Generate(c, c.ProjectName, false); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Check files exist
	for _, f := range []string{"application.yml", "docker-compose.yml", ".env"} {
		if _, err := os.Stat(filepath.Join(projName, f)); err != nil {
			t.Errorf("expected %s to exist: %v", f, err)
		}
	}

	// Parse application.yml
	appData, _ := os.ReadFile(filepath.Join(projName, "application.yml"))
	var appConfig map[string]any
	if err := yaml.Unmarshal(appData, &appConfig); err != nil {
		t.Fatalf("invalid application.yml: %v", err)
	}

	kestra := appConfig["kestra"].(map[string]any)
	if kestra["queue"].(map[string]any)["type"] != "kafka" {
		t.Error("expected queue type kafka")
	}
	if kestra["repository"].(map[string]any)["type"] != "elasticsearch" {
		t.Error("expected repository type elasticsearch")
	}
	if kestra["ee"].(map[string]any)["license"].(map[string]any)["id"] != "test-license-id" {
		t.Error("license id not set")
	}
	// Should not have datasources (no DB)
	if _, ok := appConfig["datasources"]; ok {
		t.Error("datasources should not be present for kafka+es setup")
	}

	// Check docker-compose.yml contains kafka + elasticsearch, no postgres
	composeData, _ := os.ReadFile(filepath.Join(projName, "docker-compose.yml"))
	compose := string(composeData)
	if !strings.Contains(compose, "kafka:") {
		t.Error("compose should contain kafka service")
	}
	if !strings.Contains(compose, "elasticsearch:") {
		t.Error("compose should contain elasticsearch service")
	}
	if strings.Contains(compose, "postgres:") {
		t.Error("compose should NOT contain postgres service")
	}

	// Check .env
	envData, _ := os.ReadFile(filepath.Join(projName, ".env"))
	envContent := string(envData)
	if !strings.Contains(envContent, "KESTRA_ENCRYPTION_KEY=") {
		t.Error(".env should contain encryption key")
	}
	if strings.Contains(envContent, "POSTGRES_USER") {
		t.Error(".env should NOT contain postgres vars for kafka+es setup")
	}
}

func TestGenerate_PostgresJDBC(t *testing.T) {
	dir := t.TempDir()
	projName := filepath.Join(dir, "test-postgres-jdbc")

	c := Choices{
		Edition:            "ee",
		ProjectName:        projName,
		Queue:              "jdbc",
		Repository:         "jdbc",
		QueueDatabase:      "postgres",
		RepositoryDatabase: "postgres",
		Storage:            "local",
		LicenseID:          "test-id",
		LicenseKey:         "test-key",
		TmpDirPath:         "/tmp/kestra-wd/tmp",
		KestraURL:          "http://localhost:8080/",
		SuperAdmin:         true,
		SuperAdminEmail:    "admin@test.com",
		SuperAdminPass:     "TestPass123",
		BasicAuth:          true,
	}

	if _, err := Generate(c, c.ProjectName, false); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	appData, _ := os.ReadFile(filepath.Join(projName, "application.yml"))
	var appConfig map[string]any
	yaml.Unmarshal(appData, &appConfig)

	kestra := appConfig["kestra"].(map[string]any)
	if kestra["queue"].(map[string]any)["type"] != "postgres" {
		t.Error("expected queue type postgres (jdbc resolves to db name)")
	}
	if kestra["repository"].(map[string]any)["type"] != "postgres" {
		t.Error("expected repository type postgres")
	}

	ds := appConfig["datasources"].(map[string]any)
	pg := ds["postgres"].(map[string]any)
	if !strings.Contains(pg["url"].(string), "postgres:5432") {
		t.Error("datasource url should use compose hostname")
	}

	security := kestra["security"].(map[string]any)
	superAdmin := security["super-admin"].(map[string]any)
	if superAdmin["username"] != "admin@test.com" {
		t.Errorf("super admin email not set: %v", superAdmin["username"])
	}

	micronaut := appConfig["micronaut"].(map[string]any)
	if micronaut["security"].(map[string]any)["basic-auth"].(map[string]any)["enabled"] != true {
		t.Error("basic auth not enabled")
	}

	// Check compose has postgres, no kafka/es
	composeData, _ := os.ReadFile(filepath.Join(projName, "docker-compose.yml"))
	compose := string(composeData)
	if !strings.Contains(compose, "postgres:") {
		t.Error("compose should contain postgres")
	}
	if strings.Contains(compose, "kafka:") {
		t.Error("compose should NOT contain kafka")
	}

	// Check .env has postgres + jwt vars
	envData, _ := os.ReadFile(filepath.Join(projName, ".env"))
	env := string(envData)
	if !strings.Contains(env, "POSTGRES_USER=kestra") {
		t.Error(".env missing postgres user")
	}
	if !strings.Contains(env, "JWT_SECRET=") {
		t.Error(".env missing JWT secret")
	}
}

func TestGenerate_MixedDatabases(t *testing.T) {
	dir := t.TempDir()
	projName := filepath.Join(dir, "test-mixed-db")

	c := Choices{
		Edition:            "ee",
		ProjectName:        projName,
		Queue:              "jdbc",
		Repository:         "jdbc",
		QueueDatabase:      "mysql",
		RepositoryDatabase: "postgres",
		Storage:            "local",
		LicenseID:          "test-id",
		LicenseKey:         "test-key",
		TmpDirPath:         "/tmp/kestra-wd/tmp",
		KestraURL:          "http://localhost:8080/",
	}

	if _, err := Generate(c, c.ProjectName, false); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	appData, _ := os.ReadFile(filepath.Join(projName, "application.yml"))
	var appConfig map[string]any
	yaml.Unmarshal(appData, &appConfig)

	kestra := appConfig["kestra"].(map[string]any)
	if kestra["queue"].(map[string]any)["type"] != "mysql" {
		t.Error("expected queue type mysql")
	}
	if kestra["repository"].(map[string]any)["type"] != "postgres" {
		t.Error("expected repository type postgres")
	}

	// Both datasources should be present
	ds := appConfig["datasources"].(map[string]any)
	if _, ok := ds["postgres"]; !ok {
		t.Error("expected postgres datasource")
	}
	if _, ok := ds["mysql"]; !ok {
		t.Error("expected mysql datasource")
	}

	// Compose should have both postgres and mysql
	composeData, _ := os.ReadFile(filepath.Join(projName, "docker-compose.yml"))
	compose := string(composeData)
	if !strings.Contains(compose, "postgres:") {
		t.Error("compose should contain postgres")
	}
	if !strings.Contains(compose, "mysql:") {
		t.Error("compose should contain mysql")
	}

	// .env should have both postgres and mysql vars
	envData, _ := os.ReadFile(filepath.Join(projName, ".env"))
	env := string(envData)
	if !strings.Contains(env, "POSTGRES_USER=kestra") {
		t.Error(".env missing postgres vars")
	}
	if !strings.Contains(env, "MYSQL_USER=kestra") {
		t.Error(".env missing mysql vars")
	}
}

func TestGenerate_H2(t *testing.T) {
	dir := t.TempDir()
	projName := filepath.Join(dir, "test-h2")

	c := Choices{
		Edition:     "ee",
		ProjectName: projName,
		Queue:       "h2",
		Repository:  "h2",
		Storage:     "local",
		LicenseID:   "test-id",
		LicenseKey:  "test-key",
		TmpDirPath:  "/tmp/kestra-wd/tmp",
		KestraURL:   "http://localhost:8080/",
	}

	if _, err := Generate(c, c.ProjectName, false); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	appData, _ := os.ReadFile(filepath.Join(projName, "application.yml"))
	var appConfig map[string]any
	yaml.Unmarshal(appData, &appConfig)

	kestra := appConfig["kestra"].(map[string]any)
	if kestra["queue"].(map[string]any)["type"] != "h2" {
		t.Error("expected queue type h2")
	}
	if kestra["repository"].(map[string]any)["type"] != "h2" {
		t.Error("expected repository type h2")
	}

	ds := appConfig["datasources"].(map[string]any)
	if _, ok := ds["h2"]; !ok {
		t.Error("expected h2 datasource")
	}

	// Compose should have no infra services
	composeData, _ := os.ReadFile(filepath.Join(projName, "docker-compose.yml"))
	compose := string(composeData)
	if strings.Contains(compose, "depends_on") {
		t.Error("H2 compose should not have depends_on")
	}
	if strings.Contains(compose, "postgres:") || strings.Contains(compose, "kafka:") {
		t.Error("H2 compose should have no infra services")
	}
}

func TestGenerate_OSS(t *testing.T) {
	dir := t.TempDir()
	projName := filepath.Join(dir, "test-oss")

	c := Choices{
		Edition:     "oss",
		ProjectName: projName,
		Queue:       "h2",
		Repository:  "h2",
		Storage:     "local",
		TmpDirPath:  "/tmp/kestra-wd/tmp",
		KestraURL:   "http://localhost:8080/",
	}

	if _, err := Generate(c, c.ProjectName, false); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Compose should use OSS image
	composeData, _ := os.ReadFile(filepath.Join(projName, "docker-compose.yml"))
	compose := string(composeData)
	if !strings.Contains(compose, "kestra/kestra:latest") {
		t.Error("OSS compose should use kestra/kestra:latest image")
	}
	if strings.Contains(compose, "kestra-ee") {
		t.Error("OSS compose should NOT contain kestra-ee")
	}

	// application.yml should not contain license
	appData, _ := os.ReadFile(filepath.Join(projName, "application.yml"))
	var appConfig map[string]any
	yaml.Unmarshal(appData, &appConfig)

	kestra := appConfig["kestra"].(map[string]any)
	if _, ok := kestra["ee"]; ok {
		t.Error("OSS config should not have kestra.ee section")
	}
}

func TestSelectConfigSnippets(t *testing.T) {
	tests := []struct {
		name     string
		choices  Choices
		expected []string
		excluded []string
	}{
		{
			name:     "h2 all-in-one (EE)",
			choices:  Choices{Edition: "ee", Queue: "h2", Repository: "h2"},
			expected: []string{"base.yml", "datasource-h2.yml", "license.yml"},
			excluded: []string{"queue-kafka.yml", "datasource-postgres.yml"},
		},
		{
			name:     "h2 all-in-one (OSS)",
			choices:  Choices{Edition: "oss", Queue: "h2", Repository: "h2"},
			expected: []string{"base.yml", "datasource-h2.yml"},
			excluded: []string{"license.yml", "queue-kafka.yml"},
		},
		{
			name:     "kafka + elasticsearch",
			choices:  Choices{Edition: "ee", Queue: "kafka", Repository: "elasticsearch"},
			expected: []string{"queue-kafka.yml", "repository-elasticsearch.yml", "secret-elasticsearch.yml", "license.yml"},
			excluded: []string{"datasource-postgres.yml", "datasource-mysql.yml"},
		},
		{
			name:     "redis + jdbc postgres",
			choices:  Choices{Edition: "ee", Queue: "redis", Repository: "jdbc", RepositoryDatabase: "postgres"},
			expected: []string{"queue-redis.yml", "repository-jdbc.yml", "datasource-postgres.yml", "secret-jdbc.yml"},
			excluded: []string{"datasource-mysql.yml"},
		},
		{
			name:     "jdbc mysql queue + jdbc postgres repo (mixed databases)",
			choices:  Choices{Edition: "ee", Queue: "jdbc", Repository: "jdbc", QueueDatabase: "mysql", RepositoryDatabase: "postgres"},
			expected: []string{"queue-jdbc.yml", "repository-jdbc.yml", "datasource-postgres.yml", "datasource-mysql.yml", "secret-jdbc.yml"},
			excluded: []string{"datasource-h2.yml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snippets := selectConfigSnippets(tt.choices)
			snippetSet := make(map[string]bool)
			for _, s := range snippets {
				snippetSet[s] = true
			}
			for _, e := range tt.expected {
				if !snippetSet[e] {
					t.Errorf("expected snippet %s not found in %v", e, snippets)
				}
			}
			for _, e := range tt.excluded {
				if snippetSet[e] {
					t.Errorf("snippet %s should not be present in %v", e, snippets)
				}
			}
		})
	}
}
