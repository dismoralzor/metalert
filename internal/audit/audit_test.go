package audit

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

// fakeObserver - подставной приёмник для проверки рассылки Subject.
type fakeObserver struct {
	events []Event
	err    error
}

func (f *fakeObserver) Notify(e Event) error {
	f.events = append(f.events, e)
	return f.err
}

func TestFileObserver_Notify(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	obs, err := NewFileObserver(path)
	if err != nil {
		t.Fatalf("NewFileObserver: %v", err)
	}

	ev1 := Event{TS: 100, Metrics: []string{"Alloc"}, IPAddress: "1.2.3.4"}
	ev2 := Event{TS: 200, Metrics: []string{"PollCount", "Random"}, IPAddress: "5.6.7.8"}

	if err := obs.Notify(ev1); err != nil {
		t.Fatalf("Notify ev1: %v", err)
	}
	if err := obs.Notify(ev2); err != nil {
		t.Fatalf("Notify ev2: %v", err)
	}
	// Закрываем до чтения: файл держится открытым до Close.
	if err := obs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	defer f.Close()

	var got []Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			t.Fatalf("line is not valid JSON %q: %v", line, err)
		}
		got = append(got, e)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}

	// Два вызова - две строки.
	if len(got) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(got))
	}
	if got[0].TS != ev1.TS || got[0].IPAddress != ev1.IPAddress || len(got[0].Metrics) != 1 {
		t.Errorf("line 1 mismatch: %+v", got[0])
	}
	if got[1].TS != ev2.TS || got[1].IPAddress != ev2.IPAddress || len(got[1].Metrics) != 2 {
		t.Errorf("line 2 mismatch: %+v", got[1])
	}
}

func TestHTTPObserver_Notify(t *testing.T) {
	type captured struct {
		method      string
		contentType string
		body        Event
	}
	got := make(chan captured, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var e Event
		_ = json.Unmarshal(body, &e)
		got <- captured{
			method:      r.Method,
			contentType: r.Header.Get("Content-Type"),
			body:        e,
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	obs := NewHTTPObserver(srv.URL)
	ev := Event{TS: 42, Metrics: []string{"Alloc"}, IPAddress: "9.9.9.9"}
	if err := obs.Notify(ev); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	c := <-got
	if c.method != http.MethodPost {
		t.Errorf("method = %q, want POST", c.method)
	}
	if c.contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", c.contentType)
	}
	if c.body.TS != ev.TS || c.body.IPAddress != ev.IPAddress || len(c.body.Metrics) != 1 {
		t.Errorf("body mismatch: %+v", c.body)
	}
}

func TestSubject_NotifyAll(t *testing.T) {
	t.Run("single observer receives event", func(t *testing.T) {
		s := NewSubject(zap.NewNop())
		obs := &fakeObserver{}
		s.Subscribe(obs)

		ev := Event{TS: 1, Metrics: []string{"m"}, IPAddress: "ip"}
		s.NotifyAll(ev)

		if len(obs.events) != 1 || obs.events[0].TS != 1 {
			t.Fatalf("observer did not receive the event: %+v", obs.events)
		}
	})

	t.Run("all observers receive event", func(t *testing.T) {
		s := NewSubject(zap.NewNop())
		a, b := &fakeObserver{}, &fakeObserver{}
		s.Subscribe(a)
		s.Subscribe(b)

		s.NotifyAll(Event{TS: 2})

		if len(a.events) != 1 || len(b.events) != 1 {
			t.Fatalf("not all observers notified: a=%d b=%d", len(a.events), len(b.events))
		}
	})

	t.Run("error in one observer does not stop the rest", func(t *testing.T) {
		s := NewSubject(zap.NewNop())
		bad := &fakeObserver{err: io.ErrClosedPipe}
		good := &fakeObserver{}
		s.Subscribe(bad)
		s.Subscribe(good)

		s.NotifyAll(Event{TS: 3})

		if len(good.events) != 1 {
			t.Fatalf("observer after a failing one was skipped")
		}
	})
}
