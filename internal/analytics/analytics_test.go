package analytics

import (
	"macro-impact-tracker/internal/models"
	"math"
	"testing"
	"time"
)

// ============================================================
// Tests for Phase 3: Analytics Engine
// ============================================================
// Covers all four components:
//   1. Event Window Extraction
//   2. Return Computation
//   3. Surprise Sensitivity Model (OLS regression)
//   4. Cross-Asset Lead/Lag Analysis
// ============================================================

// --- Helper functions ---

func floatEquals(a, b, tolerance float64) bool {
	return math.Abs(a-b) < tolerance
}

// makeTestStore creates a MarketDataStore with synthetic data for testing.
// Includes SPY (1min), VIX (1min), and DGS10 (1day) around two CPI events.
func makeTestStore() (*models.MarketDataStore, []models.MacroEvent) {
	store := models.NewMarketDataStore()

	// Two CPI release events at 8:30 AM ET (13:30 UTC)
	event1Time := time.Date(2026, 1, 14, 13, 30, 0, 0, time.UTC)
	event2Time := time.Date(2026, 2, 12, 13, 30, 0, 0, time.UTC)

	events := []models.MacroEvent{
		{
			EventName:   "CPI Release",
			ReleaseDate: event1Time,
			Actual:      310.0,
			Previous:    308.0,
			Expected:    309.0,
			Surprise:    1.0, // Hot CPI: Actual > Expected
		},
		{
			EventName:   "CPI Release",
			ReleaseDate: event2Time,
			Actual:      311.0,
			Previous:    310.0,
			Expected:    312.0,
			Surprise:    -1.0, // Cool CPI: Actual < Expected
		},
	}

	// --- SPY data around event 1: drops on hot CPI ---
	// Price goes from 520 → 518 (falls ~0.38% on hot surprise)
	spyEvent1 := makeMinuteData("SPY", event1Time, -30, 60, 520.0, -0.03)
	store.Add("SPY", spyEvent1)

	// --- SPY data around event 2: rises on cool CPI ---
	// Price goes from 525 → 528 (rises ~0.57% on cool surprise)
	spyEvent2 := makeMinuteData("SPY", event2Time, -30, 60, 525.0, 0.05)
	store.Add("SPY", spyEvent2)

	// --- VIX data around event 1: spikes on hot CPI ---
	// VIX goes from 15 → 17 (rises on hot surprise)
	vixEvent1 := makeMinuteData("VIX", event1Time, -30, 60, 15.0, 0.02)
	store.Add("VIX", vixEvent1)

	// --- VIX data around event 2: falls on cool CPI ---
	// VIX goes from 14 → 13 (falls on cool surprise)
	vixEvent2 := makeMinuteData("VIX", event2Time, -30, 60, 14.0, -0.01)
	store.Add("VIX", vixEvent2)

	// --- DGS10 daily data around event 1 ---
	store.Add("DGS10", &models.AssetTimeSeries{
		Symbol:   "DGS10",
		Interval: "1day",
		DataPoints: []models.MarketDataPoint{
			{Timestamp: event1Time.AddDate(0, 0, -1), Open: 4.20, High: 4.20, Low: 4.20, Close: 4.20},
			{Timestamp: event1Time, Open: 4.25, High: 4.25, Low: 4.25, Close: 4.25},
			{Timestamp: event1Time.AddDate(0, 0, 1), Open: 4.30, High: 4.30, Low: 4.30, Close: 4.30},
		},
	})

	// --- DGS10 daily data around event 2 ---
	store.Add("DGS10", &models.AssetTimeSeries{
		Symbol:   "DGS10",
		Interval: "1day",
		DataPoints: []models.MarketDataPoint{
			{Timestamp: event2Time.AddDate(0, 0, -1), Open: 4.10, High: 4.10, Low: 4.10, Close: 4.10},
			{Timestamp: event2Time, Open: 4.08, High: 4.08, Low: 4.08, Close: 4.08},
			{Timestamp: event2Time.AddDate(0, 0, 1), Open: 4.05, High: 4.05, Low: 4.05, Close: 4.05},
		},
	})

	return store, events
}

// makeMinuteData creates synthetic minute-level price data around a given time.
// startOffset: minutes before the event time to start generating
// numMinutes: total number of minutes of data
// basePrice: starting close price
// driftPerMin: price change per minute (positive = rising, negative = falling)
func makeMinuteData(symbol string, eventTime time.Time, startOffset, numMinutes int, basePrice, driftPerMin float64) *models.AssetTimeSeries {
	startTime := eventTime.Add(time.Duration(startOffset) * time.Minute)

	var points []models.MarketDataPoint
	for i := 0; i < numMinutes; i++ {
		price := basePrice + float64(i)*driftPerMin
		ts := startTime.Add(time.Duration(i) * time.Minute)
		points = append(points, models.MarketDataPoint{
			Timestamp: ts,
			Open:      price,
			High:      price + 0.01,
			Low:       price - 0.01,
			Close:     price,
			Volume:    int64(1000 + i*10),
		})
	}

	return &models.AssetTimeSeries{
		Symbol:     symbol,
		Interval:   "1min",
		DataPoints: points,
	}
}

// ============================================================
// 1. Event Window Extraction Tests
// ============================================================

func TestDefaultWindows(t *testing.T) {
	windows := DefaultWindows()

	if len(windows) != 4 {
		t.Fatalf("expected 4 windows, got %d", len(windows))
	}

	expected := []struct {
		name   string
		offset time.Duration
		end    time.Duration
	}{
		{"pre_30m", -30 * time.Minute, 0},
		{"post_30m", 0, 30 * time.Minute},
		{"post_2h", 0, 2 * time.Hour},
		{"post_1d", 0, 24 * time.Hour},
	}

	for i, w := range windows {
		if w.Name != expected[i].name {
			t.Errorf("window[%d] name: expected %q, got %q", i, expected[i].name, w.Name)
		}
		if w.Offset != expected[i].offset {
			t.Errorf("window[%d] offset: expected %v, got %v", i, expected[i].offset, w.Offset)
		}
		if w.End != expected[i].end {
			t.Errorf("window[%d] end: expected %v, got %v", i, expected[i].end, w.End)
		}
	}
}

func TestExtractEventWindows_BasicExtraction(t *testing.T) {
	store, events := makeTestStore()
	windows := DefaultWindows()

	extracted := ExtractEventWindows(events, store, windows)

	// We should get windows for SPY, VIX, and DGS10 across 2 events
	if len(extracted) == 0 {
		t.Fatal("expected extracted event windows, got none")
	}

	// Check that we have windows for SPY
	spyWindows := 0
	for _, ew := range extracted {
		if ew.Asset == "SPY" {
			spyWindows++
		}
	}
	if spyWindows == 0 {
		t.Error("expected SPY windows, got none")
	}

	t.Logf("Extracted %d total event windows (%d for SPY)", len(extracted), spyWindows)
}

func TestExtractEventWindows_OpenClosePrice(t *testing.T) {
	store, events := makeTestStore()

	// Use just the post_30m window for simplicity
	windows := []WindowDef{
		{Name: "post_30m", Offset: 0, End: 30 * time.Minute},
	}

	extracted := ExtractEventWindows(events, store, windows)

	// Find SPY post_30m for event 1
	for _, ew := range extracted {
		if ew.Asset == "SPY" && ew.EventIndex == 0 && ew.Window.Name == "post_30m" {
			// OpenPrice should be the first candle's close at event time
			// ClosePrice should be the last candle's close 30 min later
			if ew.OpenPrice == 0 {
				t.Error("expected non-zero open price for SPY post_30m")
			}
			if ew.ClosePrice == 0 {
				t.Error("expected non-zero close price for SPY post_30m")
			}
			t.Logf("SPY post_30m event1: Open=%.4f, Close=%.4f", ew.OpenPrice, ew.ClosePrice)
			return
		}
	}
	t.Error("did not find SPY post_30m window for event 1")
}

func TestExtractEventWindows_EmptyStore(t *testing.T) {
	store := models.NewMarketDataStore()
	events := []models.MacroEvent{
		{ReleaseDate: time.Date(2026, 1, 14, 13, 30, 0, 0, time.UTC)},
	}

	extracted := ExtractEventWindows(events, store, DefaultWindows())

	if len(extracted) != 0 {
		t.Errorf("expected 0 windows from empty store, got %d", len(extracted))
	}
}

func TestExtractEventWindows_NoEvents(t *testing.T) {
	store, _ := makeTestStore()

	extracted := ExtractEventWindows(nil, store, DefaultWindows())

	if len(extracted) != 0 {
		t.Errorf("expected 0 windows with no events, got %d", len(extracted))
	}
}

func TestEventWindow_String(t *testing.T) {
	ew := EventWindow{
		Event: models.MacroEvent{
			ReleaseDate: time.Date(2026, 1, 14, 13, 30, 0, 0, time.UTC),
		},
		Asset:      "SPY",
		Window:     WindowDef{Name: "post_30m"},
		Data:       &models.AssetTimeSeries{DataPoints: make([]models.MarketDataPoint, 30)},
		OpenPrice:  520.0,
		ClosePrice: 518.0,
	}

	s := ew.String()
	if s == "" {
		t.Error("expected non-empty string representation")
	}
	t.Log(s)
}

// ============================================================
// 2. Return Computation Tests
// ============================================================

func TestComputeReturns_BasicComputation(t *testing.T) {
	store, events := makeTestStore()
	windows := DefaultWindows()

	extracted := ExtractEventWindows(events, store, windows)
	matrix := ComputeReturns(extracted)

	if matrix == nil {
		t.Fatal("expected non-nil return matrix")
	}
	if len(matrix.Returns) == 0 {
		t.Fatal("expected returns in matrix, got none")
	}
	if len(matrix.Assets) == 0 {
		t.Fatal("expected assets in matrix, got none")
	}
	if len(matrix.Windows) == 0 {
		t.Fatal("expected windows in matrix, got none")
	}

	t.Logf("Return matrix: %d returns, %d assets, %d windows",
		len(matrix.Returns), len(matrix.Assets), len(matrix.Windows))
}

func TestComputeReturns_KnownValues(t *testing.T) {
	// Create a simple scenario with known return
	event := models.MacroEvent{
		ReleaseDate: time.Date(2026, 1, 14, 13, 30, 0, 0, time.UTC),
		Surprise:    0.5,
	}

	ew := EventWindow{
		EventIndex: 0,
		Event:      event,
		Asset:      "TEST",
		Window:     WindowDef{Name: "post_30m"},
		Data:       &models.AssetTimeSeries{DataPoints: make([]models.MarketDataPoint, 2)},
		OpenPrice:  100.0,
		ClosePrice: 110.0,
	}

	matrix := ComputeReturns([]EventWindow{ew})

	if len(matrix.Returns) != 1 {
		t.Fatalf("expected 1 return, got %d", len(matrix.Returns))
	}

	// Expected return: (110 - 100) / 100 = 0.10 (10%)
	expectedReturn := 0.10
	if !floatEquals(matrix.Returns[0].Return, expectedReturn, 0.001) {
		t.Errorf("expected return %.4f, got %.4f", expectedReturn, matrix.Returns[0].Return)
	}
}

func TestComputeReturns_ZeroOpenPrice(t *testing.T) {
	ew := EventWindow{
		Event:      models.MacroEvent{ReleaseDate: time.Now()},
		Asset:      "TEST",
		Window:     WindowDef{Name: "post_30m"},
		OpenPrice:  0, // Should be skipped
		ClosePrice: 100.0,
	}

	matrix := ComputeReturns([]EventWindow{ew})

	if len(matrix.Returns) != 0 {
		t.Errorf("expected 0 returns for zero open price, got %d", len(matrix.Returns))
	}
}

func TestReturnMatrix_GetReturns(t *testing.T) {
	store, events := makeTestStore()
	extracted := ExtractEventWindows(events, store, DefaultWindows())
	matrix := ComputeReturns(extracted)

	// Filter by asset
	spyReturns := matrix.GetReturns("SPY", "")
	if len(spyReturns) == 0 {
		t.Error("expected SPY returns, got none")
	}
	for _, r := range spyReturns {
		if r.Asset != "SPY" {
			t.Errorf("filtered by SPY but got asset %s", r.Asset)
		}
	}

	// Filter by window
	post30m := matrix.GetReturns("", "post_30m")
	if len(post30m) == 0 {
		t.Error("expected post_30m returns, got none")
	}
	for _, r := range post30m {
		if r.Window != "post_30m" {
			t.Errorf("filtered by post_30m but got window %s", r.Window)
		}
	}

	// Filter by both
	spyPost30m := matrix.GetReturns("SPY", "post_30m")
	if len(spyPost30m) == 0 {
		t.Error("expected SPY post_30m returns, got none")
	}
}

func TestReturnMatrix_Summary(t *testing.T) {
	store, events := makeTestStore()
	extracted := ExtractEventWindows(events, store, DefaultWindows())
	matrix := ComputeReturns(extracted)

	summary := matrix.Summary()
	if summary == "" {
		t.Error("expected non-empty summary")
	}
	t.Log(summary)
}

func TestReturnMatrix_EmptyReturns(t *testing.T) {
	matrix := ComputeReturns(nil)
	summary := matrix.Summary()
	if summary == "" {
		t.Error("expected non-empty summary for empty matrix")
	}
}

// ============================================================
// 3. Surprise Sensitivity Model Tests
// ============================================================

func TestCalculateReturn(t *testing.T) {
	// Basic return calculation
	ret := CalculateReturn(100.0, 110.0)
	if !floatEquals(ret, 0.10, 0.001) {
		t.Errorf("expected 0.10, got %.4f", ret)
	}

	// Negative return
	ret = CalculateReturn(100.0, 95.0)
	if !floatEquals(ret, -0.05, 0.001) {
		t.Errorf("expected -0.05, got %.4f", ret)
	}

	// Zero return
	ret = CalculateReturn(100.0, 100.0)
	if !floatEquals(ret, 0.0, 0.001) {
		t.Errorf("expected 0.0, got %.4f", ret)
	}
}

func TestOLSRegression_PerfectLinear(t *testing.T) {
	// Perfect linear relationship: y = 2 + 3x
	x := []float64{1, 2, 3, 4, 5}
	y := []float64{5, 8, 11, 14, 17} // 2 + 3*1=5, 2+3*2=8, etc.

	result := olsRegression(x, y)

	// Alpha (intercept) should be 2.0
	if !floatEquals(result.Alpha, 2.0, 0.001) {
		t.Errorf("alpha: expected 2.0, got %.4f", result.Alpha)
	}

	// Beta (slope) should be 3.0
	if !floatEquals(result.Beta, 3.0, 0.001) {
		t.Errorf("beta: expected 3.0, got %.4f", result.Beta)
	}

	// R² should be 1.0 (perfect fit)
	if !floatEquals(result.RSquared, 1.0, 0.001) {
		t.Errorf("R²: expected 1.0, got %.4f", result.RSquared)
	}

	// N should be 5
	if result.N != 5 {
		t.Errorf("N: expected 5, got %d", result.N)
	}

	t.Logf("Perfect linear: α=%.4f, β=%.4f, R²=%.4f, t=%.4f",
		result.Alpha, result.Beta, result.RSquared, result.TStatistic)
}

func TestOLSRegression_NegativeSlope(t *testing.T) {
	// y = 10 - 2x
	x := []float64{1, 2, 3, 4, 5}
	y := []float64{8, 6, 4, 2, 0}

	result := olsRegression(x, y)

	if !floatEquals(result.Beta, -2.0, 0.001) {
		t.Errorf("beta: expected -2.0, got %.4f", result.Beta)
	}
	if !floatEquals(result.Alpha, 10.0, 0.001) {
		t.Errorf("alpha: expected 10.0, got %.4f", result.Alpha)
	}
	if !floatEquals(result.RSquared, 1.0, 0.001) {
		t.Errorf("R²: expected 1.0, got %.4f", result.RSquared)
	}
}

func TestOLSRegression_NoisyData(t *testing.T) {
	// Noisy relationship: y ≈ 1 + 0.5x + noise
	x := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	y := []float64{1.6, 2.1, 2.3, 3.1, 3.4, 4.2, 4.3, 5.0, 5.3, 5.9}

	result := olsRegression(x, y)

	// Beta should be approximately 0.47
	if result.Beta < 0.3 || result.Beta > 0.7 {
		t.Errorf("beta: expected ~0.47, got %.4f", result.Beta)
	}

	// R² should be high (strong linear relationship)
	if result.RSquared < 0.9 {
		t.Errorf("R²: expected > 0.9 for noisy linear, got %.4f", result.RSquared)
	}

	t.Logf("Noisy linear: α=%.4f, β=%.4f, R²=%.4f, SE=%.4f, t=%.4f",
		result.Alpha, result.Beta, result.RSquared, result.StdErr, result.TStatistic)
}

func TestOLSRegression_ZeroVariance(t *testing.T) {
	// All x values are the same — can't compute slope
	x := []float64{3, 3, 3, 3, 3}
	y := []float64{1, 2, 3, 4, 5}

	result := olsRegression(x, y)

	// Beta should be 0 (can't determine slope with constant x)
	if result.Beta != 0 {
		t.Errorf("beta: expected 0.0 for constant x, got %.4f", result.Beta)
	}

	// Alpha should be the mean of y
	if !floatEquals(result.Alpha, 3.0, 0.001) {
		t.Errorf("alpha: expected 3.0, got %.4f", result.Alpha)
	}
}

func TestComputeSensitivity_WithTestData(t *testing.T) {
	store, events := makeTestStore()
	extracted := ExtractEventWindows(events, store, DefaultWindows())
	matrix := ComputeReturns(extracted)

	results := ComputeSensitivity(matrix)

	// With only 2 events, we won't get results (need >= 3)
	// This is expected behavior
	t.Logf("Sensitivity results with 2 events: %d (expected 0, need ≥3 observations)", len(results))
}

func TestComputeSensitivity_WithEnoughData(t *testing.T) {
	// Create a matrix with enough data points for regression
	returns := []AssetReturn{
		{EventIndex: 0, Asset: "SPY", Window: "post_30m", Return: -0.005, Surprise: 0.5},
		{EventIndex: 1, Asset: "SPY", Window: "post_30m", Return: -0.010, Surprise: 1.0},
		{EventIndex: 2, Asset: "SPY", Window: "post_30m", Return: 0.003, Surprise: -0.3},
		{EventIndex: 3, Asset: "SPY", Window: "post_30m", Return: -0.015, Surprise: 1.5},
		{EventIndex: 4, Asset: "SPY", Window: "post_30m", Return: 0.008, Surprise: -0.8},
	}

	matrix := &ReturnMatrix{
		Returns: returns,
		Assets:  []string{"SPY"},
		Windows: []string{"post_30m"},
	}

	results := ComputeSensitivity(matrix)

	if len(results) != 1 {
		t.Fatalf("expected 1 sensitivity result, got %d", len(results))
	}

	result := results[0]
	if result.Asset != "SPY" {
		t.Errorf("expected asset SPY, got %s", result.Asset)
	}
	if result.N != 5 {
		t.Errorf("expected N=5, got %d", result.N)
	}

	// Beta should be negative (SPY falls on positive surprise)
	if result.Beta >= 0 {
		t.Errorf("expected negative beta for SPY (falls on hot CPI), got %.6f", result.Beta)
	}

	t.Logf("SPY sensitivity: α=%.6f, β=%.6f, R²=%.4f, SE=%.6f, t=%.3f",
		result.Alpha, result.Beta, result.RSquared, result.StdErr, result.TStatistic)
}

func TestSensitivitySummary(t *testing.T) {
	results := []SensitivityResult{
		{Asset: "SPY", Window: "post_30m", Alpha: 0.001, Beta: -0.005, RSquared: 0.85, N: 10, StdErr: 0.001, TStatistic: -5.0},
		{Asset: "VIX", Window: "post_30m", Alpha: 0.002, Beta: 0.008, RSquared: 0.72, N: 10, StdErr: 0.002, TStatistic: 4.0},
	}

	summary := SensitivitySummary(results)
	if summary == "" {
		t.Error("expected non-empty sensitivity summary")
	}
	t.Log(summary)
}

func TestSensitivitySummary_Empty(t *testing.T) {
	summary := SensitivitySummary(nil)
	if summary == "" {
		t.Error("expected non-empty summary for empty results")
	}
}

// ============================================================
// 4. Lead/Lag Analysis Tests
// ============================================================

func TestPearsonCorrelation_PerfectPositive(t *testing.T) {
	x := []float64{1, 2, 3, 4, 5}
	y := []float64{2, 4, 6, 8, 10} // y = 2x → perfect positive

	corr := pearsonCorrelation(x, y)

	if !floatEquals(corr, 1.0, 0.001) {
		t.Errorf("expected correlation 1.0, got %.4f", corr)
	}
}

func TestPearsonCorrelation_PerfectNegative(t *testing.T) {
	x := []float64{1, 2, 3, 4, 5}
	y := []float64{10, 8, 6, 4, 2} // y = 12 - 2x → perfect negative

	corr := pearsonCorrelation(x, y)

	if !floatEquals(corr, -1.0, 0.001) {
		t.Errorf("expected correlation -1.0, got %.4f", corr)
	}
}

func TestPearsonCorrelation_Uncorrelated(t *testing.T) {
	// Not perfectly uncorrelated, but low correlation
	x := []float64{1, 2, 3, 4, 5, 6, 7, 8}
	y := []float64{5, 2, 6, 1, 7, 3, 8, 4}

	corr := pearsonCorrelation(x, y)

	// Correlation should be near 0 for this data
	if math.Abs(corr) > 0.5 {
		t.Errorf("expected near-zero correlation, got %.4f", corr)
	}
	t.Logf("Uncorrelated data: r=%.4f", corr)
}

func TestPearsonCorrelation_ConstantSeries(t *testing.T) {
	x := []float64{1, 2, 3, 4, 5}
	y := []float64{3, 3, 3, 3, 3} // Constant → no variance

	corr := pearsonCorrelation(x, y)

	// Correlation of constant series should be 0
	if corr != 0 {
		t.Errorf("expected 0 for constant series, got %.4f", corr)
	}
}

func TestPearsonCorrelation_TooShort(t *testing.T) {
	x := []float64{1}
	y := []float64{2}

	corr := pearsonCorrelation(x, y)

	if !math.IsNaN(corr) {
		t.Errorf("expected NaN for single-element input, got %.4f", corr)
	}
}

func TestPearsonCorrelation_UnequalLength(t *testing.T) {
	x := []float64{1, 2, 3}
	y := []float64{4, 5}

	corr := pearsonCorrelation(x, y)

	if !math.IsNaN(corr) {
		t.Errorf("expected NaN for unequal-length input, got %.4f", corr)
	}
}

func TestComputeLeadLag_WithTestData(t *testing.T) {
	store, events := makeTestStore()

	results := ComputeLeadLag(store, events, "SPY", "VIX", 5)

	// Should have 11 results: lag -5 to +5
	if len(results) != 11 {
		t.Fatalf("expected 11 lag results, got %d", len(results))
	}

	// Verify lag range
	for i, r := range results {
		expectedLag := -5 + i
		if r.LagMinutes != expectedLag {
			t.Errorf("result[%d]: expected lag %d, got %d", i, expectedLag, r.LagMinutes)
		}
	}

	// Log the results
	for _, r := range results {
		t.Logf("SPY→VIX lag=%+d: r=%.4f (n=%d)", r.LagMinutes, r.Correlation, r.NEvents)
	}
}

func TestComputeLeadLag_MissingAsset(t *testing.T) {
	store, events := makeTestStore()

	results := ComputeLeadLag(store, events, "SPY", "NONEXISTENT", 3)

	// Should still return results, but correlations will be 0
	if len(results) != 7 { // -3 to +3
		t.Fatalf("expected 7 lag results, got %d", len(results))
	}

	for _, r := range results {
		if r.NEvents != 0 {
			t.Errorf("expected 0 events for missing asset, got %d", r.NEvents)
		}
	}
}

func TestComputeAllLeadLag(t *testing.T) {
	store, events := makeTestStore()
	assets := []string{"SPY", "VIX"}

	results := ComputeAllLeadLag(store, events, assets, 3)

	// 1 pair (SPY, VIX) × 7 lags = 7 results
	if len(results) != 7 {
		t.Fatalf("expected 7 results for 1 pair, got %d", len(results))
	}
}

func TestComputeAllLeadLag_ThreeAssets(t *testing.T) {
	store, events := makeTestStore()
	assets := []string{"SPY", "VIX", "DGS10"}

	results := ComputeAllLeadLag(store, events, assets, 2)

	// 3 pairs × 5 lags = 15 results
	// Pairs: (SPY,VIX), (SPY,DGS10), (VIX,DGS10)
	expectedPairs := 3
	expectedLagsPerPair := 5 // -2, -1, 0, +1, +2
	expectedTotal := expectedPairs * expectedLagsPerPair

	if len(results) != expectedTotal {
		t.Fatalf("expected %d results for 3 pairs, got %d", expectedTotal, len(results))
	}
}

func TestLeadLagSummary(t *testing.T) {
	results := []LeadLagResult{
		{AssetA: "SPY", AssetB: "VIX", LagMinutes: -2, Correlation: 0.3, NEvents: 5},
		{AssetA: "SPY", AssetB: "VIX", LagMinutes: -1, Correlation: 0.5, NEvents: 5},
		{AssetA: "SPY", AssetB: "VIX", LagMinutes: 0, Correlation: -0.8, NEvents: 5},
		{AssetA: "SPY", AssetB: "VIX", LagMinutes: 1, Correlation: -0.6, NEvents: 5},
		{AssetA: "SPY", AssetB: "VIX", LagMinutes: 2, Correlation: -0.2, NEvents: 5},
	}

	summary := LeadLagSummary(results)
	if summary == "" {
		t.Error("expected non-empty lead/lag summary")
	}
	t.Log(summary)
}

func TestLeadLagSummary_Empty(t *testing.T) {
	summary := LeadLagSummary(nil)
	if summary == "" {
		t.Error("expected non-empty summary for empty results")
	}
}

// ============================================================
// 5. Helper function tests
// ============================================================

func TestMean(t *testing.T) {
	// Basic mean
	values := []float64{1, 2, 3, 4, 5}
	if !floatEquals(mean(values), 3.0, 0.001) {
		t.Errorf("expected 3.0, got %.4f", mean(values))
	}

	// Single value
	if !floatEquals(mean([]float64{42.0}), 42.0, 0.001) {
		t.Errorf("expected 42.0, got %.4f", mean([]float64{42.0}))
	}

	// Empty
	if mean(nil) != 0 {
		t.Errorf("expected 0 for nil, got %.4f", mean(nil))
	}
}

func TestComputeMinuteReturns(t *testing.T) {
	ts := &models.AssetTimeSeries{
		Symbol:   "TEST",
		Interval: "1min",
		DataPoints: []models.MarketDataPoint{
			{Timestamp: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC), Close: 100.0},
			{Timestamp: time.Date(2026, 1, 1, 10, 1, 0, 0, time.UTC), Close: 101.0},
			{Timestamp: time.Date(2026, 1, 1, 10, 2, 0, 0, time.UTC), Close: 99.0},
		},
	}

	returns := computeMinuteReturns(ts)

	if len(returns) != 2 {
		t.Fatalf("expected 2 returns, got %d", len(returns))
	}

	// First return: (101 - 100) / 100 = 0.01
	if !floatEquals(returns[0].Value, 0.01, 0.001) {
		t.Errorf("expected first return 0.01, got %.4f", returns[0].Value)
	}

	// Second return: (99 - 101) / 101 ≈ -0.0198
	if !floatEquals(returns[1].Value, -0.01980, 0.001) {
		t.Errorf("expected second return ~-0.0198, got %.4f", returns[1].Value)
	}
}

func TestComputeMinuteReturns_TooShort(t *testing.T) {
	ts := &models.AssetTimeSeries{
		DataPoints: []models.MarketDataPoint{
			{Close: 100.0},
		},
	}

	returns := computeMinuteReturns(ts)
	if returns != nil {
		t.Errorf("expected nil for 1-point series, got %d returns", len(returns))
	}
}

// ============================================================
// 6. Integration test — full pipeline
// ============================================================

func TestFullAnalyticsPipeline(t *testing.T) {
	store, events := makeTestStore()

	// Step 1: Extract event windows
	windows := DefaultWindows()
	extracted := ExtractEventWindows(events, store, windows)
	if len(extracted) == 0 {
		t.Fatal("no event windows extracted")
	}
	t.Logf("Step 1: Extracted %d event windows", len(extracted))

	// Step 2: Compute returns
	matrix := ComputeReturns(extracted)
	if len(matrix.Returns) == 0 {
		t.Fatal("no returns computed")
	}
	t.Logf("Step 2: Computed %d returns across %d assets × %d windows",
		len(matrix.Returns), len(matrix.Assets), len(matrix.Windows))

	// Step 3: Sensitivity analysis (may have insufficient data with 2 events)
	sensitivity := ComputeSensitivity(matrix)
	t.Logf("Step 3: %d sensitivity results (need ≥3 events for regression)", len(sensitivity))

	// Step 4: Lead/lag analysis
	intradayAssets := []string{"SPY", "VIX"}
	leadlag := ComputeAllLeadLag(store, events, intradayAssets, 5)
	t.Logf("Step 4: %d lead/lag results", len(leadlag))

	// Print summaries
	t.Log("\n" + matrix.Summary())
	if len(sensitivity) > 0 {
		t.Log("\n" + SensitivitySummary(sensitivity))
	}
	t.Log("\n" + LeadLagSummary(leadlag))
}
