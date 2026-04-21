package analytics

import (
	"fmt"
	"sort"
	"strings"
)

// ============================================================
// Return Computation — Building the Return Matrix
// ============================================================
// This module computes percentage returns for each event window
// and organizes them into a structured return matrix.
//
// The return matrix is the core data structure for all downstream
// analytics (sensitivity model, lead/lag analysis). It captures:
//   [event × asset × window] → return value
//
// Each return is computed as: (window_close - window_open) / window_open
// using the existing CalculateReturn() function.
// ============================================================

// AssetReturn represents the computed return of one asset in one window
// for one macro event. This is a single cell in the return matrix.
type AssetReturn struct {
	EventIndex int     // Index of the macro event
	EventDate  string  // Human-readable date of the event
	Asset      string  // Asset symbol
	Window     string  // Window name (e.g., "post_30m")
	Return     float64 // Percentage return (e.g., 0.005 = 0.5%)
	Surprise   float64 // The macro event's surprise value
}

// ReturnMatrix is the complete 3D return matrix: [event × asset × window].
// It holds all computed returns and provides query/filter methods.
type ReturnMatrix struct {
	Returns []AssetReturn // All computed returns
	Assets  []string      // Unique asset symbols (sorted)
	Windows []string      // Unique window names (in order)
}

// ComputeReturns takes the extracted event windows and computes percentage
// returns for each, building the full return matrix.
//
// For each EventWindow with valid open/close prices, it computes:
//
//	return = (close - open) / open
//
// Windows with zero open price are skipped (division by zero guard).
func ComputeReturns(eventWindows []EventWindow) *ReturnMatrix {
	var returns []AssetReturn
	assetSet := make(map[string]bool)
	windowSet := make(map[string]bool)
	windowOrder := make(map[string]int)

	for _, ew := range eventWindows {
		// Skip windows where we can't compute a return
		if ew.OpenPrice == 0 {
			continue
		}

		ret := CalculateReturn(ew.OpenPrice, ew.ClosePrice)

		ar := AssetReturn{
			EventIndex: ew.EventIndex,
			EventDate:  ew.Event.ReleaseDate.Format("2006-01-02"),
			Asset:      ew.Asset,
			Window:     ew.Window.Name,
			Return:     ret,
			Surprise:   ew.Event.Surprise,
		}
		returns = append(returns, ar)

		assetSet[ew.Asset] = true
		if _, exists := windowSet[ew.Window.Name]; !exists {
			windowOrder[ew.Window.Name] = len(windowSet)
			windowSet[ew.Window.Name] = true
		}
	}

	// Sort asset names alphabetically
	assets := make([]string, 0, len(assetSet))
	for a := range assetSet {
		assets = append(assets, a)
	}
	sort.Strings(assets)

	// Sort windows by order of appearance
	windows := make([]string, len(windowSet))
	for w, idx := range windowOrder {
		windows[idx] = w
	}

	return &ReturnMatrix{
		Returns: returns,
		Assets:  assets,
		Windows: windows,
	}
}

// GetReturns filters the return matrix to returns matching the given
// asset and window name. Pass empty string to match all.
func (rm *ReturnMatrix) GetReturns(asset, window string) []AssetReturn {
	var filtered []AssetReturn
	for _, r := range rm.Returns {
		if asset != "" && r.Asset != asset {
			continue
		}
		if window != "" && r.Window != window {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}

// GetReturnsByAssetWindow returns returns for a specific asset and window,
// sorted by event index. This is the input for the sensitivity regression.
func (rm *ReturnMatrix) GetReturnsByAssetWindow(asset, window string) []AssetReturn {
	filtered := rm.GetReturns(asset, window)
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].EventIndex < filtered[j].EventIndex
	})
	return filtered
}

// Summary returns a formatted string table showing all returns.
// Format: rows = events, columns = assets, grouped by window.
func (rm *ReturnMatrix) Summary() string {
	if len(rm.Returns) == 0 {
		return "Return Matrix: (no data)\n"
	}

	var sb strings.Builder
	sb.WriteString("═══════════════════════════════════════════════════════\n")
	sb.WriteString("               RETURN MATRIX SUMMARY\n")
	sb.WriteString("═══════════════════════════════════════════════════════\n\n")

	// Group by window
	for _, window := range rm.Windows {
		sb.WriteString(fmt.Sprintf("┌─── Window: %s ───────────────────────────────────\n", window))

		// Header row with asset names
		sb.WriteString(fmt.Sprintf("│ %-12s │ %-8s │", "Date", "Surprise"))
		for _, asset := range rm.Assets {
			sb.WriteString(fmt.Sprintf(" %-12s │", asset))
		}
		sb.WriteString("\n")
		sb.WriteString("│" + strings.Repeat("─", 14) + "│" + strings.Repeat("─", 10) + "│")
		for range rm.Assets {
			sb.WriteString(strings.Repeat("─", 14) + "│")
		}
		sb.WriteString("\n")

		// Build a lookup: (eventDate, asset) → return
		lookup := make(map[string]float64)
		eventDates := make(map[string]float64) // date → surprise
		var orderedDates []string
		seenDates := make(map[string]bool)

		for _, r := range rm.GetReturns("", window) {
			key := r.EventDate + "|" + r.Asset
			lookup[key] = r.Return
			eventDates[r.EventDate] = r.Surprise
			if !seenDates[r.EventDate] {
				orderedDates = append(orderedDates, r.EventDate)
				seenDates[r.EventDate] = true
			}
		}
		sort.Strings(orderedDates)

		// Data rows
		for _, date := range orderedDates {
			surprise := eventDates[date]
			sb.WriteString(fmt.Sprintf("│ %-12s │ %+7.4f │", date, surprise))
			for _, asset := range rm.Assets {
				key := date + "|" + asset
				if ret, ok := lookup[key]; ok {
					sb.WriteString(fmt.Sprintf(" %+11.4f%% │", ret*100))
				} else {
					sb.WriteString(fmt.Sprintf(" %12s │", "—"))
				}
			}
			sb.WriteString("\n")
		}
		sb.WriteString("└" + strings.Repeat("─", 60) + "\n\n")
	}

	return sb.String()
}
