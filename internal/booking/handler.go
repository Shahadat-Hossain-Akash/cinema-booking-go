package booking

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"movie-booking-go/internal/utils"
	"net/http"
	"time"
)

type Handler struct {
	service *Service
}
type holdRequest struct {
	UserID string `json:"user_id"`
}

type holdResponse struct {
	SessionID string `json:"session_id"`
	MovieID   string `json:"movie_id"`
	SeatID    string `json:"seat_id"`
	ExpiresAt string `json:"expires_at"`
}

type confirmSeatResponse struct {
	SessionID string `json:"session_id"`
	MovieID   string `json:"movie_id"`
	UserID    string `json:"user_id"`
	SeatID    string `json:"seat_id"`
	Status    string `json:"status"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

func NewHandler(s *Service) *Handler {
	return &Handler{service: s}
}

func (h *Handler) ServeSSE(broker *Broker) http.HandlerFunc {
	return broker.ServeSSE
}

func (h *Handler) emit(r *http.Request, e SeatEvent) {
	// Publish event asynchronously to avoid blocking the response
	go func() {
		if err := h.service.PublishSeatEvent(context.Background(), e); err != nil {
			log.Printf("[sse] failed to publish %s event: %v", e.Type, err)
		}
	}()
}

func (h *Handler) ListMovies(w http.ResponseWriter, r *http.Request) {
	movie := []MovieResponse{
		{ID: "1", Title: "Inception", Rows: 5, SeatsPerRow: 10, TotalSeats: 50},
		{ID: "2", Title: "The Matrix", Rows: 4, SeatsPerRow: 8, TotalSeats: 32},
	}

	utils.WriteJSON(w, http.StatusOK, movie)
}

func (h *Handler) ListBookings(w http.ResponseWriter, r *http.Request) {
	movieID := r.PathValue("movieID")
	if movieID == "" {
		http.Error(w, "Missing movieID parameter", http.StatusBadRequest)
		return
	}

	bookings, err := h.service.ListBookings(r.Context(), movieID)
	if err != nil {
		http.Error(w, "Failed to list bookings", http.StatusInternalServerError)
		return
	}

	seats := make([]seatInfo, 0, len(bookings))
	for _, b := range bookings {
		seat := seatInfo{
			SeatID:    b.SeatID,
			UserID:    b.UserID,
			Booked:    true,
			Confirmed: b.Status == "confirmed",
		}
		seats = append(seats, seat)
	}

	utils.WriteJSON(w, http.StatusOK, seats)
}

func (h *Handler) HoldSeat(w http.ResponseWriter, r *http.Request) {
	movieID := r.PathValue("movieID")
	seatID := r.PathValue("seatID")

	if movieID == "" || seatID == "" {
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}

	var req holdRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.UserID == "" {
		http.Error(w, "Missing user_id in request body", http.StatusBadRequest)
		return
	}

	payload := Booking{
		MovieID: movieID,
		SeatID:  seatID,
		UserID:  req.UserID,
	}

	booking, err := h.service.CreateBooking(r.Context(), payload)
	if err != nil {
		log.Println(err)
		if err == ErrSeatAlreadyBooked {
			http.Error(w, "Seat already booked", http.StatusConflict)
		} else {
			http.Error(w, "Failed to hold seat", http.StatusInternalServerError)
		}
		return
	}

	res := holdResponse{
		SessionID: booking.ID,
		MovieID:   movieID,
		SeatID:    seatID,
		ExpiresAt: booking.ExpiresAt.Format(time.RFC3339),
	}

	h.emit(r, SeatEvent{
		Type:    EventSeatHeld,
		MovieID: movieID,
		SeatID:  seatID,
		UserID:  req.UserID,
	})

	utils.WriteJSON(w, http.StatusCreated, res)

}

func (h *Handler) ConfirmSeat(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")
	if sessionID == "" {
		http.Error(w, "Missing sessionID parameter", http.StatusBadRequest)
		return
	}

	var req holdRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.UserID == "" {
		http.Error(w, "Missing user_id in request body", http.StatusBadRequest)
		return
	}

	booking, err := h.service.Confirm(r.Context(), sessionID, req.UserID)
	if err != nil {
		log.Println(err)
		http.Error(w, "Failed to confirm booking", http.StatusInternalServerError)
		return
	}

	res := confirmSeatResponse{
		SessionID: booking.ID,
		MovieID:   booking.MovieID,
		UserID:    booking.UserID,
		SeatID:    booking.SeatID,
		Status:    booking.Status,
	}

	h.emit(r, SeatEvent{
		Type:    EventSeatConfirmed,
		MovieID: booking.MovieID,
		SeatID:  booking.SeatID,
		UserID:  booking.UserID,
	})

	utils.WriteJSON(w, http.StatusAccepted, res)

}

func (h *Handler) ReleaseSeat(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")
	if sessionID == "" {
		http.Error(w, "Missing session_id parameter", http.StatusBadRequest)
		return
	}
	var req holdRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.UserID == "" {
		http.Error(w, "Missing user_id in request body", http.StatusBadRequest)
		return
	}
	booking, err := h.service.Release(r.Context(), sessionID, req.UserID)
	if err != nil {
		log.Println(err)
		http.Error(w, "Failed to release booking", http.StatusInternalServerError)
		return
	}

	h.emit(r, SeatEvent{
		Type:    EventSeatReleased,
		MovieID: booking.MovieID,
		SeatID:  booking.SeatID,
		UserID:  booking.UserID,
	})

	utils.WriteJSON(w, http.StatusOK, map[string]string{"message": fmt.Sprintf("Booking release successfully for %s", booking.SeatID)})
}
