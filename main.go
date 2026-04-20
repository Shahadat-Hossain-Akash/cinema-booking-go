package main

import (
	"context"
	"log"
	"movie-booking-go/internal/adapters/redis"
	"time"

	"movie-booking-go/internal/booking"
	"net/http"
)

func main() {

	mux := http.NewServeMux()

	rs := redis.NewClient("localhost:6379")
	store := booking.NewRedisStore(rs)
	service := booking.NewService(store)
	handler := booking.NewHandler(service)

	mux.HandleFunc("GET /movies", handler.ListMovies)
	mux.HandleFunc("GET /movies/{movieID}/seats", handler.ListBookings)
	mux.HandleFunc("POST /movies/{movieID}/seat/{seatID}/hold", handler.HoldSeat)

	mux.HandleFunc("PUT /sessions/{sessionID}/confirm", handler.ConfirmSeat)
	mux.HandleFunc("DELETE /sessions/{sessionID}/release", handler.ReleaseSeat)

	// Background cleanup job: runs every 1 minute to clean up expired bookings
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			deleted, err := store.CleanupExpiredBookings(ctx)
			cancel()

			if err != nil {
				log.Printf("Cleanup job error: %v", err)
			} else if deleted > 0 {
				log.Printf("Cleanup job removed %d expired bookings", deleted)
			}
		}
	}()

	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("Server failed to start: %v\n", err)
	}

}
