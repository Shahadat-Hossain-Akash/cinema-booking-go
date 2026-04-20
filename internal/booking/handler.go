package booking

import (
	"encoding/json"
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

	bookings, err := h.service.ListBookings(movieID)
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

	booking, err := h.service.CreateBooking(payload)
	if err != nil {
		log.Println(err)
		return
	}

	res := holdResponse{
		SessionID: booking.ID,
		MovieID:   movieID,
		SeatID:    seatID,
		ExpiresAt: booking.ExpiresAt.Format(time.RFC3339),
	}

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
	if err := h.service.Release(r.Context(), sessionID, req.UserID); err != nil {
		log.Println(err)
		http.Error(w, "Failed to release booking", http.StatusInternalServerError)
		return
	}
	utils.WriteJSON(w, http.StatusOK, map[string]string{"message": "Booking released successfully"})
}
