package types

type DIJobRequest struct {
	Replicas int `json:"replicas"`
}

type Object interface{}

type Response struct {
	Success bool   `json:"success"`
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    Object `json:"data"`
}

const (
	CodeSuccess = iota
	CodeFailed
)
