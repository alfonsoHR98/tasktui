package core

import (
	"strings"
)

// Formato de una línea de tarea del plugin Obsidian Tasks:
//
//   - [ ] Pagar luz #casa ⏫ 🔁 every month 📅 2026-11-06 ⏰ 18:00 🎯 2026-11-20 🆔 a1b2c
//
// «⏰» (hora) y «🎯» (fecha límite) son propios de tasktui: Obsidian Tasks no los
// interpreta, pero los conserva como texto.
const (
	mDue      = "📅"
	mDone     = "✅"
	mID       = "🆔"
	mTime     = "⏰"
	mDeadline = "🎯"
	mRec      = "🔁"
)

var prio = [4]string{"🔺", "⏫", "🔼", "🔽"}

// Otros campos de Obsidian Tasks: se conservan pero no se interpretan.
var otherMarkers = []string{"⏳", "🛫", "➕", "❌", "🏁", "⛔"}

func allMarkers() []string {
	m := []string{mDue, mDone, mID, mRec, mTime, mDeadline}
	m = append(m, prio[:]...)
	return append(m, otherMarkers...)
}

// splitMarker: si el token empieza por un marcador, devuelve (marcador, resto pegado).
func splitMarker(tok string) (string, string, bool) {
	for _, m := range allMarkers() {
		if r, ok := strings.CutPrefix(tok, m); ok {
			return m, r, true
		}
	}
	return "", "", false
}

// AsTag devuelve la etiqueta si el token lo es (#12 no lo es).
func AsTag(tok string) (string, bool) {
	t, ok := strings.CutPrefix(tok, "#")
	if !ok {
		return "", false
	}
	t = strings.TrimRight(t, ",.;:!?)")
	if t == "" || strings.HasPrefix(t, "#") || strings.IndexFunc(t, func(r rune) bool { return r < '0' || r > '9' }) < 0 {
		return "", false
	}
	return t, true
}

// ParseLine lee una línea de tarea; ok es false si no lo es.
func ParseLine(line string) (*Task, bool) {
	trimmed := strings.TrimLeft(line, " \t ")
	indent := line[:len(line)-len(trimmed)]
	after, ok := strings.CutPrefix(trimmed, "- [")
	if !ok {
		if after, ok = strings.CutPrefix(trimmed, "* ["); !ok {
			return nil, false
		}
	}
	runes := []rune(after)
	if len(runes) == 0 {
		return nil, false
	}
	status := runes[0]
	rest, ok := strings.CutPrefix(string(runes[1:]), "]")
	if !ok || !(rest == "" || strings.HasPrefix(rest, " ")) {
		return nil, false
	}

	fields := strings.Fields(rest)
	toks := make([]string, len(fields))
	for i, f := range fields {
		toks[i] = strings.ReplaceAll(f, "️", "")
	}
	t := NewTask("")
	t.Indent = indent
	t.Status = status
	t.Done = status == 'x' || status == 'X' || status == '-'

	var text []string
	for i := 0; i < len(toks); i++ {
		tok := toks[i]
		if marker, glued, isMarker := splitMarker(tok); isMarker {
			// Valor: lo pegado al emoji o el siguiente token.
			value := func() string {
				if glued != "" {
					return glued
				}
				if i+1 < len(toks) {
					i++
					return toks[i]
				}
				return ""
			}
			switch marker {
			case mDue:
				v := value()
				if d, ok := ParseDate(v); ok {
					t.Due = d.Ptr()
				} else {
					t.Extra = append(t.Extra, mDue+" "+v)
				}
			case mDone:
				v := value()
				if d, ok := ParseDate(v); ok {
					t.DoneDate = d.Ptr()
				} else {
					t.Extra = append(t.Extra, mDone+" "+v)
				}
			case mTime:
				v := value()
				if c, ok := parseHM(v); ok {
					t.Time = c.Ptr()
				} else {
					t.Extra = append(t.Extra, mTime+" "+v)
				}
			case mDeadline:
				v := value()
				if d, ok := ParseDate(v); ok {
					t.Deadline = d.Ptr()
				} else {
					t.Extra = append(t.Extra, mDeadline+" "+v)
				}
			case mID:
				if v := value(); v != "" {
					id := v
					t.ID = &id
				}
			case mRec:
				var parts []string
				if glued != "" {
					parts = append(parts, glued)
				}
				for i+1 < len(toks) {
					if _, _, isM := splitMarker(toks[i+1]); isM {
						break
					}
					i++
					parts = append(parts, toks[i])
				}
				raw := strings.Join(parts, " ")
				if r, ok := ParseObsidian(raw); ok {
					t.Recurrence = r
				} else {
					t.Extra = append(t.Extra, mRec+" "+raw)
				}
			default:
				isPrio := false
				for n, pm := range prio {
					if pm == marker {
						t.Priority = n + 1
						isPrio = true
						if glued != "" {
							text = append(text, glued)
						}
					}
				}
				if !isPrio {
					v := value()
					t.Extra = append(t.Extra, strings.TrimRight(marker+" "+v, " "))
				}
			}
		} else if tag, ok := AsTag(tok); ok {
			t.Labels = append(t.Labels, tag)
		} else {
			text = append(text, tok)
		}
	}
	t.Text = strings.Join(text, " ")
	return t, true
}

// parseHM lee exactamente HH:MM.
func parseHM(s string) (Clock, bool) {
	if strings.Count(s, ":") != 1 {
		return Clock{}, false
	}
	return ParseClock(s)
}

// RenderLine escribe la tarea en formato Obsidian Tasks.
func RenderLine(t *Task) string {
	var sb strings.Builder
	sb.WriteString(t.Indent + "- [" + string(t.Status) + "]")
	push := func(part string) {
		if part != "" {
			sb.WriteString(" " + part)
		}
	}
	push(t.Text)
	for _, l := range t.Labels {
		push("#" + l)
	}
	if t.Priority < 4 && t.Priority >= 1 {
		push(prio[t.Priority-1])
	}
	if t.Recurrence != nil {
		push(mRec + " " + t.Recurrence.ToObsidian())
	}
	for _, e := range t.Extra {
		push(e)
	}
	if t.Due != nil {
		push(mDue + " " + t.Due.String())
	}
	if t.Time != nil {
		push(mTime + " " + t.Time.String())
	}
	if t.Deadline != nil {
		push(mDeadline + " " + t.Deadline.String())
	}
	if t.ID != nil {
		push(mID + " " + *t.ID)
	}
	if t.DoneDate != nil {
		push(mDone + " " + t.DoneDate.String())
	}
	return sb.String()
}
