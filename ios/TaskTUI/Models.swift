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
    var done_date: String?
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

struct LabelInfo: Codable, Identifiable, Hashable {
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
    static let hhmm: DateFormatter = {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.dateFormat = "HH:mm"
        return f
    }()
    static func today() -> String { iso.string(from: Date()) }

    /// Fecha en `AAAA-MM-DD` / hora en `HH:MM` (lo que espera el servidor).
    static func isoString(_ d: Date) -> String { iso.string(from: d) }
    static func timeString(_ d: Date) -> String { hhmm.string(from: d) }

    /// Une `AAAA-MM-DD` y `HH:MM` en una sola fecha local.
    static func moment(_ day: String?, _ time: String?) -> Date? {
        guard let day, let d = iso.date(from: day) else { return nil }
        guard let time, let t = hhmm.date(from: time) else { return d }
        let cal = Calendar.current
        let hm = cal.dateComponents([.hour, .minute], from: t)
        return cal.date(bySettingHour: hm.hour ?? 0, minute: hm.minute ?? 0, second: 0, of: d)
    }

    /// Fecha (y hora) legibles: «Mañana, 18:00».
    static func when(_ day: String?, _ time: String?) -> String? {
        guard let d = date(day) else { return nil }
        guard let time else { return d }
        return "\(d), \(time)"
    }

    /// Venció: el día ya pasó o, si es hoy y tiene hora, la hora ya pasó.
    static func isPast(_ day: String?, _ time: String?) -> Bool {
        if isOverdue(day) { return true }
        guard time != nil, let m = moment(day, time) else { return false }
        return m < Date()
    }

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
