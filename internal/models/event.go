package models

// in this code, we fetch CPI data from a public api

import (
	"time"
)

type MacroEvent struct {
	Name      string
	TimeStamp time.Time
	Actual    float64
	Forecast  float64
	Previous  float64
}
