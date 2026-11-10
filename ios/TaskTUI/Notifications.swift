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
        for (when, t) in items.sorted(by: { $0.0 < $1.0 }).prefix(60) {
            let c = UNMutableNotificationContent()
            c.title = t.text
            c.body = t.project
            c.sound = .default
            let trig = UNCalendarNotificationTrigger(dateMatching: cal.dateComponents([.year, .month, .day, .hour, .minute], from: when), repeats: false)
            center.add(UNNotificationRequest(identifier: t.stableId, content: c, trigger: trig))
        }
    }
}
