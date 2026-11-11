package core

import (
	"fmt"
	"strings"
	"time"
)

var wdAbbr = [7]string{"lun", "mar", "mié", "jue", "vie", "sáb", "dom"}
var monAbbr = [12]string{"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"}

// WeekdayES reconoce un día de la semana en español o en inglés.
func WeekdayES(s string) (time.Weekday, bool) {
	switch s {
	case "lunes", "monday":
		return time.Monday, true
	case "martes", "tuesday":
		return time.Tuesday, true
	case "miércoles", "miercoles", "wednesday":
		return time.Wednesday, true
	case "jueves", "thursday":
		return time.Thursday, true
	case "viernes", "friday":
		return time.Friday, true
	case "sábado", "sabado", "saturday":
		return time.Saturday, true
	case "domingo", "sunday":
		return time.Sunday, true
	}
	return 0, false
}

// NextWeekday es el próximo día de la semana w (si hoy es w, la semana que viene).
func NextWeekday(today Date, w time.Weekday) (Date, bool) {
	d, ok := today.AddDays(1)
	for ok && d.Weekday() != w {
		d, ok = d.AddDays(1)
	}
	return d, ok
}

func wd(d Date) string { return wdAbbr[wdIndex(d.Weekday())] }

func dayMonth(d, today Date) string {
	s := fmt.Sprintf("%d %s", d.Day(), monAbbr[d.Month()-1])
	if d.Year() != today.Year() {
		s += fmt.Sprintf(" %d", d.Year())
	}
	return s
}

// Rel es el formato corto relativo: «hoy», «mañana», «vie», «15 nov».
func Rel(d, today Date) string {
	switch n := d.DaysSince(today); {
	case n == 0:
		return "hoy"
	case n == 1:
		return "mañana"
	case n == -1:
		return "ayer"
	case n >= 2 && n <= 6:
		return wd(d)
	}
	return dayMonth(d, today)
}

// Long es el formato largo para encabezados: «jue 6 nov · mañana».
func Long(d, today Date) string {
	base := wd(d) + " " + dayMonth(d, today)
	switch d.DaysSince(today) {
	case 0:
		return base + " · hoy"
	case 1:
		return base + " · mañana"
	}
	return base
}

func lower(s string) string { return strings.ToLower(s) }
