package market

import (
	"encoding/json"
	"fmt"
	"io"
	"macro-impact-tracker/internal/models"
	"net/http"
	"os"
	"strconv"
	"time"
)

// ============================================================
// Twelve Data API Client
// ============================================================
// Twelve Data provides minute-level OHLCV data for equities and forex.
// Free tier: 8 API credits/minute, 800 credits/day.
// Docs: https://twelvedata.com/docs#time-series
// ============================================================

const twelveDataBaseURL = "https://api.twelvedata.com/time_series"

// --- JSON Response Structures ---
// These mirror the exact JSON shape returned by Twelve Data's /time_series endpoint.

// TwelveDataMeta contains metadata about the response.
type TwelveDataMeta struct {
	Symbol   string `json:"symbol"`
	Interval string `json:"interval"`
	Currency string `json:"currency"`
	Exchange string `json:"exchange"`
	Type     string `json:"type"`
}

// TwelveDataValue represents a single candle in the response.
// All numeric fields come as strings from the API, so we parse them manually.
type TwelveDataValue struct {
	Datetime string `json:"datetime"` // "2026-04-17 09:30:00"
	Open     string `json:"open"`     // "520.10000"
	High     string `json:"high"`
	Low      string `json:"low"`
	Close    string `json:"close"`
	Volume   string `json:"volume"` // "45200"
}

// TwelveDataResponse is the top-level JSON response from the API.
type TwelveDataResponse struct {
	Meta   TwelveDataMeta    `json:"meta"`
	Values []TwelveDataValue `json:"values"`
	Status string            `json:"status"` // "ok" or "error"
	// Error fields (populated when status == "error")
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// fetchFromTwelveData calls the Twelve Data /time_series endpoint and returns
// an AssetTimeSeries with parsed OHLCV data.
//
// Parameters:
//   - symbol:    ticker like "SPY" or "EUR/USD"
//   - interval:  candle size like "1min", "5min", "1day"
//   - startDate: window start in "2006-01-02 15:04:05" format (UTC)
//   - endDate:   window end in "2006-01-02 15:04:05" format (UTC)
//
// The function handles:
//   - Building the URL with all required parameters
//   - Making the HTTP request
//   - Parsing the JSON response
//   - Converting string values to float64/int64
//   - Sorting the result chronologically (Twelve Data returns newest-first)
func fetchFromTwelveData(symbol, interval, startDate, endDate string) (*models.AssetTimeSeries, error) {
	apiKey := os.Getenv("TWELVE_DATA_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("TWELVE_DATA_API_KEY environment variable is not set")
	}

	// Build the request URL
	// outputsize=5000 requests the maximum number of data points
	url := fmt.Sprintf(
		"%s?symbol=%s&interval=%s&start_date=%s&end_date=%s&outputsize=5000&apikey=%s",
		twelveDataBaseURL, symbol, interval, startDate, endDate, apiKey,
	)

	// Make the HTTP GET request
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed for %s: %w", symbol, err)
	}
	defer resp.Body.Close()

	// Read entire body (needed because we may want to log it on error)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body for %s: %w", symbol, err)
	}

	// Parse the JSON response
	var data TwelveDataResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed to parse JSON for %s: %w", symbol, err)
	}

	// Check for API-level errors
	if data.Status == "error" {
		return nil, fmt.Errorf("Twelve Data API error for %s: [%d] %s", symbol, data.Code, data.Message)
	}

	// Convert the raw JSON values into MarketDataPoint structs
	points, err := parseTwelveDataValues(data.Values, interval)
	if err != nil {
		return nil, fmt.Errorf("failed to parse values for %s: %w", symbol, err)
	}

	// Build the result
	ts := &models.AssetTimeSeries{
		Symbol:     symbol,
		Interval:   interval,
		DataPoints: points,
	}
	ts.SortByTime() // Twelve Data returns newest-first, we want oldest-first

	return ts, nil
}

// parseTwelveDataValues converts the raw string values from Twelve Data into
// typed MarketDataPoint structs. It skips any data points with parse errors
// rather than failing the entire request (some candles may have missing volume etc.)
func parseTwelveDataValues(values []TwelveDataValue, interval string) ([]models.MarketDataPoint, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("no data points returned")
	}

	// Determine the datetime layout based on the interval
	// Intraday intervals include time, daily intervals don't
	layout := "2006-01-02 15:04:05"
	if interval == "1day" || interval == "1week" || interval == "1month" {
		layout = "2006-01-02"
	}

	var points []models.MarketDataPoint
	var parseErrors int

	for _, v := range values {
		// Parse the timestamp
		ts, err := time.Parse(layout, v.Datetime)
		if err != nil {
			parseErrors++
			continue
		}

		// Parse OHLCV values (all come as strings)
		open, err := strconv.ParseFloat(v.Open, 64)
		if err != nil {
			parseErrors++
			continue
		}
		high, err := strconv.ParseFloat(v.High, 64)
		if err != nil {
			parseErrors++
			continue
		}
		low, err := strconv.ParseFloat(v.Low, 64)
		if err != nil {
			parseErrors++
			continue
		}
		close_, err := strconv.ParseFloat(v.Close, 64)
		if err != nil {
			parseErrors++
			continue
		}
		vol, err := strconv.ParseInt(v.Volume, 10, 64)
		if err != nil {
			vol = 0 // Volume is not critical, default to 0 for forex
		}

		points = append(points, models.MarketDataPoint{
			Timestamp: ts,
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close_,
			Volume:    vol,
		})
	}

	if len(points) == 0 {
		return nil, fmt.Errorf("all %d data points had parse errors", parseErrors)
	}

	return points, nil
}
