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

	// --- Step 1: Fetch CPI series data (historical values) ---
	fmt.Println("[1/3] Fetching CPI observations from FRED...")
	series, err := macro.FetchCPIObservations()
	if err != nil {
		log.Fatalf("Failed to fetch CPI observations: %v", err)
	}
	fmt.Printf("      → Got %d CPI observations\n", len(series.Observations))

	// --- Step 2: Fetch CPI release dates ---
	fmt.Println("[2/3] Fetching CPI release dates from FRED...")
	releases, err := macro.FetchCPIReleaseDates()
	if err != nil {
		log.Fatalf("Failed to fetch CPI release dates: %v", err)
	}
	fmt.Printf("      → Got %d release dates\n", len(releases.ReleaseDates))

	// --- Step 3: Build MacroEvent structs ---
	fmt.Println("[3/3] Building MacroEvent structs...")
	events, err := macro.BuildMacroEvents(series, releases)
	if err != nil {
		log.Fatalf("Failed to build macro events: %v", err)
	}
	fmt.Printf("      → Built %d macro events\n\n", len(events))

	// Print the last 10 events (most recent)
	fmt.Println("--- Last 10 CPI Events ---")
	start := len(events) - 10
	if start < 0 {
		start = 0
	}
	for _, e := range events[start:] {
		fmt.Println(e)
	}

	fmt.Println()

	// --- Market data (scaffold) ---
	fmt.Println("--- Market Data Fetch (scaffold) ---")
	market.FetchMarketData()
}

