package handler

import (
	"github.com/labstack/echo/v4"

	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"errors"
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

func newBadRequestResp(c echo.Context, err interface{})	error {
	switch err.(type) {
		case string:
			return c.String(http.StatusBadRequest, err.(string))
		case error:
			return c.String(http.StatusBadRequest, err.(error).Error())
	}

	return c.String(http.StatusBadRequest, 	"bad request")
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

	if httpResp.StatusCode != http.StatusOK {
		return errors.New(string(body))
	}

	if err = json.Unmarshal(body, resp); err != nil {
		return err
	}

	return nil
}