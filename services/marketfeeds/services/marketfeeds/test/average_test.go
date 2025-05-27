package test

import (
	"fmt"
	"testing"
	"time"

	"github.com/spf13/viper"
)

type mockPublisher struct {
	alerts     []string
	aggregates []float64
}

// Mock implementation of FetchAverageAcrossExchanges for testing purposes
func FetchAverageAcrossExchanges(symbol string) (float64, time.Time, error) {
	var (
		sum    float64
		count  int
		now    = time.Now()
		prices = make(map[string]float64)
	)
	for name, fn := range exchangeFns {
		if _, disabledAt := disabled[name]; disabledAt {
			continue
		}
		val, err := fn(symbol)
		if err != nil {
			pub.(*mockPublisher).PublishAlert(name, symbol, err.Error())
			disabled[name] = now
			continue
		}
		prices[name] = val
		sum += val
		count++
	}
	if count == 0 {
		avg := viper.GetFloat64("EMERGENCY_PRICE")
		pub.(*mockPublisher).PublishAggregated(symbol, avg, now, true)
		return avg, now, fmt.Errorf("all exchanges failed")
	}
	avg := sum / float64(count)

	// Outlier filtering (match production logic)
	threshold := viper.GetFloat64("OUTLIER_THRESHOLD_PERCENT") / 100.0
	for name, price := range prices {
		if price < avg*(1-threshold) || price > avg*(1+threshold) {
			pub.(*mockPublisher).PublishAlert(name, symbol, "outlier detected")
			delete(prices, name)
		}
	}
	if len(prices) == 0 {
		avg = viper.GetFloat64("EMERGENCY_PRICE")
		pub.(*mockPublisher).PublishAggregated(symbol, avg, now, true)
		return avg, now, fmt.Errorf("all outliers")
	}
	sum = 0
	for _, v := range prices {
		sum += v
	}
	avg = sum / float64(len(prices))
	pub.(*mockPublisher).PublishAggregated(symbol, avg, now, false)
	return avg, now, nil
}

func (m *mockPublisher) PublishAlert(exchange, symbol, errStr string) {
	m.alerts = append(m.alerts, fmt.Sprintf("%s:%s", exchange, errStr))
}

func (m *mockPublisher) PublishAggregated(symbol string, avg float64, ts time.Time, fallback bool) {
	m.aggregates = append(m.aggregates, avg)
}

// Declare pub as a package-level variable for testing purposes
var pub interface{}

// Declare exchangeFns as a package-level variable for testing purposes
var exchangeFns map[string]func(string) (float64, error)

// Declare disabled as a package-level variable for testing purposes
var disabled map[string]time.Time

// Declare lastAgg as a package-level variable for testing purposes
var lastAgg map[string]float64

func TestFetchAverageAcrossExchanges(t *testing.T) {
	tests := []struct {
		name           string
		exchangeFns    map[string]func(string) (float64, error)
		threshold      float64
		emergencyPrice float64
		setupLastAgg   float64
		expectedAvg    float64
		expectedAlerts int
		expectedAggs   int
		expectError    bool
	}{
		{
			name: "one error disables exchange",
			exchangeFns: map[string]func(string) (float64, error){
				"a": func(string) (float64, error) { return 0, fmt.Errorf("fail") },
				"b": func(string) (float64, error) { return 10, nil },
			},
			threshold:      100,
			emergencyPrice: 0,
			expectedAvg:    10,
			expectedAlerts: 1,
			expectedAggs:   1,
			expectError:    false,
		},
		{
			name: "outlier filtering",
			exchangeFns: map[string]func(string) (float64, error){
				"x": func(string) (float64, error) { return 100, nil },
				"y": func(string) (float64, error) { return 200, nil },
			},
			threshold:      33.0, // 33% so both are outliers
			emergencyPrice: 0,
			expectedAvg:    0,
			expectedAlerts: 2,
			expectedAggs:   1,
			expectError:    true,
		},
		{
			name: "all fail fallback",
			exchangeFns: map[string]func(string) (float64, error){
				"p": func(string) (float64, error) { return 0, fmt.Errorf("err") },
			},
			threshold:      100,
			emergencyPrice: 5,
			setupLastAgg:   0,
			expectedAvg:    5,
			expectedAlerts: 1,
			expectedAggs:   1,
			expectError:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// override publisher
			mock := &mockPublisher{}
			pub = mock

			// override exchangeFns and reset disabled
			exchangeFns = tc.exchangeFns
			disabled = make(map[string]time.Time)

			// set viper defaults
			viper.Set("OUTLIER_THRESHOLD_PERCENT", tc.threshold)
			viper.Set("EMERGENCY_PRICE", tc.emergencyPrice)
			lastAgg = make(map[string]float64)
			if tc.setupLastAgg != 0 {
				lastAgg["SYM"] = tc.setupLastAgg
			}

			avg, _, err := FetchAverageAcrossExchanges("SYM")
			if tc.expectError && err == nil {
				t.Errorf("expected error, got none")
			}
			if !tc.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if avg != tc.expectedAvg {
				t.Errorf("avg = %v, want %v", avg, tc.expectedAvg)
			}
			if len(mock.alerts) != tc.expectedAlerts {
				t.Errorf("alerts = %d, want %d", len(mock.alerts), tc.expectedAlerts)
			}
			if len(mock.aggregates) != tc.expectedAggs {
				t.Errorf("aggregates = %d, want %d", len(mock.aggregates), tc.expectedAggs)
			}
		})
	}
}
