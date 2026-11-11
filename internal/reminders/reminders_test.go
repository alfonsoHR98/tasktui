package reminders

import (
	"reflect"
	"testing"

	"github.com/alfonsoHR98/tasktui/internal/core"
)

func d(s string) core.Date { return core.MustDate(s) }

func w(id, title string) Wanted {
	return Wanted{TaskID: id, Title: title, Due: d("2026-11-06"), Notes: "📁 Inbox"}
}

func e(ek string, w Wanted) Existing {
	due := w.Due
	return Existing{EKID: ek, Title: w.Title, Due: &due, Time: w.Time, Priority: w.Priority, Notes: w.Notes}
}

func TestCreaActualizaBorra(t *testing.T) {
	a, b := w("a", "Uno"), w("b", "Dos")
	bVieja := e("EK2", b)
	bVieja.Due = d("2026-11-01").Ptr()
	sobra := e("EK3", w("zz", "Ya no existe"))
	m := Mapping{"b": "EK2", "zz": "EK3"}
	p := MakePlan([]Wanted{a, b}, []Existing{bVieja, sobra}, m)
	if !reflect.DeepEqual(p.Create, []Wanted{a}) || !reflect.DeepEqual(p.Update, []Update{{"EK2", b}}) || !reflect.DeepEqual(p.Remove, []string{"EK3"}) {
		t.Errorf("%+v", p)
	}
	if _, ok := m["zz"]; ok {
		t.Error("zz debía salir del mapa")
	}
}

func TestSinCambiosNoHaceNada(t *testing.T) {
	a := w("a", "Uno")
	if p := MakePlan([]Wanted{a}, []Existing{e("EK1", a)}, Mapping{"a": "EK1"}); !p.IsEmpty() {
		t.Errorf("%+v", p)
	}
}

func TestAdoptaSiSePerdioLaRelacion(t *testing.T) {
	a := w("a", "Uno")
	m := Mapping{}
	if p := MakePlan([]Wanted{a}, []Existing{e("EK1", a)}, m); !p.IsEmpty() || m["a"] != "EK1" {
		t.Errorf("%+v %v", p, m)
	}
}

func TestImportaLoCreadoFueraYBorraLoPropio(t *testing.T) {
	huerfano := e("EK1", w("borrada", "Ya la borré"))
	tuyo := e("EK2", w("?", "Comprar leche"))
	hecho := e("EK3", w("?", "Algo viejo"))
	hecho.Completed = true
	m := Mapping{"borrada": "EK1"}
	p := MakePlan(nil, []Existing{huerfano, tuyo, hecho}, m)
	if !reflect.DeepEqual(p.Remove, []string{"EK1"}) || !reflect.DeepEqual(p.Import, []Existing{tuyo}) || len(m) != 0 {
		t.Errorf("%+v %v", p, m)
	}
}

func TestImportarUsaCapturaRapida(t *testing.T) {
	hoy := d("2026-11-05")
	r := e("EK9", w("?", "Comprar leche +Casa #super p1"))
	r.Due, r.Priority = nil, 0
	r.Notes = "deslactosada\n\n2 litros"
	task, proj := ImportTask(r, hoy)
	if task.Text != "Comprar leche" || proj != "Casa" || !reflect.DeepEqual(task.Labels, []string{"super"}) ||
		task.Priority != 1 || task.Due != nil || task.Description != "deslactosada\n2 litros" {
		t.Errorf("%+v %s", task, proj)
	}
	// La fecha y prioridad del recordatorio mandan.
	r.Title, r.Due, r.Priority = "Pagar luz mañana", d("2026-12-01").Ptr(), 5
	task, proj = ImportTask(r, hoy)
	if task.Text != "Pagar luz" || proj != "Inbox" || task.Due.String() != "2026-12-01" || task.Priority != 2 {
		t.Errorf("%+v %s", task, proj)
	}
}

func TestCompletadoEnRecordatorios(t *testing.T) {
	a := w("a", "Uno")
	hecho := e("EK1", a)
	hecho.Completed = true
	m := Mapping{"a": "EK1"}
	if got := CompletedInReminders(m, []Existing{hecho}, map[string]bool{"a": true}); !reflect.DeepEqual(got, []string{"a"}) {
		t.Errorf("%v", got)
	}
	if got := CompletedInReminders(m, []Existing{hecho}, map[string]bool{}); len(got) != 0 {
		t.Errorf("%v", got)
	}
}
