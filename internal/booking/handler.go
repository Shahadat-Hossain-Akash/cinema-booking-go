package booking

import (
	"movie-booking-go/internal/utils"
	"net/http"
)

type Handler struct {
	service *Service
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
