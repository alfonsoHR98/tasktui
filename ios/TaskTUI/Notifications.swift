import Foundation
import UserNotifications

enum Reminders {
    static func requestAccess() {
        UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound, .badge]) { _, _ in }
    }

    /// Reprograma los avisos locales a partir de las tareas pendientes con fecha.
    static func reschedule(_ tasks: [TaskItem]) {
        let center = UNUserNotificationCenter.current()
        center.removeAllPendingNotificationRequests()
        let cal = Calendar.current
        let now = Date()
        var items: [(Date, TaskItem)] = []
        for t in tasks where !t.done {
            guard let d = t.due, let day = Fmt.iso.date(from: d) else { continue }
            var comps = cal.dateComponents([.year, .month, .day], from: day)
            if let time = t.time, time.count >= 5 {
                comps.hour = Int(time.prefix(2)); comps.minute = Int(time.dropFirst(3).prefix(2))
            } else {
                comps.hour = 9; comps.minute = 0
            }
            if let when = cal.date(from: comps), when > now { items.append((when, t)) }
        }
        // Aviso insistente: al vencer y luego repetido hasta que completes la tarea.
        // (iOS limita a 64 avisos pendientes: 20 tareas × 3 avisos = 60.)
        let repeats = [0, 5, 15]
        for (when, t) in items.sorted(by: { $0.0 < $1.0 }).prefix(20) {
            for (i, extra) in repeats.enumerated() {
                let fire = when.addingTimeInterval(Double(extra) * 60)
                let c = UNMutableNotificationContent()
                c.title = i == 0 ? t.text : "Pendiente: \(t.text)"
                c.body = t.project
                c.sound = .default
                c.threadIdentifier = t.stableId
                if #available(iOS 15.0, *) { c.interruptionLevel = .active }
                let trig = UNCalendarNotificationTrigger(dateMatching: cal.dateComponents([.year, .month, .day, .hour, .minute], from: fire), repeats: false)
                center.add(UNNotificationRequest(identifier: "\(t.stableId)-\(i)", content: c, trigger: trig))
            }
        }
        // Limpia del Centro de notificaciones los avisos de tareas ya completadas.
        let doneIds = tasks.filter { $0.done }.flatMap { t in repeats.indices.map { "\(t.stableId)-\($0)" } }
        center.removeDeliveredNotifications(withIdentifiers: doneIds)
    }
}
