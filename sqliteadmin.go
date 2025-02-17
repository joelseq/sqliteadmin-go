// Package sqliteadmin allows you to view and managed your SQLite database by
// exposing an HTTP handler that you can easily integrate into any Go web
// framework.

package sqliteadmin

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

type Handler struct {
	db       *sql.DB
	username string
	password string
	logger   Logger
}

type Command string

type Filter struct {
	Column   string   `json:"column"`
	Operator Operator `json:"operator"`
	Value    string   `json:"value"`
}

type Condition struct {
	Cases           []Case          `json:"cases" mapstructure:"cases"`
	LogicalOperator LogicalOperator `json:"logicalOperator" mapstructure:"logicalOperator"`
}

type Case interface {
	ConditionCaseType() string
}

func (c Condition) ConditionCaseType() string {
	return "condition"
}

func (f Filter) ConditionCaseType() string {
	return "filter"
}

type LogicalOperator string

const (
	LogicalOperatorAnd LogicalOperator = "and"
	LogicalOperatorOr  LogicalOperator = "or"
)

type Operator string

const (
	OperatorEquals              Operator = "eq"
	OperatorLike                Operator = "like"
	OperatorNotEquals           Operator = "neq"
	OperatorLessThan            Operator = "lt"
	OperatorLessThanOrEquals    Operator = "lte"
	OperatorGreaterThan         Operator = "gt"
	OperatorGreaterThanOrEquals Operator = "gte"
	OperatorIsNull              Operator = "null"
	OperatorIsNotNull           Operator = "notnull"
)

const (
	Ping       Command = "Ping"
	ListTables Command = "ListTables"
	GetTable   Command = "GetTable"
	DeleteRows Command = "DeleteRows"
	UpdateRow  Command = "UpdateRow"
)

const pathPrefixPlaceholder = "%%__path_prefix__%%"

const (
	DefaultLimit  = 100
	DefaultOffset = 0
)

type Logger interface {
	Info(format string, args ...interface{})
	Error(format string, args ...interface{})
	Debug(format string, args ...interface{})
}

type LogLevel string

const (
	LogLevelInfo  LogLevel = "info"
	LogLevelDebug LogLevel = "debug"
)

type Config struct {
	DB       *sql.DB
	Username string
	Password string
	Logger   Logger
}

// Creates a new HTTP handler which has a HandlePost method
// that can be used to handle requests from https://sqliteadmin.dev.
func NewHandler(c Config) *Handler {
	h := &Handler{
		db:       c.DB,
		username: c.Username,
		password: c.Password,
		logger:   c.Logger,
	}

	if h.logger == nil {
		h.logger = &defaultLogger{}
	}

	return h
}

type CommandRequest struct {
	Command Command                `json:"command"`
	Params  map[string]interface{} `json:"params"`
}

// Handles the incoming HTTP POST request. This is responsible for handling
// all the supported operations from https://sqliteadmin.dev
func (h *Handler) HandlePost(w http.ResponseWriter, r *http.Request) {
	// Check for auth header that contains username and password
	w.Header().Set("Content-Type", "application/json")
	if h.username != "" && h.password != "" {
		authHeader := r.Header.Get("Authorization")
		if h.username+":"+h.password != authHeader {
			writeError(w, apiErrUnauthorized())
			return
		}
	}

	var cr CommandRequest
	err := json.NewDecoder(r.Body).Decode(&cr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid Request Body"})
		return
	}

	switch cr.Command {
	case Ping:
		h.ping(w)
		return
	case ListTables:
		h.listTables(w)
		return
	case GetTable:
		h.getTable(w, cr.Params)
		return
	case DeleteRows:
		h.deleteRows(w, cr.Params)
		return
	case UpdateRow:
		h.updateRow(w, cr.Params)
		return
	default:
		http.Error(w, "Invalid command", http.StatusBadRequest)
	}
}

var _ Logger = &defaultLogger{}

type defaultLogger struct{}

func (l *defaultLogger) Info(format string, args ...interface{}) {}

func (l *defaultLogger) Error(format string, args ...interface{}) {}

func (l *defaultLogger) Debug(format string, args ...interface{}) {}
