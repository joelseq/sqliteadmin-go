package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/joelseq/sqliteadmin-go"
	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"
)

var port uint

func init() {
	serveCmd.Flags().UintVarP(&port, "port", "p", 8080, "Port to run server on")
	rootCmd.AddCommand(serveCmd)
}

var serveCmd = &cobra.Command{
	Use:   "serve [DB_PATH]",
	Short: "Spin up an HTTP server to serve requests to the SQLiteAdmin UI",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		dbPath := args[0]
		username := os.Getenv("SQLITEADMIN_USERNAME")
		password := os.Getenv("SQLITEADMIN_PASSWORD")

		r := getRouter(dbPath, username, password)

		addr := fmt.Sprintf(":%d", port)

		// Create a done channel to signal when the shutdown is complete
		done := make(chan bool, 1)

		httpServer := newHTTPServer(addr, r)

		// Run graceful shutdown in a separate goroutine
		go gracefulShutdown(httpServer, done)

		err := httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			panic(fmt.Sprintf("http server error: %s", err))
		}

		// Wait for the graceful shutdown to complete
		<-done
		log.Println("Graceful shutdown complete.")
	},
}

func newHTTPServer(addr string, mux *chi.Mux) *http.Server {
	return &http.Server{
		Addr:         addr,
		Handler:      mux,
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
}

func getRouter(dbPath, username, password string) *chi.Mux {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}

	logger := slog.Default()

	// Setup the handler for SQLiteAdmin
	config := sqliteadmin.Config{
		DB:       db,
		Username: username,
		Password: password,
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
	r.Post("/", sh.HandlePost)

	return r
}

func gracefulShutdown(apiServer *http.Server, done chan bool) {
	// Create context that listens for the interrupt signal from the OS.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Listen for the interrupt signal.
	<-ctx.Done()

	log.Println("shutting down gracefully, press Ctrl+C again to force")

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := apiServer.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown with error: %v", err)
	}

	log.Println("Server exiting")

	// Notify the main goroutine that the shutdown is complete
	done <- true
}
