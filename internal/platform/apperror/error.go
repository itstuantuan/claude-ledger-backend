package apperror

import "net/http"

type Error struct {
	Code        string
	Message     string
	HTTPStatus  int
	FieldErrors map[string][]string
	Cause       error
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }
func (e *Error) Unwrap() error { return e.Cause }

func New(status int, code, message string) *Error {
	return &Error{Code: code, Message: message, HTTPStatus: status}
}

func Validation(fields map[string][]string) *Error {
	return &Error{Code: "VALIDATION_ERROR", Message: "请检查填写的内容。", HTTPStatus: http.StatusUnprocessableEntity, FieldErrors: fields}
}
