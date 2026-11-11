package notify

import (
	"testing"
	"time"

	"github.com/alfonsoHR98/tasktui/internal/core"
	"github.com/alfonsoHR98/tasktui/internal/store"
)

func entry(id, text, due, clock string) store.Entry {
	d, _ := core.ParseDate(due)
	t := &core.Task{ID: &id, Text: text, Due: d.Ptr()}
	if clock != "" {
		c, _ := core.ParseClock(clock)
		t.Time = c.Ptr()
	}
	return store.Entry{Task: t}
}

func TestPending(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 11, 11, 10, 0, 0, 0, loc)
	es := []store.Entry{
		entry("a", "ahora", "2026-11-11", "09:59"),
		entry("b", "futura", "2026-11-11", "18:00"),
		entry("c", "sin hora", "2026-11-11", ""),
		entry("d", "vieja", "2026-11-09", "08:00"),
	}
	got := Pending(es, now, map[string]bool{}, nil)
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "c" {
		t.Fatalf("inesperado: %+v", got)
	}
	seen := map[string]bool{got[0].Key: true}
	if g := Pending(es, now, seen, nil); len(g) != 1 {
		t.Fatalf("debía omitir lo ya mostrado: %+v", g)
	}
	sn := map[string]time.Time{got[0].Key: now.Add(10 * time.Minute)}
	seen = map[string]bool{}
	if g := Pending(es[:1], now, seen, sn); len(g) != 0 {
		t.Fatal("pospuesta aún no toca")
	}
	if g := Pending(es[:1], now.Add(11*time.Minute), seen, sn); len(g) != 1 {
		t.Fatal("pospuesta ya toca")
	}
}
