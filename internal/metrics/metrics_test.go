package metrics

import (
	"strings"
	"testing"
)

func TestRegistry_CounterIncrement(t *testing.T) {
	r := New()
	c := r.Counter("test_counter")
	c.Add(5)

	text := r.PrometheusText()
	if !strings.Contains(text, "goalos_test_counter 5") {
		t.Fatalf("expected counter value 5 in output, got: %s", text)
	}
}

func TestRegistry_GaugeSet(t *testing.T) {
	r := New()
	g := r.Gauge("active_goals")
	g.Store(3)

	text := r.PrometheusText()
	if !strings.Contains(text, "goalos_active_goals 3") {
		t.Fatalf("expected gauge value 3 in output, got: %s", text)
	}
}

func TestRegistry_PrometheusFormat(t *testing.T) {
	r := New()
	r.Counter("events_published_total").Add(100)
	r.Gauge("active_goals").Store(2)

	text := r.PrometheusText()
	if !strings.Contains(text, "# HELP") {
		t.Fatal("prometheus output should contain HELP line")
	}
	if !strings.Contains(text, "goalos_events_published_total 100") {
		t.Fatal("prometheus output should contain counter")
	}
	if !strings.Contains(text, "goalos_active_goals 2") {
		t.Fatal("prometheus output should contain gauge")
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := New()
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				r.Counter("concurrent_counter").Add(1)
				r.Gauge("concurrent_gauge").Store(int64(j))
				r.PrometheusText()
			}
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
	if r.Counter("concurrent_counter").Load() != 1000 {
		t.Fatalf("expected 1000, got %d", r.Counter("concurrent_counter").Load())
	}
}
