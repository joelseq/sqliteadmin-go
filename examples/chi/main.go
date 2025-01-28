package main

import (
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/joelseq/sqliteadmin-go"
	_ "github.com/mattn/go-sqlite3"
)

const addr string = ":8080"

func main() {
	db, err := sql.Open("sqlite3", "chinook.db")
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}

	logger := slog.Default()

	// Setup the handler for SQLiteAdmin
	config := sqliteadmin.Config{
		Db:       db,
		Username: "user",
		Password: "password",
		Logger:   logger,
	}
	sh := sqliteadmin.NewHandler(config)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("welcome"))
	})
	// Setup the handler for SQLiteAdmin
	r.Post("/admin", sh.HandlePost)

	fmt.Printf("--> Starting server on %s\n", addr)
	http.ListenAndServe(":8080", r)
}
