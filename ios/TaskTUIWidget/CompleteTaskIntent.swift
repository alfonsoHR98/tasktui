import AppIntents
import WidgetKit

/// Completa una tarea desde el widget sin abrir la app. Al terminar, WidgetKit
/// vuelve a pedir la línea de tiempo y la tarea desaparece de la lista.
struct CompleteTaskIntent: AppIntent {
    static var title: LocalizedStringResource = "Completar tarea"
    static var isDiscoverable = false

    @Parameter(title: "Tarea")
    var taskID: String

    init() {}

    init(taskID: String) {
        self.taskID = taskID
    }

    func perform() async throws -> some IntentResult {
        guard let api = Shared.api() else { throw APIError(message: "Abre tasktui para iniciar sesión") }
        _ = try await api.toggle(taskID)
        return .result()
    }
}
