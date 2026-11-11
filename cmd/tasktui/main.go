// tasktui — tareas en la terminal: TUI y CLI. Sin argumentos abre la TUI.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alfonsoHR98/tasktui/internal/core"
	"github.com/alfonsoHR98/tasktui/internal/reminders"
	"github.com/alfonsoHR98/tasktui/internal/store"
	"github.com/alfonsoHR98/tasktui/internal/tui"
)

func main() {
	if err := newRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func printJSON(v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

func printJSONCompact(v any) {
	b, _ := json.Marshal(v)
	fmt.Println(string(b))
}

// app agrupa el almacén abierto (de forma perezosa) y las opciones globales.
type app struct {
	json  bool
	store store.Store
	// mutating indica que tras el comando hay que sincronizar con Recordatorios.
	mutating bool
}

func (a *app) open() (store.Store, error) {
	if a.store != nil {
		return a.store, nil
	}
	s, err := store.Open()
	if err != nil {
		return nil, err
	}
	if n := s.Notice(); n != "" {
		fmt.Fprintln(os.Stderr, n)
	}
	a.store = s
	return s, nil
}

func newRoot() *cobra.Command {
	a := &app{}
	root := &cobra.Command{
		Use:           "tasktui",
		Short:         "Tareas en la terminal (Markdown + Obsidian + servidor). Sin argumentos abre la TUI.",
		Version:       core.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := a.open()
			if err != nil {
				return err
			}
			return tui.Run(s)
		},
		PersistentPostRun: func(*cobra.Command, []string) {
			if a.mutating {
				reminders.SyncInBackground()
			}
		},
	}
	root.PersistentFlags().BoolVar(&a.json, "json", false, "Salida en JSON (para scripts y para Claude)")
	root.CompletionOptions.DisableDefaultCmd = true
	root.AddCommand(
		cmdAdd(a), cmdList(a), cmdShow(a), cmdDone(a), cmdEdit(a), cmdDescribe(a), cmdNote(a), cmdRm(a),
		cmdSync(a), cmdProjects(a), cmdLabels(a), cmdDir(), cmdServer(a), cmdExport(a),
	)
	return root
}

func findTask(s store.Store, id string) (*core.Task, error) { return store.FindTask(s, id) }

func cmdAdd(a *app) *cobra.Command {
	var description string
	c := &cobra.Command{
		Use:   "add <texto…>",
		Short: "Añade una tarea: tasktui add Pagar luz mañana 18:00 p1 #casa +Finanzas limit 20/11",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.open()
			if err != nil {
				return err
			}
			q := core.ParseQuick(strings.Join(args, " "), core.Today())
			if q.Text == "" {
				return errors.New("La tarea necesita texto")
			}
			t := q.IntoTask(core.Inbox)
			t.Description = strings.TrimSpace(description)
			id, err := s.Add(t, t.Project)
			if err != nil {
				return err
			}
			a.mutating = true
			saved, err := findTask(s, id)
			if err != nil {
				return err
			}
			if a.json {
				printJSONCompact(core.ToDTO(saved))
			} else {
				fmt.Println("Añadida:", taskPlain(saved, true))
			}
			return nil
		},
	}
	c.Flags().StringVarP(&description, "description", "d", "", "Descripción de la tarea")
	return c
}

func cmdList(a *app) *cobra.Command {
	var all bool
	var search string
	c := &cobra.Command{
		Use:     "list [vista]",
		Aliases: []string{"ls"},
		Short:   "Lista tareas: today (por defecto), inbox, upcoming, all, +Proyecto, #etiqueta",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.open()
			if err != nil {
				return err
			}
			view := core.View{Kind: core.ViewToday}
			if len(args) == 1 {
				v, ok := core.ParseView(args[0])
				if !ok {
					return fmt.Errorf("Vista desconocida «%s». Usa: today, inbox, upcoming, all, +Proyecto, #etiqueta", args[0])
				}
				view = v
			}
			groups := store.View(s, view, core.Today(), all, search)
			showProject := view.Kind != core.ViewProject && view.Kind != core.ViewInbox && view.Kind != core.ViewAll
			if a.json {
				out := []core.TaskDTO{}
				for _, g := range groups {
					for _, p := range g.Items {
						if t := s.TaskAt(p); t != nil {
							out = append(out, core.ToDTO(t))
						}
					}
				}
				printJSON(out)
				return nil
			}
			if len(groups) == 0 {
				fmt.Println("Nada pendiente en", view.Title())
			}
			for _, g := range groups {
				if g.Title != nil {
					fmt.Printf("\n%s\n", *g.Title)
				}
				for _, p := range g.Items {
					if t := s.TaskAt(p); t != nil {
						fmt.Println(" ", taskPlain(t, showProject))
					}
				}
			}
			return nil
		},
	}
	c.Flags().BoolVarP(&all, "all", "a", false, "Incluir completadas")
	c.Flags().StringVarP(&search, "search", "s", "", "Filtrar por texto")
	return c
}

func cmdShow(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Muestra una tarea con su descripción y notas",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.open()
			if err != nil {
				return err
			}
			t, err := findTask(s, args[0])
			if err != nil {
				return fmt.Errorf("No existe la tarea %s", args[0])
			}
			if a.json {
				printJSONCompact(core.ToDTO(t))
				return nil
			}
			today := core.Today()
			fmt.Println(taskPlain(t, true))
			if t.Deadline != nil {
				fmt.Println("  🎯 límite", core.Long(*t.Deadline, today))
			}
			if t.Description != "" {
				for _, l := range strings.Split(t.Description, "\n") {
					fmt.Println(" ", l)
				}
			}
			for _, n := range t.Notes {
				if n.Date != nil {
					fmt.Printf("  › %s: %s\n", core.Rel(*n.Date, today), n.Text)
				} else {
					fmt.Println("  ›", n.Text)
				}
			}
			return nil
		},
	}
}

func cmdDone(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "done <id…>",
		Short: "Completa (o reabre) tareas por id",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.open()
			if err != nil {
				return err
			}
			for _, id := range args {
				done, err := s.Toggle(id)
				if err != nil {
					return err
				}
				a.mutating = true
				if done {
					fmt.Println("✓ completada", id)
				} else {
					fmt.Println("○ reabierta", id)
				}
			}
			return nil
		},
	}
}

func cmdEdit(a *app) *cobra.Command {
	var noDate, noDeadline bool
	c := &cobra.Command{
		Use:   "edit <id> [texto…]",
		Short: "Cambia solo lo indicado: tasktui edit a1b2c viernes 18:00 p2 limit 20/11",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.open()
			if err != nil {
				return err
			}
			id := args[0]
			q := core.ParseQuick(strings.Join(args[1:], " "), core.Today())
			err = s.Update(id, func(t *core.Task) {
				q.ApplyPartial(t)
				if noDate {
					t.Due, t.Time, t.Recurrence = nil, nil, nil
				}
				if noDeadline {
					t.Deadline = nil
				}
			})
			if err != nil {
				return err
			}
			a.mutating = true
			t, err := findTask(s, id)
			if err != nil {
				return err
			}
			if a.json {
				printJSONCompact(core.ToDTO(t))
			} else {
				fmt.Println("Guardada:", taskPlain(t, true))
			}
			return nil
		},
	}
	c.Flags().BoolVar(&noDate, "no-date", false, "Quita la fecha, la hora y la repetición")
	c.Flags().BoolVar(&noDeadline, "no-deadline", false, "Quita la fecha límite")
	c.Flags().BoolVar(&noDate, "sin-fecha", false, "")
	_ = c.Flags().MarkHidden("sin-fecha")
	return c
}

func cmdDescribe(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "describe <id> [texto…]",
		Short: "Cambia la descripción: tasktui describe a1b2c texto (sin texto abre $EDITOR)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.open()
			if err != nil {
				return err
			}
			id := args[0]
			var desc string
			if len(args) == 1 {
				cur, err := findTask(s, id)
				if err != nil {
					return fmt.Errorf("No existe la tarea %s", id)
				}
				if desc, err = EditText(id, cur.Description); err != nil {
					return err
				}
			} else {
				desc = strings.Join(args[1:], " ")
			}
			if err := s.Update(id, func(t *core.Task) { t.Description = strings.TrimSpace(desc) }); err != nil {
				return err
			}
			a.mutating = true
			t, err := findTask(s, id)
			if err != nil {
				return err
			}
			if a.json {
				printJSONCompact(core.ToDTO(t))
			} else {
				fmt.Println("Descripción guardada:", taskPlain(t, true))
			}
			return nil
		},
	}
}

func cmdNote(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "note <id> <texto…>",
		Short: "Añade una nota con la fecha de hoy: tasktui note a1b2c Llamó, sigue pendiente",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.open()
			if err != nil {
				return err
			}
			if err := s.AddNote(args[0], strings.Join(args[1:], " ")); err != nil {
				return err
			}
			a.mutating = true
			t, err := findTask(s, args[0])
			if err != nil {
				return err
			}
			if a.json {
				printJSONCompact(core.ToDTO(t))
			} else {
				fmt.Println("Nota añadida a", taskPlain(t, true))
			}
			return nil
		},
	}
}

func cmdRm(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id…>",
		Short: "Elimina tareas por id",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.open()
			if err != nil {
				return err
			}
			for _, id := range args {
				if err := s.Delete(id); err != nil {
					return err
				}
				a.mutating = true
				fmt.Println("Eliminada", id)
			}
			return nil
		},
	}
}

func cmdSync(a *app) *cobra.Command {
	var auto, agent string
	var every int
	var quiet bool
	c := &cobra.Command{
		Use:   "sync",
		Short: "Espejo con la app Recordatorios (macOS): tasktui sync [--auto on|off] [--agent on|off]",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			for _, v := range []string{auto, agent} {
				if v != "" && v != "on" && v != "off" {
					return errors.New("El valor debe ser on u off")
				}
			}
			if agent != "" {
				path, err := reminders.SetAgent(agent == "on", every)
				if err != nil {
					return err
				}
				if !quiet {
					if agent == "on" {
						fmt.Printf("Agente instalado: sincroniza cada %d min (%s)\n", every, path)
					} else {
						fmt.Println("Agente quitado")
					}
				}
				if agent == "off" {
					return nil
				}
			}
			if auto != "" {
				if err := store.SetReminders(auto == "on"); err != nil {
					return err
				}
				if !quiet {
					if auto == "on" {
						fmt.Println("Sincronización automática activada")
					} else {
						fmt.Println("Sincronización automática desactivada")
					}
				}
			}
			s, err := a.open()
			if err != nil {
				return err
			}
			r, err := reminders.Sync(s)
			if err != nil {
				return err
			}
			switch {
			case a.json:
				printJSONCompact(r)
			case !quiet:
				for _, id := range r.Completed {
					fmt.Println("Completada desde Recordatorios:", id)
				}
				for _, id := range r.Imported {
					if t, err := findTask(s, id); err == nil {
						fmt.Println("Importada desde Recordatorios:", taskPlain(t, true))
					}
				}
				fmt.Printf("Recordatorios («%s»): %d nuevos, %d actualizados, %d quitados\n", reminders.List, r.Created, r.Updated, r.Removed)
			}
			return nil
		},
	}
	c.Flags().StringVar(&auto, "auto", "", "Sincronizar en segundo plano tras cada cambio (on) o dejar de hacerlo (off)")
	c.Flags().StringVar(&agent, "agent", "", "Sincronizar cada N minutos con launchd (on), o quitarlo (off)")
	c.Flags().StringVar(&agent, "agente", "", "")
	_ = c.Flags().MarkHidden("agente")
	c.Flags().IntVar(&every, "every", 10, "Minutos entre sincronizaciones del agente")
	c.Flags().IntVar(&every, "cada", 10, "")
	_ = c.Flags().MarkHidden("cada")
	c.Flags().BoolVarP(&quiet, "quiet", "q", false, "Sin salida (para ejecutarlo en segundo plano)")
	return c
}

func cmdProjects(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "projects",
		Short: "Lista los proyectos",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			s, err := a.open()
			if err != nil {
				return err
			}
			type row struct {
				Name string `json:"name"`
				Open int    `json:"open"`
			}
			rows := []row{}
			for _, p := range s.Projects() {
				n := 0
				for _, e := range s.Tasks() {
					if !e.Task.Done && e.Task.Project == p {
						n++
					}
				}
				rows = append(rows, row{p, n})
			}
			if a.json {
				printJSON(rows)
				return nil
			}
			for _, r := range rows {
				fmt.Printf("%4d  %s\n", r.Open, r.Name)
			}
			return nil
		},
	}
}

func cmdLabels(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "labels",
		Short: "Lista las etiquetas",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			s, err := a.open()
			if err != nil {
				return err
			}
			ls := store.Labels(s)
			if a.json {
				type row struct {
					Name string `json:"name"`
					Open int    `json:"open"`
				}
				rows := []row{}
				for _, l := range ls {
					rows = append(rows, row{l.Name, l.Open})
				}
				printJSON(rows)
				return nil
			}
			for _, l := range ls {
				fmt.Printf("%4d  #%s\n", l.Open, l.Name)
			}
			return nil
		},
	}
}

func cmdDir() *cobra.Command {
	return &cobra.Command{
		Use:   "dir [ruta]",
		Short: "Muestra la carpeta de datos (modo local), o la cambia si se da una ruta",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var dir string
			var err error
			if len(args) == 1 {
				if dir, err = store.SetDataDir(args[0]); err != nil {
					return err
				}
				fmt.Println("Carpeta configurada en", store.ConfigPath())
			} else if dir, err = store.DataDir(); err != nil {
				return err
			}
			fmt.Println(dir)
			return nil
		},
	}
}

func cmdExport(a *app) *cobra.Command {
	return &cobra.Command{
		Use:     "export <carpeta>",
		Aliases: []string{"exportar"},
		Short:   "Escribe todas las tareas como .md (formato Obsidian Tasks) en una carpeta",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.open()
			if err != nil {
				return err
			}
			n, err := store.ExportMarkdown(s, args[0])
			if err != nil {
				return err
			}
			fmt.Printf("Exportados %d proyectos a %s\n", n, args[0])
			return nil
		},
	}
}

func cmdServer(a *app) *cobra.Command {
	c := &cobra.Command{
		Use:     "server",
		Aliases: []string{"servidor"},
		Short:   "Servidor: tasktui server [connect <url> | local | remote | push]",
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			m, err := store.CurrentMode()
			if err != nil {
				return err
			}
			modo := "local (" + m.Dir + ")"
			if m.Server != nil {
				modo = "remoto (" + m.Server.URL + ")"
			}
			estado := "sin servidor configurado (tasktui server connect <url>)"
			if s := store.LoadServer(); s != nil {
				if _, err := store.NewClient(*s).Get("projects"); err != nil {
					estado = s.URL + ": " + err.Error()
				} else {
					estado = s.URL + " responde y acepta el token"
				}
			}
			if a.json {
				printJSONCompact(map[string]string{"mode": modo, "server": estado})
			} else {
				fmt.Printf("Modo: %s\nServidor: %s\n", modo, estado)
			}
			return nil
		},
	}
	c.AddCommand(
		&cobra.Command{
			Use:     "connect <url>",
			Aliases: []string{"conectar"},
			Short:   "Guarda la URL y el token (lo pide por la entrada estándar) y activa el modo remoto",
			Args:    cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				fmt.Fprint(os.Stderr, "Token del servidor: ")
				token, _ := bufio.NewReader(os.Stdin).ReadString('\n')
				srv := store.Server{URL: strings.TrimRight(args[0], "/"), Token: strings.TrimSpace(token)}
				if srv.Token == "" {
					return errors.New("El token está vacío")
				}
				if _, err := store.NewClient(srv).Get("projects"); err != nil {
					return err
				}
				if err := store.SaveServer(srv); err != nil {
					return err
				}
				if err := store.SetRemote(true); err != nil {
					return err
				}
				fmt.Printf("Conectado a %s (modo remoto). Token en %s\n", srv.URL, store.ServerPath())
				return nil
			},
		},
		&cobra.Command{
			Use:   "local",
			Short: "Usar los .md locales (el servidor queda configurado)",
			Args:  cobra.NoArgs,
			RunE: func(_ *cobra.Command, _ []string) error {
				if err := store.SetRemote(false); err != nil {
					return err
				}
				d, err := store.DataDir()
				fmt.Println("Modo local:", d)
				return err
			},
		},
		&cobra.Command{
			Use:     "remote",
			Aliases: []string{"remoto"},
			Short:   "Usar el servidor",
			Args:    cobra.NoArgs,
			RunE: func(_ *cobra.Command, _ []string) error {
				s := store.LoadServer()
				if s == nil {
					return errors.New("No hay servidor configurado: tasktui server connect <url>")
				}
				if err := store.SetRemote(true); err != nil {
					return err
				}
				fmt.Println("Modo remoto:", s.URL)
				return nil
			},
		},
		&cobra.Command{
			Use:     "push",
			Aliases: []string{"subir"},
			Short:   "Sube las tareas de los .md locales al servidor (conserva ids; no borra nada)",
			Args:    cobra.NoArgs,
			RunE: func(_ *cobra.Command, _ []string) error {
				s := store.LoadServer()
				if s == nil {
					return errors.New("No hay servidor configurado: tasktui server connect <url>")
				}
				dir, err := store.DataDir()
				if err != nil {
					return err
				}
				local, err := store.OpenFiles(dir)
				if err != nil {
					return err
				}
				var tasks []core.TaskDTO
				for _, e := range local.Tasks() {
					tasks = append(tasks, core.ToDTO(e.Task))
				}
				n, err := store.NewClient(*s).Import(local.Projects(), tasks)
				if err != nil {
					return err
				}
				fmt.Printf("Subidas %d tareas de %s a %s\n", n, local.Dir(), s.URL)
				return nil
			},
		},
	)
	return c
}
