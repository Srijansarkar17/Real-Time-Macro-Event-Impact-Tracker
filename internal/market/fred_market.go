package market

import (
	"encoding/json"
	"fmt"
	"macro-impact-tracker/internal/models"
	"net/http"
	"os"
	"strconv"
	"time"
)

// ============================================================
// FRED Market Data Fetcher — Treasury Yields
// ============================================================
// This file fetches daily Treasury yield data from FRED.
// Unlike the macro layer's FRED code (which fetches CPI observations
// and release dates), this focuses on market-facing yield data:
//   - DGS2  → 2-Year Treasury Constant Maturity Rate
//   - DGS10 → 10-Year Treasury Constant Maturity Rate
//
// These are daily resolution (no intraday available for yields),
// but that's sufficient for event studies because yield movements
// around CPI releases are typically measured day-over-day.
// ============================================================

const fredBaseURL = "https://api.stlouisfed.org/fred/series/observations"

// fredObservation mirrors the JSON structure of a single FRED observation.
// This is the same shape as FredObservation in the macro package,
// but we define it separately to avoid a circular dependency.
type fredObservation struct {
	Date  string `json:"date"`
	Value string `json:"value"`
}

// fredObservationsResponse is the top-level JSON response from the FRED
// series/observations endpoint.
type fredObservationsResponse struct {
	Observations []fredObservation `json:"observations"`
}

// fetchFromFRED calls the FRED API to get daily observations for a given
// series ID (e.g., "DGS2" for 2-year yields) within a date range.
//
// Parameters:
//   - seriesID:  FRED series identifier (e.g., "DGS2", "DGS10")
//   - startDate: start of the range in "2006-01-02" format
//   - endDate:   end of the range in "2006-01-02" format
//
// Returns an AssetTimeSeries where each data point has:
//   - Timestamp = the observation date (at midnight UTC)
//   - Open = High = Low = Close = the yield value (e.g., 4.25 = 4.25%)
//   - Volume = 0 (not applicable for yields)
//
// Why Open=High=Low=Close?
// Treasury yields are reported as a single daily value, not as OHLCV candles.
// Setting all four prices to the same value lets us use the same data structures
// as equities/forex without special-casing the analytics engine.
func fetchFromFRED(seriesID, startDate, endDate string) (*models.AssetTimeSeries, error) {
	apiKey := os.Getenv("FRED_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("FRED_API_KEY environment variable is not set")
	}

	url := fmt.Sprintf(
		"%s?series_id=%s&observation_start=%s&observation_end=%s&api_key=%s&file_type=json",
		fredBaseURL, seriesID, startDate, endDate, apiKey,
	)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed for FRED %s: %w", seriesID, err)
	}
	defer resp.Body.Close()

	var data fredObservationsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to parse FRED JSON for %s: %w", seriesID, err)
	}

	// Convert FRED observations to MarketDataPoint structs
	layout := "2006-01-02"
	var points []models.MarketDataPoint

	for _, obs := range data.Observations {
		// FRED uses "." for missing/unavailable data — skip these
		if obs.Value == "." {
			continue
		}

		date, err := time.Parse(layout, obs.Date)
		if err != nil {
			continue
		}

		val, err := strconv.ParseFloat(obs.Value, 64)
		if err != nil {
			continue
		}

		// All OHLC fields set to the same yield value
		points = append(points, models.MarketDataPoint{
			Timestamp: date,
			Open:      val,
			High:      val,
			Low:       val,
			Close:     val,
			Volume:    0,
		})
	}

	if len(points) == 0 {
		return nil, fmt.Errorf("no valid observations for FRED series %s in range %s to %s", seriesID, startDate, endDate)
	}

	ts := &models.AssetTimeSeries{
		Symbol:     seriesID,
		Interval:   "1day",
		DataPoints: points,
	}
	ts.SortByTime()

	return ts, nil
}
