package market

import (
	"macro-impact-tracker/internal/models"
	"testing"
	"time"
)

// ============================================================
// Tests for fetch.go — Asset configuration & MarketDataStore
// ============================================================

func TestDefaultAssets_ReturnsAllExpectedAssets(t *testing.T) {
	assets := DefaultAssets()

	// We expect 5 fetchable assets (DXY is computed, not fetched)
	if len(assets) != 5 {
		t.Errorf("expected 5 assets, got %d", len(assets))
	}

	// Build a map for easy lookup
	assetMap := make(map[string]AssetConfig)
	for _, a := range assets {
		assetMap[a.Symbol] = a
	}

	// Verify each expected asset exists with correct source and interval
	expectedAssets := []struct {
		symbol   string
		source   string
		interval string
	}{
		{"SPY", "twelvedata", "1min"},
		{"EUR/USD", "twelvedata", "1min"},
		{"VIX", "twelvedata", "1min"},
		{"DGS2", "fred", "1day"},
		{"DGS10", "fred", "1day"},
	}

	for _, expected := range expectedAssets {
		asset, ok := assetMap[expected.symbol]
		if !ok {
			t.Errorf("expected asset %s not found in DefaultAssets()", expected.symbol)
			continue
		}
		if asset.Source != expected.source {
			t.Errorf("asset %s: expected source %q, got %q", expected.symbol, expected.source, asset.Source)
		}
		if asset.Interval != expected.interval {
			t.Errorf("asset %s: expected interval %q, got %q", expected.symbol, expected.interval, asset.Interval)
		}
	}
}

func TestDefaultAssets_NoDXYInFetchList(t *testing.T) {
	// DXY should NOT be in the fetch list — it's computed from EUR/USD
	assets := DefaultAssets()
	for _, a := range assets {
		if a.Symbol == "DXY" || a.Symbol == "DXY (proxy)" {
			t.Error("DXY should not be in the fetchable asset list — it is computed from EUR/USD")
		}
	}
}

// ============================================================
// Tests for MarketDataStore (in models/market_data.go)
// ============================================================

func TestMarketDataStore_AddAndGet(t *testing.T) {
	store := models.NewMarketDataStore()

	ts := &models.AssetTimeSeries{
		Symbol:   "SPY",
		Interval: "1min",
		DataPoints: []models.MarketDataPoint{
			{Timestamp: time.Date(2026, 1, 14, 13, 30, 0, 0, time.UTC), Open: 520.0, High: 520.5, Low: 519.8, Close: 520.2, Volume: 1000},
			{Timestamp: time.Date(2026, 1, 14, 13, 31, 0, 0, time.UTC), Open: 520.2, High: 520.7, Low: 520.0, Close: 520.5, Volume: 1500},
		},
	}

	store.Add("SPY", ts)

	// Retrieve and verify
	retrieved := store.Get("SPY")
	if retrieved == nil {
		t.Fatal("expected to retrieve SPY data, got nil")
	}
	if retrieved.Len() != 2 {
		t.Errorf("expected 2 data points, got %d", retrieved.Len())
	}
	if retrieved.Symbol != "SPY" {
		t.Errorf("expected symbol SPY, got %s", retrieved.Symbol)
	}
}

func TestMarketDataStore_GetMissing(t *testing.T) {
	store := models.NewMarketDataStore()

	result := store.Get("NONEXISTENT")
	if result != nil {
		t.Error("expected nil for non-existent symbol, got data")
	}
}

func TestMarketDataStore_MergesOnDuplicateAdd(t *testing.T) {
	store := models.NewMarketDataStore()

	// Add first batch
	ts1 := &models.AssetTimeSeries{
		Symbol:   "SPY",
		Interval: "1min",
		DataPoints: []models.MarketDataPoint{
			{Timestamp: time.Date(2026, 1, 14, 13, 30, 0, 0, time.UTC), Close: 520.0},
		},
	}
	store.Add("SPY", ts1)

	// Add second batch (different timestamp — should be merged)
	ts2 := &models.AssetTimeSeries{
		Symbol:   "SPY",
		Interval: "1min",
		DataPoints: []models.MarketDataPoint{
			{Timestamp: time.Date(2026, 1, 14, 13, 31, 0, 0, time.UTC), Close: 520.5},
		},
	}
	store.Add("SPY", ts2)

	// Should have 2 data points total
	retrieved := store.Get("SPY")
	if retrieved.Len() != 2 {
		t.Errorf("expected 2 merged data points, got %d", retrieved.Len())
	}

	// Should be sorted chronologically
	if !retrieved.DataPoints[0].Timestamp.Before(retrieved.DataPoints[1].Timestamp) {
		t.Error("expected data points to be sorted chronologically after merge")
	}
}

func TestMarketDataStore_Symbols(t *testing.T) {
	store := models.NewMarketDataStore()

	store.Add("SPY", &models.AssetTimeSeries{Symbol: "SPY", DataPoints: []models.MarketDataPoint{{Close: 1}}})
	store.Add("VIX", &models.AssetTimeSeries{Symbol: "VIX", DataPoints: []models.MarketDataPoint{{Close: 2}}})
	store.Add("DGS10", &models.AssetTimeSeries{Symbol: "DGS10", DataPoints: []models.MarketDataPoint{{Close: 3}}})

	symbols := store.Symbols()
	if len(symbols) != 3 {
		t.Errorf("expected 3 symbols, got %d", len(symbols))
	}

	// Should be sorted alphabetically
	expected := []string{"DGS10", "SPY", "VIX"}
	for i, sym := range symbols {
		if sym != expected[i] {
			t.Errorf("symbol[%d]: expected %q, got %q", i, expected[i], sym)
		}
	}
}

func TestMarketDataStore_GetWindow(t *testing.T) {
	store := models.NewMarketDataStore()

	// Add 5 data points spanning 5 minutes
	baseTime := time.Date(2026, 1, 14, 13, 30, 0, 0, time.UTC)
	ts := &models.AssetTimeSeries{
		Symbol:   "SPY",
		Interval: "1min",
		DataPoints: []models.MarketDataPoint{
			{Timestamp: baseTime, Close: 520.0},
			{Timestamp: baseTime.Add(1 * time.Minute), Close: 520.1},
			{Timestamp: baseTime.Add(2 * time.Minute), Close: 520.2},
			{Timestamp: baseTime.Add(3 * time.Minute), Close: 520.3},
			{Timestamp: baseTime.Add(4 * time.Minute), Close: 520.4},
		},
	}
	store.Add("SPY", ts)

	// Window: minutes 1–3 (should get 3 points)
	from := baseTime.Add(1 * time.Minute)
	to := baseTime.Add(3 * time.Minute)
	window := store.GetWindow("SPY", from, to)

	if window == nil {
		t.Fatal("expected window data, got nil")
	}
	if window.Len() != 3 {
		t.Errorf("expected 3 data points in window, got %d", window.Len())
	}
}

// ============================================================
// Tests for DXY proxy computation
// ============================================================

func TestComputeDXYFromEURUSD(t *testing.T) {
	eurusd := &models.AssetTimeSeries{
		Symbol:   "EUR/USD",
		Interval: "1min",
		DataPoints: []models.MarketDataPoint{
			{
				Timestamp: time.Date(2026, 1, 14, 13, 30, 0, 0, time.UTC),
				Open:      1.1000,
				High:      1.1050,
				Low:       1.0950,
				Close:     1.1020,
				Volume:    5000,
			},
		},
	}

	dxy, err := ComputeDXYFromEURUSD(eurusd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if dxy.Symbol != "DXY (proxy)" {
		t.Errorf("expected symbol 'DXY (proxy)', got %q", dxy.Symbol)
	}
	if dxy.Len() != 1 {
		t.Errorf("expected 1 data point, got %d", dxy.Len())
	}

	dp := dxy.DataPoints[0]

	// DXY Open = 1 / EUR_USD Open = 1 / 1.1000 ≈ 0.9091
	expectedOpen := 1.0 / 1.1000
	if abs(dp.Open-expectedOpen) > 0.0001 {
		t.Errorf("DXY Open: expected %.4f, got %.4f", expectedOpen, dp.Open)
	}

	// DXY High = 1 / EUR_USD Low (inverted)
	expectedHigh := 1.0 / 1.0950
	if abs(dp.High-expectedHigh) > 0.0001 {
		t.Errorf("DXY High: expected %.4f, got %.4f", expectedHigh, dp.High)
	}

	// DXY Low = 1 / EUR_USD High (inverted)
	expectedLow := 1.0 / 1.1050
	if abs(dp.Low-expectedLow) > 0.0001 {
		t.Errorf("DXY Low: expected %.4f, got %.4f", expectedLow, dp.Low)
	}

	// DXY Close = 1 / EUR_USD Close
	expectedClose := 1.0 / 1.1020
	if abs(dp.Close-expectedClose) > 0.0001 {
		t.Errorf("DXY Close: expected %.4f, got %.4f", expectedClose, dp.Close)
	}
}

func TestComputeDXYFromEURUSD_NilInput(t *testing.T) {
	_, err := ComputeDXYFromEURUSD(nil)
	if err == nil {
		t.Error("expected error for nil input, got nil")
	}
}

func TestComputeDXYFromEURUSD_EmptyData(t *testing.T) {
	eurusd := &models.AssetTimeSeries{
		Symbol:     "EUR/USD",
		DataPoints: []models.MarketDataPoint{},
	}
	_, err := ComputeDXYFromEURUSD(eurusd)
	if err == nil {
		t.Error("expected error for empty data, got nil")
	}
}

// abs returns the absolute value of a float64.
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
