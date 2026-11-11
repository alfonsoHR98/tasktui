package reminders

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/alfonsoHR98/tasktui/internal/store"
)

const agentLabel = "dev.madaradev.tasktui.sync"

func agentPath() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "Library/LaunchAgents", agentLabel+".plist")
}

func launchctl(args ...string) bool {
	return exec.Command("launchctl", args...).Run() == nil
}

// lockFile toma un candado exclusivo sobre el archivo; unlock lo suelta.
func lockFile(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}

// SetAgent instala (o quita) un agente de launchd que ejecuta «tasktui sync»
// cada minutes minutos y al iniciar sesión. Devuelve la ruta del plist.
func SetAgent(on bool, minutes int) (string, error) {
	path := agentPath()
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	_ = launchctl("bootout", domain+"/"+agentLabel)
	if !on {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return path, err
		}
		return path, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return path, err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return path, err
	}
	h, _ := os.UserHomeDir()
	logPath := filepath.Join(h, "Library/Logs/tasktui-sync.log")
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>sync</string>
		<string>--quiet</string>
	</array>
	<key>StartInterval</key>
	<integer>%d</integer>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Background</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, agentLabel, exe, max(minutes, 1)*60, logPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return path, err
	}
	if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
		return path, err
	}
	if !launchctl("bootstrap", domain, path) {
		return path, fmt.Errorf("launchctl no pudo cargar %s", path)
	}
	return path, nil
}

// SyncInBackground lanza «tasktui sync» en segundo plano si el espejo está activado.
// No bloquea ni falla: si algo sale mal, el siguiente sync lo arregla.
func SyncInBackground() {
	if envOverride() || !store.RemindersEnabled() {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exe, "sync", "--quiet")
	if cmd.Start() == nil {
		go func() { _ = cmd.Wait() }()
	}
}
