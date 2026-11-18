package core

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// Casos compartidos con el Worker en Rust (worker/core): los dos parsers deben
// interpretar igual la captura rápida.
type sharedCase struct {
	In       string           `json:"in"`
	Known    *bool            `json:"known"`
	Text     string           `json:"text"`
	Project  *json.RawMessage `json:"project"`
	Due      *string          `json:"due"`
	Time     *string          `json:"time"`
	Deadline *string          `json:"deadline"`
	Priority *int             `json:"priority"`
	Labels   []string         `json:"labels"`
}

func TestCasosCompartidos(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/captura.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Today    string       `json:"today"`
		Projects []string     `json:"projects"`
		Cases    []sharedCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	today := MustDate(f.Today)
	for _, c := range f.Cases {
		ps := f.Projects
		if c.Known != nil && !*c.Known {
			ps = nil
		}
		q := ParseQuickIn(c.In, today, ps)
		if q.Text != c.Text {
			t.Errorf("%q: texto %q, quería %q", c.In, q.Text, c.Text)
		}
		if c.Project != nil {
			var want *string
			_ = json.Unmarshal(*c.Project, &want)
			if (want == nil) != (q.Project == nil) || (want != nil && *want != *q.Project) {
				t.Errorf("%q: proyecto %v, quería %v", c.In, q.Project, want)
			}
		}
		str := func(p interface{ String() string }, isNil bool) string {
			if isNil {
				return ""
			}
			return p.String()
		}
		if c.Due != nil && str(q.Due, q.Due == nil) != *c.Due {
			t.Errorf("%q: fecha %v, quería %s", c.In, q.Due, *c.Due)
		}
		if c.Time != nil && str(q.Time, q.Time == nil) != *c.Time {
			t.Errorf("%q: hora %v, quería %s", c.In, q.Time, *c.Time)
		}
		if c.Deadline != nil && str(q.Deadline, q.Deadline == nil) != *c.Deadline {
			t.Errorf("%q: límite %v, quería %s", c.In, q.Deadline, *c.Deadline)
		}
		if c.Priority != nil && (q.Priority == nil || *q.Priority != *c.Priority) {
			t.Errorf("%q: prioridad %v, quería %d", c.In, q.Priority, *c.Priority)
		}
		if c.Labels != nil && !reflect.DeepEqual(q.Labels, c.Labels) {
			t.Errorf("%q: etiquetas %v, quería %v", c.In, q.Labels, c.Labels)
		}
	}
}
