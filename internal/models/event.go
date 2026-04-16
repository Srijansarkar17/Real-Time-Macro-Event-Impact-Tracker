package models

// in this code, we fetch CPI data from a public api

import (
	"time"
)

type MacroEvent struct {
	EventName   string
	ReleaseDate time.Time
	Actual      float64
	Previous    float64
}
