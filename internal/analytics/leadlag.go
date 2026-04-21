package analytics

import (
	"fmt"
	"math"
	"macro-impact-tracker/internal/models"
	"strings"
	"time"
)

// ============================================================
// Cross-Asset Lead/Lag Analysis
// ============================================================
// This module calculates cross-correlations between asset returns
// at different time lag offsets around CPI releases.
//
// The key question: does one asset react before another?
//   - Does VIX spike 1 minute before SPY drops?
//   - Does EUR/USD move before the DXY proxy?
//   - Do short-term yields (DGS2) react before long-term (DGS10)?
//
// Method:
//   1. For each CPI event, extract minute-level close prices for
//      both assets in the post-event window (0 to +30 minutes).
//   2. Compute minute-by-minute returns for each asset.
//   3. Shift one series by lag offsets: -5, -4, ..., 0, ..., +4, +5 min.
//   4. Compute Pearson correlation at each lag.
//   5. Average the correlation across all events.
//
// A positive peak lag means Asset A leads Asset B by that many minutes.
// ============================================================

// LeadLagResult holds the cross-correlation at a specific lag for
// a pair of assets.
type LeadLagResult struct {
	AssetA      string  // First asset
	AssetB      string  // Second asset
	LagMinutes  int     // Lag in minutes (positive = A leads B)
	Correlation float64 // Pearson correlation at this lag
	NEvents     int     // Number of events used in averaging
}

// ComputeLeadLag calculates cross-correlations between two assets at
// different lag offsets, averaged across all events.
//
// Parameters:
//   - store:         the filled MarketDataStore
//   - events:        the macro events from Phase 1
//   - assetA:        first asset symbol
//   - assetB:        second asset symbol
//   - maxLagMinutes: maximum lag to test (e.g., 5 → tests -5 to +5)
//
// Returns a slice of LeadLagResult, one per lag offset.
func ComputeLeadLag(store *models.MarketDataStore, events []models.MacroEvent, assetA, assetB string, maxLagMinutes int) []LeadLagResult {
	var results []LeadLagResult

	// For each lag offset
	for lag := -maxLagMinutes; lag <= maxLagMinutes; lag++ {
		var correlations []float64

		// For each event, compute correlation at this lag
		for _, event := range events {
			corr := computeEventCorrelation(store, event, assetA, assetB, lag)
			if !math.IsNaN(corr) {
				correlations = append(correlations, corr)
			}
		}

		// Average correlations across events
		avgCorr := 0.0
		if len(correlations) > 0 {
			avgCorr = mean(correlations)
		}

		results = append(results, LeadLagResult{
			AssetA:      assetA,
			AssetB:      assetB,
			LagMinutes:  lag,
			Correlation: avgCorr,
			NEvents:     len(correlations),
		})
	}

	return results
}

// computeEventCorrelation computes the Pearson correlation between
// minute-by-minute returns of two assets around a single event,
// with a specific lag applied to asset A.
func computeEventCorrelation(store *models.MarketDataStore, event models.MacroEvent, assetA, assetB string, lagMinutes int) float64 {
	// Extract 30-minute post-event windows
	from := event.ReleaseDate
	to := event.ReleaseDate.Add(30 * time.Minute)

	seriesA := store.GetWindow(assetA, from, to)
	seriesB := store.GetWindow(assetB, from, to)

	if seriesA == nil || seriesB == nil || seriesA.Len() < 3 || seriesB.Len() < 3 {
		return math.NaN()
	}

	// Compute minute-by-minute returns
	returnsA := computeMinuteReturns(seriesA)
	returnsB := computeMinuteReturns(seriesB)

	// Build aligned return maps by timestamp
	mapA := make(map[time.Time]float64)
	for _, r := range returnsA {
		mapA[r.Timestamp] = r.Value
	}

	mapB := make(map[time.Time]float64)
	for _, r := range returnsB {
		mapB[r.Timestamp] = r.Value
	}

	// Build aligned series with lag applied to A
	var alignedA, alignedB []float64
	lagDuration := time.Duration(lagMinutes) * time.Minute

	for ts, retB := range mapB {
		// Look for A's return at (ts - lag)
		// If lag > 0: we look at A's earlier return → A leads B
		laggedTs := ts.Add(-lagDuration)
		retA, ok := mapA[laggedTs]
		if ok {
			alignedA = append(alignedA, retA)
			alignedB = append(alignedB, retB)
		}
	}

	if len(alignedA) < 3 {
		return math.NaN()
	}

	return pearsonCorrelation(alignedA, alignedB)
}

// minuteReturn is a timestamped return value.
type minuteReturn struct {
	Timestamp time.Time
	Value     float64
}

// computeMinuteReturns computes the percentage return between consecutive
// data points in a time series.
func computeMinuteReturns(series *models.AssetTimeSeries) []minuteReturn {
	if series.Len() < 2 {
		return nil
	}

	var returns []minuteReturn
	for i := 1; i < series.Len(); i++ {
		prev := series.DataPoints[i-1]
		curr := series.DataPoints[i]

		if prev.Close == 0 {
			continue
		}

		ret := CalculateReturn(prev.Close, curr.Close)
		returns = append(returns, minuteReturn{
			Timestamp: curr.Timestamp,
			Value:     ret,
		})
	}

	return returns
}

// pearsonCorrelation computes the Pearson product-moment correlation
// coefficient between two equal-length slices.
//
// Formula: r = Σ((xi-x̄)(yi-ȳ)) / sqrt(Σ(xi-x̄)² × Σ(yi-ȳ)²)
func pearsonCorrelation(x, y []float64) float64 {
	if len(x) != len(y) || len(x) < 2 {
		return math.NaN()
	}

	xMean := mean(x)
	yMean := mean(y)

	var sumXYDev float64
	var sumXXDev float64
	var sumYYDev float64

	for i := 0; i < len(x); i++ {
		xDev := x[i] - xMean
		yDev := y[i] - yMean
		sumXYDev += xDev * yDev
		sumXXDev += xDev * xDev
		sumYYDev += yDev * yDev
	}

	denom := math.Sqrt(sumXXDev * sumYYDev)
	if denom == 0 {
		return 0
	}

	return sumXYDev / denom
}

// ComputeAllLeadLag runs lead/lag analysis for all unique pairs of
// intraday assets in the store.
func ComputeAllLeadLag(store *models.MarketDataStore, events []models.MacroEvent, assets []string, maxLagMinutes int) []LeadLagResult {
	var allResults []LeadLagResult

	for i := 0; i < len(assets); i++ {
		for j := i + 1; j < len(assets); j++ {
			results := ComputeLeadLag(store, events, assets[i], assets[j], maxLagMinutes)
			allResults = append(allResults, results...)
		}
	}

	return allResults
}

// LeadLagSummary returns a formatted summary of lead/lag results.
func LeadLagSummary(results []LeadLagResult) string {
	if len(results) == 0 {
		return "Lead/Lag Analysis: (no data)\n"
	}

	var sb strings.Builder
	sb.WriteString("═══════════════════════════════════════════════════════════════\n")
	sb.WriteString("              CROSS-ASSET LEAD/LAG ANALYSIS\n")
	sb.WriteString("═══════════════════════════════════════════════════════════════\n\n")

	// Group by asset pair
	type pairKey struct{ A, B string }
	pairResults := make(map[pairKey][]LeadLagResult)
	var pairOrder []pairKey

	for _, r := range results {
		key := pairKey{r.AssetA, r.AssetB}
		if _, exists := pairResults[key]; !exists {
			pairOrder = append(pairOrder, key)
		}
		pairResults[key] = append(pairResults[key], r)
	}

	for _, pair := range pairOrder {
		pairRes := pairResults[pair]
		sb.WriteString(fmt.Sprintf("┌─── %s vs %s ───────────────────────────────\n", pair.A, pair.B))
		sb.WriteString(fmt.Sprintf("│ %-8s │ %12s │ %8s\n", "Lag(min)", "Correlation", "N events"))
		sb.WriteString("│" + strings.Repeat("─", 10) + "│" + strings.Repeat("─", 14) + "│" + strings.Repeat("─", 10) + "\n")

		bestLag := 0
		bestCorr := 0.0

		for _, r := range pairRes {
			bar := buildCorrBar(r.Correlation)
			sb.WriteString(fmt.Sprintf("│ %+5d    │ %+11.4f  │ %5d    %s\n",
				r.LagMinutes, r.Correlation, r.NEvents, bar))

			if math.Abs(r.Correlation) > math.Abs(bestCorr) {
				bestCorr = r.Correlation
				bestLag = r.LagMinutes
			}
		}

		sb.WriteString("│\n")
		if bestLag > 0 {
			sb.WriteString(fmt.Sprintf("│ → Peak at lag=%+d: %s leads %s by %d min (r=%.4f)\n",
				bestLag, pair.A, pair.B, bestLag, bestCorr))
		} else if bestLag < 0 {
			sb.WriteString(fmt.Sprintf("│ → Peak at lag=%+d: %s leads %s by %d min (r=%.4f)\n",
				bestLag, pair.B, pair.A, -bestLag, bestCorr))
		} else {
			sb.WriteString(fmt.Sprintf("│ → Peak at lag=0: simultaneous movement (r=%.4f)\n", bestCorr))
		}
		sb.WriteString("└" + strings.Repeat("─", 50) + "\n\n")
	}

	return sb.String()
}

// buildCorrBar creates a visual bar for the correlation value.
func buildCorrBar(corr float64) string {
	// Scale to a bar of max 20 chars
	width := int(math.Abs(corr) * 20)
	if width > 20 {
		width = 20
	}

	bar := strings.Repeat("█", width)
	if corr >= 0 {
		return "│ " + bar
	}
	return "│" + strings.Repeat(" ", 20-width) + bar
}
