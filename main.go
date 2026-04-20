package main

import (
	"log"
	"movie-booking-go/internal/adapters/redis"

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

	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("Server failed to start: %v\n", err)
	}

}
