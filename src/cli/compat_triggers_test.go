package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The two shapes GET /triggers/search answers with, captured verbatim from a
// live Kestra 2.0.0-rc13 and a live Kestra 1.3.35 (issue #118).
const (
	trigger20SearchBody = `{"results":[{"trigger":{"id":"every_min","type":"io.kestra.plugin.core.trigger.Schedule","cron":"*/5 * * * *"},` +
		`"state":{"namespace":"bug118","flowId":"sched118","triggerId":"every_min","updatedAt":"2026-09-04T10:39:14Z",` +
		`"nextEvaluationDate":"2026-09-04T10:40:00Z","disabled":false,"locked":false,"kind":"SCHEDULE"}}],"total":1}`

	trigger13SearchBody = `{"results":[{"abstractTrigger":{"id":"every_min","type":"io.kestra.plugin.core.trigger.Schedule","cron":"*/5 * * * *"},` +
		`"triggerContext":{"tenantId":"main","namespace":"bug118","flowId":"sched118","triggerId":"every_min",` +
		`"date":"2026-09-04T10:39:15Z","nextExecutionDate":"2026-09-04T10:40:00Z","disabled":false}}],"total":1}`

	// A single trigger, as unlock, the backfill ops and the per-flow search
	// return it: ApiTriggerState on 2.0, Trigger on 1.x.
	trigger20StateBody = `{"namespace":"bug118","flowId":"sched118","triggerId":"every_min",` +
		`"updatedAt":"2026-09-04T10:39:14Z","nextEvaluationDate":"2026-09-04T10:40:00Z","disabled":false,"locked":false}`

	trigger13StateBody = `{"tenantId":"main","namespace":"bug118","flowId":"sched118","triggerId":"every_min",` +
		`"date":"2026-09-04T10:39:15Z","nextExecutionDate":"2026-09-04T10:40:00Z","disabled":false}`
)

// triggerVersionHandler serves a fixed Kestra version on /api/v1/configs — the
// endpoint the era probe reads — and the given body for everything else. An
// empty version makes /configs unreadable, which is the unknown era.
func triggerVersionHandler(version, body string, seen *[]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/configs" {
			if version == "" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"` + version + `"}`))
			return
		}
		if seen != nil {
			*seen = append(*seen, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

func triggerTestServer(t *testing.T, version, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(triggerVersionHandler(version, body, nil))
	t.Cleanup(server.Close)
	return server
}

// fetchTriggerRows must read both response shapes into the same row, so
// `triggers list` renders the same thing on both servers (issue #118: the 2.0
// body used to decode into empty fields).
func TestFetchTriggerRows_BothEras(t *testing.T) {
	for _, tt := range []struct {
		name, version, body string
	}{
		{"kestra 2.0 trigger/state", "2.0.0-rc13", trigger20SearchBody},
		{"kestra 1.3 abstractTrigger/triggerContext", "1.3.35", trigger13SearchBody},
		{"unknown version takes the 2.0 shape", "", trigger20SearchBody},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := triggerTestServer(t, tt.version, tt.body)

			rows, total, err := fetchTriggerRows(newTestClient(t, server.URL), 1, 50)
			if err != nil {
				t.Fatalf("fetchTriggerRows: %v", err)
			}
			if total != 1 || len(rows) != 1 {
				t.Fatalf("expected 1 row and total 1, got %d rows, total %d", len(rows), total)
			}

			got := rows[0]
			if got.Namespace != "bug118" || got.FlowID != "sched118" || got.TriggerID != "every_min" {
				t.Errorf("unexpected identity: %+v", got)
			}
			if got.Type != "io.kestra.plugin.core.trigger.Schedule" {
				t.Errorf("expected the trigger type, got %q", got.Type)
			}
			if got.Disabled {
				t.Error("expected the trigger to be enabled")
			}
			if got.NextExecutionDate == nil || got.NextExecutionDate.UTC().Format("2006-01-02T15:04:05Z") != "2026-09-04T10:40:00Z" {
				t.Errorf("expected the next evaluation date, got %v", got.NextExecutionDate)
			}
		})
	}
}

// Both eras use the same path — only the response model differs — so a wrong
// dispatch shows up as a decode error rather than a 404. This pins the path all
// the same.
func TestFetchTriggerRows_UsesSearchPath(t *testing.T) {
	var seen []string
	server := httptest.NewServer(triggerVersionHandler("2.0.0-rc13", trigger20SearchBody, &seen))
	t.Cleanup(server.Close)

	if _, _, err := fetchTriggerRows(newTestClient(t, server.URL), 1, 50); err != nil {
		t.Fatalf("fetchTriggerRows: %v", err)
	}
	if len(seen) != 1 || seen[0] != "/api/v1/main/triggers/search" {
		t.Fatalf("expected one call to /api/v1/main/triggers/search, got %v", seen)
	}
}

func TestFetchFlowTriggerRefs_BothEras(t *testing.T) {
	for _, tt := range []struct {
		version, body string
	}{
		{"2.0.0-rc13", `{"results":[` + trigger20StateBody + `],"total":1}`},
		{"1.3.35", `{"results":[` + trigger13StateBody + `],"total":1}`},
	} {
		t.Run(tt.version, func(t *testing.T) {
			server := triggerTestServer(t, tt.version, tt.body)

			refs, total, err := fetchFlowTriggerRefs(newTestClient(t, server.URL), "bug118", "sched118", 1, 50, "")
			if err != nil {
				t.Fatalf("fetchFlowTriggerRefs: %v", err)
			}
			if total != 1 || len(refs) != 1 {
				t.Fatalf("expected 1 ref and total 1, got %d refs, total %d", len(refs), total)
			}
			if refs[0] != (triggerRef{Namespace: "bug118", FlowID: "sched118", TriggerID: "every_min"}) {
				t.Fatalf("unexpected ref: %+v", refs[0])
			}
		})
	}
}

func TestUnlockTriggerRef_BothEras(t *testing.T) {
	for _, tt := range []struct {
		version, body string
	}{
		{"2.0.0-rc13", trigger20StateBody},
		{"1.3.35", trigger13StateBody},
	} {
		t.Run(tt.version, func(t *testing.T) {
			server := triggerTestServer(t, tt.version, tt.body)

			ref, err := unlockTriggerRef(newTestClient(t, server.URL), "bug118", "sched118", "every_min")
			if err != nil {
				t.Fatalf("unlockTriggerRef: %v", err)
			}
			if ref == nil || *ref != (triggerRef{Namespace: "bug118", FlowID: "sched118", TriggerID: "every_min"}) {
				t.Fatalf("unexpected ref: %+v", ref)
			}
		})
	}
}

func TestBackfillTriggerRef_BothEras(t *testing.T) {
	for _, op := range []string{"pause", "unpause", "delete"} {
		for _, tt := range []struct {
			version, body string
		}{
			{"2.0.0-rc13", trigger20StateBody},
			{"1.3.35", trigger13StateBody},
		} {
			t.Run(op+"/"+tt.version, func(t *testing.T) {
				server := triggerTestServer(t, tt.version, tt.body)

				ref, err := backfillTriggerRef(newTestClient(t, server.URL), "bug118", "sched118", "every_min", op)
				if err != nil {
					t.Fatalf("backfillTriggerRef(%s): %v", op, err)
				}
				if ref == nil || ref.TriggerID != "every_min" {
					t.Fatalf("unexpected ref: %+v", ref)
				}
			})
		}
	}
}

// A 1.3 server legitimately omits `date` from a trigger context whose state was
// reset or never evaluated. The generated 1.x models mark it required, so
// reading these endpoints through them failed the whole command with "no value
// given for required property date" — hence the raw decode on the legacy path.
const (
	trigger13DatelessSearchBody = `{"results":[{"abstractTrigger":{"id":"every_min","type":"io.kestra.plugin.core.trigger.Schedule","cron":"*/5 * * * *"},` +
		`"triggerContext":{"tenantId":"main","namespace":"bug118","flowId":"sched118","triggerId":"every_min",` +
		`"nextExecutionDate":"2026-09-04T10:40:00Z","disabled":false}}],"total":1}`

	trigger13DatelessStateBody = `{"tenantId":"main","namespace":"bug118","flowId":"sched118","triggerId":"every_min",` +
		`"nextExecutionDate":"2026-09-04T10:45:00Z","disabled":false}`
)

func TestFetchTriggerRows_DatelessLegacyBody(t *testing.T) {
	server := triggerTestServer(t, "1.3.35", trigger13DatelessSearchBody)

	rows, total, err := fetchTriggerRows(newTestClient(t, server.URL), 1, 50)
	if err != nil {
		t.Fatalf("fetchTriggerRows: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("expected 1 row and total 1, got %d rows, total %d", len(rows), total)
	}
	got := rows[0]
	if got.Namespace != "bug118" || got.FlowID != "sched118" || got.TriggerID != "every_min" {
		t.Errorf("unexpected identity: %+v", got)
	}
	if got.Type != "io.kestra.plugin.core.trigger.Schedule" {
		t.Errorf("expected the trigger type, got %q", got.Type)
	}
	if got.NextExecutionDate == nil {
		t.Error("expected the next execution date to survive")
	}
}

func TestFetchFlowTriggerRefs_DatelessLegacyBody(t *testing.T) {
	server := triggerTestServer(t, "1.3.35", `{"results":[`+trigger13DatelessStateBody+`],"total":1}`)

	refs, total, err := fetchFlowTriggerRefs(newTestClient(t, server.URL), "bug118", "sched118", 1, 50, "")
	if err != nil {
		t.Fatalf("fetchFlowTriggerRefs: %v", err)
	}
	if total != 1 || len(refs) != 1 {
		t.Fatalf("expected 1 ref and total 1, got %d refs, total %d", len(refs), total)
	}
	if refs[0] != (triggerRef{Namespace: "bug118", FlowID: "sched118", TriggerID: "every_min"}) {
		t.Fatalf("unexpected ref: %+v", refs[0])
	}
}

func TestUnlockAndBackfill_DatelessLegacyBody(t *testing.T) {
	want := triggerRef{Namespace: "bug118", FlowID: "sched118", TriggerID: "every_min"}

	t.Run("unlock", func(t *testing.T) {
		server := triggerTestServer(t, "1.3.35", trigger13DatelessStateBody)
		ref, err := unlockTriggerRef(newTestClient(t, server.URL), "bug118", "sched118", "every_min")
		if err != nil {
			t.Fatalf("unlockTriggerRef: %v", err)
		}
		if ref == nil || *ref != want {
			t.Fatalf("unexpected ref: %+v", ref)
		}
	})

	for _, op := range []string{"pause", "unpause", "delete"} {
		t.Run("backfill-"+op, func(t *testing.T) {
			server := triggerTestServer(t, "1.3.35", trigger13DatelessStateBody)
			ref, err := backfillTriggerRef(newTestClient(t, server.URL), "bug118", "sched118", "every_min", op)
			if err != nil {
				t.Fatalf("backfillTriggerRef(%s): %v", op, err)
			}
			if ref == nil || *ref != want {
				t.Fatalf("unexpected ref: %+v", ref)
			}
		})
	}
}

// The legacy path must still surface a server error rather than an empty row.
func TestLegacyTriggerRequest_SurfacesServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/configs" {
			_, _ = w.Write([]byte(`{"version":"1.3.35"}`))
			return
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"Illegal state: Trigger is not locked"}`))
	}))
	t.Cleanup(server.Close)

	_, err := unlockTriggerRef(newTestClient(t, server.URL), "bug118", "sched118", "every_min")
	if err == nil {
		t.Fatal("expected the server error to be surfaced")
	}
	if err.Error() != "API error: Illegal state: Trigger is not locked" {
		t.Fatalf("unexpected error: %v", err)
	}
}
