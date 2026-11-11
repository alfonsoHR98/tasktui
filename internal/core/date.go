// Package core es el núcleo de tasktui: modelo de tareas, captura rápida en
// español, fechas, formato Obsidian Tasks, vistas y el contrato JSON de la API.
//
// No usa reloj, archivos ni red: quien llama pasa la fecha de «hoy».
package core

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Rango de años aceptado: fuera de él, las operaciones de fecha devuelven «no válida»
// en lugar de desbordar (p. ej. «en 99999999 días»).
const (
	minYear = 1
	maxYear = 9999
)

// Date es un día del calendario, sin hora ni zona.
type Date struct{ t time.Time }

// NewDate crea una fecha; ok es false si no existe (31/2) o está fuera de rango.
func NewDate(y, m, d int) (Date, bool) {
	if y < minYear || y > maxYear || m < 1 || m > 12 || d < 1 || d > 31 {
		return Date{}, false
	}
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	if t.Day() != d { // 31/2 se «desborda» a marzo en Go
		return Date{}, false
	}
	return Date{t}, true
}

// ParseDate lee AAAA-MM-DD.
func ParseDate(s string) (Date, bool) {
	parts := strings.Split(s, "-")
	if len(parts) != 3 || len(parts[0]) != 4 || len(parts[1]) == 0 || len(parts[1]) > 2 || len(parts[2]) == 0 || len(parts[2]) > 2 {
		return Date{}, false
	}
	y, e1 := strconv.Atoi(parts[0])
	m, e2 := strconv.Atoi(parts[1])
	d, e3 := strconv.Atoi(parts[2])
	if e1 != nil || e2 != nil || e3 != nil {
		return Date{}, false
	}
	return NewDate(y, m, d)
}

// MustDate es para pruebas y constantes: entra en pánico si la fecha no existe.
func MustDate(s string) Date {
	d, ok := ParseDate(s)
	if !ok {
		panic("fecha inválida: " + s)
	}
	return d
}

func (d Date) String() string        { return d.t.Format("2006-01-02") }
func (d Date) Year() int             { return d.t.Year() }
func (d Date) Month() int            { return int(d.t.Month()) }
func (d Date) Day() int              { return d.t.Day() }
func (d Date) Weekday() time.Weekday { return d.t.Weekday() }
func (d Date) Before(o Date) bool    { return d.t.Before(o.t) }
func (d Date) After(o Date) bool     { return d.t.After(o.t) }
func (d Date) Equal(o Date) bool     { return d.t.Equal(o.t) }
func (d Date) IsZero() bool          { return d.t.IsZero() }

// DaysSince devuelve d − o en días.
func (d Date) DaysSince(o Date) int { return int(d.t.Sub(o.t).Hours() / 24) }

// AddDays suma n días (n puede ser negativo); ok es false si se sale del rango.
func (d Date) AddDays(n int64) (Date, bool) {
	if n > 4_000_000 || n < -4_000_000 {
		return Date{}, false
	}
	t := d.t.AddDate(0, 0, int(n))
	if t.Year() < minYear || t.Year() > maxYear {
		return Date{}, false
	}
	return Date{t}, true
}

// AddMonths suma n meses y ajusta el día al fin de mes (31 ene + 1 mes = 28 feb).
func (d Date) AddMonths(n uint32) (Date, bool) {
	if n > 12*(maxYear-minYear) {
		return Date{}, false
	}
	total := d.t.Year()*12 + int(d.t.Month()) - 1 + int(n)
	y, m := total/12, total%12+1
	if y > maxYear {
		return Date{}, false
	}
	day := d.t.Day()
	if last := daysIn(y, time.Month(m)); day > last {
		day = last
	}
	return NewDate(y, m, day)
}

func daysIn(y int, m time.Month) int {
	return time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// MarshalJSON escribe "AAAA-MM-DD".
func (d Date) MarshalJSON() ([]byte, error) { return []byte(`"` + d.String() + `"`), nil }

// UnmarshalJSON lee "AAAA-MM-DD".
func (d *Date) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	v, ok := ParseDate(s)
	if !ok {
		return fmt.Errorf("fecha inválida %q", s)
	}
	*d = v
	return nil
}

// Today devuelve el día de hoy según el reloj local (solo para clientes; el núcleo no lo usa).
func Today() Date {
	n := time.Now()
	d, _ := NewDate(n.Year(), int(n.Month()), n.Day())
	return d
}

// Ptr devuelve un puntero a la fecha (cómodo para los campos opcionales).
func (d Date) Ptr() *Date { return &d }

// Clock es una hora del día (HH:MM).
type Clock struct{ H, M int }

// NewClock valida hora y minutos.
func NewClock(h, m int) (Clock, bool) {
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return Clock{}, false
	}
	return Clock{h, m}, true
}

// ParseClock lee HH:MM o HH:MM:SS.
func ParseClock(s string) (Clock, bool) {
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return Clock{}, false
	}
	h, e1 := strconv.Atoi(parts[0])
	m, e2 := strconv.Atoi(parts[1])
	if e1 != nil || e2 != nil {
		return Clock{}, false
	}
	return NewClock(h, m)
}

func (c Clock) String() string { return fmt.Sprintf("%02d:%02d", c.H, c.M) }

// Less ordena por hora.
func (c Clock) Less(o Clock) bool { return c.H < o.H || (c.H == o.H && c.M < o.M) }

func (c Clock) MarshalJSON() ([]byte, error) { return []byte(`"` + c.String() + `"`), nil }

func (c *Clock) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	v, ok := ParseClock(s)
	if !ok {
		return fmt.Errorf("hora inválida %q", s)
	}
	*c = v
	return nil
}

// Ptr devuelve un puntero a la hora.
func (c Clock) Ptr() *Clock { return &c }
