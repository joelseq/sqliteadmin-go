// Package sqliteadmin allows you to view and managed your SQLite database by
// exposing an HTTP handler that you can easily integrate into any Go web
// framework.

package sqliteadmin

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/mitchellh/mapstructure"
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
	default:
		http.Error(w, "Invalid query", http.StatusBadRequest)
	}
}

func (h *Handler) ping(w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *Handler) listTables(w http.ResponseWriter) {
	rows, err := h.db.Query("SELECT name FROM sqlite_master WHERE type='table';")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tables = append(tables, table)
	}

	json.NewEncoder(w).Encode(map[string][]string{"tables": tables})
}

func (h *Handler) getTable(w http.ResponseWriter, params map[string]interface{}) {
	// Parse table name
	table, ok := params["tableName"].(string)
	if !ok {
		writeError(w, fmt.Errorf("Missing table name"), http.StatusBadRequest)
		return
	}

	// Parse limit
	limit := DefaultLimit
	if params["limit"] != nil {
		// convert the limit parameter to an int
		limit, ok = convertNumber(params["limit"])
		if !ok {
			limit = DefaultLimit
		}
	}

	// Parse offset
	offset := DefaultOffset
	if params["offset"] != nil {
		// convert the offset parameter to an int
		offset, ok = convertNumber(params["offset"])
		if !ok {
			offset = DefaultOffset
		}
	}

	var condition *Condition
	conditionParam, ok := params["condition"].(interface{})
	if ok {
		condition, ok = toCondition(conditionParam)
		if !ok {
			// TODO: use better logging
			fmt.Println("Could not convert condition")
		}
	} else {
		// TODO: use better logging
		fmt.Println("No filters provided")
	}

	data, err := queryTable(h.db, table, condition, limit, offset)
	if err != nil {
		// TODO: use better logging
		fmt.Printf("Error querying table: %v\n", err)
		writeError(w, fmt.Errorf("Something went wrong"), http.StatusInternalServerError)
		return
	}
	response := map[string]interface{}{"rows": data}

	if params["includeInfo"] == true {
		tableInfo, err := getTableInfo(h.db, table)
		if err != nil {
			// TODO: use better logging
			fmt.Printf("Error getting table info: %v\n", err)
			writeError(w, fmt.Errorf("Something went wrong"), http.StatusInternalServerError)
			return
		}
		response["tableInfo"] = tableInfo
	}

	json.NewEncoder(w).Encode(response)
}

func (h *Handler) deleteRows(w http.ResponseWriter, params map[string]interface{}) {
	table, ok := params["tableName"].(string)
	if !ok {
		writeError(w, fmt.Errorf("Missing table name"), http.StatusBadRequest)
		return
	}

	ids, ok := convertToStrSlice(params["ids"])
	if !ok {
		writeError(w, fmt.Errorf("Invalid or missing ids"), http.StatusBadRequest)
		return
	}

	exists, err := checkTableExists(h.db, table)
	if err != nil {
		// TODO: use better logging
		fmt.Printf("Error checking table existence: %v\n", err)
		writeError(w, fmt.Errorf("Bad Request"), http.StatusBadRequest)
		return
	}
	if !exists {
		// TODO: use better logging
		fmt.Printf("Error table does not exist: %s\n", table)
		writeError(w, fmt.Errorf("Bad Request"), http.StatusBadRequest)
		return
	}

	rowsAffected, err := batchDelete(h.db, table, ids)
	if err != nil {
		// TODO: use better logging
		fmt.Printf("Error deleting rows from table: %v\n", err)
		writeError(w, fmt.Errorf("Something went wrong"), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"rowsAffected": fmt.Sprintf("%d", rowsAffected)})
}

func checkTableExists(db *sql.DB, tableName string) (bool, error) {
	var exists int
	err := db.QueryRow(`
				SELECT COUNT(*) FROM sqlite_master 
				WHERE type='table' AND name=?`, tableName).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("error checking table existence: %v", err)
	}
	return exists > 0, nil
}

func queryTable(db *sql.DB, tableName string, condition *Condition, limit int, offset int) ([]map[string]interface{}, error) {
	// First, verify the table exists to prevent SQL injection
	exists, err := checkTableExists(db, tableName)
	if err != nil {
		return nil, fmt.Errorf("error checking table existence: %v", err)
	}
	if !exists {
		return nil, fmt.Errorf("table %s does not exist", tableName)
	}

	// Query to get column names
	rows, err := db.Query(fmt.Sprintf("SELECT * FROM %q LIMIT 0", tableName))
	if err != nil {
		return nil, fmt.Errorf("error getting columns: %v", err)
	}
	columns, err := rows.Columns()
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("error reading columns: %v", err)
	}

	var query string

	var args []interface{}
	if condition != nil && len(condition.Cases) > 0 {
		// Build the query
		query = fmt.Sprintf("SELECT * FROM %s WHERE ", tableName)

		// Generate the conditions for the where clause
		var conditionQuery string
		conditionQuery, args = getCondition(condition)
		// TODO: use better logging
		fmt.Printf("Query: %s\n", conditionQuery)
		// TODO: use better logging
		fmt.Printf("Args: %v\n", args)
		query += conditionQuery
		query += fmt.Sprintf(" LIMIT %d", limit)
	} else {
		query = fmt.Sprintf("SELECT * FROM %q LIMIT %d OFFSET %d", tableName, limit, offset)
	}

	// TODO: use better logging
	fmt.Printf("About to perform query: `%s`\n", query)

	// Now perform the actual query
	rows, err = db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("error querying table: %v", err)
	}
	defer rows.Close()

	// Prepare the result slice
	var result []map[string]interface{}

	// Prepare value holders
	values := make([]interface{}, len(columns))
	scanArgs := make([]interface{}, len(columns))
	for i := range values {
		scanArgs[i] = &values[i]
	}

	// Iterate through rows
	for rows.Next() {
		err := rows.Scan(scanArgs...)
		if err != nil {
			return nil, fmt.Errorf("error scanning row: %v", err)
		}

		// Create a map for this row
		row := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			switch v := val.(type) {
			case []byte:
				row[col] = string(v)
			default:
				row[col] = v
			}
		}
		result = append(result, row)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading rows: %v", err)
	}

	return result, nil
}

func getCondition(condition *Condition) (string, []interface{}) {
	var clause string
	var args []interface{}

	for i, c := range condition.Cases {
		if i > 0 {
			clause += fmt.Sprintf(" %s ", condition.LogicalOperator)
		}
		switch c.ConditionCaseType() {
		case "condition":
			condition := c.(Condition)
			subClause, subArgs := getCondition(&condition)
			clause += "(" + subClause + ")"
			args = append(args, subArgs...)
		case "filter":
			filter := c.(Filter)
			clause += getClause(filter)
			args = append(args, filter.Value)
		}
	}
	return clause, args
}

func getClause(filter Filter) string {
	switch filter.Operator {
	case OperatorEquals:
		return fmt.Sprintf("%s = ?", filter.Column)
	case OperatorLike:
		return fmt.Sprintf("%s LIKE '%%' || ? || '%%'", filter.Column)
	case OperatorNotEquals:
		return fmt.Sprintf("%s != ?", filter.Column)
	case OperatorLessThan:
		return fmt.Sprintf("%s < ?", filter.Column)
	case OperatorLessThanOrEquals:
		return fmt.Sprintf("%s <= ?", filter.Column)
	case OperatorGreaterThan:
		return fmt.Sprintf("%s > ?", filter.Column)
	case OperatorGreaterThanOrEquals:
		return fmt.Sprintf("%s >= ?", filter.Column)
	default:
		return ""
	}
}

func batchDelete(db *sql.DB, tableName string, ids []any) (int64, error) {
	// Handle empty case
	if len(ids) == 0 {
		return 0, nil
	}

	// Get the primary key of the table
	tableInfo, err := getTableInfo(db, tableName)
	if err != nil {
		return 0, fmt.Errorf("error getting primary key for delete: %v", err)
	}
	columns, ok := tableInfo["columns"].([]map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("error getting primary key for delete")
	}
	var primaryKey string
	for _, column := range columns {
		if column["pk"].(int) == 1 {
			primaryKey = column["name"].(string)
			break
		}
	}

	if primaryKey == "" {
		return 0, fmt.Errorf("table %s does not have a primary key", tableName)
	}

	// Create the placeholders for the query (?,?,?)
	placeholders := make([]string, len(ids))
	for i := range ids {
		placeholders[i] = "?"
	}

	// Build the query
	query := fmt.Sprintf(
		"DELETE FROM %s WHERE %s IN (%s)",
		tableName,
		primaryKey,
		strings.Join(placeholders, ","),
	)

	// Execute the delete
	result, err := db.Exec(query, ids...)
	if err != nil {
		return 0, fmt.Errorf("batch delete failed: %v", err)
	}

	// Return number of rows affected
	return result.RowsAffected()
}

func getTableInfo(db *sql.DB, tableName string) (map[string]interface{}, error) {
	// First, verify the table exists to prevent SQL injection
	exists, err := checkTableExists(db, tableName)
	if err != nil {
		return nil, fmt.Errorf("error checking table existence: %v", err)
	}
	if !exists {
		return nil, fmt.Errorf("table %s does not exist", tableName)
	}

	// Query to get column names
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%q)", tableName))
	if err != nil {
		return nil, fmt.Errorf("error getting columns: %v", err)
	}
	defer rows.Close()

	// Prepare the result slice
	var result []map[string]interface{}

	// Iterate through rows
	for rows.Next() {
		var cid int
		var name string
		var dataType string
		var notNull int
		var defaultValue interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return nil, fmt.Errorf("error scanning row: %v", err)
		}

		// Create a map for this row
		row := map[string]interface{}{
			"cid":      cid,
			"name":     name,
			"dataType": dataType,
			"notNull":  notNull,
			"pk":       pk,
		}
		result = append(result, row)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading rows: %v", err)
	}

	// Get the number of rows
	var count int
	err = db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %q", tableName)).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("error getting row count: %v", err)
	}

	return map[string]interface{}{"columns": result, "count": count}, nil
}

func writeError(w http.ResponseWriter, err error, status int) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func convertToStrSlice(val interface{}) ([]any, bool) {
	// Check if the value is a slice
	slice, ok := val.([]interface{})
	if !ok {
		return nil, false
	}

	// Convert each element to a string
	var result []any
	for _, v := range slice {
		str, ok := v.(string)
		if !ok {
			return nil, false
		}
		result = append(result, str)
	}

	return result, true
}

func toCondition(val interface{}) (*Condition, bool) {
	// Check if val is a map
	valMap, ok := val.(map[string]interface{})
	if !ok {
		return nil, false
	}

	// Decode the value into a Condition
	condition := Condition{}

	if valMap["cases"] != nil {
		cases, ok := valMap["cases"].([]interface{})
		if !ok {
			// TODO: use better logging
			fmt.Println("Cases is not an array")
			return nil, false
		}
		for _, c := range cases {
			caseMap, ok := c.(map[string]interface{})
			if !ok {
				// TODO: use better logging
				fmt.Println("Case is not a map")
				return nil, false
			}
			// If the logicalOperator field exists then it is a Sub-Condition
			if caseMap["logicalOperator"] != nil {
				subCondition, ok := toCondition(caseMap)
				if !ok {
					// TODO: use better logging
					fmt.Println("Could not convert sub-condition")
					return nil, false
				}
				condition.Cases = append(condition.Cases, *subCondition)
			} else {
				filter := Filter{}
				err := mapstructure.Decode(c, &filter)
				if err != nil {
					// TODO: use better logging
					fmt.Printf("Error decoding filter: %v\n", err)
					return nil, false
				}
				condition.Cases = append(condition.Cases, filter)
			}
		}
	}

	if valMap["logicalOperator"] != nil {
		condition.LogicalOperator = LogicalOperator(valMap["logicalOperator"].(string))
	}

	return &condition, true
}

func convertNumber(val interface{}) (int, bool) {
	switch v := val.(type) {
	case int:
		return v, true
	case float64:
		return int(v), true
	case string:
		i, err := strconv.Atoi(v)
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}
