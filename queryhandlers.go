package sqliteadmin

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/mitchellh/mapstructure"
)

func (a *Admin) ping(w http.ResponseWriter) {
	a.logger.Info("Command: Ping")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (a *Admin) listTables(w http.ResponseWriter) {
	a.logger.Info("Command: ListTables")
	rows, err := a.db.Query("SELECT name FROM sqlite_master WHERE type='table';")
	if err != nil {
		a.logger.Error(fmt.Sprintf("Error listing tables: %v", err))
		writeError(w, apiErrSomethingWentWrong())
		return
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			a.logger.Error(fmt.Sprintf("Error scanning rows: %v", err))
			writeError(w, apiErrSomethingWentWrong())
			return
		}
		tables = append(tables, table)
	}

	json.NewEncoder(w).Encode(map[string][]string{"tables": tables})
}

func (a *Admin) getTable(w http.ResponseWriter, params map[string]interface{}) {
	// Parse table name
	table, ok := params["tableName"].(string)
	if !ok {
		writeError(w, apiErrBadRequest(ErrMissingTableName.Error()))
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

	a.logger.Info(fmt.Sprintf("Command: GetTable, table=%s, limit=%d, offset=%d", table, limit, offset))

	var condition *Condition
	conditionParam, ok := params["condition"]
	if ok {
		condition, ok = toCondition(conditionParam, a.logger)
		if !ok {
			writeError(w, apiErrBadRequest("Invalid condition"))
			return
		}
		a.logger.Debug(fmt.Sprintf("Condition provided: %v", condition))
	} else {
		a.logger.Debug("No condition provided")
	}

	data, err := queryTable(a.db, table, condition, limit, offset, a.logger)
	if err != nil {
		a.logger.Error(fmt.Sprintf("Error querying table: %v", err))
		writeError(w, apiErrSomethingWentWrong())
		return
	}
	response := map[string]interface{}{"rows": data}

	if params["includeInfo"] == true {
		tableInfo, err := getTableInfo(a.db, table)
		if err != nil {
			a.logger.Error(fmt.Sprintf("Error getting table info: %v", err))
			writeError(w, apiErrSomethingWentWrong())
			return
		}
		response["tableInfo"] = tableInfo
	}
	a.logger.Info(fmt.Sprintf("Fetched %d rows", len(data)))

	json.NewEncoder(w).Encode(response)
}

func (a *Admin) deleteRows(w http.ResponseWriter, params map[string]interface{}) {
	table, ok := params["tableName"].(string)
	if !ok {
		writeError(w, apiErrBadRequest(ErrMissingTableName.Error()))
		return
	}

	ids, ok := convertToStrSlice(params["ids"])
	if !ok {
		writeError(w, apiErrBadRequest(ErrInvalidOrMissingIds.Error()))
		return
	}

	a.logger.Info(fmt.Sprintf("Command: DeleteRows, table=%s, ids=%v", table, ids))

	exists, err := checkTableExists(a.db, table)
	if err != nil {
		a.logger.Error(fmt.Sprintf("Error checking table existence: %v", err))
		writeError(w, apiErrSomethingWentWrong())
		return
	}
	if !exists {
		a.logger.Error(fmt.Sprintf("Error table does not exist: %s", table))
		writeError(w, apiErrBadRequest(ErrInvalidInput.Error()))
		return
	}

	rowsAffected, err := batchDelete(a.db, table, ids)
	if err != nil {
		a.logger.Error(fmt.Sprintf("Error deleting rows from table: %v", err))
		writeError(w, apiErrSomethingWentWrong())
		return
	}
	a.logger.Info(fmt.Sprintf("Deleted %d row(s)", rowsAffected))

	json.NewEncoder(w).Encode(map[string]string{"rowsAffected": fmt.Sprintf("%d", rowsAffected)})
}

func (a *Admin) updateRow(w http.ResponseWriter, params map[string]interface{}) {
	table, ok := params["tableName"].(string)
	if !ok {
		writeError(w, apiErrBadRequest(ErrMissingTableName.Error()))
		return
	}

	row, ok := params["row"].(map[string]interface{})
	if !ok {
		writeError(w, apiErrBadRequest(ErrMissingRow.Error()))
		return
	}

	a.logger.Info(fmt.Sprintf("Command: UpdateRow, table=%s, row=%v", table, row))

	err := editRow(a.db, table, row)
	if err != nil {
		a.logger.Error(fmt.Sprintf("Error editing row: %v", err))
		writeError(w, apiErrSomethingWentWrong())
		return
	}
	a.logger.Info("Row updated")

	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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

func queryTable(db *sql.DB, tableName string, condition *Condition, limit int, offset int, logger Logger) ([]map[string]interface{}, error) {
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
		logger.Debug(fmt.Sprintf("ConditionQuery: %s", conditionQuery))
		logger.Debug(fmt.Sprintf("Args: %v", args))
		query += conditionQuery
		query += fmt.Sprintf(" LIMIT %d", limit)
	} else {
		query = fmt.Sprintf("SELECT * FROM %q LIMIT %d OFFSET %d", tableName, limit, offset)
	}

	logger.Info(fmt.Sprintf("About to perform query: `%s`", query))

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
		err = rows.Scan(scanArgs...)
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
	case OperatorIsNull:
		return fmt.Sprintf("%s IS NULL", filter.Column)
	case OperatorIsNotNull:
		return fmt.Sprintf("%s IS NOT NULL", filter.Column)
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
		if err = rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
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

func editRow(db *sql.DB, tableName string, row map[string]interface{}) error {
	// Get the primary key of the table
	tableInfo, err := getTableInfo(db, tableName)
	if err != nil {
		return fmt.Errorf("error getting primary key for edit: %v", err)
	}
	columns, ok := tableInfo["columns"].([]map[string]interface{})
	if !ok {
		return fmt.Errorf("error getting primary key for edit")
	}
	var primaryKey string
	for _, column := range columns {
		if column["pk"].(int) == 1 {
			primaryKey = column["name"].(string)
			break
		}
	}

	if primaryKey == "" {
		return fmt.Errorf("table %s does not have a primary key", tableName)
	}

	if _, ok := row[primaryKey]; !ok {
		return fmt.Errorf("row does not contain primary key")
	}

	nonPKColumns := make(map[string]interface{})
	for k, v := range row {
		if k != primaryKey {
			nonPKColumns[k] = v
		}
	}

	// Create the placeholders for the query (?,?,?)
	// We exclude the primary key from the placeholders
	placeholders := make([]string, len(row)-1)
	values := make([]interface{}, len(row)-1)
	i := 0
	for k, v := range nonPKColumns {
		// Add the column name to the placeholder string
		placeholders[i] = fmt.Sprintf("%s = ?", k)
		values[i] = v
		i++
	}

	// Build the query
	query := fmt.Sprintf(
		"UPDATE %s SET %s WHERE %s = ?",
		tableName,
		strings.Join(placeholders, ","),
		primaryKey,
	)

	// Add the primary key value to the end of the values slice
	values = append(values, row[primaryKey])

	// Execute the update
	_, err = db.Exec(query, values...)
	if err != nil {
		return fmt.Errorf("edit row failed: %v", err)
	}

	return nil
}

func writeError(w http.ResponseWriter, err APIError) {
	w.WriteHeader(err.StatusCode)
	json.NewEncoder(w).Encode(err)
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

func toCondition(val interface{}, logger Logger) (*Condition, bool) {
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
			logger.Debug("Cases is not an array")
			return nil, false
		}
		for _, c := range cases {
			caseMap, ok := c.(map[string]interface{})
			if !ok {
				logger.Debug("Case is not a map")
				return nil, false
			}
			// If the logicalOperator field exists then it is a Sub-Condition
			if caseMap["logicalOperator"] != nil {
				subCondition, ok := toCondition(caseMap, logger)
				if !ok {
					logger.Debug("Could not convert sub-condition")
					return nil, false
				}
				condition.Cases = append(condition.Cases, *subCondition)
			} else {
				filter := Filter{}
				err := mapstructure.Decode(c, &filter)
				if err != nil {
					logger.Error(fmt.Sprintf("Error decoding filter: %v", err))
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
