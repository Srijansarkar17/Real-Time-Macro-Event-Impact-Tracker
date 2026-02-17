package macro

type FredObservation struct {
	Date  string `json:"date"`
	Value string `json:"value"`
}

type FredResponse struct {
	Observations []FredObservation `json:"observations"` //we store the observations as a map that will store all the FredObservation struct data
	//Example:
	//{
	//"observations": [
	//{"date": "2024-01-01", "value": "3.1"},
	//]
	//}
}
