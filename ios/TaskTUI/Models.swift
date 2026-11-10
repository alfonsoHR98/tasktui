import Foundation

struct Note: Codable, Hashable {
    var date: String?
    var text: String
}

struct TaskItem: Codable, Identifiable, Hashable {
    var id: String?
    var text: String
    var done: Bool
    var due: String?
    var time: String?
    var deadline: String?
    var priority: Int
    var labels: [String]
    var recurrence: String?
    var recurrence_rule: String?
    var project: String
    var section: String?
    var description: String
    var notes: [Note]

    var stableId: String { id ?? text }
    var hasDescription: Bool { !description.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }
}

struct TaskGroup: Codable, Identifiable {
    var title: String?
    var tasks: [TaskItem]
    var id: String { (title ?? "") + (tasks.first?.id ?? "") }
}

struct ListResponse: Codable {
    var view: String
    var groups: [TaskGroup]
}

struct ProjectInfo: Codable, Identifiable, Hashable {
    var name: String
    var open: Int
    var id: String { name }
}

struct DoneResponse: Codable {
    var task: TaskItem
    var completed: Bool
}

enum Fmt {
    static let iso: DateFormatter = {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.dateFormat = "yyyy-MM-dd"
        return f
    }()
    static let human: DateFormatter = {
        let f = DateFormatter()
        f.locale = Locale(identifier: "es")
        f.setLocalizedDateFormatFromTemplate("EEE d MMM")
        return f
    }()
    static func today() -> String { iso.string(from: Date()) }

    static func date(_ s: String?) -> String? {
        guard let s, let d = iso.date(from: s) else { return nil }
        let cal = Calendar.current
        if cal.isDateInToday(d) { return "Hoy" }
        if cal.isDateInTomorrow(d) { return "Mañana" }
        if cal.isDateInYesterday(d) { return "Ayer" }
        return human.string(from: d)
    }

    static func isOverdue(_ s: String?) -> Bool {
        guard let s, let d = iso.date(from: s) else { return false }
        return d < Calendar.current.startOfDay(for: Date())
    }
}
