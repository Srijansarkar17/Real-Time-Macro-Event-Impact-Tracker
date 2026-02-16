package analytics

func CalculateReturn(before float64, after float64) float64 { //It calculates percentage return between two prices.
	//standard return formula -> return = (finalPrice - initialPrice)/ initialPrice
	return (after - before) / before
}
