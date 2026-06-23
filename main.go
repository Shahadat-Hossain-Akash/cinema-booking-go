package main

import (
	"context"
	"log"
	"movie-booking-go/internal/adapters/redis"
	"os/signal"
	"syscall"
	"time"

	"movie-booking-go/internal/booking"
	"net/http"
)

func main() {

	mux := http.NewServeMux()

	rs := redis.NewClient("localhost:6379")
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

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
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
