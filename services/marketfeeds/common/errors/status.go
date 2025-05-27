package errors

import (
	"net/http"
)

type StatusCode int

// Error implements error
func (status StatusCode) Error() string {
	return http.StatusText(int(status))
}

func Status(code int) *Error {
	return Wrap(StatusCode(code)).Reason(http.StatusText(code))
}

var (
	Invalid       *Error = Status(http.StatusBadRequest)
	NotFound      *Error = Status(http.StatusNotFound)
	Conflict      *Error = Status(http.StatusConflict)
	BadGateway    *Error = Status(http.StatusBadGateway)
	Unavailable   *Error = Status(http.StatusServiceUnavailable)
	Unprocessable *Error = Status(http.StatusUnprocessableEntity)
)
