package store

import "github.com/alfonsoHR98/tasktui/internal/core"

// SeedDemo llena un almacén vacío con tareas de ejemplo (para `tasktui demo`
// y para las capturas de la documentación).
func SeedDemo(s Store, today core.Date) error {
	type item struct {
		quick, desc string
		notes       []string
		done        bool
		late        int // días de atraso
	}
	items := []item{
		{quick: "Renovar el dominio hoy p1 +Trabajo #web limit hoy", late: 1, desc: "Vence el registro; pagar dos años de una vez.", notes: []string{"El cobro automático falló, hay que hacerlo a mano"}},
		{quick: "Pagar la luz hoy 18:00 p1 +Finanzas #casa cada mes"},
		{quick: "Revisar el pull request de la API hoy p2 +Trabajo #código", desc: "Faltan pruebas del endpoint de exportación."},
		{quick: "Llamar al dentista hoy 12:30 p3 #salud"},
		{quick: "Regar las plantas hoy #casa cada 3 días"},
		{quick: "Comprar café en grano mañana #compras"},
		{quick: "Responder el correo del contador hoy +Finanzas", late: 3},
		{quick: "Preparar la demo del viernes viernes 10:00 p2 +Trabajo limit viernes", notes: []string{"Usar los datos de prueba, no producción"}},
		{quick: "Respaldar la NAS sábado +Casa cada semana"},
		{quick: "Leer el capítulo 4 de «Designing Data-Intensive Applications» +Lectura #libros"},
		{quick: "Planear las vacaciones de diciembre p3 +Casa"},
		{quick: "Declaración mensual 17/12 p2 +Finanzas limit 17/12"},
		{quick: "Ordenar el escritorio #casa"},
		{quick: "Enviar la factura de octubre hoy +Finanzas", done: true},
		{quick: "Sacar la basura hoy #casa", done: true},
	}
	for _, it := range items {
		q := core.ParseQuick(it.quick, today)
		t := q.IntoTask(core.Inbox)
		t.Description = it.desc
		if it.late > 0 {
			if d, ok := today.AddDays(int64(-it.late)); ok {
				t.Due = &d
			}
		}
		id, err := s.Add(t, t.Project)
		if err != nil {
			return err
		}
		for _, n := range it.notes {
			if err := s.AddNote(id, n); err != nil {
				return err
			}
		}
		if it.done {
			if _, err := s.Toggle(id); err != nil {
				return err
			}
		}
	}
	return nil
}
