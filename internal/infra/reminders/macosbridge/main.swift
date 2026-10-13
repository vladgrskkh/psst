import Foundation
import EventKit

let eventStore = EKEventStore()
let dateFormatter = DateFormatter()
dateFormatter.locale = Locale(identifier: "en_US_POSIX")
dateFormatter.dateFormat = "yyyy-MM-dd'T'HH:mm:ssXXX"

func send(_ id: Int, result: [String: Any]) {
    let out: [String: Any] = ["id": id, "result": result]
    if let data = try? JSONSerialization.data(withJSONObject: out),
       let str = String(data: data, encoding: .utf8) {
        print(str)
        fflush(stdout)
    }
}

func sendError(_ id: Int, code: String, message: String) {
    let out: [String: Any] = ["id": id, "error": ["code": code, "message": message]]
    if let data = try? JSONSerialization.data(withJSONObject: out),
       let str = String(data: data, encoding: .utf8) {
        print(str)
        fflush(stdout)
    }
}

func dateToString(_ date: Date?) -> Any {
    guard let date = date else { return NSNull() }
    return dateFormatter.string(from: date)
}

func stringToDate(_ s: String) -> Date? {
    return dateFormatter.date(from: s)
}

func requestAccessIfNeeded() {
    if EKEventStore.authorizationStatus(for: .reminder) == .notDetermined {
        let sem = DispatchSemaphore(value: 0)
        eventStore.requestAccess(to: .reminder) { _, _ in sem.signal() }
        sem.wait()
    }
}

func permissionStatus() -> String {
    switch EKEventStore.authorizationStatus(for: .reminder) {
    case .authorized: return "authorized"
    case .notDetermined: return "not-determined"
    default: return "denied"
    }
}

func findReminder(_ id: String) -> EKReminder? {
    for cal in eventStore.calendars(for: .reminder) {
        let predicate = eventStore.predicateForReminders(in: [cal])
        let sem = DispatchSemaphore(value: 0)
        var found: EKReminder?
        eventStore.fetchReminders(matching: predicate) { reminders in
            found = reminders?.first(where: { $0.calendarItemIdentifier == id })
            sem.signal()
        }
        sem.wait()
        if found != nil { return found }
    }
    return nil
}

func handle(_ id: Int, _ method: String, _ params: [String: Any]) {
    switch method {
    case "permissions":
        requestAccessIfNeeded()
        send(id, result: ["status": permissionStatus()])

    case "list-lists":
        var lists: [[String: Any]] = []
        for cal in eventStore.calendars(for: .reminder) {
            lists.append(["id": cal.calendarIdentifier, "name": cal.title])
        }
        send(id, result: ["lists": lists])

    case "ensure-list":
        let name = params["name"] as? String ?? ""
        if name.isEmpty {
            sendError(id, code: "bad-params", message: "name required")
            return
        }
        if let existing = eventStore.calendars(for: .reminder).first(where: { $0.title == name }) {
            send(id, result: ["id": existing.calendarIdentifier])
            return
        }
        let cal = EKCalendar(for: .reminder, eventStore: eventStore)
        cal.title = name
        do {
            try eventStore.saveCalendar(cal, commit: true)
            send(id, result: ["id": cal.calendarIdentifier])
        } catch {
            sendError(id, code: "create-failed", message: error.localizedDescription)
        }

    case "pull":
        let listIDs = params["listIDs"] as? [String] ?? []
        var reminders: [[String: Any]] = []
        var seen = Set<String>()
        let cals = eventStore.calendars(for: .reminder).filter { listIDs.contains($0.calendarIdentifier) }
        for cal in cals {
            let predicate = eventStore.predicateForReminders(in: [cal])
            let sem = DispatchSemaphore(value: 0)
            eventStore.fetchReminders(matching: predicate) { fetched in
                for r in fetched ?? [] where !seen.contains(r.calendarItemIdentifier) {
                    seen.insert(r.calendarItemIdentifier)
                    reminders.append([
                        "id": r.calendarItemIdentifier,
                        "listID": cal.calendarIdentifier,
                        "title": r.title ?? "",
                        "notes": r.notes ?? "",
                        "dueDate": dateToString(r.dueDateComponents.flatMap {
                            Calendar.current.date(from: $0)
                        }),
                        "completed": r.isCompleted,
                        "completionDate": dateToString(r.completionDate),
                    ])
                }
                sem.signal()
            }
            sem.wait()
        }
        send(id, result: ["reminders": reminders])

    case "create":
        guard let listID = params["listID"] as? String,
              let cal = eventStore.calendars(for: .reminder).first(where: { $0.calendarIdentifier == listID }) else {
            sendError(id, code: "bad-params", message: "listID required")
            return
        }
        let reminder = EKReminder(eventStore: eventStore)
        reminder.title = params["title"] as? String ?? ""
        reminder.notes = params["notes"] as? String ?? ""
        reminder.calendar = cal
        if let due = params["dueDate"] as? String, let d = stringToDate(due) {
            reminder.dueDateComponents = Calendar.current.dateComponents(
                [.year, .month, .day, .hour, .minute], from: d)
        }
        do {
            try eventStore.save(reminder, commit: true)
            send(id, result: ["id": reminder.calendarItemIdentifier])
        } catch {
            sendError(id, code: "save-failed", message: error.localizedDescription)
        }

    case "update":
        guard let rid = params["id"] as? String,
              let reminder = findReminder(rid) else {
            sendError(id, code: "not-found", message: "reminder not found")
            return
        }
        let fields = params["fields"] as? [String: Any] ?? [:]
        if let title = fields["title"] as? String { reminder.title = title }
        if let notes = fields["notes"] as? String { reminder.notes = notes }
        if let listID = fields["listID"] as? String,
           let cal = eventStore.calendars(for: .reminder).first(where: { $0.calendarIdentifier == listID }) {
            reminder.calendar = cal
        }
        if fields["dueDate"] is NSNull {
            reminder.dueDateComponents = nil
        } else if let due = fields["dueDate"] as? String, let d = stringToDate(due) {
            reminder.dueDateComponents = Calendar.current.dateComponents(
                [.year, .month, .day, .hour, .minute], from: d)
        }
        if fields["completionDate"] is NSNull {
            reminder.completionDate = nil
        }
        if let completed = fields["completed"] as? Bool {
            reminder.isCompleted = completed
            if completed {
                reminder.completionDate = (fields["completionDate"] as? String).flatMap(stringToDate) ?? Date()
            } else {
                reminder.completionDate = nil
            }
        }
        do {
            try eventStore.save(reminder, commit: true)
            send(id, result: [:])
        } catch {
            sendError(id, code: "save-failed", message: error.localizedDescription)
        }

    case "delete":
        guard let rid = params["id"] as? String,
              let reminder = findReminder(rid) else {
            sendError(id, code: "not-found", message: "reminder not found")
            return
        }
        do {
            try eventStore.remove(reminder, commit: true)
            send(id, result: [:])
        } catch {
            sendError(id, code: "delete-failed", message: error.localizedDescription)
        }

    default:
        sendError(id, code: "unknown-method", message: method)
    }
}

while let line = readLine() {
    guard let data = line.data(using: .utf8),
          let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
          let id = obj["id"] as? Int,
          let method = obj["method"] as? String else {
        continue
    }
    handle(id, method, obj["params"] as? [String: Any] ?? [:])
}
