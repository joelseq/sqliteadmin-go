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

type APIError struct {
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
}

func (e APIError) Error() string {
	return fmt.Sprintf("api error: %d,  %s", e.StatusCode, e.Message)
}

func apiErrUnauthorized() APIError {
	return APIError{StatusCode: http.StatusUnauthorized, Message: "Invalid credentials"}
}

func apiErrBadRequest(details string) APIError {
	return APIError{StatusCode: http.StatusBadRequest, Message: "Bad request: " + details}
}

func apiErrSomethingWentWrong() APIError {
	return APIError{StatusCode: http.StatusInternalServerError, Message: "Something went wrong"}
}
