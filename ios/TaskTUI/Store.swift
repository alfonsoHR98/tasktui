import SwiftUI

@MainActor
final class Store: ObservableObject {
    @Published var api: API?
    @Published var error: String?
    @Published var projects: [ProjectInfo] = []

    init() {
        if let url = UserDefaults.standard.string(forKey: "server"), let tok = Keychain.get("token") {
            api = API(baseURL: url, token: tok)
        }
    }

    func login(url: String, token: String) async -> String? {
        var u = url.trimmingCharacters(in: .whitespacesAndNewlines)
        if !u.hasPrefix("http") { u = "https://" + u }
        while u.hasSuffix("/") { u.removeLast() }
        let candidate = API(baseURL: u, token: token.trimmingCharacters(in: .whitespacesAndNewlines))
        do {
            let _: [ProjectInfo] = try await candidate.request("projects")
        } catch { return error.localizedDescription }
        UserDefaults.standard.set(u, forKey: "server")
        Keychain.set("token", candidate.token)
        api = candidate
        Reminders.requestAccess()
        return nil
    }

    func logout() {
        Keychain.set("token", nil)
        api = nil
    }

    func refreshProjects() async {
        guard let api else { return }
        projects = (try? await api.projects()) ?? projects
    }

    /// Reprograma los avisos con todas las tareas pendientes.
    func syncReminders() async {
        guard let api, let r = try? await api.tasks(view: "all") else { return }
        Reminders.reschedule(r.groups.flatMap(\.tasks))
    }
}
