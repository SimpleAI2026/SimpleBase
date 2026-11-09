//go:build ignore

package main

type GreetRequest struct {
	Name string `json:"name"`
}

type GreetResponse struct {
	Message string `json:"message"`
}

func Greet(req GreetRequest) GreetResponse {
	name := req.Name
	if name == "" {
		name = "world"
	}
	return GreetResponse{Message: "hello " + name}
}
