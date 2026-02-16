package macro

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func FetchSampleCPI() error { //If everything works → returns nil, If something fails → returns the error

	//resp contains response from API, err contains error if something failed
	resp, err := http.Get("https://api.sampleapis.com/fakebank/accounts")

	if err != nil {
		return err
	}

	defer resp.Body.Close() // When you open something (like a network connection), you must close it. defer means run this when function ends

	var data interface{} //This creates a variable called data, interface{} means:“It can store anything.”

	json.NewDecoder(resp.Body).Decode(&data)
	fmt.Println("Fetched macro data successfully")
	return nil
}
