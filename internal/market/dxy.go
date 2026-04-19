package market

import (
	"fmt"
	"macro-impact-tracker/internal/models"
)

// ============================================================
// DXY Proxy Calculation from EUR/USD
// ============================================================
// The US Dollar Index (DXY) measures the dollar against a basket of
// 6 major currencies. The EUR/USD pair has the largest weight (~57.6%).
//
// DXY is not freely available as a direct ticker on most free-tier APIs.
// As a practical approximation, we compute:
//
//     DXY_proxy ≈ 1 / EUR_USD_close
//
// This inverts the EUR/USD price to get a dollar-strength signal.
// When EUR/USD goes down (dollar strengthens), our proxy goes up — exactly
// how the real DXY behaves.
//
// Important limitations:
//   - This is NOT the real DXY (which includes JPY, GBP, CAD, SEK, CHF)
//   - It only captures the EUR component (~57.6% of real DXY)
//   - Good enough for directional analysis in CPI event studies
// ============================================================

// ComputeDXYFromEURUSD takes an EUR/USD time series and returns a new
// time series representing the DXY proxy (inverted EUR/USD).
//
// Each data point's OHLCV is inverted:
//   - DXY.Open  = 1 / EURUSD.Open
//   - DXY.High  = 1 / EURUSD.Low   (inverted: EUR/USD low = dollar high)
//   - DXY.Low   = 1 / EURUSD.High  (inverted: EUR/USD high = dollar low)
//   - DXY.Close = 1 / EURUSD.Close
//   - DXY.Volume = EURUSD.Volume    (preserved as-is)
func ComputeDXYFromEURUSD(eurusd *models.AssetTimeSeries) (*models.AssetTimeSeries, error) {
	if eurusd == nil {
		return nil, fmt.Errorf("EUR/USD time series is nil")
	}
	if len(eurusd.DataPoints) == 0 {
		return nil, fmt.Errorf("EUR/USD time series has no data points")
	}

	var dxyPoints []models.MarketDataPoint

	for _, dp := range eurusd.DataPoints {
		// Guard against division by zero (shouldn't happen for real EUR/USD)
		if dp.Open == 0 || dp.High == 0 || dp.Low == 0 || dp.Close == 0 {
			continue
		}

		dxyPoints = append(dxyPoints, models.MarketDataPoint{
			Timestamp: dp.Timestamp,
			Open:      1.0 / dp.Open,
			High:      1.0 / dp.Low,   // note the swap: low EUR → high USD
			Low:       1.0 / dp.High,  // high EUR → low USD
			Close:     1.0 / dp.Close,
			Volume:    dp.Volume,
		})
	}

	return &models.AssetTimeSeries{
		Symbol:     "DXY (proxy)",
		Interval:   eurusd.Interval,
		DataPoints: dxyPoints,
	}, nil
}
