package booking

import "context"

type Service struct {
	store BookingStore
}

func NewService(store BookingStore) *Service {
	return &Service{store}
}

func (s *Service) CreateBooking(b Booking) (Booking, error) {
	return s.store.CreateBooking(b)
}

func (s *Service) ListBookings(movieID string) ([]Booking, error) {
	return s.store.ListBookings(movieID)
}

func (s *Service) Confirm(ctx context.Context, sessionID string, userID string) (Booking, error) {
	return s.store.Confirm(ctx, sessionID, userID)
}

func (s *Service) Release(ctx context.Context, sessionID string, userID string) error {
	return s.store.Release(ctx, sessionID, userID)
}

// func (s *Service) CleanupExpiredBookings(ctx context.Context) (int64, error) {
// 	return s.store.CleanupExpiredBookings(ctx)
// }
