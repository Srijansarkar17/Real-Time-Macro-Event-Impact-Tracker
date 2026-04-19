package models

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// MarketDataPoint represents a single OHLCV candle (one row of price data).
// For daily data like Treasury yields, Open/High/Low/Close are all the same value
// and Volume is 0.
type MarketDataPoint struct {
	Timestamp time.Time // UTC timestamp of this candle
	Open      float64   // Opening price at the start of the interval
	High      float64   // Highest price during the interval
	Low       float64   // Lowest price during the interval
	Close     float64   // Closing price at the end of the interval
	Volume    int64     // Number of shares/contracts traded (0 for yields)
}

// String returns a human-readable representation of a single data point.
func (p MarketDataPoint) String() string {
	return fmt.Sprintf("%s | O:%.4f H:%.4f L:%.4f C:%.4f V:%d",
		p.Timestamp.Format("2006-01-02 15:04"), p.Open, p.High, p.Low, p.Close, p.Volume)
}

// AssetTimeSeries holds all the candles for a single asset within a time window.
// Think of it as a mini price chart for one symbol.
type AssetTimeSeries struct {
	Symbol     string            // e.g., "SPY", "EUR/USD", "DGS10"
	Interval   string            // e.g., "1min", "1day"
	DataPoints []MarketDataPoint // sorted chronologically (oldest first)
}

// Len returns the number of data points in this time series.
func (ts *AssetTimeSeries) Len() int {
	return len(ts.DataPoints)
}

// SortByTime ensures data points are sorted oldest → newest.
func (ts *AssetTimeSeries) SortByTime() {
	sort.Slice(ts.DataPoints, func(i, j int) bool {
		return ts.DataPoints[i].Timestamp.Before(ts.DataPoints[j].Timestamp)
	})
}

// Window returns a new AssetTimeSeries containing only data points
// between 'from' and 'to' (inclusive). Useful for extracting the price
// action around a specific CPI release.
func (ts *AssetTimeSeries) Window(from, to time.Time) *AssetTimeSeries {
	var filtered []MarketDataPoint
	for _, dp := range ts.DataPoints {
		if (dp.Timestamp.Equal(from) || dp.Timestamp.After(from)) &&
			(dp.Timestamp.Equal(to) || dp.Timestamp.Before(to)) {
			filtered = append(filtered, dp)
		}
	}
	return &AssetTimeSeries{
		Symbol:     ts.Symbol,
		Interval:   ts.Interval,
		DataPoints: filtered,
	}
}

// String returns a summary of the time series.
func (ts *AssetTimeSeries) String() string {
	if len(ts.DataPoints) == 0 {
		return fmt.Sprintf("[%s] (no data)", ts.Symbol)
	}
	first := ts.DataPoints[0].Timestamp.Format("2006-01-02 15:04")
	last := ts.DataPoints[len(ts.DataPoints)-1].Timestamp.Format("2006-01-02 15:04")
	return fmt.Sprintf("[%s] %d points | %s → %s", ts.Symbol, len(ts.DataPoints), first, last)
}

// MarketDataStore is a thread-safe in-memory store for market data.
// It maps symbol names to their time-series data.
// Thread-safety is needed because we fetch multiple assets concurrently
// (goroutines) and they all write to this shared store.
type MarketDataStore struct {
	mu   sync.RWMutex                // protects concurrent reads/writes to the map
	Data map[string]*AssetTimeSeries // symbol → time series
}

// NewMarketDataStore creates an initialized, empty store.
func NewMarketDataStore() *MarketDataStore {
	return &MarketDataStore{
		Data: make(map[string]*AssetTimeSeries),
	}
}

// Add inserts or replaces the time series for a given symbol.
// If data already exists for this symbol, the new data points are appended
// and the series is re-sorted.
func (s *MarketDataStore) Add(symbol string, ts *AssetTimeSeries) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.Data[symbol]
	if ok {
		// Merge: append new data points to existing series
		existing.DataPoints = append(existing.DataPoints, ts.DataPoints...)
		existing.SortByTime()
	} else {
		ts.SortByTime()
		s.Data[symbol] = ts
	}
}

// Get retrieves the full time series for a symbol. Returns nil if not found.
func (s *MarketDataStore) Get(symbol string) *AssetTimeSeries {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.Data[symbol]
}

// GetWindow retrieves a time-windowed slice of data for a symbol.
// Returns nil if the symbol is not in the store.
func (s *MarketDataStore) GetWindow(symbol string, from, to time.Time) *AssetTimeSeries {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ts, ok := s.Data[symbol]
	if !ok {
		return nil
	}
	return ts.Window(from, to)
}

// Symbols returns all symbol names currently in the store.
func (s *MarketDataStore) Symbols() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	symbols := make([]string, 0, len(s.Data))
	for sym := range s.Data {
		symbols = append(symbols, sym)
	}
	sort.Strings(symbols) // deterministic ordering
	return symbols
}

// Summary prints a human-readable summary of all stored data.
func (s *MarketDataStore) Summary() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := fmt.Sprintf("MarketDataStore: %d assets\n", len(s.Data))
	for _, sym := range s.Symbols() {
		ts := s.Data[sym]
		result += fmt.Sprintf("  %s\n", ts.String())
	}
	return result
}
