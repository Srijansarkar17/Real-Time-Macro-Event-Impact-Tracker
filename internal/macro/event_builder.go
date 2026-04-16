package macro

import (
	"fmt"
	"macro-impact-tracker/internal/models"
	"sort"
	"strconv"
	"time"
)

// BuildReleaseTimestamp converts a date string (e.g., "2026-02-18") into a precise
// UTC timestamp at 8:30 AM Eastern Time — the standard CPI release time.
func BuildReleaseTimestamp(dateStr string) (time.Time, error) {
	layout := "2006-01-02" // Go's reference layout for parsing

	date, err := time.Parse(layout, dateStr)
	if err != nil {
		return time.Time{}, err
	}

	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to load timezone: %w", err)
	}

	releaseTime := time.Date(
		date.Year(),
		date.Month(),
		date.Day(),
		8, 30, 0, 0, // 08:30:00 AM
		loc,
	)

	return releaseTime.UTC(), nil
}

// parseObservationValue safely parses a FRED observation value string to float64.
// FRED uses "." for missing/unavailable data points, which we return as 0.
func parseObservationValue(val string) (float64, error) {
	if val == "." {
		return 0, fmt.Errorf("missing value (FRED uses '.' for unavailable data)")
	}
	return strconv.ParseFloat(val, 64)
}

// BuildMacroEvents merges FRED CPI series observations with release dates
// to produce a slice of MacroEvent structs.
//
// Algorithm:
//  1. Parse all observation dates and values, skipping any with missing data.
//  2. Sort observations by date (they should already be sorted, but we enforce it).
//  3. For each release date, use binary search to find the latest observation
//     whose date falls on or before the release date — this is the "Actual" CPI
//     value being announced on that day.
//  4. The observation immediately before that becomes the "Previous" value.
//  5. Compute the surprise (Actual - Previous, since we don't have consensus data).
func BuildMacroEvents(series *FredSeriesResponse, releases *FredReleaseResponse) ([]models.MacroEvent, error) {
	if series == nil || releases == nil {
		return nil, fmt.Errorf("series and releases must not be nil")
	}

	layout := "2006-01-02"

	// --- Step 1: Parse observations into a usable form ---
	type parsedObs struct {
		Date  time.Time
		Value float64
	}

	var observations []parsedObs
	for _, obs := range series.Observations {
		date, err := time.Parse(layout, obs.Date)
		if err != nil {
			continue // skip unparseable dates
		}
		val, err := parseObservationValue(obs.Value)
		if err != nil {
			continue // skip missing values (".")
		}
		observations = append(observations, parsedObs{Date: date, Value: val})
	}

	// --- Step 2: Sort observations by date ---
	sort.Slice(observations, func(i, j int) bool {
		return observations[i].Date.Before(observations[j].Date)
	})

	if len(observations) < 2 {
		return nil, fmt.Errorf("need at least 2 valid observations, got %d", len(observations))
	}

	// --- Step 3-5: Match each release date to its observation ---
	var events []models.MacroEvent

	for _, rel := range releases.ReleaseDates {
		releaseDate, err := time.Parse(layout, rel.Date)
		if err != nil {
			continue // skip unparseable release dates
		}

		// Build the precise release timestamp (8:30 AM ET → UTC)
		releaseTimestamp, err := BuildReleaseTimestamp(rel.Date)
		if err != nil {
			continue
		}

		// Binary search: find the index of the first observation AFTER the release date.
		// The observation just before that index is the one being released.
		idx := sort.Search(len(observations), func(i int) bool {
			return observations[i].Date.After(releaseDate)
		})
		// idx points to the first obs after releaseDate, so idx-1 is the latest obs
		// on or before the release date (the "Actual" value being announced).

		if idx < 2 {
			continue // need at least one previous observation
		}

		actual := observations[idx-1].Value
		previous := observations[idx-2].Value

		event := models.MacroEvent{
			EventName:   "CPI Release",
			ReleaseDate: releaseTimestamp,
			Actual:      actual,
			Previous:    previous,
			Expected:    0, // No consensus data available — will be populated later
		}
		event.CalcSurprise() // computes Actual - Previous (fallback since Expected=0)

		events = append(events, event)
	}

	return events, nil
}

