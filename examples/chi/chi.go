package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/joelseq/sqliteadmin-go"
	_ "github.com/mattn/go-sqlite3"
)

const addr string = ":8080"

func main() {
	db, err := sql.Open("sqlite3", "sakila.db")
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	sh := sqliteadmin.NewHandler(db, "user", "password")

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
	r.Post("/admin", sh.Handle)

	fmt.Printf(`

 _______         __ __ __          _______     __            __        
|     __|.-----.|  |__|  |_.-----.|   _   |.--|  |.--------.|__|.-----.
|__     ||  _  ||  |  |   _|  -__||       ||  _  ||        ||  ||     |
|_______||__   ||__|__|____|_____||___|___||_____||__|__|__||__||__|__|
            |__|                                                       

`)
	fmt.Printf("--> Starting server on %s\n", addr)
	http.ListenAndServe(":8080", r)
}
