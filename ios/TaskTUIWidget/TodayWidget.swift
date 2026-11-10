import SwiftUI
import WidgetKit

@main
struct TaskTUIWidgets: WidgetBundle {
    var body: some Widget {
        TodayWidget()
    }
}

// MARK: - Datos

struct TodayEntry: TimelineEntry {
    enum State { case ok, loggedOut, failed(String) }

    var date: Date
    var tasks: [TaskItem]
    var state: State = .ok

    var count: Int { tasks.count }

    static let sample = TodayEntry(date: .now, tasks: [
        .sample("Pagar luz", priority: 1, time: "18:00"),
        .sample("Revisar contrato", priority: 2),
        .sample("Comprar leche", priority: 4),
        .sample("Llamar a mamá", priority: 3),
    ])
}

struct TodayProvider: TimelineProvider {
    func placeholder(in context: Context) -> TodayEntry { .sample }

    func getSnapshot(in context: Context, completion: @escaping (TodayEntry) -> Void) {
        if context.isPreview { return completion(.sample) }
        Task { completion(await load()) }
    }

    func getTimeline(in context: Context, completion: @escaping (Timeline<TodayEntry>) -> Void) {
        Task {
            let entry = await load()
            // Se refresca cada 15 min y justo a medianoche (cambia qué es «hoy»).
            let cal = Calendar.current
            let midnight = cal.startOfDay(for: cal.date(byAdding: .day, value: 1, to: .now) ?? .now)
            let next = min(Date.now.addingTimeInterval(15 * 60), midnight)
            completion(Timeline(entries: [entry], policy: .after(next)))
        }
    }

    private func load() async -> TodayEntry {
        guard let api = Shared.api() else { return TodayEntry(date: .now, tasks: [], state: .loggedOut) }
        do {
            let r = try await api.tasks(view: "today")
            return TodayEntry(date: .now, tasks: r.groups.flatMap(\.tasks).filter { !$0.done })
        } catch {
            return TodayEntry(date: .now, tasks: [], state: .failed(error.localizedDescription))
        }
    }
}

// MARK: - Widget

struct TodayWidget: Widget {
    var body: some WidgetConfiguration {
        StaticConfiguration(kind: "Hoy", provider: TodayProvider()) { entry in
            TodayWidgetView(entry: entry)
                .containerBackground(.background, for: .widget)
        }
        .configurationDisplayName("Hoy")
        .description("Tus tareas de hoy y las vencidas. Toca el círculo para completarlas.")
        .supportedFamilies([.systemSmall, .systemMedium, .systemLarge,
                            .accessoryCircular, .accessoryRectangular, .accessoryInline])
    }
}

struct TodayWidgetView: View {
    @Environment(\.widgetFamily) private var family
    let entry: TodayEntry

    private let style = SmartList.today.style

    var body: some View {
        switch family {
        case .accessoryCircular: circular
        case .accessoryRectangular: rectangular
        case .accessoryInline: Text(inlineText)
        case .systemMedium: medium
        default: list(max: family == .systemLarge ? 7 : 2)
        }
    }

    // MARK: Pantalla de inicio

    /// Pequeño y grande: encabezado en una línea y la lista debajo.
    private func list(max: Int) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 6) {
                ListIcon(style: style, size: 24)
                Text("Hoy")
                    .font(.headline)
                    .foregroundStyle(style.color)
                Spacer(minLength: 4)
                Text("\(entry.count)")
                    .font(.title2.bold())
                    .foregroundStyle(style.color)
                    .contentTransition(.numericText())
            }
            tasks(max: max)
        }
        // Alineado arriba: si algo no cabe, se recorta por abajo y no el encabezado.
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .widgetURL(URL(string: "tasktui://hoy"))
    }

    /// Mediano: contador a la izquierda y tareas a la derecha, como en Recordatorios.
    private var medium: some View {
        HStack(alignment: .top, spacing: 14) {
            VStack(alignment: .leading, spacing: 4) {
                ListIcon(style: style, size: 30)
                Spacer(minLength: 0)
                Text("\(entry.count)")
                    .font(.largeTitle.bold())
                    .foregroundStyle(style.color)
                    .contentTransition(.numericText())
                Text("Hoy")
                    .font(.headline)
                    .foregroundStyle(style.color)
            }
            tasks(max: 4)
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        }
        .widgetURL(URL(string: "tasktui://hoy"))
    }

    @ViewBuilder private func tasks(max: Int) -> some View {
        switch entry.state {
        case .loggedOut:
            message("Abre tasktui para iniciar sesión", icon: "person.crop.circle.badge.exclamationmark")
        case .failed(let e):
            message(e, icon: "wifi.exclamationmark")
        case .ok where entry.tasks.isEmpty:
            message("Todo listo por hoy", icon: "checkmark.circle")
        case .ok:
            VStack(alignment: .leading, spacing: 6) {
                ForEach(entry.tasks.prefix(max), id: \.stableId) { WidgetTaskRow(task: $0) }
                if entry.tasks.count > max {
                    Text("y \(entry.tasks.count - max) más")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
        }
    }

    private func message(_ text: String, icon: String) -> some View {
        Label(text, systemImage: icon)
            .font(.caption)
            .foregroundStyle(.secondary)
    }

    // MARK: Pantalla de bloqueo

    private var circular: some View {
        ZStack {
            AccessoryWidgetBackground()
            VStack(spacing: 0) {
                Image(systemName: "checklist")
                    .font(.caption)
                Text("\(entry.count)")
                    .font(.title3.bold())
            }
        }
        .widgetURL(URL(string: "tasktui://hoy"))
        .accessibilityLabel("\(entry.count) tareas para hoy")
    }

    private var rectangular: some View {
        VStack(alignment: .leading, spacing: 1) {
            Label("Hoy · \(entry.count)", systemImage: "checklist")
                .font(.headline)
                .widgetAccentable()
            if entry.tasks.isEmpty {
                Text(entry.state.isLoggedOut ? "Inicia sesión en tasktui" : "Todo listo")
                    .foregroundStyle(.secondary)
            }
            ForEach(entry.tasks.prefix(2), id: \.stableId) { t in
                Text(t.text).lineLimit(1)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .widgetURL(URL(string: "tasktui://hoy"))
    }

    private var inlineText: String {
        switch entry.count {
        case 0: "Hoy: todo listo"
        case 1: "Hoy: 1 tarea"
        default: "Hoy: \(entry.count) tareas"
        }
    }
}

/// Fila de tarea: el círculo la completa; el resto abre la tarea en la app.
struct WidgetTaskRow: View {
    let task: TaskItem

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 6) {
            if let id = task.id {
                Button(intent: CompleteTaskIntent(taskID: id)) {
                    Image(systemName: "circle")
                        .font(.body)
                        .foregroundStyle(Priority.color(task.priority))
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Completar \(task.text)")
            }
            Link(destination: URL(string: "tasktui://tarea/\(task.id ?? "")") ?? URL(string: "tasktui://hoy")!) {
                VStack(alignment: .leading, spacing: 0) {
                    Text(task.text)
                        .font(.subheadline)
                        .lineLimit(1)
                    if let when = subtitle {
                        Text(when)
                            .font(.caption2)
                            .foregroundStyle(isLate ? .red : .secondary)
                            .lineLimit(1)
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }
        }
    }

    /// En el widget de Hoy solo interesa la hora o, si venció, desde cuándo.
    private var subtitle: String? {
        if Fmt.isOverdue(task.due) { return Fmt.when(task.due, task.time) }
        if let time = task.time { return time }
        if Fmt.isOverdue(task.deadline) || task.deadline == Fmt.today() {
            return Fmt.date(task.deadline).map { "Límite: \($0)" }
        }
        return nil
    }

    private var isLate: Bool {
        Fmt.isPast(task.due, task.time) || Fmt.isOverdue(task.deadline)
    }
}

private extension TodayEntry.State {
    var isLoggedOut: Bool { if case .loggedOut = self { true } else { false } }
}

private extension TaskItem {
    static func sample(_ text: String, priority: Int, time: String? = nil) -> TaskItem {
        TaskItem(id: String(text.prefix(5)), text: text, done: false, due: Fmt.today(), time: time, deadline: nil, done_date: nil,
                 priority: priority, labels: [], recurrence: nil, recurrence_rule: nil, project: "Inbox",
                 section: nil, description: "", notes: [])
    }
}

#Preview(as: .systemMedium) {
    TodayWidget()
} timeline: {
    TodayEntry.sample
    TodayEntry(date: .now, tasks: [])
}

#Preview(as: .systemSmall) {
    TodayWidget()
} timeline: {
    TodayEntry.sample
}
