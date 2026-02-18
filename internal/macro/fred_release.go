package macro

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

type FredReleaseDate struct {
	Date string `json:"date"`
}

type FredReleaseResponse struct {
	ReleaseDates []FredReleaseDate `json:"release_dates"`
}

// Fetch CPI Release Dates
func FetchCPIReleaseDates() (*FredReleaseResponse, error) {
	apiKey := os.Getenv("FRED_API_KEY")

	url := fmt.Sprintf(
		"https://api.stlouisfed.org/fred/release/dates?release_id=9&api_key=%s&file_type=json",
		apiKey,
	)

	resp, err := http.Get(url)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	var data FredReleaseResponse
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return nil, err
	}
	return &data, nil

}
