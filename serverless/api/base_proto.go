package api

type BaseRequest struct {
}

type BaseResponse struct {
	ErrCode    int    `json:"err_code"`
	ErrMessage string `json:"err_message"`
}
