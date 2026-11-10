import SwiftUI

/// Valores iniciales de una tarea nueva según la lista desde donde se crea.
struct TaskDefaults {
    var project = "Inbox"
    var due: Date?
}

/// Repetición elegible desde la interfaz. `custom` conserva reglas escritas
/// por texto («cada 2 semanas», «cada lunes») al editar.
enum Repeat: Hashable {
    case none, daily, weekly, monthly, yearly
    case custom(es: String, rule: String?)

    static let presets: [Repeat] = [.none, .daily, .weekly, .monthly, .yearly]

    init(es: String?, rule: String?) {
        switch es {
        case nil: self = .none
        case "cada día": self = .daily
        case "cada semana": self = .weekly
        case "cada mes": self = .monthly
        case "cada año": self = .yearly
        case let es?: self = .custom(es: es, rule: rule)
        }
    }

    /// Como se escribe en la captura rápida.
    var es: String? {
        switch self {
        case .none: nil
        case .daily: "cada día"
        case .weekly: "cada semana"
        case .monthly: "cada mes"
        case .yearly: "cada año"
        case .custom(let es, _): es
        }
    }

    /// Formato de Obsidian Tasks (lo que lee el servidor en `PUT`).
    var rule: String? {
        switch self {
        case .none: nil
        case .daily: "every day"
        case .weekly: "every week"
        case .monthly: "every month"
        case .yearly: "every year"
        case .custom(_, let rule): rule
        }
    }

    var title: String {
        guard let es else { return "Nunca" }
        return es.prefix(1).uppercased() + es.dropFirst()
    }
}

/// Hoja para crear o editar una tarea, con la misma disposición en ambos casos:
/// título, notas y botones para fecha, hora, límite, repetición, prioridad,
/// lista y etiquetas.
struct TaskEditorView: View {
    enum Mode {
        case create(TaskDefaults)
        case edit(TaskItem)
    }

    private enum Picking { case due, time, deadline }

    @EnvironmentObject var store: Store
    @Environment(\.dismiss) private var dismiss
    let mode: Mode
    var onSaved: () -> Void = {}

    @State private var title: String
    @State private var notes: String
    @State private var due: Date?
    @State private var time: Date?
    @State private var deadline: Date?
    @State private var repeatRule: Repeat
    @State private var priority: Int
    @State private var project: String
    @State private var labels: [String]
    @State private var comments: [Note]

    @State private var picking: Picking?
    @State private var newComment = ""
    @State private var newLabel = ""
    @State private var askLabel = false
    @State private var confirmDelete = false
    @State private var busy = false
    @State private var error: String?
    @FocusState private var titleFocused: Bool

    init(mode: Mode, onSaved: @escaping () -> Void = {}) {
        self.mode = mode
        self.onSaved = onSaved
        switch mode {
        case .create(let d):
            _title = State(initialValue: "")
            _notes = State(initialValue: "")
            _due = State(initialValue: d.due)
            _time = State(initialValue: nil)
            _deadline = State(initialValue: nil)
            _repeatRule = State(initialValue: .none)
            _priority = State(initialValue: 4)
            _project = State(initialValue: d.project)
            _labels = State(initialValue: [])
            _comments = State(initialValue: [])
        case .edit(let t):
            _title = State(initialValue: t.text)
            _notes = State(initialValue: t.description)
            _due = State(initialValue: t.due.flatMap { Fmt.iso.date(from: $0) })
            _time = State(initialValue: t.time == nil ? nil : Fmt.moment(Fmt.today(), t.time))
            _deadline = State(initialValue: t.deadline.flatMap { Fmt.iso.date(from: $0) })
            _repeatRule = State(initialValue: Repeat(es: t.recurrence, rule: t.recurrence_rule))
            _priority = State(initialValue: t.priority)
            _project = State(initialValue: t.project)
            _labels = State(initialValue: t.labels)
            _comments = State(initialValue: t.notes)
        }
    }

    private var editing: TaskItem? {
        if case .edit(let t) = mode { return t }
        return nil
    }

    private var canSave: Bool { !title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && !busy }

    // MARK: - Vista

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("Título", text: $title, axis: .vertical)
                        .font(.title3.weight(.semibold))
                        .focused($titleFocused)
                    TextField("Notas", text: $notes, axis: .vertical)
                        .lineLimit(2...8)
                }

                Section {
                    FlowLayout(spacing: 8) {
                        dateChip
                        timeChip
                        deadlineChip
                        repeatChip
                        priorityChip
                        projectChip
                        labelsChip
                    }
                    .padding(.vertical, 4)
                }

                if let picking { pickerSection(picking) }

                if editing != nil { commentsSection }

                if let error {
                    Text(error).foregroundStyle(.red)
                }

                if editing != nil {
                    Section {
                        Button("Eliminar tarea", role: .destructive) { confirmDelete = true }
                            .frame(maxWidth: .infinity)
                    }
                }
            }
            .navigationTitle(editing == nil ? "Nueva tarea" : "Detalles")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Cancelar") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) {
                    Button(editing == nil ? "Añadir" : "Guardar") { save() }
                        .fontWeight(.semibold)
                        .disabled(!canSave)
                }
            }
            .confirmationDialog("¿Eliminar esta tarea?", isPresented: $confirmDelete, titleVisibility: .visible) {
                Button("Eliminar", role: .destructive) { remove() }
            }
            .alert("Nueva etiqueta", isPresented: $askLabel) {
                TextField("Nombre", text: $newLabel)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                Button("Cancelar", role: .cancel) { newLabel = "" }
                Button("Añadir") { addLabel() }
            }
        }
        .presentationDragIndicator(.visible)
        .onAppear { if editing == nil { titleFocused = true } }
        .task {
            if store.labels.isEmpty, let api = store.api { store.labels = (try? await api.labels()) ?? [] }
            if store.projects.isEmpty { await store.refreshProjects() }
        }
    }

    // MARK: - Botones (chips)

    private var dateChip: some View {
        Menu {
            quickDates { setDue($0) }
            Button("Elegir fecha…", systemImage: "calendar") {
                if due == nil { setDue(today) }
                show(.due)
            }
            if due != nil {
                Divider()
                Button("Quitar fecha", systemImage: "xmark", role: .destructive) {
                    due = nil; time = nil; repeatRule = .none
                    if picking != .deadline { picking = nil }
                }
            }
        } label: {
            Chip(title: due.flatMap { Fmt.date(Fmt.isoString($0)) } ?? "Fecha",
                 icon: "calendar", color: .red, active: due != nil,
                 alert: due.map { $0 < today } ?? false)
        }
        .accessibilityLabel("Fecha")
        .accessibilityValue(due.flatMap { Fmt.date(Fmt.isoString($0)) } ?? "Sin fecha")
    }

    private var timeChip: some View {
        Menu {
            ForEach([9, 12, 15, 18, 21], id: \.self) { h in
                Button(String(format: "%02d:00", h)) { setTime(hour: h, minute: 0) }
            }
            Button("Elegir hora…", systemImage: "clock") {
                if time == nil { setTime(hour: 9, minute: 0) }
                show(.time)
            }
            if time != nil {
                Divider()
                Button("Quitar hora", systemImage: "xmark", role: .destructive) {
                    time = nil
                    if picking == .time { picking = nil }
                }
            }
        } label: {
            Chip(title: time.map(Fmt.timeString) ?? "Hora", icon: "clock", color: .blue, active: time != nil)
        }
        .accessibilityLabel("Hora")
        .accessibilityValue(time.map(Fmt.timeString) ?? "Sin hora")
    }

    private var deadlineChip: some View {
        Menu {
            quickDates { deadline = $0 }
            Button("Elegir fecha límite…", systemImage: "calendar") {
                if deadline == nil { deadline = today }
                show(.deadline)
            }
            if deadline != nil {
                Divider()
                Button("Quitar fecha límite", systemImage: "xmark", role: .destructive) {
                    deadline = nil
                    if picking == .deadline { picking = nil }
                }
            }
        } label: {
            Chip(title: deadline.flatMap { Fmt.date(Fmt.isoString($0)) }.map { "Límite: \($0)" } ?? "Límite",
                 icon: "flag", color: .orange, active: deadline != nil,
                 alert: deadline.map { $0 < today } ?? false)
        }
        .accessibilityLabel("Fecha límite")
        .accessibilityValue(deadline.flatMap { Fmt.date(Fmt.isoString($0)) } ?? "Sin fecha límite")
    }

    private var repeatChip: some View {
        Menu {
            Picker("Repetir", selection: Binding(get: { repeatRule }, set: { r in
                repeatRule = r
                // Una repetición necesita fecha de inicio.
                if r != .none && due == nil { setDue(today) }
            })) {
                ForEach(repeatOptions, id: \.self) { Text($0.title).tag($0) }
            }
        } label: {
            Chip(title: repeatRule == .none ? "Repetir" : repeatRule.title, icon: "repeat",
                 color: .gray, active: repeatRule != .none)
        }
        .accessibilityLabel("Repetir")
        .accessibilityValue(repeatRule.title)
    }

    private var repeatOptions: [Repeat] {
        if case .custom = repeatRule { return Repeat.presets + [repeatRule] }
        return Repeat.presets
    }

    private var priorityChip: some View {
        Menu {
            Picker("Prioridad", selection: $priority) {
                ForEach([1, 2, 3, 4], id: \.self) { p in
                    Label(Priority.name(p), systemImage: Priority.icon(p)).tag(p)
                }
            }
        } label: {
            Chip(title: priority < 4 ? Priority.name(priority) : "Prioridad",
                 icon: priority < 4 ? Priority.icon(priority) : "exclamationmark",
                 color: Priority.color(priority), active: priority < 4)
        }
        .accessibilityLabel("Prioridad")
        .accessibilityValue(Priority.name(priority))
    }

    private var projectChip: some View {
        let s = store.style(for: project)
        return Menu {
            Picker("Lista", selection: $project) {
                ForEach(projectNames, id: \.self) { name in
                    Label(name, systemImage: store.style(for: name).icon).tag(name)
                }
            }
        } label: {
            Chip(title: project, icon: s.icon, color: s.color, active: true)
        }
        .accessibilityLabel("Lista")
        .accessibilityValue(project)
    }

    private var projectNames: [String] {
        var names = store.projects.map(\.name)
        if !names.contains(where: { $0.caseInsensitiveCompare(project) == .orderedSame }) { names.append(project) }
        // Inbox primero, como la lista por defecto.
        return names.sorted { store.isInbox($0) && !store.isInbox($1) }
    }

    private var labelsChip: some View {
        Menu {
            ForEach(allLabels, id: \.self) { l in
                Toggle("#" + l, isOn: Binding(
                    get: { labels.contains(l) },
                    set: { on in if on { labels.append(l) } else { labels.removeAll { $0 == l } } }
                ))
            }
            if !allLabels.isEmpty { Divider() }
            Button("Nueva etiqueta…", systemImage: "plus") { askLabel = true }
        } label: {
            Chip(title: labels.isEmpty ? "Etiquetas" : labels.map { "#" + $0 }.joined(separator: " "),
                 icon: "number", color: .accentColor, active: !labels.isEmpty)
        }
        .accessibilityLabel("Etiquetas")
        .accessibilityValue(labels.isEmpty ? "Ninguna" : labels.joined(separator: ", "))
    }

    private var allLabels: [String] {
        Array(Set(store.labels.map(\.name) + labels)).sorted { $0.localizedCaseInsensitiveCompare($1) == .orderedAscending }
    }

    /// Atajos comunes de fecha (los mismos que en Recordatorios).
    @ViewBuilder private func quickDates(_ pick: @escaping (Date) -> Void) -> some View {
        let cal = Calendar.current
        Button("Hoy", systemImage: "calendar.circle") { pick(today) }
        Button("Mañana", systemImage: "sunrise") { pick(cal.date(byAdding: .day, value: 1, to: today) ?? today) }
        Button("Este fin de semana", systemImage: "sofa") { pick(next(weekday: 7)) }
        Button("Próxima semana", systemImage: "calendar.badge.plus") { pick(next(weekday: 2)) }
    }

    @ViewBuilder private func pickerSection(_ p: Picking) -> some View {
        Section {
            switch p {
            case .due:
                DatePicker("Fecha", selection: Binding(get: { due ?? today }, set: { setDue($0) }),
                           displayedComponents: .date)
                    .datePickerStyle(.graphical)
            case .time:
                DatePicker("Hora", selection: Binding(get: { time ?? Date() }, set: { time = $0 }),
                           displayedComponents: .hourAndMinute)
                    .datePickerStyle(.wheel)
                    .frame(maxWidth: .infinity)
            case .deadline:
                DatePicker("Fecha límite", selection: Binding(get: { deadline ?? today }, set: { deadline = $0 }),
                           displayedComponents: .date)
                    .datePickerStyle(.graphical)
                    .tint(.orange)
            }
            Button("Listo") { withAnimation { picking = nil } }
                .frame(maxWidth: .infinity)
        } header: {
            switch p {
            case .due: Text("Fecha")
            case .time: Text("Hora")
            case .deadline: Text("Fecha límite")
            }
        }
        .environment(\.locale, Locale(identifier: "es"))
    }

    private var commentsSection: some View {
        Section("Comentarios") {
            ForEach(comments, id: \.self) { n in
                VStack(alignment: .leading, spacing: 2) {
                    if let d = n.date { Text(Fmt.date(d) ?? d).font(.caption).foregroundStyle(.secondary) }
                    Text(n.text)
                }
            }
            HStack {
                TextField("Añadir comentario", text: $newComment, axis: .vertical)
                Button("Añadir") { addComment() }
                    .disabled(newComment.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            }
        }
    }

    // MARK: - Fechas

    private var today: Date { Calendar.current.startOfDay(for: Date()) }

    /// Siguiente día de la semana (1 domingo … 7 sábado), hoy incluido.
    private func next(weekday: Int) -> Date {
        let cal = Calendar.current
        if cal.component(.weekday, from: today) == weekday { return today }
        return cal.nextDate(after: today, matching: DateComponents(weekday: weekday), matchingPolicy: .nextTime) ?? today
    }

    private func setDue(_ d: Date) {
        due = Calendar.current.startOfDay(for: d)
    }

    /// Una hora sin fecha es para hoy (igual que la captura rápida).
    private func setTime(hour: Int, minute: Int) {
        time = Calendar.current.date(bySettingHour: hour, minute: minute, second: 0, of: Date())
        if due == nil { setDue(today) }
    }

    private func show(_ p: Picking) {
        withAnimation { picking = p }
    }

    private func addLabel() {
        let l = newLabel.trimmingCharacters(in: .whitespaces)
            .trimmingCharacters(in: CharacterSet(charactersIn: "#@"))
            .split(whereSeparator: \.isWhitespace).joined(separator: "_")
        newLabel = ""
        if !l.isEmpty && !labels.contains(l) { labels.append(l) }
    }

    // MARK: - Guardar

    /// Texto de captura rápida a partir de lo elegido en la hoja. Cada palabra
    /// del título se escapa con `\` para que el servidor no la interprete como
    /// fecha, prioridad o etiqueta («Llamar a mañana» se queda tal cual).
    private var quickText: String {
        var out = title.split(whereSeparator: \.isWhitespace).map { "\\" + $0 }
        if let due { out.append(Fmt.isoString(due)) }
        if let time { out.append(Fmt.timeString(time)) }
        if let deadline { out.append("limit " + Fmt.isoString(deadline)) }
        if let r = repeatRule.es { out.append(r) }
        if priority < 4 { out.append("p\(priority)") }
        out += labels.map { "#" + $0 }
        return out.joined(separator: " ")
    }

    private func save() {
        guard let api = store.api else { return }
        busy = true
        error = nil
        let description = notes.trimmingCharacters(in: .whitespacesAndNewlines)
        Task {
            do {
                if var t = editing {
                    t.text = title.trimmingCharacters(in: .whitespacesAndNewlines)
                    t.description = description
                    t.due = due.map(Fmt.isoString)
                    t.time = due == nil ? nil : time.map(Fmt.timeString)
                    t.deadline = deadline.map(Fmt.isoString)
                    t.recurrence = due == nil ? nil : repeatRule.es
                    t.recurrence_rule = due == nil ? nil : repeatRule.rule
                    t.priority = priority
                    t.project = project
                    t.labels = labels
                    t.notes = comments
                    _ = try await api.put(t)
                } else {
                    _ = try await api.add(quickText, project: project, description: description)
                }
                onSaved()
                await store.refreshHome()
                await store.syncReminders()
                dismiss()
            } catch {
                self.error = error.localizedDescription
            }
            busy = false
        }
    }

    private func addComment() {
        guard let id = editing?.id, let api = store.api else { return }
        let text = newComment.trimmingCharacters(in: .whitespacesAndNewlines)
        Task {
            do {
                _ = try await api.addNote(id, text)
                comments = try await api.get(id).notes
                newComment = ""
                onSaved()
            } catch { self.error = error.localizedDescription }
        }
    }

    private func remove() {
        guard let id = editing?.id, let api = store.api else { return }
        Task {
            do {
                _ = try await api.delete(id)
                onSaved()
                await store.syncReminders()
                dismiss()
            } catch { self.error = error.localizedDescription }
        }
    }
}

/// Botón con forma de cápsula; se tiñe cuando tiene un valor.
struct Chip: View {
    let title: String
    let icon: String
    let color: Color
    var active = false
    /// Fecha vencida: el texto va en rojo.
    var alert = false

    var body: some View {
        Label(title, systemImage: icon)
            .font(.subheadline.weight(.medium))
            .lineLimit(1)
            .padding(.horizontal, 12)
            .padding(.vertical, 7)
            .foregroundStyle(alert ? Color.red : (active ? color : Color.primary))
            .background(active ? color.opacity(0.16) : Color(.tertiarySystemFill), in: Capsule())
            .contentShape(Capsule())
    }
}

/// Coloca las vistas en filas y salta de línea cuando no caben (útil con Dynamic Type).
struct FlowLayout: Layout {
    var spacing: CGFloat = 8

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let rows = arrange(width: proposal.width ?? .infinity, subviews: subviews)
        let height = rows.map(\.height).reduce(0, +) + spacing * CGFloat(max(rows.count - 1, 0))
        let width = rows.map(\.width).max() ?? 0
        return CGSize(width: proposal.width ?? width, height: height)
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        var y = bounds.minY
        for row in arrange(width: bounds.width, subviews: subviews) {
            var x = bounds.minX
            for i in row.indices {
                let size = subviews[i].sizeThatFits(ProposedViewSize(width: bounds.width, height: nil))
                subviews[i].place(at: CGPoint(x: x, y: y), proposal: ProposedViewSize(width: min(size.width, bounds.width), height: size.height))
                x += size.width + spacing
            }
            y += row.height + spacing
        }
    }

    private struct Row {
        var indices: [Int] = []
        var width: CGFloat = 0
        var height: CGFloat = 0
    }

    private func arrange(width: CGFloat, subviews: Subviews) -> [Row] {
        var rows: [Row] = [Row()]
        for i in subviews.indices {
            let size = subviews[i].sizeThatFits(ProposedViewSize(width: width, height: nil))
            let needed = rows[rows.count - 1].indices.isEmpty ? size.width : rows[rows.count - 1].width + spacing + size.width
            if needed > width && !rows[rows.count - 1].indices.isEmpty {
                rows.append(Row())
            }
            var r = rows[rows.count - 1]
            r.width = r.indices.isEmpty ? size.width : r.width + spacing + size.width
            r.height = max(r.height, size.height)
            r.indices.append(i)
            rows[rows.count - 1] = r
        }
        return rows
    }
}
