package macro

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

type FredObservation struct {
	Date  string `json:"date"`
	Value string `json:"value"`
}

type FredSeriesResponse struct {
	Observations []FredObservation `json:"observations"` //we store the observations as a map that will store all the FredObservation struct data
	//Example:
	//{
	//"observations": [
	//{"date": "2024-01-01", "value": "3.1"},
	//]
	//}
}

func FetchCPIObservations() (*FredSeriesResponse, error) { //*FredSeriesResponse → pointer to CPI data, error → in case something goes wrong
	apiKey := os.Getenv("FRED_API_KEY")

	url := fmt.Sprintf(
		"https://api.stlouisfed.org/fred/series/observations?series_id=CPIAUCSL&api_key=%s&file_type=json",
		apiKey,
	) //series_id=CPIAUCSL → CPI data, api_key=%s → inserts your API key, file_type=json → response will be JSON, fmt.Sprintf replaces %s with your API key.

	resp, err := http.Get(url)

	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //this means close the connection after function finishes

	var data FredSeriesResponse // So data will hold all CPI observations.
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}
