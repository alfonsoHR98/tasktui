import SwiftUI

struct TaskListView: View {
    @EnvironmentObject var store: Store
    @Environment(\.scenePhase) private var scenePhase
    let route: Route

    @State private var groups: [TaskGroup] = []
    @State private var loaded = false
    @State private var query = ""
    @State private var draft = ""
    @State private var selected: TaskItem?
    @State private var newTask = false
    @State private var error: String?
    @State private var listInfo: ListSheet.Mode?
    /// Tareas recién marcadas: se ven completadas un momento antes de ocultarse.
    @State private var justDone: Set<String> = []
    @AppStorage private var showCompleted: Bool
    @FocusState private var quickFocused: Bool

    init(route: Route) {
        self.route = route
        let key: String
        switch route {
        case .smart(let s): key = s.view
        case .project(let p): key = "+" + p.lowercased()
        }
        _showCompleted = AppStorage(wrappedValue: false, "mostrarCompletadas." + key)
    }

    // MARK: - Datos de la lista

    private var title: String {
        switch route {
        case .smart(let s): s.title
        case .project(let p): p
        }
    }

    private var view: String {
        switch route {
        case .smart(let s): s.view
        case .project(let p): "+" + p
        }
    }

    /// Proyecto donde van las tareas nuevas (`nil`: el servidor usa Inbox).
    private var project: String? {
        if case .project(let p) = route { return p }
        return nil
    }

    private var style: ListStyle {
        switch route {
        case .smart(let s): s.style
        case .project(let p): store.style(for: p)
        }
    }

    /// En las listas que mezclan proyectos, cada fila muestra el suyo.
    private var showsProject: Bool {
        // «Todas» ya agrupa por proyecto, así que no se repite en cada fila.
        if case .smart(let s) = route { return s == .today || s == .upcoming }
        return false
    }

    private var pending: Int { groups.reduce(0) { $0 + $1.tasks.filter { !$0.done }.count } }

    // MARK: - Vista

    var body: some View {
        List {
            header
            if loaded && groups.allSatisfy({ $0.tasks.isEmpty }) {
                ContentUnavailableView(
                    query.isEmpty ? "Sin tareas" : "Sin resultados",
                    systemImage: query.isEmpty ? "checkmark.circle" : "magnifyingglass",
                    description: Text(query.isEmpty ? "Todo listo por aquí." : "Prueba con otra búsqueda.")
                )
                .listRowSeparator(.hidden)
            }
            ForEach(groups) { g in
                Section {
                    ForEach(g.tasks) { t in row(t) }
                } header: {
                    if let title = g.title, !title.isEmpty {
                        // Fondo opaco: el encabezado se queda fijo al desplazar.
                        Text(title.prefix(1).uppercased() + title.dropFirst())
                            .font(.headline)
                            .foregroundStyle(title == "Vencidas" ? .red : style.color)
                            .textCase(nil)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .padding(.horizontal)
                            .padding(.vertical, 6)
                            .background(Color(.systemBackground))
                            .listRowInsets(EdgeInsets())
                    }
                }
            }
        }
        .listStyle(.plain)
        .navigationTitle(title)
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Menu {
                    Button(showCompleted ? "Ocultar completadas" : "Mostrar completadas",
                           systemImage: showCompleted ? "eye.slash" : "eye") { toggleCompleted() }
                    if case .project(let p) = route {
                        Button("Información de la lista", systemImage: "info.circle") { listInfo = .edit(p) }
                    }
                } label: {
                    Label("Más", systemImage: "ellipsis.circle")
                }
            }
        }
        .tint(style.color)
        .searchable(text: $query, prompt: "Buscar")
        .task(id: query) {
            // Espera un poco al escribir para no pedir en cada tecla.
            if !query.isEmpty { try? await Task.sleep(for: .milliseconds(250)) }
            await load()
        }
        .refreshable { await load() }
        .onChange(of: scenePhase) { _, phase in
            if phase == .active { Task { await load() } }
        }
        .task(id: store.openTaskID) { await openRequestedTask() }
        .safeAreaInset(edge: .bottom) { quickAdd }
        .sheet(item: $selected) { t in
            TaskEditorView(mode: .edit(t)) { Task { await load() } }
        }
        .sheet(isPresented: $newTask) {
            TaskEditorView(mode: .create(defaults)) { Task { await load() } }
        }
        .sheet(item: $listInfo) { ListSheet(mode: $0) }
        .alert("Error", isPresented: Binding(get: { error != nil }, set: { if !$0 { error = nil } })) {
            Button("OK") { error = nil }
        } message: { Text(error ?? "") }
    }

    /// Encabezado grande con el color de la lista y el contador de pendientes.
    private var header: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(alignment: .firstTextBaseline) {
                Text(title)
                    .font(.largeTitle.bold())
                Spacer()
                Text("\(pending)")
                    .font(.largeTitle.bold())
                    .monospacedDigit()
                    .accessibilityLabel("\(pending) pendientes")
            }
            .foregroundStyle(style.color)
            Button(showCompleted ? "Ocultar completadas" : "Mostrar completadas") { toggleCompleted() }
                .font(.subheadline)
                .buttonStyle(.borderless)
        }
        .listRowSeparator(.hidden)
        .padding(.top, 4)
    }

    private func row(_ t: TaskItem) -> some View {
        let done = t.done || justDone.contains(t.stableId)
        return TaskRow(task: t, done: done, showsProject: showsProject) { toggle(t) }
            .contentShape(Rectangle())
            .onTapGesture { selected = t }
            .swipeActions(edge: .trailing) {
                Button(role: .destructive) { remove(t) } label: { Label("Eliminar", systemImage: "trash") }
                    .tint(.red)
            }
            .swipeActions(edge: .leading) {
                Button { toggle(t) } label: {
                    Label(done ? "Pendiente" : "Completar", systemImage: done ? "circle" : "checkmark.circle")
                }
                .tint(.green)
            }
            .accessibilityAction(named: "Editar") { selected = t }
    }

    /// Barra inferior: botón de nueva tarea (hoja) y captura rápida por texto.
    private var quickAdd: some View {
        HStack(spacing: 10) {
            Button { newTask = true } label: {
                Image(systemName: "plus.circle.fill")
                    .font(.title2)
                    .foregroundStyle(style.color)
            }
            .accessibilityLabel("Nueva tarea")
            TextField("Captura rápida: mañana 18:00 p1 #etiqueta", text: $draft)
                .focused($quickFocused)
                .submitLabel(.send)
                .onSubmit(add)
                .padding(.horizontal, 12)
                .padding(.vertical, 8)
                .background(Color(.tertiarySystemFill), in: Capsule())
            if !draft.trimmingCharacters(in: .whitespaces).isEmpty {
                Button(action: add) {
                    Image(systemName: "arrow.up.circle.fill")
                        .font(.title2)
                        .foregroundStyle(style.color)
                }
                .accessibilityLabel("Añadir")
            }
        }
        .padding(.horizontal)
        .padding(.vertical, 8)
        .background(.bar)
    }

    /// Valores iniciales de la hoja según la lista abierta.
    private var defaults: TaskDefaults {
        var d = TaskDefaults()
        if let project { d.project = project }
        if case .smart(.today) = route { d.due = Calendar.current.startOfDay(for: Date()) }
        return d
    }

    // MARK: - Acciones

    /// Abre la tarea pedida desde el widget.
    private func openRequestedTask() async {
        guard let id = store.openTaskID, let api = store.api else { return }
        do {
            selected = try await api.get(id)
            // Se limpia al final: cambiarlo antes cancelaría esta misma `.task(id:)`.
            store.openTaskID = nil
        } catch is CancellationError {
        } catch let e as URLError where e.code == .cancelled {
        } catch {
            store.openTaskID = nil
            self.error = error.localizedDescription
        }
    }

    private func toggleCompleted() {
        showCompleted.toggle()
        Task { await load() }
    }

    func load() async {
        guard let api = store.api else { return }
        do {
            groups = try await api.tasks(view: view, query: query, all: showCompleted).groups
            justDone = []
            loaded = true
        } catch is CancellationError {
        } catch let e as URLError where e.code == .cancelled {
        } catch { self.error = error.localizedDescription }
    }

    func add() {
        let text = draft.trimmingCharacters(in: .whitespaces)
        guard !text.isEmpty, let api = store.api else { return }
        draft = ""
        Task {
            do {
                _ = try await api.add(text, project: project)
                await load()
                await store.syncReminders()
            } catch { self.error = error.localizedDescription }
        }
    }

    func toggle(_ t: TaskItem) {
        guard let id = t.id, let api = store.api else { return }
        let completing = !t.done && !justDone.contains(t.stableId)
        withAnimation(.snappy) {
            if completing { justDone.insert(t.stableId) } else { justDone.remove(t.stableId) }
        }
        Task {
            do {
                _ = try await api.toggle(id)
                // Deja ver la marca antes de que desaparezca, como en Recordatorios.
                if completing && !showCompleted { try? await Task.sleep(for: .milliseconds(700)) }
                await load()
                await store.syncReminders()
            } catch {
                justDone.remove(t.stableId)
                self.error = error.localizedDescription
            }
        }
    }

    func remove(_ t: TaskItem) {
        guard let id = t.id, let api = store.api else { return }
        withAnimation {
            for i in groups.indices { groups[i].tasks.removeAll { $0.id == id } }
        }
        Task {
            do {
                _ = try await api.delete(id)
                await load()
                await store.syncReminders()
            } catch {
                self.error = error.localizedDescription
                await load()
            }
        }
    }
}

struct TaskRow: View {
    @EnvironmentObject var store: Store
    let task: TaskItem
    let done: Bool
    var showsProject = false
    let toggle: () -> Void

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 12) {
            Button(action: toggle) {
                Image(systemName: done ? "largecircle.fill.circle" : "circle")
                    .font(.title2)
                    .foregroundStyle(Priority.color(task.priority))
                    .contentTransition(.symbolEffect(.replace))
            }
            .buttonStyle(.plain)
            .accessibilityLabel(done ? "Marcar como pendiente" : "Completar")
            .accessibilityValue(task.priority < 4 ? "Prioridad \(Priority.name(task.priority).lowercased())" : "")

            VStack(alignment: .leading, spacing: 3) {
                Text(task.text)
                    .foregroundStyle(done ? .secondary : .primary)
                if task.hasDescription {
                    Text(task.description)
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                        .lineLimit(2)
                }
                details
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .padding(.vertical, 2)
    }

    /// Fecha (roja si venció), límite, repetición, comentarios, etiquetas y proyecto.
    @ViewBuilder private var details: some View {
        let parts = detailParts
        if let first = parts.first {
            // Un único `Text` concatenado se ajusta en varias líneas con Dynamic Type.
            parts.dropFirst().reduce(first) { $0 + Text("   ") + $1 }
                .font(.subheadline)
                .foregroundStyle(.secondary)
        }
    }

    private var detailParts: [Text] {
        var out: [Text] = []
        if let w = Fmt.when(task.due, task.time) {
            let late = !done && Fmt.isPast(task.due, task.time)
            out.append(Text(w).foregroundStyle(late ? Color.red : Color.secondary))
        }
        if let d = Fmt.date(task.deadline) {
            let late = !done && Fmt.isOverdue(task.deadline)
            out.append(Text("\(Image(systemName: "flag.fill")) \(d)").foregroundStyle(late ? Color.red : Color.orange))
        }
        if let r = task.recurrence { out.append(Text("\(Image(systemName: "repeat")) \(r)")) }
        if !task.notes.isEmpty { out.append(Text("\(Image(systemName: "bubble.left")) \(task.notes.count)")) }
        if !task.labels.isEmpty {
            out.append(Text(task.labels.map { "#" + $0 }.joined(separator: " ")).foregroundStyle(Color.accentColor))
        }
        if showsProject {
            let s = store.style(for: task.project)
            out.append(Text("\(Image(systemName: s.icon)) \(task.project)").foregroundStyle(s.color))
        }
        return out
    }
}
