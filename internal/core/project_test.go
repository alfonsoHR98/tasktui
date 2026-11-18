package core

import "testing"

var proyectos = []string{Inbox, "Ganbaru", "Niuko Consultores", "Ramírez Medellín"}

// Los casos de captura con proyectos están en testdata/captura.json (TestCasosCompartidos).

func TestClaveDeProyecto(t *testing.T) {
	if !SameProject("Ramírez Medellín", "ramirez_medellin") || !SameProject(`"Niuko Consultores"`, "niuko  consultores") {
		t.Error("deberían ser el mismo proyecto")
	}
	if SameProject("Niuko", "Niuko Consultores") {
		t.Error("«Niuko» no es «Niuko Consultores»")
	}
	if s := SuggestProjects(proyectos, "Niuko"); len(s) != 1 || s[0] != "Niuko Consultores" {
		t.Errorf("sugerencias: %v", s)
	}
}

func TestVistaDeProyectoSinAcentos(t *testing.T) {
	v, ok := ParseView("+ramirez medellin")
	if !ok {
		t.Fatal("vista")
	}
	task := NewTask("Checador")
	task.Project = "Ramírez Medellín"
	gs := BuildView([]Item{{Key: 0, Task: task}}, proyectos, v, hoy, false, "")
	if len(gs) != 1 || len(gs[0].Items) != 1 {
		t.Errorf("grupos: %v", gs)
	}
}
