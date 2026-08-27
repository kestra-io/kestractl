module github.com/kestra-io/kestractl

go 1.25.0

require (
	// Pseudo-version pinned to the go-sdk/v2.0.0-rc1 tag commit (Kestra 2.0 API).
	// The upstream go-sdk module path still lacks a /v2 suffix at this tag, so Go
	// can't resolve "v2.0.0-rc1" as a proper semver import and this had to be
	// pinned by commit hash instead. Do not `go get -u` this: v1.3.0 is a real
	// tag from releases/v1.3.x (Kestra 1.3.x's API) and sorts higher than this
	// pseudo-version, so an unguarded upgrade would silently swap back to the
	// wrong API surface. Re-resolve deliberately once upstream tags a proper
	// v2.x release (or adds the /v2 module suffix).
	github.com/kestra-io/client-sdk/go-sdk v1.1.1-0.20260827105951-d3f68c23ef0d
	github.com/posthog/posthog-go v1.10.0
	github.com/spf13/cobra v1.8.0
	github.com/spf13/pflag v1.0.10
	github.com/spf13/viper v1.21.0
	gopkg.in/yaml.v3 v3.0.1
)

require github.com/dustin/go-humanize v1.0.1

require (
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/sagikazarmark/locafero v0.11.0 // indirect
	github.com/sourcegraph/conc v0.3.1-0.20240121214520-5f936abd7ae8 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/tmaxmax/go-sse v0.11.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/sys v0.44.0 // indirect
	golang.org/x/text v0.39.0 // indirect
	gopkg.in/validator.v2 v2.0.1 // indirect
)
