package booking

import "context"

type Service struct {
	store BookingStore
}

func NewService(store BookingStore) *Service {
	return &Service{store}
}

func (s *Service) CreateBooking(ctx context.Context, b Booking) (Booking, error) {
	return s.store.CreateBooking(ctx, b)
}

func (s *Service) ListBookings(ctx context.Context, movieID string) ([]Booking, error) {
	return s.store.ListBookings(ctx, movieID)
}

func (s *Service) Confirm(ctx context.Context, sessionID string, userID string) (Booking, error) {
	return s.store.Confirm(ctx, sessionID, userID)
}

func (s *Service) Release(ctx context.Context, sessionID string, userID string) (Booking, error) {
	return s.store.Release(ctx, sessionID, userID)
}

func (s *Service) Subscribe(ctx context.Context, channel string) <-chan string {
	return s.store.Subscribe(ctx, channel)
}

func (s *Service) PublishSeatEvent(ctx context.Context, e SeatEvent) error {
	return s.store.PublishSeatEvent(ctx, e)
}

func (s *Service) CleanupExpiredBookings(ctx context.Context) (int64, error) {
	return s.store.CleanupExpiredBookings(ctx)
}
