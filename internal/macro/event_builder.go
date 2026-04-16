package macro

import "time"

//This code contains a helper function to convert Dates to Proper Timestamp

// This function takes a date string, sets the time to 8:30am in New York, converts that to UTC, Returns the final UTC timestamp
func BuildReleaseTimestamp(dateStr string) (time.Time, error) {
	layout := "2006-01-02" // This tells Go: "The input date string will look like:"

	date, err := time.Parse(layout, dateStr) //If dateStr = "2026-02-18", Then this creates: 2026-02-18 00:00:00 +0000 UTC

	if err != nil {
		return time.Time{}, err //time.Time{} = empty time (zero value)
	}

	loc, _ := time.LoadLocation("America/New_York") //This loads the timezone for New York.

	releaseTime := time.Date( //This creates a new time:
		date.Year(),  // Same year
		date.Month(), // Same month
		date.Day(),   // Same day
		8, 30, 0, 0,  // BUT time = 08:30:00
		loc, // Timezone = America/New_York
	)
	//So if input was: 2026-02-18, Now it becomes: 2026-02-18 08:30:00 America/New_York

	return releaseTime.UTC(), nil //This converts New York time → UTC.

}
