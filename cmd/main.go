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

	err := macro.FetchSampleCPI()
	if err != nil {
		fmt.Println("Error: ", err)
	}

	market.FetchMarketData()
}
