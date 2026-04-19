package main

import (
	"fmt"
	"log"
	"macro-impact-tracker/internal/macro"
	"macro-impact-tracker/internal/market"

	"github.com/joho/godotenv"
)

func init() {
	err := godotenv.Load("configs/.env")
	if err != nil {
		log.Fatal("Error loading .env file")
	}
}

func main() {
	fmt.Println("=== Macro Event Impact Tracker ===")
	fmt.Println()

	// =========================================================
	// PHASE 1: Build MacroEvent structs from FRED data
	// =========================================================

	// --- Step 1: Fetch CPI series data (historical values) ---
	fmt.Println("[1/4] Fetching CPI observations from FRED...")
	series, err := macro.FetchCPIObservations()
	if err != nil {
		log.Fatalf("Failed to fetch CPI observations: %v", err)
	}
	fmt.Printf("      → Got %d CPI observations\n", len(series.Observations))

	// --- Step 2: Fetch CPI release dates ---
	fmt.Println("[2/4] Fetching CPI release dates from FRED...")
	releases, err := macro.FetchCPIReleaseDates()
	if err != nil {
		log.Fatalf("Failed to fetch CPI release dates: %v", err)
	}
	fmt.Printf("      → Got %d release dates\n", len(releases.ReleaseDates))

	// --- Step 3: Build MacroEvent structs ---
	fmt.Println("[3/4] Building MacroEvent structs...")
	events, err := macro.BuildMacroEvents(series, releases)
	if err != nil {
		log.Fatalf("Failed to build macro events: %v", err)
	}
	fmt.Printf("      → Built %d macro events\n\n", len(events))

	// Print the last 5 events (most recent)
	fmt.Println("--- Last 5 CPI Events ---")
	start := len(events) - 5
	if start < 0 {
		start = 0
	}
	for _, e := range events[start:] {
		fmt.Println(e)
	}

	fmt.Println()

	// =========================================================
	// PHASE 2: Fetch real market data around each CPI release
	// =========================================================

	fmt.Println("[4/4] Fetching market data around CPI releases...")
	fmt.Println()

	// Fetch market data for the last 3 events only (to stay within
	// Twelve Data free tier rate limits: 8 req/min, 800 req/day).
	// Change maxEvents to 0 for all events (requires patience or a paid key).
	maxEvents := 3
	store := market.FetchMarketDataForEvents(events, maxEvents)

	// --- Print summary of all fetched data ---
	fmt.Println()
	fmt.Println("=== Market Data Summary ===")
	fmt.Println(store.Summary())

	// --- Show a sample window around the most recent event ---
	if len(events) > 0 {
		latestEvent := events[len(events)-1]
		fmt.Printf("--- Sample: SPY data around %s ---\n", latestEvent.ReleaseDate.Format("2006-01-02 15:04 UTC"))

		spyData := store.Get("SPY")
		if spyData != nil && spyData.Len() > 0 {
			// Show first 5 and last 5 data points
			fmt.Printf("Total SPY data points: %d\n", spyData.Len())
			limit := 5
			if spyData.Len() < limit {
				limit = spyData.Len()
			}
			fmt.Println("First candles:")
			for _, dp := range spyData.DataPoints[:limit] {
				fmt.Printf("  %s\n", dp)
			}
			if spyData.Len() > limit*2 {
				fmt.Printf("  ... (%d more) ...\n", spyData.Len()-limit*2)
			}
			fmt.Println("Last candles:")
			endStart := spyData.Len() - limit
			if endStart < 0 {
				endStart = 0
			}
			for _, dp := range spyData.DataPoints[endStart:] {
				fmt.Printf("  %s\n", dp)
			}
		} else {
			fmt.Println("  (no SPY data available)")
		}

		// Show Treasury yield data
		fmt.Println()
		fmt.Println("--- Treasury Yields ---")
		for _, sym := range []string{"DGS2", "DGS10"} {
			ts := store.Get(sym)
			if ts != nil && ts.Len() > 0 {
				latest := ts.DataPoints[ts.Len()-1]
				fmt.Printf("  %s: %.2f%% (as of %s)\n", sym, latest.Close, latest.Timestamp.Format("2006-01-02"))
			} else {
				fmt.Printf("  %s: (no data)\n", sym)
			}
		}

		// Show DXY proxy
		fmt.Println()
		fmt.Println("--- DXY Proxy ---")
		dxy := store.Get("DXY (proxy)")
		if dxy != nil && dxy.Len() > 0 {
			latest := dxy.DataPoints[dxy.Len()-1]
			fmt.Printf("  DXY (proxy): %.4f (as of %s)\n", latest.Close, latest.Timestamp.Format("2006-01-02 15:04"))
		} else {
			fmt.Println("  (no DXY proxy data — EUR/USD may not have been fetched)")
		}
	}

	fmt.Println()
	fmt.Println("=== Done ===")
}
