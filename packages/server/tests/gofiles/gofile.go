package tests

import (
	"encoding/json"

	"github.com/linkxzhou/gongx/packages/server"
)

func Handle(c server.Context) interface{} {
	coronaVirusJSON := `{
        "name" : "covid-11",
        "country" : "China",
        "city" : "Wuhan",
        "reason" : "Non vedge Food"
    }`
	var result map[string]interface{}
	json.Unmarshal([]byte(coronaVirusJSON), &result)
	result["RealIP"] = c.RealIP()
	result["Path"] = c.Path()
	return result
}
