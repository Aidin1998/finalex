package screening_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/compliance/aml/screening"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestNewSanctionsUpdater(t *testing.T) {
	logger := zap.NewNop().Sugar()
	updater := screening.NewSanctionsUpdater(logger)

	assert.NotNil(t, updater)
	config := updater.GetConfig()
	assert.True(t, config.EnableAutoUpdate)
	assert.Equal(t, time.Hour*6, config.UpdateInterval)
	assert.Equal(t, 3, config.MaxRetries)
}

func TestSanctionsUpdater_UpdateFromOFAC(t *testing.T) {
	// Create mock OFAC XML response
	mockXML := `<?xml version="1.0" encoding="UTF-8"?>
<sdnList>
	<sdnEntry>
		<uid>1</uid>
		<firstName>John</firstName>
		<lastName>Doe</lastName>
		<title>Sanctions Target</title>
		<sdnType>Individual</sdnType>
		<remarks>Test sanctions entry</remarks>
	</sdnEntry>
	<sdnEntry>
		<uid>2</uid>
		<firstName>Jane</firstName>
		<lastName>Smith</lastName>
		<title>Another Target</title>
		<sdnType>Individual</sdnType>
		<remarks>Another test entry</remarks>
	</sdnEntry>
</sdnList>`

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "sdn.xml") {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(mockXML))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	logger := zap.NewNop().Sugar()
	updater := screening.NewSanctionsUpdater(logger)

	// Update config to use test server
	config := updater.GetConfig()
	config.OFACEndpoint = server.URL + "/sdn.xml"
	updater.UpdateConfig(config)

	ctx := context.Background()
	sanctions, err := updater.UpdateFromOFAC(ctx)

	assert.NoError(t, err)
	assert.NotNil(t, sanctions)
	assert.Equal(t, "ofac", sanctions.Source)
	assert.True(t, len(sanctions.Entries) >= 2)

	// Check first entry
	entry := sanctions.Entries["1"]
	assert.NotNil(t, entry)
	assert.Equal(t, "John Doe", entry.Name)
	assert.Equal(t, "Individual", entry.Type)
}

func TestSanctionsUpdater_UpdateFromUN(t *testing.T) {
	// Create mock UN JSON response
	mockJSON := `{
		"results": [
			{
				"_id": "1",
				"first_name": "Ahmed",
				"second_name": "Hassan",
				"un_list_type": "Al-Qaida",
				"list_type": {
					"value": "UN"
				},
				"comments1": "Terrorist leader"
			},
			{
				"_id": "2",
				"first_name": "Mohammed",
				"second_name": "Ali",
				"un_list_type": "Taliban",
				"list_type": {
					"value": "UN"
				},
				"comments1": "Taliban member"
			}
		]
	}`

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "un") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(mockJSON))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	logger := zap.NewNop().Sugar()
	updater := screening.NewSanctionsUpdater(logger)

	// Update config to use test server
	config := updater.GetConfig()
	config.UNEndpoint = server.URL + "/un"
	updater.UpdateConfig(config)

	ctx := context.Background()
	sanctions, err := updater.UpdateFromUN(ctx)

	assert.NoError(t, err)
	assert.NotNil(t, sanctions)
	assert.Equal(t, "un", sanctions.Source)
	assert.True(t, len(sanctions.Entries) >= 2)

	// Check first entry
	entry := sanctions.Entries["1"]
	assert.NotNil(t, entry)
	assert.Equal(t, "Ahmed Hassan", entry.Name)
	assert.Equal(t, "UN", entry.Type)
}

func TestSanctionsUpdater_UpdateAll(t *testing.T) {
	logger := zap.NewNop().Sugar()
	updater := screening.NewSanctionsUpdater(logger)

	// Create mock servers for all sources
	ofacServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mockXML := `<?xml version="1.0" encoding="UTF-8"?>
<sdnList>
	<sdnEntry>
		<uid>1</uid>
		<firstName>OFAC</firstName>
		<lastName>Target</lastName>
		<sdnType>Individual</sdnType>
	</sdnEntry>
</sdnList>`
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockXML))
	}))
	defer ofacServer.Close()

	unServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mockJSON := `{
			"results": [
				{
					"_id": "1",
					"first_name": "UN",
					"second_name": "Target",
					"un_list_type": "Al-Qaida",
					"list_type": {
						"value": "UN"
					}
				}
			]
		}`
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockJSON))
	}))
	defer unServer.Close()

	// Update config to use test servers
	config := updater.GetConfig()
	config.OFACEndpoint = ofacServer.URL
	config.UNEndpoint = unServer.URL
	updater.UpdateConfig(config)

	ctx := context.Background()
	results, err := updater.UpdateAll(ctx)

	assert.NoError(t, err)
	assert.True(t, len(results) >= 2)

	// Check that sources were updated
	sources := make(map[string]bool)
	for _, result := range results {
		sources[result.Source] = true
		assert.True(t, len(result.Entries) > 0)
		assert.True(t, result.LastUpdated.After(time.Now().Add(-time.Minute)))
	}

	assert.True(t, sources["ofac"])
	assert.True(t, sources["un"])
}

func TestSanctionsUpdater_GetUpdateStatus(t *testing.T) {
	logger := zap.NewNop().Sugar()
	updater := screening.NewSanctionsUpdater(logger)

	status := updater.GetUpdateStatus()
	assert.NotNil(t, status)
	assert.False(t, status.IsRunning)
	assert.Equal(t, 0, status.SuccessfulUpdates)
	assert.Equal(t, 0, status.FailedUpdates)
}

func BenchmarkSanctionsUpdater_UpdateFromOFAC(b *testing.B) {
	logger := zap.NewNop().Sugar()
	updater := screening.NewSanctionsUpdater(logger)

	// Create large mock XML for benchmarking
	mockXML := `<?xml version="1.0" encoding="UTF-8"?><sdnList>`
	for i := 0; i < 100; i++ {
		mockXML += `<sdnEntry>
			<uid>` + string(rune(i+65)) + `</uid>
			<firstName>First` + string(rune(i+65)) + `</firstName>
			<lastName>Last` + string(rune(i+65)) + `</lastName>
			<sdnType>Individual</sdnType>
		</sdnEntry>`
	}
	mockXML += `</sdnList>`

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockXML))
	}))
	defer server.Close()

	config := updater.GetConfig()
	config.OFACEndpoint = server.URL
	updater.UpdateConfig(config)

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := updater.UpdateFromOFAC(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}
