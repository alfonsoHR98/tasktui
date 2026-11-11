package core

import (
	"encoding/json"
)

// NoteDTO es un comentario de seguimiento en JSON.
type NoteDTO struct {
	Date *Date  `json:"date"`
	Text string `json:"text"`
}

// UnmarshalJSON acepta también el formato anterior (solo el texto).
func (n *NoteDTO) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		*n = NoteDTO{Text: s}
		return nil
	}
	type plain NoteDTO
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*n = NoteDTO(p)
	return nil
}

// TaskDTO es la tarea en JSON: lo que devuelve `tasktui --json` y lo que intercambia la API.
type TaskDTO struct {
	ID       *string  `json:"id"`
	Text     string   `json:"text"`
	Done     bool     `json:"done"`
	Due      *Date    `json:"due"`
	Time     *Clock   `json:"time"`
	Deadline *Date    `json:"deadline"`
	DoneDate *Date    `json:"done_date,omitempty"`
	Priority int      `json:"priority"`
	Labels   []string `json:"labels"`
	// Recurrence va en español («cada lunes»); RecurrenceRule, en el formato de Obsidian Tasks.
	Recurrence     *string   `json:"recurrence"`
	RecurrenceRule *string   `json:"recurrence_rule,omitempty"`
	Project        string    `json:"project"`
	Section        *string   `json:"section"`
	Description    string    `json:"description"`
	Notes          []NoteDTO `json:"notes"`
}

// ToDTO convierte la tarea al contrato JSON.
func ToDTO(t *Task) TaskDTO {
	d := TaskDTO{
		ID: t.ID, Text: t.Text, Done: t.Done, Due: t.Due, Time: t.Time, Deadline: t.Deadline,
		DoneDate: t.DoneDate, Priority: t.Priority, Labels: t.Labels, Project: t.Project,
		Section: t.Section, Description: t.Description, Notes: []NoteDTO{},
	}
	if d.Labels == nil {
		d.Labels = []string{}
	}
	if t.Recurrence != nil {
		es, ob := t.Recurrence.ToES(), t.Recurrence.ToObsidian()
		d.Recurrence, d.RecurrenceRule = &es, &ob
	}
	for _, n := range t.Notes {
		d.Notes = append(d.Notes, NoteDTO{Date: n.Date, Text: n.Text})
	}
	return d
}

// FromDTO convierte el JSON en una tarea (la prioridad se limita a 1..4).
func FromDTO(d *TaskDTO) *Task {
	t := NewTask(d.Text)
	t.ID = d.ID
	t.Done = d.Done
	if d.Done {
		t.Status = 'x'
	}
	t.Due, t.Time, t.Deadline, t.DoneDate = d.Due, d.Time, d.Deadline, d.DoneDate
	t.Priority = min(max(d.Priority, 1), 4)
	if d.Priority == 0 { // campo ausente: sin prioridad
		t.Priority = 4
	}
	t.Labels = append([]string(nil), d.Labels...)
	if d.RecurrenceRule != nil {
		if r, ok := ParseObsidian(*d.RecurrenceRule); ok {
			t.Recurrence = r
		}
	}
	t.Project, t.Section, t.Description = d.Project, d.Section, d.Description
	for _, n := range d.Notes {
		t.Notes = append(t.Notes, Note{Date: n.Date, Text: n.Text})
	}
	return t
}
