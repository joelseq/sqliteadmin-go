package sqliteadmin

import (
	"errors"
	"fmt"
	"net/http"
)

var (
	ErrMissingTableName    = errors.New("missing table name")
	ErrMissingRow          = errors.New("missing row")
	ErrInvalidOrMissingIds = errors.New("invalid or missing ids")
	ErrInvalidInput        = errors.New("invalid input")
)

type ApiError struct {
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
}

func (e ApiError) Error() string {
	return fmt.Sprintf("api error: %d,  %s", e.StatusCode, e.Message)
}

func apiErrUnauthorized() ApiError {
	return ApiError{StatusCode: http.StatusUnauthorized, Message: "Invalid credentials"}
}

func apiErrBadRequest(details string) ApiError {
	return ApiError{StatusCode: http.StatusBadRequest, Message: "Bad request: " + details}
}

func apiErrSomethingWentWrong() ApiError {
	return ApiError{StatusCode: http.StatusInternalServerError, Message: "Something went wrong"}
}
