package store

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func newTestClock(t time.Time) (func() time.Time, func(d time.Duration)) {
	var mu sync.Mutex
	cur := t
	now := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return cur
	}
	adv := func(d time.Duration) {
		mu.Lock()
		defer mu.Unlock()
		cur = cur.Add(d)
	}
	return now, adv
}

func TestPutGetDelRoundTrip(t *testing.T) {
	s := New(Options{})
	defer s.Close()

	id, err := s.Put([]byte("ciphertext"), time.Minute)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if id == "" {
		t.Fatal("empty id")
	}
	if s.Len() != 1 {
		t.Fatalf("Len = %d, want 1", s.Len())
	}
	got, err := s.GetDel(id)
	if err != nil {
		t.Fatalf("GetDel: %v", err)
	}
	if string(got) != "ciphertext" {
		t.Fatalf("got %q", got)
	}
	// Burned: second read is not found.
	if _, err := s.GetDel(id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second GetDel err = %v, want ErrNotFound", err)
	}
	if s.Len() != 0 {
		t.Fatalf("Len = %d, want 0", s.Len())
	}
}

func TestPutCopiesBlob(t *testing.T) {
	s := New(Options{})
	defer s.Close()
	blob := []byte("abc")
	id, err := s.Put(blob, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	blob[0] = 'X' // mutate caller's slice
	got, err := s.GetDel(id)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "abc" {
		t.Fatalf("stored blob was mutated: %q", got)
	}
}

func TestPutRejects(t *testing.T) {
	s := New(Options{MaxSize: 4})
	defer s.Close()
	if _, err := s.Put(nil, time.Minute); err == nil {
		t.Fatal("want error for empty blob")
	}
	if _, err := s.Put([]byte("toolong"), time.Minute); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("want ErrTooLarge, got %v", err)
	}
	if s.MaxSize() != 4 {
		t.Fatalf("MaxSize = %d", s.MaxSize())
	}
}

func TestPutRandIDError(t *testing.T) {
	sentinel := errors.New("boom")
	s := New(Options{RandID: func() (string, error) { return "", sentinel }})
	defer s.Close()
	if _, err := s.Put([]byte("x"), time.Minute); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel", err)
	}
}

func TestGetDelNotFound(t *testing.T) {
	s := New(Options{})
	defer s.Close()
	if _, err := s.GetDel("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestGetDelExpired(t *testing.T) {
	now, adv := newTestClock(time.Unix(1_000_000, 0))
	s := New(Options{Now: now, MinTTL: time.Minute})
	defer s.Close()
	id, err := s.Put([]byte("x"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	adv(2 * time.Minute) // past expiry
	if _, err := s.GetDel(id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	// Entry was deleted even though expired.
	if s.Len() != 0 {
		t.Fatalf("Len = %d", s.Len())
	}
}

func TestClampTTL(t *testing.T) {
	s := New(Options{MinTTL: time.Minute, MaxTTL: time.Hour})
	defer s.Close()
	cases := []struct {
		in, want time.Duration
	}{
		{0, time.Minute},                // default
		{-5, time.Minute},               // negative -> default
		{30 * time.Second, time.Minute}, // below floor
		{10 * time.Minute, 10 * time.Minute},
		{2 * time.Hour, time.Hour}, // above ceiling
	}
	for _, c := range cases {
		if got := s.clampTTL(c.in); got != c.want {
			t.Fatalf("clampTTL(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestSweepRemovesExpired(t *testing.T) {
	now, adv := newTestClock(time.Unix(2_000_000, 0))
	s := New(Options{Now: now, MinTTL: time.Minute})
	defer s.Close()
	if _, err := s.Put([]byte("a"), time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put([]byte("b"), time.Hour); err != nil {
		t.Fatal(err)
	}
	adv(2 * time.Minute)
	s.sweep()
	if s.Len() != 1 { // "a" expired, "b" survives
		t.Fatalf("Len = %d, want 1", s.Len())
	}
}

func TestJanitorTickerSweeps(t *testing.T) {
	now, adv := newTestClock(time.Unix(3_000_000, 0))
	s := New(Options{Now: now, MinTTL: time.Millisecond, Sweep: time.Millisecond})
	defer s.Close()
	if _, err := s.Put([]byte("a"), time.Millisecond); err != nil {
		t.Fatal(err)
	}
	adv(time.Second) // make it expired relative to the injected clock
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s.Len() == 0 {
			return // janitor ticker swept it
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("janitor did not sweep expired entry")
}

func TestRandomIDUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id, err := randomID()
		if err != nil {
			t.Fatal(err)
		}
		if id == "" || seen[id] {
			t.Fatalf("bad id %q", id)
		}
		seen[id] = true
	}
}

func TestRandomIDError(t *testing.T) {
	orig := randRead
	defer func() { randRead = orig }()
	randRead = func([]byte) (int, error) { return 0, errors.New("no entropy") }
	if _, err := randomID(); err == nil {
		t.Fatal("want error from randomID when entropy source fails")
	}
}
