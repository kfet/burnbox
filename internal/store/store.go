// Package store provides burnbox's blob storage: an in-memory map of
// opaque ciphertext blobs, each with a time-to-live and exactly-once
// read semantics ("burn after reading").
//
// The store knows nothing about encryption. A blob is an opaque []byte.
// IDs are random, URL-safe, and unguessable. Reads are atomic: the
// first GetDel for an id returns the blob and deletes it; every
// subsequent (or concurrent-loser) read sees ErrNotFound.
package store

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

// ErrNotFound is returned when an id is absent, expired, or already
// burned. Callers must not distinguish these cases to avoid leaking
// whether a secret ever existed.
var ErrNotFound = errors.New("not found")

// ErrTooLarge is returned by Put when the blob exceeds the configured
// maximum size.
var ErrTooLarge = errors.New("blob too large")

// randRead is the entropy source for ids; overridable in tests.
var randRead = rand.Read

// idBytes is the number of random bytes per id (192 bits of entropy,
// rendered as 32 url-safe base64 chars).
const idBytes = 24

type entry struct {
	blob   []byte
	expiry time.Time
}

// Store is a concurrency-safe, in-memory, TTL blob store.
type Store struct {
	mu      sync.Mutex
	m       map[string]entry
	maxSize int
	maxTTL  time.Duration
	minTTL  time.Duration

	now    func() time.Time       // injectable clock (tests)
	randID func() (string, error) // injectable id source (tests)
	stop   chan struct{}
	wg     sync.WaitGroup
}

// Options configures a Store. Zero values select sensible defaults.
type Options struct {
	MaxSize int                    // max blob bytes (default 256 KiB)
	MaxTTL  time.Duration          // ceiling for requested TTLs (default 7 days)
	MinTTL  time.Duration          // floor / default TTL (default 1 hour)
	Sweep   time.Duration          // janitor interval (default 1 minute)
	Now     func() time.Time       // injectable clock (tests)
	RandID  func() (string, error) // injectable id source (tests)
}

// New creates a Store and starts its background janitor. Call Close to
// stop the janitor and release resources.
func New(opts Options) *Store {
	if opts.MaxSize <= 0 {
		opts.MaxSize = 256 << 10
	}
	if opts.MaxTTL <= 0 {
		opts.MaxTTL = 7 * 24 * time.Hour
	}
	if opts.MinTTL <= 0 {
		opts.MinTTL = time.Hour
	}
	if opts.Sweep <= 0 {
		opts.Sweep = time.Minute
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.RandID == nil {
		opts.RandID = randomID
	}
	s := &Store{
		m:       make(map[string]entry),
		maxSize: opts.MaxSize,
		maxTTL:  opts.MaxTTL,
		minTTL:  opts.MinTTL,
		now:     opts.Now,
		randID:  opts.RandID,
		stop:    make(chan struct{}),
	}
	s.wg.Add(1)
	go s.janitor(opts.Sweep)
	return s
}

// clampTTL maps a requested TTL into [minTTL, maxTTL]. A non-positive
// request selects minTTL (the default).
func (s *Store) clampTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return s.minTTL
	}
	if ttl < s.minTTL {
		return s.minTTL
	}
	if ttl > s.maxTTL {
		return s.maxTTL
	}
	return ttl
}

// Put stores blob with the given TTL (clamped) and returns its id.
func (s *Store) Put(blob []byte, ttl time.Duration) (string, error) {
	if len(blob) == 0 {
		return "", errors.New("empty blob")
	}
	if len(blob) > s.maxSize {
		return "", ErrTooLarge
	}
	ttl = s.clampTTL(ttl)
	id, err := s.randID()
	if err != nil {
		return "", err
	}
	// Copy so callers can't mutate stored bytes after Put.
	cp := make([]byte, len(blob))
	copy(cp, blob)

	s.mu.Lock()
	s.m[id] = entry{blob: cp, expiry: s.now().Add(ttl)}
	s.mu.Unlock()
	return id, nil
}

// GetDel atomically returns the blob for id and deletes it. The second
// caller (or a concurrent loser) gets ErrNotFound. Expired entries are
// treated as not found and removed.
func (s *Store) GetDel(id string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[id]
	if !ok {
		return nil, ErrNotFound
	}
	delete(s.m, id)
	if !e.expiry.After(s.now()) {
		return nil, ErrNotFound
	}
	return e.blob, nil
}

// Len reports the number of live (not yet swept) entries. Intended for
// tests and metrics.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.m)
}

// MaxSize reports the configured maximum blob size in bytes.
func (s *Store) MaxSize() int { return s.maxSize }

// Close stops the janitor goroutine. Safe to call once.
func (s *Store) Close() {
	close(s.stop)
	s.wg.Wait()
}

func (s *Store) janitor(every time.Duration) {
	defer s.wg.Done()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			s.sweep()
		}
	}
}

// sweep removes expired entries.
func (s *Store) sweep() {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, e := range s.m {
		if !e.expiry.After(now) {
			delete(s.m, id)
		}
	}
}

// randomID returns a url-safe, unpadded, unguessable id.
func randomID() (string, error) {
	b := make([]byte, idBytes)
	if _, err := randRead(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
