import SwiftUI
import WidgetKit

@MainActor
final class Store: ObservableObject {
    @Published var api: API?
    @Published var error: String?
    @Published var projects: [ProjectInfo] = []
    @Published var labels: [LabelInfo] = []
    /// Pendientes por lista inteligente (para las tarjetas de inicio).
    @Published var counts: [SmartList: Int] = [:]
    /// Color e ícono de cada proyecto (clave en minúsculas).
    @Published private(set) var styles: [String: ListStyle] = [:]
    /// Tarea por abrir al llegar desde el widget (`tasktui://tarea/<id>`).
    @Published var openTaskID: String?

    private static let stylesKey = "estilosListas"

    init() {
        Shared.migrate()
        api = Shared.api()
        styles = Shared.listStyles()
    }

    func login(url: String, token: String) async -> String? {
        var u = url.trimmingCharacters(in: .whitespacesAndNewlines)
        if !u.hasPrefix("http") { u = "https://" + u }
        while u.hasSuffix("/") { u.removeLast() }
        let candidate = API(baseURL: u, token: token.trimmingCharacters(in: .whitespacesAndNewlines))
        do {
            let _: [ProjectInfo] = try await candidate.request("projects")
        } catch { return error.localizedDescription }
        Shared.defaults.set(u, forKey: "server")
        Keychain.set("token", candidate.token)
        api = candidate
        Reminders.requestAccess()
        WidgetCenter.shared.reloadAllTimelines()
        return nil
    }

    func logout() {
        Keychain.set("token", nil)
        api = nil
        WidgetCenter.shared.reloadAllTimelines()
    }

    // MARK: - Listas

    /// Proyectos del usuario sin el Inbox (que tiene su propia tarjeta).
    var userLists: [ProjectInfo] { projects.filter { !isInbox($0.name) } }

    func isInbox(_ name: String) -> Bool { name.caseInsensitiveCompare("Inbox") == .orderedSame }

    func style(for project: String) -> ListStyle {
        if isInbox(project) { return .inbox }
        return styles[project.lowercased()] ?? .automatic(for: project)
    }

    func setStyle(_ style: ListStyle, for project: String) {
        styles[project.lowercased()] = style
        if let data = try? JSONEncoder().encode(styles) {
            Shared.defaults.set(data, forKey: Self.stylesKey)
        }
        WidgetCenter.shared.reloadAllTimelines()
    }

    func count(_ list: SmartList) -> Int? {
        switch list {
        case .inbox: projects.first { isInbox($0.name) }?.open
        case .all: projects.isEmpty ? nil : projects.reduce(0) { $0 + $1.open }
        default: counts[list]
        }
    }

    func refreshProjects() async {
        guard let api else { return }
        projects = (try? await api.projects()) ?? projects
    }

    /// Proyectos, etiquetas y contadores de Hoy y Próximos en paralelo.
    func refreshHome() async {
        guard let api else { return }
        async let p = api.projects()
        async let l = api.labels()
        async let t = api.tasks(view: SmartList.today.view)
        async let u = api.tasks(view: SmartList.upcoming.view)
        do {
            projects = try await p
            labels = (try? await l) ?? labels
            counts[.today] = try await t.groups.reduce(0) { $0 + $1.tasks.filter { !$0.done }.count }
            counts[.upcoming] = try await u.groups.reduce(0) { $0 + $1.tasks.count }
        } catch is CancellationError {
        } catch {
            self.error = error.localizedDescription
        }
    }

    func createProject(_ name: String) async throws {
        guard let api else { return }
        _ = try await api.createProject(name)
        await refreshProjects()
    }

    /// Reprograma los avisos con todas las tareas pendientes y refresca el widget.
    /// Se llama después de cada cambio (crear, editar, completar, eliminar).
    func syncReminders() async {
        WidgetCenter.shared.reloadAllTimelines()
        guard let api, let r = try? await api.tasks(view: "all") else { return }
        Reminders.reschedule(r.groups.flatMap(\.tasks))
    }
}
