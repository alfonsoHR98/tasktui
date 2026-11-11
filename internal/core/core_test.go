package core

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func d(s string) Date { return MustDate(s) }

// 2026-11-05 es jueves
var hoy = d("2026-11-05")

func eqDate(t *testing.T, got *Date, want string) {
	t.Helper()
	if want == "" {
		if got != nil {
			t.Errorf("fecha = %v, quería ninguna", got)
		}
		return
	}
	if got == nil || got.String() != want {
		t.Errorf("fecha = %v, quería %s", got, want)
	}
}

func TestRecurrenciaIdaYVuelta(t *testing.T) {
	for _, s := range []string{"every day", "every 2 weeks", "every week on Monday", "every month", "every 3 years"} {
		r, ok := ParseObsidian(s)
		if !ok || r.ToObsidian() != s {
			t.Errorf("%q → %v", s, r)
		}
	}
	r, _ := ParseObsidian("every monday")
	if r.ToObsidian() != "every week on Monday" || r.ToES() != "cada lunes" {
		t.Errorf("every monday → %s / %s", r.ToObsidian(), r.ToES())
	}
}

func TestSiguienteFecha(t *testing.T) {
	lunes, _ := ParseObsidian("every week on Monday")
	if got := lunes.Next(d("2026-11-05")); got.String() != "2026-11-09" {
		t.Errorf("got %s", got)
	}
	if got := lunes.Next(d("2026-11-09")); got.String() != "2026-11-16" {
		t.Errorf("got %s", got)
	}
	mes, _ := ParseObsidian("every month")
	if got := mes.Next(d("2026-01-31")); got.String() != "2026-02-28" {
		t.Errorf("got %s", got)
	}
}

func TestCompletar(t *testing.T) {
	tk := NewTask("Gym")
	id := "aaaaa"
	tk.ID = &id
	tk.Notes = []Note{NewNote("rutina A", nil)}
	tk.Due = d("2026-11-05").Ptr()
	c := tk.Complete(d("2026-11-06"), "bbbbb")
	if c.Recurring() || !c.Simple.Done {
		t.Fatal("debía completarse sin repetir")
	}
	tk.Recurrence, _ = ParseObsidian("every week on Monday")
	c = tk.Complete(d("2026-11-06"), "bbbbb")
	if !c.Recurring() || *c.Next.ID != "aaaaa" || c.Next.Done || c.Next.Due.String() != "2026-11-09" {
		t.Errorf("next mal: %+v", c.Next)
	}
	if len(c.Next.Notes) != 1 || *c.Done.ID != "bbbbb" || !c.Done.Done || len(c.Done.Notes) != 0 {
		t.Errorf("done mal: %+v", c.Done)
	}
}

func TestCapturaCompleta(t *testing.T) {
	q := ParseQuick("Pagar luz mañana p1 #casa @finanzas +Mis_Cuentas", hoy)
	if q.Text != "Pagar luz" || *q.Priority != 1 || !reflect.DeepEqual(q.Labels, []string{"casa", "finanzas"}) || *q.Project != "Mis Cuentas" {
		t.Errorf("%+v", q)
	}
	eqDate(t, q.Due, "2026-11-06")
}

func TestFechas(t *testing.T) {
	casos := map[string]string{
		"x viernes": "2026-11-06", "x jueves": "2026-11-12", "x pasado mañana": "2026-11-07",
		"x en 2 semanas": "2026-11-19", "x 1/3": "2027-03-01", "x 15/11/26": "2026-11-15",
		"Ir en bici": "",
	}
	for in, want := range casos {
		eqDate(t, ParseQuick(in, hoy).Due, want)
	}
	if q := ParseQuick("Ir en bici", hoy); q.Text != "Ir en bici" {
		t.Errorf("texto = %q", q.Text)
	}
}

func TestHoraYLimite(t *testing.T) {
	clock := func(q Quick) string {
		if q.Time == nil {
			return ""
		}
		return q.Time.String()
	}
	q := ParseQuick("Llamar mañana 18:00", hoy)
	if q.Text != "Llamar" || clock(q) != "18:00" {
		t.Errorf("%+v", q)
	}
	for in, want := range map[string]string{"x 9am": "09:00", "x 6:30pm": "18:30", "x 12am": "00:00", "x 12pm": "12:00", "Versión 2:3": "", "x 25:00": ""} {
		if got := clock(ParseQuick(in, hoy)); got != want {
			t.Errorf("%q → %q, quería %q", in, got, want)
		}
	}
	eqDate(t, ParseQuick("x 9:30", hoy).Due, "2026-11-05") // hora sin fecha: hoy
	if ParseQuick("x 25:00", hoy).Text != "x 25:00" {
		t.Error("25:00 no es hora")
	}
	q = ParseQuick("Entregar reporte viernes limit 20/11 p1", hoy)
	eqDate(t, q.Deadline, "2026-11-20")
	eqDate(t, q.Due, "2026-11-06")
	if q.Text != "Entregar reporte" {
		t.Errorf("texto %q", q.Text)
	}
	eqDate(t, ParseQuick("x limit viernes", hoy).Deadline, "2026-11-06")
	eqDate(t, ParseQuick("x {20/11}", hoy).Deadline, "2026-11-20")
	q = ParseQuick("x {en 2 semanas} hoy", hoy)
	eqDate(t, q.Deadline, "2026-11-19")
	eqDate(t, q.Due, "2026-11-05")
	if q := ParseQuick("Sin limit claro", hoy); q.Text != "Sin limit claro" || q.Deadline != nil {
		t.Errorf("%+v", q)
	}
}

func TestFechasEnIngles(t *testing.T) {
	eqDate(t, ParseQuick("x today", hoy).Due, "2026-11-05")
	eqDate(t, ParseQuick("x tomorrow 9am", hoy).Due, "2026-11-06")
	eqDate(t, ParseQuick("x friday", hoy).Due, "2026-11-06")
	eqDate(t, ParseQuick("x in 2 weeks", hoy).Due, "2026-11-19")
	if r := ParseQuick("Gym every monday", hoy).Recurrence; r == nil || r.ToObsidian() != "every week on Monday" {
		t.Errorf("%+v", r)
	}
	if r := ParseQuick("x every 2 weeks", hoy).Recurrence; r == nil || r.ToES() != "cada 2 semanas" {
		t.Errorf("%+v", r)
	}
	eqDate(t, ParseQuick("x limit friday", hoy).Deadline, "2026-11-06")
	if ParseQuick("Log in to the server", hoy).Text != "Log in to the server" {
		t.Error("«in» suelto es texto")
	}
}

func TestRepeticion(t *testing.T) {
	q := ParseQuick("Gym cada lunes", hoy)
	if q.Recurrence.ToObsidian() != "every week on Monday" {
		t.Error(q.Recurrence)
	}
	eqDate(t, q.Due, "2026-11-09")
	q = ParseQuick("Regar cada 3 días", hoy)
	if q.Recurrence.ToES() != "cada 3 días" {
		t.Error(q.Recurrence.ToES())
	}
	eqDate(t, q.Due, "2026-11-05")
}

func TestIdaYVueltaConEscape(t *testing.T) {
	tk := NewTask("Revisar lo de hoy en 3 días")
	tk.Labels = []string{"work"}
	tk.Priority = 2
	tk.Due = d("2026-12-01").Ptr()
	tk.Time = Clock{18, 0}.Ptr()
	tk.Deadline = d("2026-12-05").Ptr()
	tk.Project = "Mi Trabajo"
	s := ToQuick(tk, hoy)
	want := `Revisar lo de \hoy \en 3 días #work p2 2026-12-01 18:00 limit 2026-12-05 +Mi_Trabajo`
	if s != want {
		t.Fatalf("got  %s\nwant %s", s, want)
	}
	q := ParseQuick(s, hoy)
	if q.Text != tk.Text || *q.Project != "Mi Trabajo" {
		t.Errorf("%+v", q)
	}
}

func TestSinPanico(t *testing.T) {
	alfabeto := []string{"t", "{", "}", "{}", "{ }", "limit", "cada", "every", "en", "in", "+", "#", "\\", "p", "9", "am", ":", "/", "x",
		"{t}", "{t", "t}", "{en", "3}", "hoy", "mañana", "días", "limit{", "{}}", "{{", "pm", "12:", "1/", "/1"}
	for _, a := range alfabeto {
		for _, b := range alfabeto {
			for _, c := range alfabeto {
				ParseQuick(a+" "+b+" "+c, hoy)
			}
		}
	}
	for _, c := range []string{"x en 99999999 días", "x in 4294967295 weeks", "x cada 99999999 meses", "x every 0 days",
		"x cada 0 semanas", "x every 4294967295 years", "x 31/2", "x 0/0", "x 99/99/99999", "x 1/1/99999",
		"x limit 31/2/2026", "x {99999999999/1}", "x 25:99", "x 0am", "x 13pm", "x en 0 días", "x cada 4294967295 lunes"} {
		q := ParseQuick(c, hoy)
		if q.Recurrence != nil {
			q.Recurrence.First(hoy)
			q.Recurrence.Next(hoy)
			q.Recurrence.ToES()
		}
	}
}

func TestMarkdownIdaYVuelta(t *testing.T) {
	line := "  - [ ] Pagar luz #casa #finanzas ⏫ 🔁 every month 📅 2026-11-06 🆔 a1b2c"
	tk, ok := ParseLine(line)
	if !ok || tk.Text != "Pagar luz" || !reflect.DeepEqual(tk.Labels, []string{"casa", "finanzas"}) || tk.Priority != 2 ||
		tk.Due.String() != "2026-11-06" || *tk.ID != "a1b2c" || tk.Indent != "  " {
		t.Fatalf("%+v", tk)
	}
	if got := RenderLine(tk); got != line {
		t.Errorf("got %q", got)
	}
	line = "- [ ] Entregar ⏫ 📅 2026-11-06 ⏰ 18:00 🎯 2026-11-20 🆔 a1b2c"
	tk, _ = ParseLine(line)
	if tk.Time.String() != "18:00" || tk.Deadline.String() != "2026-11-20" || tk.Text != "Entregar" || RenderLine(tk) != line {
		t.Errorf("%+v / %s", tk, RenderLine(tk))
	}
	tk, _ = ParseLine("- [x] Llamar ⏳ 2026-11-01 ✅ 2026-11-02")
	if !tk.Done || !reflect.DeepEqual(tk.Extra, []string{"⏳ 2026-11-01"}) || tk.DoneDate.String() != "2026-11-02" ||
		RenderLine(tk) != "- [x] Llamar ⏳ 2026-11-01 ✅ 2026-11-02" {
		t.Errorf("%+v", tk)
	}
}

func TestMarkdownNoEsTarea(t *testing.T) {
	for _, s := range []string{"- texto", "## Sección", "- [ ]sin espacio"} {
		if _, ok := ParseLine(s); ok {
			t.Errorf("%q no es tarea", s)
		}
	}
	tk, _ := ParseLine("- [ ] issue #12 y emoji 📅️ 2026-01-01")
	if tk.Text != "issue #12 y emoji" || tk.Due == nil {
		t.Errorf("%+v", tk)
	}
}

func TestDTOIdaYVuelta(t *testing.T) {
	tk, _ := ParseLine("- [ ] Pagar luz #casa ⏫ 🔁 every month 📅 2026-11-06 🆔 a1b2c")
	tk.Project = "Finanzas"
	tk.Time = Clock{18, 0}.Ptr()
	tk.Deadline = d("2026-11-20").Ptr()
	tk.Description = "Recibo de CFE\nPagar en línea"
	tk.Notes = []Note{NewNote("ya llegó el recibo", d("2026-11-06").Ptr())}
	dto := ToDTO(tk)
	b, _ := json.Marshal(dto)
	js := string(b)
	for _, want := range []string{`"due":"2026-11-06"`, `"recurrence":"cada mes"`, `"time":"18:00"`, `"notes":[{"date":"2026-11-06","text":"ya llegó el recibo"}]`} {
		if !strings.Contains(js, want) {
			t.Errorf("falta %s en %s", want, js)
		}
	}
	var back TaskDTO
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	t2 := FromDTO(&back)
	if !reflect.DeepEqual(t2.Recurrence, tk.Recurrence) || !reflect.DeepEqual(ToDTO(t2), dto) {
		t.Error("la ida y vuelta cambió la tarea")
	}
}

func TestDTONotasFormatoAnterior(t *testing.T) {
	var dto TaskDTO
	err := json.Unmarshal([]byte(`{"id":"a","text":"x","done":false,"due":null,"priority":4,"recurrence":null,"project":"Inbox","section":null,"notes":["vieja"]}`), &dto)
	if err != nil || len(dto.Notes) != 1 || dto.Notes[0].Text != "vieja" || dto.Notes[0].Date != nil || dto.Time != nil || dto.Description != "" {
		t.Errorf("%v %+v", err, dto)
	}
}

func TestVistaHoy(t *testing.T) {
	h := d("2026-11-06")
	a := NewTask("Sin hora p1")
	a.Priority, a.Due = 1, h.Ptr()
	b := NewTask("A las 9")
	b.Due, b.Time = h.Ptr(), Clock{9, 0}.Ptr()
	c := NewTask("Límite hoy, planeada para después")
	c.Due, c.Deadline = d("2026-11-20").Ptr(), h.Ptr()
	e := NewTask("Límite vencido")
	e.Deadline = d("2026-11-01").Ptr()
	f := NewTask("Sin fechas")
	tasks := []*Task{a, b, c, e, f}
	items := make([]Item, len(tasks))
	for i, tk := range tasks {
		items[i] = Item{i, tk}
	}
	g := BuildView(items, []string{Inbox}, View{Kind: ViewToday}, h, false, "")
	var got [][]string
	for _, grp := range g {
		var names []string
		for _, k := range grp.Items {
			names = append(names, tasks[k].Text)
		}
		got = append(got, names)
	}
	want := [][]string{{"Límite vencido"}, {"A las 9", "Sin hora p1", "Límite hoy, planeada para después"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestMCPTools(t *testing.T) {
	if len(MCPTools()) != 11 {
		t.Error("deben ser 10 herramientas")
	}
	c, err := PlanTool("list_tasks", map[string]any{"view": "+Niuko Consultores", "include_done": true})
	if err != nil || c.Path != "/api/tasks?view=%2BNiuko+Consultores&all=1" {
		t.Errorf("%+v %v", c, err)
	}
	if _, err := PlanTool("get_task", map[string]any{}); err == nil {
		t.Error("debía pedir id")
	}
}
