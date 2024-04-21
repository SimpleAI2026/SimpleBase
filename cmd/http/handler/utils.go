package handler

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
		Data: data,
	}
}