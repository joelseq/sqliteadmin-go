package sqliteadmin_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/joelseq/sqliteadmin-go"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
)

func TestPing(t *testing.T) {
	ts, close := setupTestServer(t)
	defer close()

	body := sqliteadmin.CommandRequest{
		Command: sqliteadmin.Ping,
	}

	req := makeRequest(t, ts.server.URL, body)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)

	assert.Equal(t, http.StatusOK, res.StatusCode)

	result := readBody(t, res.Body)
	assert.Equal(t, "ok", result["status"])
}

func TestListTables(t *testing.T) {
	ts, close := setupTestServer(t)
	defer close()

	body := sqliteadmin.CommandRequest{
		Command: sqliteadmin.ListTables,
	}

	req := makeRequest(t, ts.server.URL, body)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)

	assert.Equal(t, http.StatusOK, res.StatusCode)

	result := readBody(t, res.Body)
	assert.Equal(t, []interface{}{"users"}, result["tables"])
}

type TestCase struct {
	name             string
	params           map[string]interface{}
	expectedStatus   int
	expectedResponse map[string]interface{}
}

func TestDeleteRows(t *testing.T) {
	ts, close := setupTestServer(t)
	defer close()

	cases := []TestCase{
		{
			name: "Failure: Missing Table Name",
			params: map[string]interface{}{
				"ids": []string{"1", "2"},
			},
			expectedStatus: http.StatusBadRequest,
			expectedResponse: map[string]interface{}{
				"statusCode": float64(http.StatusBadRequest),
				"message":    "Bad request: missing table name",
			},
		},
		{
			name: "Failure: Missing IDs",
			params: map[string]interface{}{
				"tableName": "users",
			},
			expectedStatus: http.StatusBadRequest,
			expectedResponse: map[string]interface{}{
				"statusCode": float64(http.StatusBadRequest),
				"message":    "Bad request: invalid or missing ids",
			},
		},
		{
			name: "Failure: Invalid IDs",
			params: map[string]interface{}{
				"tableName": "invalid",
				"ids":       []string{"1", "2"},
			},
			expectedStatus: http.StatusBadRequest,
			expectedResponse: map[string]interface{}{
				"statusCode": float64(http.StatusBadRequest),
				"message":    "Bad request: invalid input",
			},
		},
		{
			name: "Success: Delete Rows",
			params: map[string]interface{}{
				"tableName": "users",
				"ids":       []string{"1", "2"},
			},
			expectedStatus: http.StatusOK,
			expectedResponse: map[string]interface{}{
				"rowsAffected": "2",
			},
		},
	}

	runTestCases(cases, sqliteadmin.DeleteRows, t, ts.server)

	rows, err := getTableValues(ts.db, "users")

	assert.NoError(t, err)
	assert.Equal(t, 7, len(rows))
}

func TestUpdateRow(t *testing.T) {
	ts, close := setupTestServer(t)
	defer close()

	cases := []TestCase{
		{
			name: "Failure: Missing Table Name",
			params: map[string]interface{}{
				"row": map[string]interface{}{
					"id":    "1",
					"name":  "Alice",
					"email": "alice-updated@gmail.com",
				},
			},
			expectedStatus: http.StatusBadRequest,
			expectedResponse: map[string]interface{}{
				"statusCode": float64(http.StatusBadRequest),
				"message":    "Bad request: missing table name",
			},
		},
		{
			name: "Failure: Invalid Table Name",
			params: map[string]interface{}{
				"tableName": "invalid",
				"row": map[string]interface{}{
					"id":    "1",
					"name":  "Alice",
					"email": "alice-updated@gmail.com",
				},
			},
			expectedStatus: http.StatusInternalServerError,
			expectedResponse: map[string]interface{}{
				"statusCode": float64(http.StatusInternalServerError),
				"message":    "Something went wrong",
			},
		},
		{
			name: "Failure: Row does not contain primary key column",
			params: map[string]interface{}{
				"tableName": "invalid",
				"row": map[string]interface{}{
					"name":  "Alice",
					"email": "alice-updated@gmail.com",
				},
			},
			expectedStatus: http.StatusInternalServerError,
			expectedResponse: map[string]interface{}{
				"statusCode": float64(http.StatusInternalServerError),
				"message":    "Something went wrong",
			},
		},
		{
			name: "Success: Update Row",
			params: map[string]interface{}{
				"tableName": "users",
				"row": map[string]interface{}{
					"id":    "1",
					"name":  "Alice",
					"email": "alice-updated@gmail.com",
				},
			},
			expectedStatus: http.StatusOK,
			expectedResponse: map[string]interface{}{
				"status": "ok",
			},
		},
	}

	runTestCases(cases, sqliteadmin.UpdateRow, t, ts.server)

	rows, err := getTableValues(ts.db, "users")

	assert.NoError(t, err)
	assert.Equal(t, "alice-updated@gmail.com", rows[0]["email"])
}

func TestGetTable(t *testing.T) {
	ts, close := setupTestServer(t)
	defer close()

	cases := []TestCase{
		{
			name: "Failure: Missing Table Name",
			params: map[string]interface{}{
				"limit": 10,
			},
			expectedStatus: http.StatusBadRequest,
			expectedResponse: map[string]interface{}{
				"statusCode": float64(http.StatusBadRequest),
				"message":    "Bad request: missing table name",
			},
		},
		{
			name: "Failure: Invalid Table Name",
			params: map[string]interface{}{
				"tableName": "invalid",
				"limit":     10,
			},
			expectedStatus: http.StatusInternalServerError,
			expectedResponse: map[string]interface{}{
				"statusCode": float64(http.StatusInternalServerError),
				"message":    "Something went wrong",
			},
		},
		{
			name: "Success: Get Table with limit and offset",
			params: map[string]interface{}{
				"tableName": "users",
				"limit":     2,
				"offset":    2,
			},
			expectedStatus: http.StatusOK,
			expectedResponse: map[string]interface{}{
				"rows": []interface{}{
					map[string]interface{}{
						"id":    float64(3),
						"name":  "Charlie",
						"email": "charlie@gmail.com",
					},
					map[string]interface{}{
						"id":    float64(4),
						"name":  "David",
						"email": "david@gmail.com",
					},
				},
			},
		},
		makeGetTableCondition("Success: Get Table with equal condition",
			sqliteadmin.Condition{
				Cases: []sqliteadmin.Case{
					sqliteadmin.Filter{
						Column:   "id",
						Operator: sqliteadmin.OperatorEquals,
						Value:    "1",
					},
				},
			},
			makeGetTableResponse([]responseRow{
				{id: 1, name: "Alice", email: "alice@gmail.com"},
			}),
		),
		makeGetTableCondition("Success: Get Table with like condition",
			sqliteadmin.Condition{
				Cases: []sqliteadmin.Case{
					sqliteadmin.Filter{
						Column:   "email",
						Operator: sqliteadmin.OperatorLike,
						Value:    "@gmail.com",
					},
				},
			},
			makeGetTableResponse([]responseRow{
				{id: 1, name: "Alice", email: "alice@gmail.com"},
				{id: 2, name: "Bob", email: "bob@gmail.com"},
				{id: 3, name: "Charlie", email: "charlie@gmail.com"},
				{id: 4, name: "David", email: "david@gmail.com"},
				{id: 7, name: "Grace", email: "grace@gmail.com"},
				{id: 8, name: "Henry", email: "henry@gmail.com"},
			}),
		),
		makeGetTableCondition("Success: Get Table with not equals condition",
			sqliteadmin.Condition{
				Cases: []sqliteadmin.Case{
					sqliteadmin.Filter{
						Column:   "id",
						Operator: sqliteadmin.OperatorNotEquals,
						Value:    "1",
					},
				},
			},
			makeGetTableResponse([]responseRow{
				{id: 2, name: "Bob", email: "bob@gmail.com"},
				{id: 3, name: "Charlie", email: "charlie@gmail.com"},
				{id: 4, name: "David", email: "david@gmail.com"},
				{id: 5, name: "Eve", email: "eve@outlook.com"},
				{id: 6, name: "Frank", email: "frank@yahoo.com"},
				{id: 7, name: "Grace", email: "grace@gmail.com"},
				{id: 8, name: "Henry", email: "henry@gmail.com"},
				{id: 9, name: "Ivy", email: nil},
			}),
		),
		makeGetTableCondition("Success: Get Table with less than condition",
			sqliteadmin.Condition{
				Cases: []sqliteadmin.Case{
					sqliteadmin.Filter{
						Column:   "id",
						Operator: sqliteadmin.OperatorLessThan,
						Value:    "3",
					},
				},
			},
			makeGetTableResponse([]responseRow{
				{id: 1, name: "Alice", email: "alice@gmail.com"},
				{id: 2, name: "Bob", email: "bob@gmail.com"},
			}),
		),
		makeGetTableCondition("Success: Get Table with less than or equals condition",
			sqliteadmin.Condition{
				Cases: []sqliteadmin.Case{
					sqliteadmin.Filter{
						Column:   "id",
						Operator: sqliteadmin.OperatorLessThanOrEquals,
						Value:    "2",
					},
				},
			},
			makeGetTableResponse([]responseRow{
				{id: 1, name: "Alice", email: "alice@gmail.com"},
				{id: 2, name: "Bob", email: "bob@gmail.com"},
			}),
		),
		makeGetTableCondition("Success: Get Table with greater than condition",
			sqliteadmin.Condition{
				Cases: []sqliteadmin.Case{
					sqliteadmin.Filter{
						Column:   "id",
						Operator: sqliteadmin.OperatorGreaterThan,
						Value:    "6",
					},
				},
			},
			makeGetTableResponse([]responseRow{
				{id: 7, name: "Grace", email: "grace@gmail.com"},
				{id: 8, name: "Henry", email: "henry@gmail.com"},
				{id: 9, name: "Ivy", email: nil},
			}),
		),
		makeGetTableCondition("Success: Get Table with greater than or equals condition",
			sqliteadmin.Condition{
				Cases: []sqliteadmin.Case{
					sqliteadmin.Filter{
						Column:   "id",
						Operator: sqliteadmin.OperatorGreaterThanOrEquals,
						Value:    "7",
					},
				},
			},
			makeGetTableResponse([]responseRow{
				{id: 7, name: "Grace", email: "grace@gmail.com"},
				{id: 8, name: "Henry", email: "henry@gmail.com"},
				{id: 9, name: "Ivy", email: nil},
			}),
		),
		makeGetTableCondition("Success: Get Table with null condition",
			sqliteadmin.Condition{
				Cases: []sqliteadmin.Case{
					sqliteadmin.Filter{
						Column:   "email",
						Operator: sqliteadmin.OperatorIsNull,
					},
				},
			},
			makeGetTableResponse([]responseRow{
				{id: 9, name: "Ivy", email: nil},
			}),
		),
		makeGetTableCondition("Success: Get Table with not null condition",
			sqliteadmin.Condition{
				Cases: []sqliteadmin.Case{
					sqliteadmin.Filter{
						Column:   "email",
						Operator: sqliteadmin.OperatorIsNotNull,
					},
				},
			},
			makeGetTableResponse([]responseRow{
				{id: 1, name: "Alice", email: "alice@gmail.com"},
				{id: 2, name: "Bob", email: "bob@gmail.com"},
				{id: 3, name: "Charlie", email: "charlie@gmail.com"},
				{id: 4, name: "David", email: "david@gmail.com"},
				{id: 5, name: "Eve", email: "eve@outlook.com"},
				{id: 6, name: "Frank", email: "frank@yahoo.com"},
				{id: 7, name: "Grace", email: "grace@gmail.com"},
				{id: 8, name: "Henry", email: "henry@gmail.com"},
			}),
		),
	}

	runTestCases(cases, sqliteadmin.GetTable, t, ts.server)
}

func runTestCases(testCases []TestCase, command sqliteadmin.Command, t *testing.T, srv *httptest.Server) {
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			body := sqliteadmin.CommandRequest{
				Command: command,
				Params:  tc.params,
			}

			req := makeRequest(t, srv.URL, body)
			res, err := http.DefaultClient.Do(req)
			assert.NoError(t, err)

			assert.Equal(t, tc.expectedStatus, res.StatusCode)

			result := readBody(t, res.Body)
			assert.EqualValues(t, tc.expectedResponse, result)
		})
	}
}

func makeGetTableCondition(name string, condition sqliteadmin.Condition, expectedResponse map[string]interface{}) TestCase {
	return TestCase{
		name: name,
		params: map[string]interface{}{
			"tableName": "users",
			"condition": condition,
		},
		expectedStatus:   http.StatusOK,
		expectedResponse: expectedResponse,
	}
}

type responseRow struct {
	id    int
	name  string
	email any
}

func makeGetTableResponse(values []responseRow) map[string]interface{} {
	rows := make([]interface{}, len(values))

	for i, v := range values {
		rows[i] = map[string]interface{}{
			"id":    float64(v.id),
			"name":  v.name,
			"email": v.email,
		}
	}

	return map[string]interface{}{
		"rows": rows,
	}
}

var testValues = [][]string{
	{"Alice", "alice@gmail.com"},
	{"Bob", "bob@gmail.com"},
	{"Charlie", "charlie@gmail.com"},
	{"David", "david@gmail.com"},
	{"Eve", "eve@outlook.com"},
	{"Frank", "frank@yahoo.com"},
	{"Grace", "grace@gmail.com"},
	{"Henry", "henry@gmail.com"},
	{"Ivy"},
}

type TestServer struct {
	server *httptest.Server
	db     *sql.DB
}

func setupTestServer(t *testing.T) (*TestServer, func()) {
	db := setupDB(t)

	c := sqliteadmin.Config{
		DB:       db,
		Username: "user",
		Password: "password",
	}

	h := sqliteadmin.NewHandler(c)
	mux := http.NewServeMux()

	mux.HandleFunc("/", h.HandlePost)

	srv := httptest.NewServer(mux)

	// Create a simple http server
	// and send a request to the handler
	// to test the Ping command
	return &TestServer{
			server: srv,
			db:     db,
		}, func() {
			srv.Close()
			db.Close()
		}
}

func makeRequest(t *testing.T, url string, body interface{}) *http.Request {
	bodyJSON, err := json.Marshal(body)
	assert.NoError(t, err)

	bodyRdr := bytes.NewReader(bodyJSON)

	req, err := http.NewRequest("POST", url, bodyRdr)
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "user:password")

	return req
}

func readBody(t *testing.T, body io.ReadCloser) map[string]interface{} {
	var res map[string]interface{}
	err := json.NewDecoder(body).Decode(&res)
	assert.NoError(t, err)
	return res
}

func setupDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	assert.NoError(t, err)

	err = seedData(db)
	assert.NoError(t, err)

	return db
}

func seedData(db *sql.DB) error {
	_, err := db.Exec(`
    CREATE TABLE users (
      id INTEGER PRIMARY KEY,
      name TEXT NOT NULL,
      email TEXT
    );
  `)
	if err != nil {
		return err
	}

	for _, v := range testValues {
		if len(v) != 2 {
			_, err = db.Exec(`
      INSERT INTO users (name) VALUES (?)
    `, v[0])
			if err != nil {
				return err
			}
		} else {
			_, err = db.Exec(`
      INSERT INTO users (name, email) VALUES (?, ?)
    `, v[0], v[1])
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func getTableValues(db *sql.DB, tableName string) ([]map[string]interface{}, error) {
	rows, err := db.Query("SELECT * FROM " + tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	values := make([]map[string]interface{}, 0)

	for rows.Next() {
		rowValues := make([]interface{}, len(columns))
		rowPointers := make([]interface{}, len(columns))
		for i := range rowValues {
			rowPointers[i] = &rowValues[i]
		}

		err = rows.Scan(rowPointers...)
		if err != nil {
			return nil, err
		}

		rowMap := make(map[string]interface{})
		for i, colName := range columns {
			val := rowValues[i]
			if b, ok := val.([]byte); ok {
				rowMap[colName] = string(b)
			} else {
				rowMap[colName] = val
			}
		}
		values = append(values, rowMap)
	}

	return values, nil
}
