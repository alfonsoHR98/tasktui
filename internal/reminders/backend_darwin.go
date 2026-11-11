//go:build darwin

package reminders

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework EventKit -framework Foundation -Wl,-sectcreate,__TEXT,__info_plist,${SRCDIR}/Info.plist
#include <stdlib.h>
#include "eventkit.h"
*/
import "C"

import (
	"encoding/json"
	"errors"
	"unsafe"

	"github.com/alfonsoHR98/tasktui/internal/core"
)

// ek es EventKit a través del puente de Objective-C. El Info.plist se incrusta en
// el binario: macOS exige las descripciones de uso para pedir permiso a Recordatorios.
type ek struct{}

type envelope struct {
	Error string          `json:"error"`
	Data  json.RawMessage `json:"data"`
}

// call libera la respuesta de C y devuelve su contenido.
func call(p *C.char) (json.RawMessage, error) {
	defer C.free(unsafe.Pointer(p))
	var env envelope
	if err := json.Unmarshal([]byte(C.GoString(p)), &env); err != nil {
		return nil, err
	}
	if env.Error != "" {
		return nil, errors.New(env.Error)
	}
	return env.Data, nil
}

func openBackend() (backend, error) {
	name := C.CString(List)
	defer C.free(unsafe.Pointer(name))
	if _, err := call(C.ek_open(name)); err != nil {
		return nil, err
	}
	return ek{}, nil
}

func (ek) Fetch() ([]Existing, error) {
	data, err := call(C.ek_fetch())
	if err != nil {
		return nil, err
	}
	var raws []struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		HasDate   bool   `json:"hasDate"`
		Y         int    `json:"y"`
		M         int    `json:"m"`
		D         int    `json:"d"`
		HasTime   bool   `json:"hasTime"`
		H         int    `json:"h"`
		Min       int    `json:"min"`
		Priority  int    `json:"priority"`
		Notes     string `json:"notes"`
		Completed bool   `json:"completed"`
	}
	if err := json.Unmarshal(data, &raws); err != nil {
		return nil, err
	}
	out := make([]Existing, 0, len(raws))
	for _, r := range raws {
		e := Existing{EKID: r.ID, Title: r.Title, Priority: r.Priority, Notes: r.Notes, Completed: r.Completed}
		if r.HasDate {
			if d, ok := core.NewDate(r.Y, r.M, r.D); ok {
				e.Due = d.Ptr()
			}
		}
		if r.HasTime {
			if c, ok := core.NewClock(r.H, r.Min); ok {
				e.Time = c.Ptr()
			}
		}
		out = append(out, e)
	}
	return out, nil
}

func wantedJSON(w Wanted) *C.char {
	m := map[string]any{
		"title": w.Title, "y": w.Due.Year(), "m": w.Due.Month(), "d": w.Due.Day(),
		"hasTime": w.Time != nil, "priority": w.Priority, "notes": w.Notes,
	}
	if w.Time != nil {
		m["h"], m["min"] = w.Time.H, w.Time.M
	}
	b, _ := json.Marshal(m)
	return C.CString(string(b))
}

func (ek) Create(w Wanted) (string, error) {
	js := wantedJSON(w)
	defer C.free(unsafe.Pointer(js))
	data, err := call(C.ek_create(js))
	if err != nil {
		return "", err
	}
	var id string
	return id, json.Unmarshal(data, &id)
}

func (ek) Update(id string, w Wanted) error {
	cid, js := C.CString(id), wantedJSON(w)
	defer C.free(unsafe.Pointer(cid))
	defer C.free(unsafe.Pointer(js))
	_, err := call(C.ek_update(cid, js))
	return err
}

func (ek) Remove(id string) error {
	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid))
	_, err := call(C.ek_remove(cid))
	return err
}

func (ek) Commit() error {
	_, err := call(C.ek_commit())
	return err
}
