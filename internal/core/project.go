package core

import (
	"sort"
	"strings"
	"unicode"
)

// Nombres de proyecto: cómo se limpian, se comparan y se reconocen en la captura
// rápida. Así «+niuko_consultores», «+Niuko Consultores», «+"Niuko Consultores"»
// y «Ramirez Medellin» llegan al proyecto que ya existe en vez de crear otro.

var fold = strings.NewReplacer(
	"á", "a", "à", "a", "ä", "a", "â", "a",
	"é", "e", "è", "e", "ë", "e", "ê", "e",
	"í", "i", "ì", "i", "ï", "i", "î", "i",
	"ó", "o", "ò", "o", "ö", "o", "ô", "o",
	"ú", "u", "ù", "u", "ü", "u", "û", "u",
	"ñ", "n", "ç", "c",
)

// quotes son las comillas que pueden rodear un nombre: +"Niuko Consultores", +«Mi proyecto».
const quotes = "\"'«»“”‘’`"

// CleanProjectName limpia un nombre escrito por el usuario: quita comillas y
// espacios sobrantes y cambia «_» por espacio.
func CleanProjectName(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.Trim(strings.TrimSpace(s), quotes)
	return strings.Join(strings.Fields(s), " ")
}

// ProjectKey es la forma de comparar nombres de proyecto: sin mayúsculas, acentos,
// comillas, «_» ni espacios repetidos.
func ProjectKey(s string) string {
	return fold.Replace(strings.ToLower(CleanProjectName(s)))
}

// SameProject dice si dos nombres son el mismo proyecto.
func SameProject(a, b string) bool { return ProjectKey(a) == ProjectKey(b) }

// MatchProject devuelve el proyecto existente equivalente a name.
func MatchProject(projects []string, name string) (string, bool) {
	k := ProjectKey(name)
	if k == "" {
		return "", false
	}
	for _, p := range projects {
		if ProjectKey(p) == k {
			return p, true
		}
	}
	return "", false
}

// ResolveProject devuelve el nombre canónico: el existente si lo hay o, si no, el
// nombre limpio (proyecto nuevo).
func ResolveProject(projects []string, name string) (string, bool) {
	if p, ok := MatchProject(projects, name); ok {
		return p, true
	}
	return CleanProjectName(name), false
}

// SuggestProjects devuelve los proyectos que se parecen a name (empiezan igual o lo
// contienen), para avisar de un posible error al escribirlo.
func SuggestProjects(projects []string, name string) []string {
	k := ProjectKey(name)
	if k == "" {
		return nil
	}
	var out []string
	for _, p := range projects {
		pk := ProjectKey(p)
		if pk != k && (strings.HasPrefix(pk, k) || strings.HasPrefix(k, pk) || strings.Contains(pk, k)) {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// maxProjectWords limita cuántas palabras se prueban tras «+» para encontrar un
// proyecto existente con espacios.
const maxProjectWords = 6

// projectToken lee un +Proyecto que empieza en toks[i]. Devuelve el nombre y
// cuántos tokens consume. Entiende:
//   - +Finanzas, +Niuko_Consultores
//   - +"Niuko Consultores" o +«Niuko Consultores» (con comillas)
//   - +Niuko Consultores, si «Niuko Consultores» ya es un proyecto
func projectToken(toks []string, i int, projects []string) (string, int, bool) {
	p := strings.TrimPrefix(toks[i], "+")
	if p == "" {
		return "", 0, false
	}
	// Con comillas: hasta el token que las cierra.
	if strings.ContainsRune(quotes, []rune(p)[0]) {
		end := -1
		for j := i; j < len(toks); j++ {
			w := toks[j]
			if j == i {
				w = strings.TrimLeft(p, quotes)
			}
			if w != "" && strings.ContainsRune(quotes, []rune(w)[len([]rune(w))-1]) {
				end = j
				break
			}
		}
		if end < 0 {
			end = i // sin cierre: solo esta palabra
		}
		name := CleanProjectName(strings.Join(append([]string{p}, toks[i+1:end+1]...), " "))
		if strings.IndexFunc(name, unicode.IsLetter) < 0 {
			return "", 0, false
		}
		if m, ok := MatchProject(projects, name); ok {
			name = m
		}
		return name, end - i + 1, true
	}
	if strings.IndexFunc(p, unicode.IsLetter) < 0 {
		return "", 0, false
	}
	// Proyecto existente con espacios: el más largo que coincida.
	for n := min(len(toks)-i, maxProjectWords); n >= 2; n-- {
		cand := strings.Join(append([]string{p}, toks[i+1:i+n]...), " ")
		if m, ok := MatchProject(projects, cand); ok {
			return m, n, true
		}
	}
	name := CleanProjectName(p)
	if m, ok := MatchProject(projects, name); ok {
		name = m
	}
	return name, 1, true
}
