package analytics

import (
	"fmt"
	"macro-impact-tracker/internal/models"
	"time"
)

// ============================================================
// Event Window Extraction
// ============================================================
// For each MacroEvent (CPI release), this module slices the market
// time-series into standardized analysis windows:
//
//   [-30min, 0]   — Pre-event positioning
//   [0, +30min]   — Immediate market reaction
//   [0, +2h]      — First stabilization period
//   [0, +1d]      — Full-day impact
//
// These windows follow the classic "event study" methodology from
// quantitative finance. Markets react fastest in the first 30 minutes,
// then stabilize over 2 hours, with residual effects over the full day.
// ============================================================

// WindowDef defines a named time window relative to a macro event's
// release timestamp. Offset is the start (relative to event time),
// End is the end (relative to event time).
type WindowDef struct {
	Name   string        // Human-readable name: "pre_30m", "post_30m", etc.
	Offset time.Duration // Start of window relative to event time (negative = before)
	End    time.Duration // End of window relative to event time
}

// EventWindow holds the extracted market data and prices for one
// combination of [event × asset × window].
type EventWindow struct {
	EventIndex int               // Index into the events slice
	Event      models.MacroEvent // The macro event (CPI release)
	Asset      string            // Asset symbol ("SPY", "DGS10", etc.)
	Window     WindowDef         // Which window this represents
	Data       *models.AssetTimeSeries // The sliced time-series data
	OpenPrice  float64           // Close price of the first candle in window
	ClosePrice float64           // Close price of the last candle in window
}

// DefaultWindows returns the four standard analysis windows used in
// event studies. These are the windows we extract for every event/asset.
func DefaultWindows() []WindowDef {
	return []WindowDef{
		{
			Name:   "pre_30m",
			Offset: -30 * time.Minute,
			End:    0,
		},
		{
			Name:   "post_30m",
			Offset: 0,
			End:    30 * time.Minute,
		},
		{
			Name:   "post_2h",
			Offset: 0,
			End:    2 * time.Hour,
		},
		{
			Name:   "post_1d",
			Offset: 0,
			End:    24 * time.Hour,
		},
	}
}

// ExtractEventWindows slices the market data store into EventWindow structs
// for every combination of [event × asset × window].
//
// Parameters:
//   - events: the macro events from Phase 1 (CPI releases)
//   - store:  the filled MarketDataStore from Phase 2
//   - windows: which time windows to extract (use DefaultWindows())
//
// Returns a slice of EventWindow structs. Windows with no data (e.g.,
// intraday windows for daily-only assets) are skipped.
func ExtractEventWindows(events []models.MacroEvent, store *models.MarketDataStore, windows []WindowDef) []EventWindow {
	var result []EventWindow

	symbols := store.Symbols()

	for eventIdx, event := range events {
		for _, symbol := range symbols {
			for _, window := range windows {
				// Compute the absolute time range for this window
				from := event.ReleaseDate.Add(window.Offset)
				to := event.ReleaseDate.Add(window.End)

				// Slice the market data for this symbol within the window
				windowed := store.GetWindow(symbol, from, to)
				if windowed == nil || windowed.Len() == 0 {
					// No data available for this window — skip
					// (e.g., daily Treasury yields don't have intraday data)
					continue
				}

				// Extract the open and close prices from the window
				// Open = first data point's Close price (the "before" price)
				// Close = last data point's Close price (the "after" price)
				openPrice := windowed.DataPoints[0].Close
				closePrice := windowed.DataPoints[windowed.Len()-1].Close

				ew := EventWindow{
					EventIndex: eventIdx,
					Event:      event,
					Asset:      symbol,
					Window:     window,
					Data:       windowed,
					OpenPrice:  openPrice,
					ClosePrice: closePrice,
				}
				result = append(result, ew)
			}
		}
	}

	return result
}

// String returns a human-readable summary of an EventWindow.
func (ew EventWindow) String() string {
	nPoints := 0
	if ew.Data != nil {
		nPoints = ew.Data.Len()
	}
	ret := 0.0
	if ew.OpenPrice != 0 {
		ret = CalculateReturn(ew.OpenPrice, ew.ClosePrice)
	}
	return fmt.Sprintf("[%s] %s | %s | %d pts | Open: %.4f → Close: %.4f | Return: %.4f%%",
		ew.Event.ReleaseDate.Format("2006-01-02"), ew.Asset, ew.Window.Name,
		nPoints, ew.OpenPrice, ew.ClosePrice, ret*100)
}
