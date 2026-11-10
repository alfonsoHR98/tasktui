import Foundation
import Security

struct APIError: LocalizedError {
    var message: String
    var errorDescription: String? { message }
}

/// Datos compartidos entre la app y el widget (App Group).
enum Shared {
    static let group = "group.dev.madaradev.tasktui"
    static let defaults = UserDefaults(suiteName: group) ?? .standard

    /// Cliente con la sesión guardada, o `nil` si no hay sesión.
    /// Color e ícono elegidos para cada lista (clave en minúsculas).
    static func listStyles() -> [String: ListStyle] {
        guard let data = defaults.data(forKey: "estilosListas") else { return [:] }
        return (try? JSONDecoder().decode([String: ListStyle].self, from: data)) ?? [:]
    }

    static func api() -> API? {
        guard let url = defaults.string(forKey: "server"), let tok = Keychain.get("token") else { return nil }
        return API(baseURL: url, token: tok)
    }

    /// Mueve la sesión de versiones anteriores (solo de la app) al espacio compartido.
    static func migrate() {
        if defaults.string(forKey: "server") == nil, let old = UserDefaults.standard.string(forKey: "server") {
            defaults.set(old, forKey: "server")
        }
        if defaults.data(forKey: "estilosListas") == nil, let old = UserDefaults.standard.data(forKey: "estilosListas") {
            defaults.set(old, forKey: "estilosListas")
        }
        // Volver a guardarlo lo deja en el grupo compartido y accesible tras el primer desbloqueo.
        if let tok = Keychain.get("token") { Keychain.set("token", tok) }
    }
}

/// Token en el Llavero, en el App Group para que el widget también lo lea.
enum Keychain {
    static func get(_ key: String) -> String? {
        let q: [String: Any] = [kSecClass as String: kSecClassGenericPassword, kSecAttrAccount as String: key,
                                kSecReturnData as String: true, kSecMatchLimit as String: kSecMatchLimitOne]
        var out: AnyObject?
        guard SecItemCopyMatching(q as CFDictionary, &out) == errSecSuccess, let d = out as? Data else { return nil }
        return String(data: d, encoding: .utf8)
    }
    static func set(_ key: String, _ value: String?) {
        // Sin grupo en la consulta borra también la copia antigua (fuera del grupo).
        let base: [String: Any] = [kSecClass as String: kSecClassGenericPassword, kSecAttrAccount as String: key]
        SecItemDelete(base as CFDictionary)
        guard let value else { return }
        var add = base
        add[kSecValueData as String] = Data(value.utf8)
        add[kSecAttrAccessGroup as String] = Shared.group
        // El widget se actualiza también con el teléfono bloqueado.
        add[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlock
        if SecItemAdd(add as CFDictionary, nil) != errSecSuccess {
            // Sin el App Group en la firma: se guarda solo para la app y no se pierde la sesión.
            add.removeValue(forKey: kSecAttrAccessGroup as String)
            SecItemAdd(add as CFDictionary, nil)
        }
    }
}

final class API {
    var baseURL: String
    var token: String

    init(baseURL: String, token: String) {
        self.baseURL = baseURL
        self.token = token
    }

    func request<T: Decodable>(_ path: String, method: String = "GET", body: [String: Any]? = nil, as: T.Type = T.self) async throws -> T {
        let data = try body.map { try JSONSerialization.data(withJSONObject: $0) }
        return try await request(path, method: method, data: data)
    }

    func request<T: Decodable>(_ path: String, method: String, data body: Data?) async throws -> T {
        let sep = path.contains("?") ? "&" : "?"
        guard let url = URL(string: "\(baseURL)/api/\(path)\(sep)today=\(Fmt.today())") else { throw APIError(message: "URL inválida") }
        var req = URLRequest(url: url)
        req.httpMethod = method
        req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        if let body {
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
            req.httpBody = body
        }
        let (data, resp) = try await URLSession.shared.data(for: req)
        let code = (resp as? HTTPURLResponse)?.statusCode ?? 0
        guard (200..<300).contains(code) else {
            if code == 401 { throw APIError(message: "Token incorrecto") }
            let msg = (try? JSONSerialization.jsonObject(with: data) as? [String: Any])?["error"] as? String
            throw APIError(message: msg ?? "Error \(code)")
        }
        return try JSONDecoder().decode(T.self, from: data)
    }

    struct Empty: Decodable {}

    func tasks(view: String, query: String = "", all: Bool = false) async throws -> ListResponse {
        var p = "tasks?view=\(view.urlEncoded)"
        if !query.isEmpty { p += "&q=\(query.urlEncoded)" }
        if all { p += "&all=1" }
        return try await request(p)
    }
    func projects() async throws -> [ProjectInfo] { try await request("projects") }
    func labels() async throws -> [LabelInfo] { try await request("labels") }
    func createProject(_ name: String) async throws -> Empty { try await request("projects", method: "POST", body: ["name": name]) }
    func add(_ text: String, project: String?, description: String = "") async throws -> TaskItem {
        var b: [String: Any] = ["text": text]
        if let project { b["project"] = project }
        if !description.isEmpty { b["description"] = description }
        return try await request("tasks", method: "POST", body: b)
    }
    func toggle(_ id: String) async throws -> DoneResponse { try await request("tasks/\(id)/done", method: "POST") }
    func patch(_ id: String, _ body: [String: Any]) async throws -> TaskItem { try await request("tasks/\(id)", method: "PATCH", body: body) }
    /// Reemplaza la tarea completa (`PUT`).
    func put(_ task: TaskItem) async throws -> TaskItem {
        guard let id = task.id else { throw APIError(message: "La tarea no tiene id") }
        return try await request("tasks/\(id)", method: "PUT", data: try JSONEncoder().encode(task))
    }
    func addNote(_ id: String, _ text: String) async throws -> Empty { try await request("tasks/\(id)/notes", method: "POST", body: ["text": text]) }
    func delete(_ id: String) async throws -> Empty { try await request("tasks/\(id)", method: "DELETE") }
    func get(_ id: String) async throws -> TaskItem { try await request("tasks/\(id)") }
}

extension String {
    var urlEncoded: String { addingPercentEncoding(withAllowedCharacters: .alphanumerics) ?? self }
}
