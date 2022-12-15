package api

import "encoding/json"

const emptyString = ""

func CommonResp(code int, message string, data interface{}) interface{} {
	r := map[string]interface{}{
		"code":    code,
		"message": message,
		"data":    data,
	}

	if rsp, err := json.Marshal(r); err != nil {
		return nil
	} else {
		return string(rsp)
	}
}
