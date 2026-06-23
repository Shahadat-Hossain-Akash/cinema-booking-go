package booking

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"movie-booking-go/internal/utils"
	"net/http"
	"sync"
)

type client struct {
	ch chan SeatEvent
}

type clients map[string]map[*client]struct{}

type Broker struct {
	mu      sync.RWMutex
	clients clients
	store   BookingStore
}

func NewBroker(store BookingStore) *Broker {
	return &Broker{clients: make(clients), store: store}
}

func (b *Broker) subscribe(movieID string) (*client, func()) {
	c := &client{
		ch: make(chan SeatEvent, 16),
	}

	b.mu.Lock()
	if b.clients[movieID] == nil {
		b.clients[movieID] = make(map[*client]struct{})
	}
	b.clients[movieID][c] = struct{}{}
	b.mu.Unlock()

	cancel := func() {
		b.mu.Lock()
		delete(b.clients[movieID], c)
		if len(b.clients[movieID]) == 0 {
			delete(b.clients, movieID)
		}
		b.mu.Unlock()
		close(c.ch)
	}

	return c, cancel

}

func (b *Broker) ServeSSE(w http.ResponseWriter, r *http.Request) {
	movieID := r.PathValue("movieID")

	if movieID == "" {
		http.Error(w, "movieID is required", http.StatusBadRequest)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	utils.SetSSEHeader(w)

	c, cancel := b.subscribe(movieID)
	defer cancel()

	fmt.Fprintf(w, "event: connected\ndata: {\"movie_id\":%q}\n\n", movieID)
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case e, ok := <-c.ch:
			if !ok {
				return
			}

			data, err := json.Marshal(e)
			if err != nil {
				log.Printf("[sse] marshal error: %v", err)
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, data)
			flusher.Flush()
		}
	}

}

func (b *Broker) Publish(e SeatEvent) {
	b.mu.Lock()
	clients := b.clients[e.MovieID]
	b.mu.Unlock()

	for client := range clients {
		select {
		case client.ch <- e:
		default:
			log.Printf("[sse] dropped event for slow client movie=%s", e.MovieID)

		}
	}
}

func (b *Broker) StartRedisListener(ctx context.Context) {
	ch := b.store.Subscribe(ctx, RedisSeatEvents)

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				log.Println("[sse] redis subscriber channel closed")
				return
			}
			var s SeatEvent
			if err := json.Unmarshal([]byte(msg), &s); err != nil {
				log.Printf("[sse] bad redis message: %v", err)
				continue
			}

			b.Publish(s)
		}
	}
}
