package main

import (
	"fmt"
	"log"
	"macro-impact-tracker/internal/macro"
	"macro-impact-tracker/internal/market"

	"github.com/joho/godotenv"
)

func init() {
	err := godotenv.Load("configs/.env")
	if err != nil {
		log.Fatal("Error loading .env file")
	}
}
func main() {
	fmt.Println("Macro Event Starting")

	data, err := macro.FetchCPIObservations()
	if err != nil {
		panic(err)
	}
	fmt.Println("Number of CPI Observations:", len(data.Observations))

	for i := 0; i < 5 && i < len(data.Observations); i++ {
		fmt.Println(data.Observations[i])
	}

	market.FetchMarketData()
}
