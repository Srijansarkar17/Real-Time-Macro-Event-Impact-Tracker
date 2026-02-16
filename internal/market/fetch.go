package market

import (
	"fmt"
	"sync"
)

var wg sync.WaitGroup

func FetchMarketData() { //Fetches market data, For multiple assets, At the same time (concurrently), And waits until all are done
	assets := []string{"SPY", "EURUSD", "VIX"} //SPY → S&P 500 ETF, EURUSD → Forex pair, VIX → Volatility index

	for _, asset := range assets {
		wg.Add(1) //increases the wait counter, means one new task is starting

		go func(a string) { //go keyword means: Run this function concurrently (in parallel). so instead of SPY → wait → EURUSD → wait → VIX , it becomes SPY EURUSD VIX (all running at same time)
			defer wg.Done()                       //When the goroutine finishes: it decreases the counter by 1
			fmt.Println("Fetching data for: ", a) //Just prints the asset name.
		}(asset) //uses the concept of variable capture
	}
	wg.Wait() //this blocks the main thread to exit until wait counter == 0
}
