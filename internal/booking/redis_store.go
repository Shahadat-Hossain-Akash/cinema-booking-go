package booking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	defaultHoldTTL     = 2 * time.Minute
	pendingSessionsKey = "pending_sessions"
)

type RedisStore struct {
	rdb *redis.Client
	ctx context.Context
}

func NewRedisStore(rdb *redis.Client) *RedisStore {
	return &RedisStore{rdb: rdb, ctx: context.Background()}
}

func sessionKey(id string) string {
	return fmt.Sprintf("session:%s", id)
}

func (s *RedisStore) ListBookings(movieID string) ([]Booking, error) {
	pattern := fmt.Sprintf("seat:%s:*", movieID)

	var bookings []Booking

	iter := s.rdb.Scan(s.ctx, 0, pattern, 0).Iterator()
	for iter.Next(s.ctx) {
		val, err := s.rdb.Get(s.ctx, iter.Val()).Result()
		if err != nil {
			continue
		}
		booking, err := parseSession(val)
		if err != nil {
			continue
		}
		bookings = append(bookings, booking)
	}

	return bookings, nil

}

func parseSession(val string) (Booking, error) {
	var b Booking

	err := json.Unmarshal([]byte(val), &b)
	if err != nil {
		return Booking{}, err
	}
	return Booking{
		ID:      b.ID,
		MovieID: b.MovieID,
		UserID:  b.UserID,
		SeatID:  b.SeatID,
		Status:  b.Status,
	}, nil
}

func (s *RedisStore) hold(b Booking) (Booking, error) {
	id := uuid.New().String()
	now := time.Now()
	key := fmt.Sprintf("seat:%s:%s", b.MovieID, b.SeatID)

	b.ID = id
	val, _ := json.Marshal(b)

	res := s.rdb.SetArgs(s.ctx, key, val, redis.SetArgs{
		TTL:  defaultHoldTTL,
		Mode: "NX",
	})

	if err := res.Err(); err != nil {
		return Booking{}, fmt.Errorf("hold: set seat key: %w", err)
	}

	if res.Val() != "OK" {
		return Booking{}, ErrSeatAlreadyBooked
	}

	if err := s.rdb.Set(s.ctx, sessionKey(id), key, defaultHoldTTL).Err(); err != nil {
		s.rdb.Del(s.ctx, key) // rollback seat lock
		return Booking{}, fmt.Errorf("hold: set session key: %w", err)
	}

	// Track this session in pending_sessions set for efficient cleanup
	if err := s.rdb.SAdd(s.ctx, "pending_sessions", id).Err(); err != nil {
		log.Printf("hold: warning - failed to add session to pending set: %v", err)
		// Don't fail the hold operation if tracking fails
	}

	return Booking{
		ID:        id,
		MovieID:   b.MovieID,
		UserID:    b.UserID,
		SeatID:    b.SeatID,
		Status:    "held",
		ExpiresAt: now.Add(defaultHoldTTL),
	}, nil
}

func (s *RedisStore) CreateBooking(b Booking) (Booking, error) {
	session, err := s.hold(b)

	if err != nil {
		return Booking{}, err
	}

	log.Printf("Booking created: %s for movie %s, seat %s", session.ID, session.MovieID, session.SeatID)

	return session, nil
}

func (s *RedisStore) getSession(ctx context.Context, sessionID string, userID string) (Booking, string, error) {
	sk, err := s.rdb.Get(ctx, sessionKey(sessionID)).Result()

	if errors.Is(err, redis.Nil) {
		return Booking{}, "", ErrBookingNotFound
	} else if err != nil {
		return Booking{}, "", ErrBookingNotFound
	}

	val, err := s.rdb.Get(ctx, sk).Result()
	if err != nil {
		return Booking{}, "", ErrBookingNotFound
	}

	booking, err := parseSession(val)

	if errors.Is(err, redis.Nil) {
		return Booking{}, "", ErrBookingNotFound
	} else if err != nil {
		return Booking{}, "", ErrBookingNotFound
	}
	if booking.UserID != userID {
		return Booking{}, "", ErrBookingNotFound
	}
	return booking, sk, nil

}

func (s *RedisStore) Confirm(ctx context.Context, sessionID string, userID string) (Booking, error) {
	booking, sk, err := s.getSession(ctx, sessionID, userID)
	if err != nil {
		return Booking{}, err
	}
	if err := s.rdb.Persist(ctx, sessionKey(sessionID)).Err(); err != nil {
		return Booking{}, fmt.Errorf("confirm: persist session key: %w", err)
	}
	if err := s.rdb.Persist(ctx, sk).Err(); err != nil {
		return Booking{}, fmt.Errorf("confirm: persist seat key: %w", err)
	}

	booking.Status = "confirmed"
	data := Booking{
		ID:      booking.ID,
		MovieID: booking.MovieID,
		UserID:  booking.UserID,
		SeatID:  booking.SeatID,
		Status:  booking.Status,
	}
	val, err := json.Marshal(data)
	if err != nil {
		return Booking{}, fmt.Errorf("confirm: marshal booking data: %w", err)
	}
	if err := s.rdb.Set(ctx, sk, val, 0).Err(); err != nil {
		return Booking{}, fmt.Errorf("confirm: update seat key: %w", err)
	}

	log.Printf("Booking confirmed: %s for movie %s, seat %s", booking.ID, booking.MovieID, booking.SeatID)

	// Remove from pending_sessions since it's confirmed
	if err := s.rdb.SRem(ctx, "pending_sessions", sessionID).Err(); err != nil {
		log.Printf("confirm: warning - failed to remove session from pending set: %v", err)
	}

	return booking, nil
}

func (s *RedisStore) Release(ctx context.Context, sessionID string, userID string) error {
	_, sk, err := s.getSession(ctx, sessionID, userID)
	if err != nil {
		return err
	}

	if err := s.rdb.Del(ctx, sk, sessionKey(sessionID)).Err(); err != nil {
		return fmt.Errorf("release: delete keys: %w", err)
	}

	// Remove from pending_sessions
	if err := s.rdb.SRem(ctx, "pending_sessions", sessionID).Err(); err != nil {
		log.Printf("release: warning - failed to remove session from pending set: %v", err)
	}

	log.Printf("Booking released: %s for user %s", sessionID, userID)
	return nil
}

// CleanupExpiredBookings removes any expired hold sessions
func (s *RedisStore) CleanupExpiredBookings(ctx context.Context) (int64, error) {
	sessions, err := s.rdb.SMembers(ctx, pendingSessionsKey).Result()
	if err != nil {
		return 0, fmt.Errorf("cleanup: failed to get pending sessions: %w", err)
	}

	var deletedCount int64
	for _, sessionID := range sessions {
		cleaned, err := s.cleanupSession(ctx, sessionID)
		if err != nil {
			log.Printf("cleanup: session %s: %v", sessionID, err)
			continue
		}
		if cleaned {
			deletedCount++
			log.Printf("cleanup: removed expired session %s", sessionID)
		}
	}

	return deletedCount, nil
}

// cleanupSession inspects a single session and removes it if expired.
// Returns true if the session was cleaned up.
func (s *RedisStore) cleanupSession(ctx context.Context, sessionID string) (bool, error) {
	seatKey, err := s.rdb.Get(ctx, sessionKey(sessionID)).Result()
	if errors.Is(err, redis.Nil) {
		// Session key expired naturally — just remove from the tracking set.
		return s.removePendingSession(ctx, sessionID)
	}
	if err != nil {
		return false, fmt.Errorf("reading session key: %w", err)
	}

	exists, err := s.rdb.Exists(ctx, seatKey).Result()
	if err != nil {
		return false, fmt.Errorf("checking seat key %s: %w", seatKey, err)
	}

	if exists > 0 {
		return false, nil // Seat still held — nothing to do.
	}

	// Seat key is gone but session key lingers — delete both atomically.
	return s.removeOrphanedSession(ctx, sessionID)
}

// removePendingSession removes a naturally-expired session from the tracking set.
func (s *RedisStore) removePendingSession(ctx context.Context, sessionID string) (bool, error) {
	if err := s.rdb.SRem(ctx, pendingSessionsKey, sessionID).Err(); err != nil {
		return false, fmt.Errorf("removing session from pending set: %w", err)
	}
	return true, nil
}

// removeOrphanedSession deletes a session whose seat key no longer exists.
func (s *RedisStore) removeOrphanedSession(ctx context.Context, sessionID string) (bool, error) {
	_, err := s.rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Del(ctx, sessionKey(sessionID))
		pipe.SRem(ctx, pendingSessionsKey, sessionID)
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("deleting orphaned session: %w", err)
	}
	return true, nil
}
