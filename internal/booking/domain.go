package booking

import (
	"context"
	"errors"
	"time"
)

var (
	ErrBookingNotFound   = errors.New("booking not found")
	ErrSeatUnavailable   = errors.New("seat unavailable")
	ErrSeatAlreadyBooked = errors.New("seat already booked")
)

type Booking struct {
	ID        string    `json:"id"`
	MovieID   string    `json:"movie_id"`
	UserID    string    `json:"user_id"`
	SeatID    string    `json:"seat_id"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
}

type BookingStore interface {
	CreateBooking(b Booking) (Booking, error)
	ListBookings(movieID string) ([]Booking, error)
	Confirm(ctx context.Context, sessionID string, userID string) (Booking, error)
	Release(ctx context.Context, sessionID string, userID string) error
}
