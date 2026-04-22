package market

import (
	"encoding/json"
	"fmt"
	"io"
	"macro-impact-tracker/internal/models"
	"net/http"
	"net/url"
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

	// Build the request URL using proper URL encoding.
	// - url.QueryEscape handles symbols with special characters (e.g., EUR/USD → EUR%2FUSD)
	// - timezone=UTC ensures the API interprets our start_date/end_date as UTC
	//   (without this, the API defaults to exchange timezone, e.g., America/New_York for SPY)
	// - outputsize=5000 requests the maximum number of data points
	// - order=asc returns data chronologically (oldest first)
	requestURL := fmt.Sprintf(
		"%s?symbol=%s&interval=%s&start_date=%s&end_date=%s&outputsize=5000&timezone=UTC&order=asc&format=JSON&apikey=%s",
		twelveDataBaseURL,
		url.QueryEscape(symbol),
		url.QueryEscape(interval),
		url.QueryEscape(startDate),
		url.QueryEscape(endDate),
		apiKey,
	)

	// Make the HTTP GET request
	resp, err := http.Get(requestURL)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed for %s: %w", symbol, err)
	}
	defer resp.Body.Close()

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d for %s: %s", resp.StatusCode, symbol, string(body))
	}

	// Read entire body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body for %s: %w", symbol, err)
	}

	// Parse the JSON response
	var data TwelveDataResponse
	if err := json.Unmarshal(body, &data); err != nil {
		// Log a snippet of the raw body for debugging
		snippet := string(body)
		if len(snippet) > 200 {
			snippet = snippet[:200] + "..."
		}
		return nil, fmt.Errorf("failed to parse JSON for %s: %w\n  Raw response: %s", symbol, err, snippet)
	}

	// Check for API-level errors
	if data.Status == "error" {
		return nil, fmt.Errorf("Twelve Data API error for %s: [%d] %s", symbol, data.Code, data.Message)
	}

	if len(data.Values) == 0 {
		return nil, fmt.Errorf("Twelve Data returned 0 data points for %s (status: %s)", symbol, data.Status)
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
	ts.SortByTime() // Ensure oldest-first ordering

	return ts, nil
}

// parseTwelveDataValues converts the raw string values from Twelve Data into
// typed MarketDataPoint structs. It skips any data points with parse errors
// rather than failing the entire request (some candles may have missing volume etc.)
func parseTwelveDataValues(values []TwelveDataValue, interval string) ([]models.MarketDataPoint, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("no data points returned")
	}

	var points []models.MarketDataPoint
	var parseErrors int

	for _, v := range values {
		// Parse the timestamp with flexible format detection.
		// Twelve Data returns different formats depending on interval:
		//   Intraday: "2026-01-14 09:30:00" (with or without seconds)
		//   Daily:    "2026-01-14"
		ts, err := parseFlexibleDatetime(v.Datetime, interval)
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
			vol = 0 // Volume is not critical, default to 0 for forex/indices
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

// parseFlexibleDatetime tries multiple datetime formats returned by Twelve Data.
// The API can return:
//   - "2026-01-14 09:30:00" (intraday with seconds)
//   - "2026-01-14 09:30"    (intraday without seconds — some responses)
//   - "2026-01-14"          (daily/weekly/monthly)
func parseFlexibleDatetime(datetime, interval string) (time.Time, error) {
	// For daily/weekly/monthly intervals, only try date format
	if interval == "1day" || interval == "1week" || interval == "1month" {
		return time.Parse("2006-01-02", datetime)
	}

	// For intraday, try multiple formats in order of likelihood
	layouts := []string{
		"2006-01-02 15:04:05", // Full datetime with seconds
		"2006-01-02 15:04",    // Datetime without seconds
		"2006-01-02T15:04:05", // ISO format variant
		"2006-01-02",          // Fallback to date only
	}

	var lastErr error
	for _, layout := range layouts {
		if t, err := time.Parse(layout, datetime); err == nil {
			return t, nil
		} else {
			lastErr = err
		}
	}

	return time.Time{}, fmt.Errorf("could not parse datetime %q: %w", datetime, lastErr)
}
