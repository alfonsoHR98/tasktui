package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/alfonsoHR98/tasktui/internal/notify"
)

const launchdLabel = "dev.madaradev.tasktui.notify"

func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist")
}

func cmdNotify(a *app) *cobra.Command {
	var every time.Duration
	c := &cobra.Command{
		Use:   "notify",
		Short: "Avisos de vencimiento en la Mac: alertas que se quedan hasta que las cierras",
		Long: "Revisa las tareas con fecha y muestra una alerta fija al llegar su hora\n" +
			"(9:00 si no tiene hora) con los botones Completar, Posponer 10 min y Cerrar.\n" +
			"Sin subcomando se queda corriendo; usa «notify install» para que arranque solo.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := a.open()
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			fmt.Fprintln(os.Stderr, "tasktui notify: vigilando vencimientos (Ctrl+C para salir)")
			return notify.Run(ctx, s, every, func(m string) { fmt.Fprintln(os.Stderr, m) })
		},
	}
	c.Flags().DurationVar(&every, "every", 30*time.Second, "Cada cuánto revisar las tareas")
	c.AddCommand(
		&cobra.Command{
			Use:   "test",
			Short: "Muestra una alerta de prueba",
			Args:  cobra.NoArgs,
			Run: func(*cobra.Command, []string) {
				fmt.Println("botón:", notify.Alert(notify.Due{Text: "Alerta de prueba", Proj: "tasktui", When: time.Now()}))
			},
		},
		&cobra.Command{
			Use:   "install",
			Short: "Arranca el vigilante al iniciar sesión (launchd)",
			Args:  cobra.NoArgs,
			RunE: func(*cobra.Command, []string) error {
				exe, err := os.Executable()
				if err != nil {
					return err
				}
				home, _ := os.UserHomeDir()
				plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>%s</string>
<key>ProgramArguments</key><array><string>%s</string><string>notify</string></array>
<key>RunAtLoad</key><true/>
<key>KeepAlive</key><true/>
<key>StandardErrorPath</key><string>%s/Library/Logs/tasktui-notify.log</string>
</dict></plist>
`, launchdLabel, exe, home)
				p := plistPath()
				if err := os.WriteFile(p, []byte(plist), 0o644); err != nil {
					return err
				}
				_ = exec.Command("launchctl", "unload", p).Run()
				if out, err := exec.Command("launchctl", "load", p).CombinedOutput(); err != nil {
					return fmt.Errorf("launchctl: %v %s", err, out)
				}
				fmt.Println("Instalado:", p)
				return nil
			},
		},
		&cobra.Command{
			Use:   "uninstall",
			Short: "Quita el arranque automático",
			Args:  cobra.NoArgs,
			RunE: func(*cobra.Command, []string) error {
				p := plistPath()
				_ = exec.Command("launchctl", "unload", p).Run()
				if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
					return err
				}
				fmt.Println("Desinstalado")
				return nil
			},
		},
	)
	return c
}
