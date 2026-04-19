package market

import (
	"testing"
)

// ============================================================
// Tests for Twelve Data JSON parsing (twelvedata.go)
// ============================================================
// These tests verify the parseTwelveDataValues function, which converts
// the raw string values from the API response into typed MarketDataPoint structs.
// We don't test the actual API call (that requires a real key + network).
// ============================================================

func TestParseTwelveDataValues_IntradayFormat(t *testing.T) {
	// Simulate a Twelve Data response with 1min candles
	values := []TwelveDataValue{
		{
			Datetime: "2026-01-14 09:30:00",
			Open:     "520.10000",
			High:     "520.25000",
			Low:      "520.05000",
			Close:    "520.15000",
			Volume:   "12500",
		},
		{
			Datetime: "2026-01-14 09:31:00",
			Open:     "520.15000",
			High:     "520.30000",
			Low:      "520.10000",
			Close:    "520.20000",
			Volume:   "8300",
		},
	}

	points, err := parseTwelveDataValues(values, "1min")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(points) != 2 {
		t.Fatalf("expected 2 data points, got %d", len(points))
	}

	// Verify first candle
	p := points[0]
	if p.Open != 520.10 {
		t.Errorf("Open: expected 520.10, got %.2f", p.Open)
	}
	if p.High != 520.25 {
		t.Errorf("High: expected 520.25, got %.2f", p.High)
	}
	if p.Low != 520.05 {
		t.Errorf("Low: expected 520.05, got %.2f", p.Low)
	}
	if p.Close != 520.15 {
		t.Errorf("Close: expected 520.15, got %.2f", p.Close)
	}
	if p.Volume != 12500 {
		t.Errorf("Volume: expected 12500, got %d", p.Volume)
	}

	// Verify timestamp was parsed correctly
	expected := "2026-01-14 09:30:00"
	actual := p.Timestamp.Format("2006-01-02 15:04:05")
	if actual != expected {
		t.Errorf("Timestamp: expected %q, got %q", expected, actual)
	}
}

func TestParseTwelveDataValues_DailyFormat(t *testing.T) {
	values := []TwelveDataValue{
		{
			Datetime: "2026-01-14",
			Open:     "520.00000",
			High:     "522.00000",
			Low:      "519.00000",
			Close:    "521.50000",
			Volume:   "1000000",
		},
	}

	points, err := parseTwelveDataValues(values, "1day")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(points) != 1 {
		t.Fatalf("expected 1 data point, got %d", len(points))
	}

	// Verify daily date format was parsed correctly
	expected := "2026-01-14"
	actual := points[0].Timestamp.Format("2006-01-02")
	if actual != expected {
		t.Errorf("Timestamp: expected %q, got %q", expected, actual)
	}
}

func TestParseTwelveDataValues_EmptyInput(t *testing.T) {
	_, err := parseTwelveDataValues([]TwelveDataValue{}, "1min")
	if err == nil {
		t.Error("expected error for empty input, got nil")
	}
}

func TestParseTwelveDataValues_SkipsBadRows(t *testing.T) {
	values := []TwelveDataValue{
		{
			Datetime: "2026-01-14 09:30:00",
			Open:     "520.10",
			High:     "520.25",
			Low:      "520.05",
			Close:    "520.15",
			Volume:   "12500",
		},
		{
			Datetime: "bad-date",       // This will fail to parse
			Open:     "520.20",
			High:     "520.30",
			Low:      "520.10",
			Close:    "520.25",
			Volume:   "8300",
		},
		{
			Datetime: "2026-01-14 09:32:00",
			Open:     "not-a-number", // This will fail to parse
			High:     "520.35",
			Low:      "520.15",
			Close:    "520.30",
			Volume:   "9500",
		},
	}

	points, err := parseTwelveDataValues(values, "1min")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only the first row should parse successfully
	if len(points) != 1 {
		t.Errorf("expected 1 valid data point (2 skipped), got %d", len(points))
	}
}

func TestParseTwelveDataValues_ZeroVolumeForForex(t *testing.T) {
	// Forex pairs often return "0" or non-numeric volume
	values := []TwelveDataValue{
		{
			Datetime: "2026-01-14 09:30:00",
			Open:     "1.10000",
			High:     "1.10050",
			Low:      "1.09950",
			Close:    "1.10020",
			Volume:   "0",
		},
	}

	points, err := parseTwelveDataValues(values, "1min")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if points[0].Volume != 0 {
		t.Errorf("expected volume 0 for forex, got %d", points[0].Volume)
	}
}
