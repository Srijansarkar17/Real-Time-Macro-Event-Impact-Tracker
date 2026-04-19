package market

import (
	"fmt"
	"macro-impact-tracker/internal/models"
	"sync"
	"time"
)

// ============================================================
// Market Data Orchestrator
// ============================================================
// This file coordinates fetching market data for all assets
// around each CPI release event. It:
//   1. Defines which assets to track and where to fetch them
//   2. Computes time windows around each macro event
//   3. Fetches all assets concurrently using goroutines
//   4. Stores everything in a thread-safe MarketDataStore
//   5. Computes derived assets (DXY proxy from EUR/USD)
// ============================================================

// AssetConfig defines a single asset that we want to track.
// The Source field tells the orchestrator which API to call.
type AssetConfig struct {
	Symbol   string // Ticker: "SPY", "EUR/USD", "DGS2", "DGS10"
	Source   string // API source: "twelvedata" or "fred"
	Interval string // Candle size: "1min" for intraday, "1day" for daily
}

// DefaultAssets returns the full list of assets to fetch.
// This is the single source of truth for what we track.
//
// Asset list:
//   - SPY       → S&P 500 ETF (broad equity market)
//   - EUR/USD   → Euro/Dollar forex pair (currency markets)
//   - VIX       → CBOE Volatility Index (fear gauge)
//   - DGS2      → 2-Year Treasury yield (short-term rates)
//   - DGS10     → 10-Year Treasury yield (long-term rates)
//   - DXY       → US Dollar Index (computed from EUR/USD, not fetched)
func DefaultAssets() []AssetConfig {
	return []AssetConfig{
		{Symbol: "SPY", Source: "twelvedata", Interval: "1min"},
		{Symbol: "EUR/USD", Source: "twelvedata", Interval: "1min"},
		{Symbol: "VIX", Source: "twelvedata", Interval: "1min"},
		{Symbol: "DGS2", Source: "fred", Interval: "1day"},
		{Symbol: "DGS10", Source: "fred", Interval: "1day"},
		// DXY is computed from EUR/USD — not a direct fetch target
	}
}

// rateLimiter controls how fast we call the Twelve Data API.
// Free tier allows 8 requests per minute, so we wait at least
// 8 seconds between calls to stay safely under the limit.
var rateLimiter = time.NewTicker(8 * time.Second)

// FetchMarketDataForEvents fetches market data for all configured assets
// around each MacroEvent release date.
//
// For each event, the time window is:
//   - Intraday (1min): [ReleaseDate - 30min, ReleaseDate + 2h]
//   - Daily (1day):    [ReleaseDate - 5 days, ReleaseDate + 5 days]
//
// Why these windows?
//   - 30min before: captures pre-positioning and early leaks
//   - 2h after: captures the initial reaction and first stabilization
//   - 5 days for dailies: Treasury yields move slower, need broader context
//
// The function fetches assets concurrently (goroutines + WaitGroup)
// but with rate limiting for the Twelve Data API.
// After fetching, it computes the DXY proxy from EUR/USD data.
//
// Parameters:
//   - events: the macro events from Phase 1 (used to compute time windows)
//   - maxEvents: limit how many events to fetch (0 = all). Use this to
//     stay within API rate limits during development.
//
// Returns a MarketDataStore containing all fetched data.
func FetchMarketDataForEvents(events []models.MacroEvent, maxEvents int) *models.MarketDataStore {
	store := models.NewMarketDataStore()
	assets := DefaultAssets()

	// Limit the number of events to process (for rate-limit safety)
	eventsToProcess := events
	if maxEvents > 0 && maxEvents < len(events) {
		// Take the LAST N events (most recent)
		eventsToProcess = events[len(events)-maxEvents:]
	}

	fmt.Printf("      → Fetching market data for %d events × %d assets\n", len(eventsToProcess), len(assets))

	// --- Fetch each asset for each event ---
	// We use a WaitGroup to wait for all goroutines, but serialize
	// Twelve Data calls via a channel to respect rate limits.
	var wg sync.WaitGroup

	// Separate assets by source for different handling
	var twelveDataAssets []AssetConfig
	var fredAssets []AssetConfig
	for _, a := range assets {
		switch a.Source {
		case "twelvedata":
			twelveDataAssets = append(twelveDataAssets, a)
		case "fred":
			fredAssets = append(fredAssets, a)
		}
	}

	// --- FRED assets: can be fetched concurrently (generous rate limits) ---
	for _, asset := range fredAssets {
		wg.Add(1)
		go func(a AssetConfig) {
			defer wg.Done()
			fetchFREDForEvents(store, a, eventsToProcess)
		}(asset)
	}

	// --- Twelve Data assets: fetch sequentially with rate limiting ---
	// We run this in a single goroutine that processes all events × assets
	// to control the request rate.
	wg.Add(1)
	go func() {
		defer wg.Done()
		fetchTwelveDataForEvents(store, twelveDataAssets, eventsToProcess)
	}()

	wg.Wait()

	// --- Compute derived assets ---
	computeDerivedAssets(store)

	return store
}

// fetchTwelveDataForEvents handles all Twelve Data API calls sequentially
// with rate limiting between requests.
func fetchTwelveDataForEvents(store *models.MarketDataStore, assets []AssetConfig, events []models.MacroEvent) {
	for _, event := range events {
		for _, asset := range assets {
			// Compute the intraday window: -30min to +2h around release
			startTime := event.ReleaseDate.Add(-30 * time.Minute)
			endTime := event.ReleaseDate.Add(2 * time.Hour)

			startStr := startTime.Format("2006-01-02 15:04:05")
			endStr := endTime.Format("2006-01-02 15:04:05")

			fmt.Printf("      → [TwelveData] Fetching %s for %s window...\n",
				asset.Symbol, event.ReleaseDate.Format("2006-01-02"))

			// Wait for rate limiter before making the request
			<-rateLimiter.C

			ts, err := fetchFromTwelveData(asset.Symbol, asset.Interval, startStr, endStr)
			if err != nil {
				fmt.Printf("        ⚠ Error fetching %s: %v\n", asset.Symbol, err)
				continue
			}

			// Store the result (thread-safe, merges if data already exists)
			store.Add(asset.Symbol, ts)
			fmt.Printf("        ✓ Got %d data points for %s\n", ts.Len(), asset.Symbol)
		}
	}
}

// fetchFREDForEvents fetches daily data from FRED for all events.
// FRED data is daily, so we use a wider window: -5 days to +5 days.
func fetchFREDForEvents(store *models.MarketDataStore, asset AssetConfig, events []models.MacroEvent) {
	for _, event := range events {
		// Compute the daily window: -5 days to +5 days around release
		startDate := event.ReleaseDate.AddDate(0, 0, -5)
		endDate := event.ReleaseDate.AddDate(0, 0, 5)

		startStr := startDate.Format("2006-01-02")
		endStr := endDate.Format("2006-01-02")

		fmt.Printf("      → [FRED] Fetching %s for %s window...\n",
			asset.Symbol, event.ReleaseDate.Format("2006-01-02"))

		ts, err := fetchFromFRED(asset.Symbol, startStr, endStr)
		if err != nil {
			fmt.Printf("        ⚠ Error fetching %s: %v\n", asset.Symbol, err)
			continue
		}

		store.Add(asset.Symbol, ts)
		fmt.Printf("        ✓ Got %d data points for %s\n", ts.Len(), asset.Symbol)
	}
}

// computeDerivedAssets generates computed assets from the raw fetched data.
// Currently this computes the DXY proxy from EUR/USD.
func computeDerivedAssets(store *models.MarketDataStore) {
	eurusd := store.Get("EUR/USD")
	if eurusd == nil || eurusd.Len() == 0 {
		fmt.Println("      ⚠ Cannot compute DXY proxy: no EUR/USD data available")
		return
	}

	dxy, err := ComputeDXYFromEURUSD(eurusd)
	if err != nil {
		fmt.Printf("      ⚠ Error computing DXY proxy: %v\n", err)
		return
	}

	store.Add("DXY (proxy)", dxy)
	fmt.Printf("      ✓ Computed DXY proxy: %d data points\n", dxy.Len())
}
