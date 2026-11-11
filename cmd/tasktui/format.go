package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/alfonsoHR98/tasktui/internal/core"
)

// taskPlain es la línea de texto de una tarea para la CLI.
func taskPlain(t *core.Task, showProject bool) string {
	today := core.Today()
	check := "○"
	if t.Done {
		check = "✓"
	}
	prio := "  "
	if t.Priority < 4 {
		prio = fmt.Sprintf("p%d", t.Priority)
	}
	due := ""
	if t.Due != nil {
		due = core.Rel(*t.Due, today)
	}
	if t.Time != nil {
		due += " " + t.Time.String()
	}
	id := "-"
	if t.ID != nil {
		id = *t.ID
	}
	s := fmt.Sprintf("%-6s %s %s %-8s %s", id, check, prio, due, t.Text)
	if t.Deadline != nil {
		s += " 🎯" + core.Rel(*t.Deadline, today)
	}
	if strings.TrimSpace(t.Description) != "" {
		s += " ¶"
	}
	if len(t.Notes) > 0 {
		s += fmt.Sprintf(" ✎%d", len(t.Notes))
	}
	if t.Recurrence != nil {
		s += " ↻"
	}
	for _, l := range t.Labels {
		s += " #" + l
	}
	if showProject {
		s += "  · " + t.Project
	}
	return s
}

// EditText edita un texto en $VISUAL/$EDITOR (o nvim/vim/nano) y devuelve el resultado.
func EditText(id, current string) (string, error) {
	path := fmt.Sprintf("%s/tasktui-%s.md", os.TempDir(), id)
	if err := os.WriteFile(path, []byte(current), 0o600); err != nil {
		return "", err
	}
	defer os.Remove(path)
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		// En macOS «nano» es pico (cursor poco visible): preferir nvim/vim.
		editor = "nano"
		for _, e := range []string{"nvim", "vim", "nano"} {
			if _, err := exec.LookPath(e); err == nil {
				editor = e
				break
			}
		}
	}
	parts := strings.Fields(editor)
	cmd := exec.Command(parts[0], append(parts[1:], path)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("El editor terminó con error")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(b), " \t\r\n"), nil
}
