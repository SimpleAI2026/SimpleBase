package handler

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
)

func newNoAuthResp() DataResp {
	return DataResp{
		Code:    -999,
		Message: "No Auth",
	}
}

func newNoWriteAuthResp() DataResp {
	return DataResp{
		Code:    -998,
		Message: "No Write Auth",
	}
}

func newOKResp(data interface{}) DataResp {
	return DataResp{
		Code:    0,
		Message: "OK",
		Data:    data,
	}
}

func newFailResp(code int, message string) DataResp {
	return DataResp{
		Code:    code,
		Message: message,
	}
}

func httpRequest(url string, req interface{}, resp interface{}) (err error) {
	var data *bytes.Buffer
	var method = "GET"
	if req != nil {
		jsonData, err := json.Marshal(req)
		if err != nil {
			return err
		}
		data = bytes.NewBuffer(jsonData)
		method = "POST"
	}

	httpReq, err := http.NewRequest(method, url, data)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()

	body, err := ioutil.ReadAll(httpResp.Body)
	if err != nil {
		return err
	}

	if err = json.Unmarshal(body, resp); err != nil {
		return
	}

	return nil
}
