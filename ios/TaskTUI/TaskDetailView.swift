import SwiftUI

struct TaskDetailView: View {
    @EnvironmentObject var store: Store
    @Environment(\.dismiss) var dismiss
    @State var task: TaskItem
    var onChange: () -> Void

    @State private var text = ""
    @State private var description = ""
    @State private var newNote = ""
    @State private var error: String?
    @State private var confirmDelete = false

    var body: some View {
        NavigationStack {
            Form {
                Section("Tarea") {
                    TextField("Texto (acepta fecha, p1, +proyecto…)", text: $text, axis: .vertical)
                    LabeledContent("Proyecto", value: task.project)
                    if let d = Fmt.date(task.due) { LabeledContent("Fecha", value: d + (task.time.map { " " + $0 } ?? "")) }
                    if let d = Fmt.date(task.deadline) { LabeledContent("Límite", value: d) }
                    if let r = task.recurrence { LabeledContent("Repite", value: r) }
                    if !task.labels.isEmpty { LabeledContent("Etiquetas", value: task.labels.map { "#" + $0 }.joined(separator: " ")) }
                }
                Section("Descripción") {
                    TextEditor(text: $description).frame(minHeight: 120)
                }
                Section("Notas") {
                    ForEach(task.notes, id: \.self) { n in
                        VStack(alignment: .leading) {
                            if let d = n.date { Text(Fmt.date(d) ?? d).font(.caption).foregroundStyle(.secondary) }
                            Text(n.text)
                        }
                    }
                    HStack {
                        TextField("Añadir nota", text: $newNote)
                        Button("Añadir") { addNote() }.disabled(newNote.trimmingCharacters(in: .whitespaces).isEmpty)
                    }
                }
                if let error { Text(error).foregroundStyle(.red) }
                Section {
                    Button("Eliminar tarea", role: .destructive) { confirmDelete = true }
                }
            }
            .navigationTitle("Detalle")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Cerrar") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) { Button("Guardar") { save() } }
            }
            .confirmationDialog("¿Eliminar esta tarea?", isPresented: $confirmDelete, titleVisibility: .visible) {
                Button("Eliminar", role: .destructive) { remove() }
            }
        }
        .onAppear { text = task.text; description = task.description }
    }

    func save() {
        guard let id = task.id, let api = store.api else { return }
        var body: [String: Any] = ["description": description]
        if text != task.text { body["text"] = text }
        Task {
            do {
                _ = try await api.patch(id, body)
                onChange()
                await store.syncReminders()
                dismiss()
            } catch { self.error = error.localizedDescription }
        }
    }

    func addNote() {
        guard let id = task.id, let api = store.api else { return }
        let t = newNote
        Task {
            do {
                _ = try await api.addNote(id, t)
                task = try await api.get(id)
                newNote = ""
                onChange()
            } catch { self.error = error.localizedDescription }
        }
    }

    func remove() {
        guard let id = task.id, let api = store.api else { return }
        Task {
            do {
                _ = try await api.delete(id)
                onChange()
                await store.syncReminders()
                dismiss()
            } catch { self.error = error.localizedDescription }
        }
    }
}
