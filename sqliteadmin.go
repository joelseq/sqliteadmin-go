// Package sqliteadmin allows you to view and managed your SQLite database by
// exposing an HTTP handler that you can easily integrate into any Go web
// framework.

package sqliteadmin

import (
	"database/sql"
	"embed"
	"encoding/json"
	"net/http"
)

type Handler struct {
	db       *sql.DB
	username string
	password string
}

type Query string

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
)

const (
	Ping       Query = "Ping"
	ListTables Query = "ListTables"
	GetTable   Query = "GetTable"
	DeleteRows Query = "DeleteRows"
	UpdateRow  Query = "UpdateRow"
)

const pathPrefixPlaceholder = "%%__path_prefix__%%"

const DefaultLimit = 100
const DefaultOffset = 0

var indexHtml embed.FS

func NewHandler(db *sql.DB, username, password string) *Handler {
	return &Handler{
		db:       db,
		username: username,
		password: password,
	}
}

type QueryRequest struct {
	Query  Query                  `json:"query"`
	Params map[string]interface{} `json:"params"`
}

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request) {
	// Check for auth header that contains username and password
	if h.username != "" && h.password != "" {
		authHeader := r.Header.Get("Authorization")
		if h.username+":"+h.password != authHeader {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	// w.Header().Set("Access-Control-Allow-Origin", "*")
	var qr QueryRequest
	err := json.NewDecoder(r.Body).Decode(&qr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid Request Body"})
		return
	}

	switch qr.Query {
	case Ping:
		h.ping(w)
		return
	case ListTables:
		h.listTables(w)
		return
	case GetTable:
		h.getTable(w, qr.Params)
		return
	case DeleteRows:
		h.deleteRows(w, qr.Params)
		return
	case UpdateRow:
		h.updateRow(w, qr.Params)
		return
	default:
		http.Error(w, "Invalid query", http.StatusBadRequest)
	}
}
