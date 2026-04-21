package analytics

import (
	"fmt"
	"math"
	"strings"
)

// ============================================================
// Surprise Sensitivity Model — OLS Linear Regression
// ============================================================
// For each asset × window combination, we run:
//
//   Return = α + β × Surprise + ε
//
// Where:
//   - Return  = percentage return of the asset in the window
//   - Surprise = Actual CPI - Expected CPI (or Actual - Previous)
//   - α (alpha) = intercept (baseline return unrelated to surprise)
//   - β (beta)  = sensitivity coefficient (how much 1 unit of surprise
//                 moves the asset)
//   - ε (epsilon) = residual error
//
// A large positive β for DXY means the dollar strengthens on hot CPI.
// A large negative β for SPY means stocks fall on hot CPI.
//
// All math is implemented from scratch — no external dependencies.
// ============================================================

// SensitivityResult holds the output of an OLS regression for one
// asset × window combination.
type SensitivityResult struct {
	Asset      string  // Asset symbol
	Window     string  // Window name
	Alpha      float64 // Intercept (baseline return)
	Beta       float64 // Sensitivity coefficient
	RSquared   float64 // Goodness of fit (0–1)
	N          int     // Number of observations
	StdErr     float64 // Standard error of beta
	TStatistic float64 // t-statistic for H0: β = 0
}

// ComputeSensitivity runs OLS regression for every asset × window pair
// in the return matrix. Returns a slice of SensitivityResult.
//
// A minimum of 3 observations is required for meaningful regression.
// Asset/window pairs with fewer observations are skipped.
func ComputeSensitivity(matrix *ReturnMatrix) []SensitivityResult {
	var results []SensitivityResult

	for _, asset := range matrix.Assets {
		for _, window := range matrix.Windows {
			returns := matrix.GetReturnsByAssetWindow(asset, window)

			// Need at least 3 observations for OLS with 2 parameters
			if len(returns) < 3 {
				continue
			}

			// Extract surprise (x) and return (y) values
			x := make([]float64, len(returns))
			y := make([]float64, len(returns))
			for i, r := range returns {
				x[i] = r.Surprise
				y[i] = r.Return
			}

			result := olsRegression(x, y)
			result.Asset = asset
			result.Window = window

			results = append(results, result)
		}
	}

	return results
}

// olsRegression performs ordinary least squares regression of y on x.
// Returns the fitted model parameters.
//
// Formulas:
//
//	β = Σ((xi - x̄)(yi - ȳ)) / Σ((xi - x̄)²)
//	α = ȳ - β × x̄
//	R² = 1 - SS_res / SS_tot
//	SE(β) = sqrt(SS_res / ((n-2) × Σ(xi - x̄)²))
//	t = β / SE(β)
func olsRegression(x, y []float64) SensitivityResult {
	n := len(x)

	// Compute means
	xMean := mean(x)
	yMean := mean(y)

	// Compute sums for β
	var sumXYDev float64 // Σ((xi - x̄)(yi - ȳ))
	var sumXXDev float64 // Σ((xi - x̄)²)

	for i := 0; i < n; i++ {
		xDev := x[i] - xMean
		yDev := y[i] - yMean
		sumXYDev += xDev * yDev
		sumXXDev += xDev * xDev
	}

	// Handle zero variance in x (all surprises identical)
	if sumXXDev == 0 {
		return SensitivityResult{
			Alpha:    yMean,
			Beta:     0,
			RSquared: 0,
			N:        n,
			StdErr:   math.NaN(),
			TStatistic: 0,
		}
	}

	// β = Cov(X,Y) / Var(X)
	beta := sumXYDev / sumXXDev

	// α = ȳ - β × x̄
	alpha := yMean - beta*xMean

	// Compute R²
	var ssRes float64 // Sum of squared residuals
	var ssTot float64 // Total sum of squares

	for i := 0; i < n; i++ {
		predicted := alpha + beta*x[i]
		residual := y[i] - predicted
		ssRes += residual * residual
		ssTot += (y[i] - yMean) * (y[i] - yMean)
	}

	rSquared := 0.0
	if ssTot > 0 {
		rSquared = 1.0 - ssRes/ssTot
	}

	// Standard error of β
	stdErr := 0.0
	tStat := 0.0
	if n > 2 {
		mse := ssRes / float64(n-2)
		stdErr = math.Sqrt(mse / sumXXDev)
		if stdErr > 0 {
			tStat = beta / stdErr
		}
	}

	return SensitivityResult{
		Alpha:      alpha,
		Beta:       beta,
		RSquared:   rSquared,
		N:          n,
		StdErr:     stdErr,
		TStatistic: tStat,
	}
}

// mean computes the arithmetic mean of a float64 slice.
func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// SensitivitySummary returns a formatted table of all sensitivity results.
func SensitivitySummary(results []SensitivityResult) string {
	if len(results) == 0 {
		return "Sensitivity Analysis: (insufficient data)\n"
	}

	var sb strings.Builder
	sb.WriteString("═══════════════════════════════════════════════════════════════════════════\n")
	sb.WriteString("                    SURPRISE SENSITIVITY MODEL\n")
	sb.WriteString("               Return = α + β × Surprise + ε\n")
	sb.WriteString("═══════════════════════════════════════════════════════════════════════════\n\n")

	sb.WriteString(fmt.Sprintf("%-14s %-10s │ %8s │ %10s │ %7s │ %3s │ %8s │ %7s\n",
		"Asset", "Window", "α", "β", "R²", "N", "SE(β)", "t-stat"))
	sb.WriteString(strings.Repeat("─", 85) + "\n")

	for _, r := range results {
		significance := ""
		absT := math.Abs(r.TStatistic)
		if absT > 2.576 {
			significance = "***" // p < 0.01
		} else if absT > 1.960 {
			significance = "**" // p < 0.05
		} else if absT > 1.645 {
			significance = "*" // p < 0.10
		}

		sb.WriteString(fmt.Sprintf("%-14s %-10s │ %+8.5f │ %+10.5f │ %7.4f │ %3d │ %8.5f │ %+7.3f %s\n",
			r.Asset, r.Window, r.Alpha, r.Beta, r.RSquared, r.N, r.StdErr, r.TStatistic, significance))
	}

	sb.WriteString("\n")
	sb.WriteString("Significance: * p<0.10, ** p<0.05, *** p<0.01\n")
	sb.WriteString("Interpretation: β > 0 means asset rises on positive CPI surprise (hot inflation)\n")
	sb.WriteString("                β < 0 means asset falls on positive CPI surprise\n")

	return sb.String()
}
