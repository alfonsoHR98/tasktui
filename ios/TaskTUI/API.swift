import Foundation
import Security

struct APIError: LocalizedError {
    var message: String
    var errorDescription: String? { message }
}

enum Keychain {
    static func get(_ key: String) -> String? {
        let q: [String: Any] = [kSecClass as String: kSecClassGenericPassword, kSecAttrAccount as String: key,
                                kSecReturnData as String: true, kSecMatchLimit as String: kSecMatchLimitOne]
        var out: AnyObject?
        guard SecItemCopyMatching(q as CFDictionary, &out) == errSecSuccess, let d = out as? Data else { return nil }
        return String(data: d, encoding: .utf8)
    }
    static func set(_ key: String, _ value: String?) {
        let base: [String: Any] = [kSecClass as String: kSecClassGenericPassword, kSecAttrAccount as String: key]
        SecItemDelete(base as CFDictionary)
        guard let value else { return }
        var add = base
        add[kSecValueData as String] = Data(value.utf8)
        SecItemAdd(add as CFDictionary, nil)
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
        let sep = path.contains("?") ? "&" : "?"
        guard let url = URL(string: "\(baseURL)/api/\(path)\(sep)today=\(Fmt.today())") else { throw APIError(message: "URL inválida") }
        var req = URLRequest(url: url)
        req.httpMethod = method
        req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        if let body {
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
            req.httpBody = try JSONSerialization.data(withJSONObject: body)
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
    func add(_ text: String, project: String?) async throws -> TaskItem {
        var b: [String: Any] = ["text": text]
        if let project { b["project"] = project }
        return try await request("tasks", method: "POST", body: b)
    }
    func toggle(_ id: String) async throws -> DoneResponse { try await request("tasks/\(id)/done", method: "POST") }
    func patch(_ id: String, _ body: [String: Any]) async throws -> TaskItem { try await request("tasks/\(id)", method: "PATCH", body: body) }
    func addNote(_ id: String, _ text: String) async throws -> Empty { try await request("tasks/\(id)/notes", method: "POST", body: ["text": text]) }
    func delete(_ id: String) async throws -> Empty { try await request("tasks/\(id)", method: "DELETE") }
    func get(_ id: String) async throws -> TaskItem { try await request("tasks/\(id)") }
}

extension String {
    var urlEncoded: String { addingPercentEncoding(withAllowedCharacters: .alphanumerics) ?? self }
}
