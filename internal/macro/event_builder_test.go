package macro

import (
	"testing"
	"time"
)

// ========================================================================
// Tests for BuildReleaseTimestamp
// ========================================================================

func TestBuildReleaseTimestamp_ValidDate(t *testing.T) {
	// CPI is released at 8:30 AM ET.
	// During EST (Nov-Mar): ET = UTC-5, so 8:30 AM ET = 13:30 UTC
	// During EDT (Mar-Nov): ET = UTC-4, so 8:30 AM ET = 12:30 UTC

	tests := []struct {
		name     string
		input    string
		wantHour int // expected UTC hour
		wantMin  int // expected UTC minute
	}{
		{
			name:     "Winter date (EST, UTC-5)",
			input:    "2026-01-15",
			wantHour: 13,
			wantMin:  30,
		},
		{
			name:     "Summer date (EDT, UTC-4)",
			input:    "2026-07-15",
			wantHour: 12,
			wantMin:  30,
		},
		{
			name:     "February release",
			input:    "2026-02-18",
			wantHour: 13,
			wantMin:  30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := BuildReleaseTimestamp(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Hour() != tt.wantHour {
				t.Errorf("hour = %d, want %d", result.Hour(), tt.wantHour)
			}
			if result.Minute() != tt.wantMin {
				t.Errorf("minute = %d, want %d", result.Minute(), tt.wantMin)
			}
			if result.Location() != time.UTC {
				t.Errorf("expected UTC timezone, got %v", result.Location())
			}
		})
	}
}

func TestBuildReleaseTimestamp_InvalidDate(t *testing.T) {
	_, err := BuildReleaseTimestamp("not-a-date")
	if err == nil {
		t.Fatal("expected error for invalid date, got nil")
	}
}

func TestBuildReleaseTimestamp_PreservesDate(t *testing.T) {
	result, err := BuildReleaseTimestamp("2025-12-10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 2025-12-10 is in EST (UTC-5), so 8:30 AM EST = 13:30 UTC same day
	if result.Year() != 2025 || result.Month() != time.December || result.Day() != 10 {
		t.Errorf("date mismatch: got %v", result)
	}
}

// ========================================================================
// Tests for parseObservationValue
// ========================================================================

func TestParseObservationValue_Valid(t *testing.T) {
	val, err := parseObservationValue("314.175")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 314.175 {
		t.Errorf("got %f, want 314.175", val)
	}
}

func TestParseObservationValue_MissingDot(t *testing.T) {
	_, err := parseObservationValue(".")
	if err == nil {
		t.Fatal("expected error for '.' value, got nil")
	}
}

func TestParseObservationValue_Integer(t *testing.T) {
	val, err := parseObservationValue("100")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 100.0 {
		t.Errorf("got %f, want 100.0", val)
	}
}

// ========================================================================
// Tests for BuildMacroEvents
// ========================================================================

func TestBuildMacroEvents_BasicMerge(t *testing.T) {
	// Simulate 3 monthly CPI observations
	series := &FredSeriesResponse{
		Observations: []FredObservation{
			{Date: "2025-10-01", Value: "310.0"},
			{Date: "2025-11-01", Value: "312.5"},
			{Date: "2025-12-01", Value: "315.0"},
		},
	}

	// Simulate 2 release dates:
	// - Nov 13 release announces the Oct observation (310.0)  — but we need idx >= 2
	// - Dec 11 release announces the Nov observation (312.5)
	// - Jan 14 release announces the Dec observation (315.0)
	releases := &FredReleaseResponse{
		ReleaseDates: []FredReleaseDate{
			{Date: "2025-12-11"}, // latest obs on/before this = Dec 1 (315.0), prev = Nov 1 (312.5)
			{Date: "2026-01-14"}, // latest obs on/before this = Dec 1 (315.0), prev = Nov 1 (312.5)
		},
	}

	events, err := BuildMacroEvents(series, releases)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("expected at least 1 event, got 0")
	}

	// Check the first event: release on Dec 11
	// Latest obs on or before Dec 11 = Dec 1 (315.0), previous = Nov 1 (312.5)
	e := events[0]
	if e.EventName != "CPI Release" {
		t.Errorf("EventName = %q, want %q", e.EventName, "CPI Release")
	}
	if e.Actual != 315.0 {
		t.Errorf("Actual = %f, want 315.0", e.Actual)
	}
	if e.Previous != 312.5 {
		t.Errorf("Previous = %f, want 312.5", e.Previous)
	}
	// Surprise = Actual - Previous = 315.0 - 312.5 = 2.5
	expectedSurprise := 2.5
	if e.Surprise != expectedSurprise {
		t.Errorf("Surprise = %f, want %f", e.Surprise, expectedSurprise)
	}
}

func TestBuildMacroEvents_SkipsMissingValues(t *testing.T) {
	series := &FredSeriesResponse{
		Observations: []FredObservation{
			{Date: "2025-09-01", Value: "308.0"},
			{Date: "2025-10-01", Value: "."},      // missing — should be skipped
			{Date: "2025-11-01", Value: "312.5"},
		},
	}
	releases := &FredReleaseResponse{
		ReleaseDates: []FredReleaseDate{
			{Date: "2025-12-11"},
		},
	}

	events, err := BuildMacroEvents(series, releases)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	// With the "." skipped, the sorted valid observations are:
	// [0] Sep 1 → 308.0, [1] Nov 1 → 312.5
	// Release Dec 11: latest on/before = Nov 1 (312.5), prev = Sep 1 (308.0)
	e := events[0]
	if e.Actual != 312.5 {
		t.Errorf("Actual = %f, want 312.5", e.Actual)
	}
	if e.Previous != 308.0 {
		t.Errorf("Previous = %f, want 308.0", e.Previous)
	}
}

func TestBuildMacroEvents_NilInputs(t *testing.T) {
	_, err := BuildMacroEvents(nil, nil)
	if err == nil {
		t.Fatal("expected error for nil inputs, got nil")
	}
}

func TestBuildMacroEvents_InsufficientObservations(t *testing.T) {
	series := &FredSeriesResponse{
		Observations: []FredObservation{
			{Date: "2025-10-01", Value: "310.0"},
		},
	}
	releases := &FredReleaseResponse{
		ReleaseDates: []FredReleaseDate{
			{Date: "2025-11-13"},
		},
	}

	_, err := BuildMacroEvents(series, releases)
	if err == nil {
		t.Fatal("expected error for insufficient observations, got nil")
	}
}

func TestBuildMacroEvents_ReleaseTimestampIsCorrect(t *testing.T) {
	series := &FredSeriesResponse{
		Observations: []FredObservation{
			{Date: "2025-10-01", Value: "310.0"},
			{Date: "2025-11-01", Value: "312.5"},
		},
	}
	releases := &FredReleaseResponse{
		ReleaseDates: []FredReleaseDate{
			{Date: "2025-12-11"}, // Winter → EST → 8:30 AM = 13:30 UTC
		},
	}

	events, err := BuildMacroEvents(series, releases)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	e := events[0]
	if e.ReleaseDate.Hour() != 13 || e.ReleaseDate.Minute() != 30 {
		t.Errorf("ReleaseDate time = %02d:%02d UTC, want 13:30 UTC",
			e.ReleaseDate.Hour(), e.ReleaseDate.Minute())
	}
}

func TestBuildMacroEvents_ExpectedDefaultsToZero(t *testing.T) {
	series := &FredSeriesResponse{
		Observations: []FredObservation{
			{Date: "2025-10-01", Value: "310.0"},
			{Date: "2025-11-01", Value: "312.5"},
		},
	}
	releases := &FredReleaseResponse{
		ReleaseDates: []FredReleaseDate{
			{Date: "2025-12-11"},
		},
	}

	events, err := BuildMacroEvents(series, releases)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if events[0].Expected != 0 {
		t.Errorf("Expected = %f, want 0 (no consensus data)", events[0].Expected)
	}
}
