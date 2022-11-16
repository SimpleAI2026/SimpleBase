package tests

import (
	"encoding/json"
)

func Handle2() interface{} {
	coronaVirusJSON := `{
        "name" : "covid-11",
        "country" : "China",
        "city" : "Wuhan",
        "reason" : "Non vedge Food"
    }`
	var result map[string]interface{}
	json.Unmarshal([]byte(coronaVirusJSON), &result)
	return result
}
