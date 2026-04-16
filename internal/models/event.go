package models

// MacroEvent represents a single macroeconomic data release (e.g., CPI)
// along with the values needed to compute market "surprise" impact.

import (
	"fmt"
	"time"
)

type MacroEvent struct {
	EventName   string
	ReleaseDate time.Time // Exact timestamp of the public release (8:30 AM ET → UTC)
	Actual      float64   // The reported CPI value for this release
	Previous    float64   // The CPI value from the prior release
	Expected    float64   // Consensus forecast (0 = unknown/unavailable)
	Surprise    float64   // Actual - Expected (or Actual - Previous if Expected is 0)
}

// CalcSurprise computes the surprise component.
// If an Expected (consensus) value is available, surprise = Actual - Expected.
// Otherwise it falls back to Actual - Previous (month-over-month change).
func (e *MacroEvent) CalcSurprise() {
	if e.Expected != 0 {
		e.Surprise = e.Actual - e.Expected
	} else {
		e.Surprise = e.Actual - e.Previous
	}
}

// String returns a human-readable summary of the event.
func (e MacroEvent) String() string {
	return fmt.Sprintf("[%s] %s | Actual: %.2f | Previous: %.2f | Expected: %.2f | Surprise: %.4f",
		e.ReleaseDate.Format("2006-01-02 15:04 UTC"), e.EventName, e.Actual, e.Previous, e.Expected, e.Surprise)
}
