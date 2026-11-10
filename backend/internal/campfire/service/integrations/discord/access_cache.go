package discord

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"sync"
	"time"
)

type accessEntry struct {
	value   Access
	expires time.Time
}

type accessFlight struct {
	done  chan struct{}
	value Access
	err   error
}

type accessCache struct {
	mu         sync.Mutex
	entries    map[string]accessEntry
	flights    map[string]*accessFlight
	generation uint64
}

func accessKey(token, guild string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:]) + "/" + guild
}

func cloneAccess(value Access) Access {
	value.Channels = slices.Clone(value.Channels)
	for i := range value.Channels {
		value.Channels[i].Missing = slices.Clone(value.Channels[i].Missing)
	}

	return value
}

func (c *Client) invalidateAccess() {
	c.access.mu.Lock()
	defer c.access.mu.Unlock()

	c.access.entries = nil
	c.access.generation++
}

// CheckDelivery shares short-lived permission snapshots between worker slots.
// Setup checks remain fresh, and native authorization failures invalidate them.
func (c *Client) CheckDelivery(ctx context.Context, token, guild string) (Access, error) {
	key := accessKey(token, guild)

	c.access.mu.Lock()
	if entry, ok := c.access.entries[key]; ok && time.Now().Before(entry.expires) {
		c.access.mu.Unlock()
		return cloneAccess(entry.value), nil
	}

	if flight := c.access.flights[key]; flight != nil {
		c.access.mu.Unlock()

		select {
		case <-ctx.Done():
			return Access{}, ctx.Err()
		case <-flight.done:
			return cloneAccess(flight.value), flight.err
		}
	}

	flight := &accessFlight{done: make(chan struct{})}
	if c.access.flights == nil {
		c.access.flights = make(map[string]*accessFlight)
	}

	c.access.flights[key] = flight
	c.access.mu.Unlock()
	flight.value, flight.err = c.Check(ctx, token, guild)
	c.access.mu.Lock()
	delete(c.access.flights, key)
	close(flight.done)
	c.access.mu.Unlock()

	return cloneAccess(flight.value), flight.err
}
