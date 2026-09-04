package main

import (
	"context"
	"log"
	"movie-booking-go/internal/adapters/redis"
	"os"
	"os/signal"
	"syscall"
	"time"

	"movie-booking-go/internal/booking"
	"net/http"

	"github.com/joho/godotenv"
)

// corsMiddleware sets CORS headers for the configured frontend origin and
// short-circuits preflight OPTIONS requests. Browsers send preflight for
// POST/PUT/DELETE with a JSON body, and EventSource (SSE) is cross-origin
// too, so this wraps the whole mux rather than individual routes.
func corsMiddleware(next http.Handler, allowedOrigin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println(".env not found, using existing environment variables")
	}

	mux := http.NewServeMux()

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		log.Fatal("REDIS_URL is required")
	}

	rs := redis.NewClient(redisURL)
	defer rs.Close()

	store := booking.NewRedisStore(rs)
	service := booking.NewService(store)
	broker := booking.NewBroker(store)
	handler := booking.NewHandler(service)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go broker.StartRedisListener(ctx)

	mux.HandleFunc("GET /movies", handler.ListMovies)
	mux.HandleFunc("GET /movies/{movieID}/seats", handler.ListBookings)
	mux.HandleFunc("POST /movies/{movieID}/seat/{seatID}/hold", handler.HoldSeat)

	mux.HandleFunc("PUT /sessions/{sessionID}/confirm", handler.ConfirmSeat)
	mux.HandleFunc("DELETE /sessions/{sessionID}/release", handler.ReleaseSeat)

	mux.HandleFunc("/movies/{movieID}/seat/events", handler.ServeSSE(broker))

	// Background cleanup job: runs every 1 minute to clean up expired bookings
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			deleted, err := service.CleanupExpiredBookings(ctx)
			cancel()

			if err != nil {
				log.Printf("Cleanup job error: %v", err)
			} else if deleted > 0 {
				log.Printf("Cleanup job removed %d expired bookings", deleted)
			}
		}
	}()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	allowedOrigin := os.Getenv("ALLOWED_ORIGIN")
	if allowedOrigin == "" {
		allowedOrigin = "*"
		log.Println("ALLOWED_ORIGIN not set, allowing all origins (fine for local dev, set explicitly in production)")
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: corsMiddleware(mux, allowedOrigin),
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Server started on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v\n", err)
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	log.Println("Shutdown signal received, gracefully shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v\n", err)
	}

}
