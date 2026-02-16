package main

import (
	"fmt"
	"macro-impact-tracker/internal/macro"
	"macro-impact-tracker/internal/market"
)

func main() {
	fmt.Println("Macro Event Starting")

	err := macro.FetchSampleCPI()
	if err != nil {
		fmt.Println("Error: ", err)
	}

	market.FetchMarketData()
}
