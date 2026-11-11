package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Configuración: ~/.config/tasktui/config.toml
//
//	data_dir = "~/Library/Mobile Documents/iCloud~md~obsidian/Documents/MiVault/Tareas"
//	recordatorios = true   # espejo automático hacia Recordatorios
//	remoto = true          # usar el servidor de servidor.toml (por defecto, si existe)
//
// El servidor va aparte, en ~/.config/tasktui/servidor.toml (permisos 600, porque
// guarda el token):
//
//	url = "https://tasks.ejemplo.com"
//	token = "…"
//
// Variables de entorno (tienen prioridad sobre los archivos):
//   - TASKTUI_DIR: modo local con esa carpeta.
//   - TASKTUI_SERVER + TASKTUI_TOKEN: modo remoto con ese servidor.

type config struct {
	DataDir       *string `toml:"data_dir,omitempty"`
	Recordatorios *bool   `toml:"recordatorios,omitempty"`
	Remoto        *bool   `toml:"remoto,omitempty"`
}

// Server es el servidor de tasktui al que conectarse.
type Server struct {
	URL   string `toml:"url"`
	Token string `toml:"token"`
}

// Mode es dónde viven las tareas: Dir (local) o Server (remoto).
type Mode struct {
	Dir    string
	Server *Server
}

func home() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return h
}

// ConfigPath es la ruta de config.toml.
func ConfigPath() string { return filepath.Join(home(), ".config/tasktui/config.toml") }

func expand(p string) string {
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		return filepath.Join(home(), rest)
	}
	return p
}

func loadConfig() (config, error) {
	var c config
	b, err := os.ReadFile(ConfigPath())
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := toml.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("Configuración inválida en %s: %w", ConfigPath(), err)
	}
	return c, nil
}

func saveConfig(c config) error {
	path := ConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := toml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// DataDir es la carpeta del modo local.
func DataDir() (string, error) {
	if d := os.Getenv("TASKTUI_DIR"); d != "" {
		return expand(d), nil
	}
	c, err := loadConfig()
	if err != nil {
		return "", err
	}
	if c.DataDir != nil {
		return expand(*c.DataDir), nil
	}
	return filepath.Join(home(), ".local/share/tasktui"), nil
}

// SetDataDir guarda la carpeta del modo local.
func SetDataDir(dir string) (string, error) {
	c, err := loadConfig()
	if err != nil {
		return "", err
	}
	c.DataDir = &dir
	return expand(dir), saveConfig(c)
}

// RemindersEnabled dice si el espejo automático hacia Recordatorios está activo.
func RemindersEnabled() bool {
	c, err := loadConfig()
	return err == nil && c.Recordatorios != nil && *c.Recordatorios
}

// SetReminders activa o desactiva el espejo automático.
func SetReminders(on bool) error {
	c, err := loadConfig()
	if err != nil {
		return err
	}
	c.Recordatorios = &on
	return saveConfig(c)
}

// ServerPath es la ruta de servidor.toml.
func ServerPath() string { return filepath.Join(filepath.Dir(ConfigPath()), "servidor.toml") }

// LoadServer devuelve el servidor configurado, o nil.
func LoadServer() *Server {
	if u, t := os.Getenv("TASKTUI_SERVER"), os.Getenv("TASKTUI_TOKEN"); u != "" && t != "" {
		return &Server{URL: u, Token: t}
	}
	b, err := os.ReadFile(ServerPath())
	if err != nil {
		return nil
	}
	var s Server
	if toml.Unmarshal(b, &s) != nil || s.URL == "" {
		return nil
	}
	return &s
}

// SaveServer guarda el servidor (permisos 600: incluye el token).
func SaveServer(s Server) error {
	path := ServerPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := toml.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

// SetRemote fuerza (o no) el modo remoto.
func SetRemote(on bool) error {
	c, err := loadConfig()
	if err != nil {
		return err
	}
	c.Remoto = &on
	return saveConfig(c)
}

// CurrentMode es el modo activo: TASKTUI_DIR → local; remoto = false → local;
// si hay servidor.toml → remoto; si no → local.
func CurrentMode() (Mode, error) {
	if os.Getenv("TASKTUI_DIR") != "" {
		d, err := DataDir()
		return Mode{Dir: d}, err
	}
	if os.Getenv("TASKTUI_SERVER") != "" {
		if s := LoadServer(); s != nil {
			return Mode{Server: s}, nil
		}
	}
	c, err := loadConfig()
	if err != nil {
		return Mode{}, err
	}
	if c.Remoto == nil || *c.Remoto {
		if s := LoadServer(); s != nil {
			return Mode{Server: s}, nil
		}
	}
	d, err := DataDir()
	return Mode{Dir: d}, err
}

// Open abre el almacén del modo configurado.
func Open() (Store, error) {
	m, err := CurrentMode()
	if err != nil {
		return nil, err
	}
	if m.Server != nil {
		return OpenRemote(*m.Server)
	}
	return OpenFiles(m.Dir)
}
