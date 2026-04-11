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
	defaultHoldTTL = 2 * time.Minute
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

	return nil
}
