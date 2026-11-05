package service

import (
	"context"
	"sync"
)

// Definition locks cover short creation/delivery transactions. Long-running
// workflows additionally hold durable protection through server_jobs.
var definitionLocks = struct {
	sync.Mutex
	entries map[string]*deliveryLock
}{entries: make(map[string]*deliveryLock)}

type heldDefinitionsKey struct{}

func holdDefinition(ctx context.Context, id string) (context.Context, func(), error) {
	held, _ := ctx.Value(heldDefinitionsKey{}).(map[string]bool)
	if held[id] {
		return ctx, func() {}, nil
	}

	definitionLocks.Lock()

	entry := definitionLocks.entries[id]
	if entry == nil {
		entry = &deliveryLock{gate: make(chan struct{}, 1)}
		definitionLocks.entries[id] = entry
	}

	entry.users++
	definitionLocks.Unlock()

	unref := func() {
		definitionLocks.Lock()

		entry.users--
		if entry.users == 0 {
			delete(definitionLocks.entries, id)
		}
		definitionLocks.Unlock()
	}

	select {
	case entry.gate <- struct{}{}:
	case <-ctx.Done():
		unref()
		return ctx, nil, ctx.Err()
	}

	next := make(map[string]bool, len(held)+1)
	for key, value := range held {
		next[key] = value
	}

	next[id] = true

	var once sync.Once

	release := func() { once.Do(func() { <-entry.gate; unref() }) }

	return context.WithValue(ctx, heldDefinitionsKey{}, next), release, nil
}
