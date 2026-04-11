package booking

import (
	"sync"
	"sync/atomic"
	"testing"

	"movie-booking-go/internal/adapters/redis"

	"github.com/google/uuid"
)

func TestConcurrentStoreExactlyOneWin(t *testing.T) {
	store := NewRedisStore(redis.NewClient("localhost:6379"))
	service := NewService(store)

	const numGoroutines = 100_000

	var (
		success  atomic.Int64
		failures atomic.Int64
		wg       sync.WaitGroup
	)

	wg.Add(numGoroutines)

	for i := range numGoroutines {
		go func(i int) {
			defer wg.Done()

			booking := Booking{
				MovieID: "movie1",
				SeatID:  "A1",
				UserID:  uuid.New().String(),
			}
			_, err := service.CreateBooking(booking)
			if err == nil {
				success.Add(1)
			} else {
				failures.Add(1)
			}
		}(i)
	}

	wg.Wait()

	if got := success.Load(); got != 1 {
		t.Errorf("Expected exactly 1 successful booking, got %d", success.Load())
	}
	if got := failures.Load(); got != int64(numGoroutines-1) {
		t.Errorf("Expected %d failures, got %d", numGoroutines-1, got)
	}
}
