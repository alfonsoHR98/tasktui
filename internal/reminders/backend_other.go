//go:build !darwin

package reminders

import "errors"

func openBackend() (backend, error) {
	return nil, errors.New("La sincronización con Recordatorios solo funciona en macOS")
}
