import SwiftUI

struct TaskListView: View {
    @EnvironmentObject var store: Store
    let title: String
    let view: String
    var project: String? = nil

    @State private var groups: [TaskGroup] = []
    @State private var query = ""
    @State private var draft = ""
    @State private var selected: TaskItem?
    @State private var error: String?
    @FocusState private var focused: Bool

    var body: some View {
        List {
            if groups.allSatisfy({ $0.tasks.isEmpty }) {
                ContentUnavailableView("Sin tareas", systemImage: "checkmark.circle")
                    .listRowBackground(Color.clear)
            }
            ForEach(groups) { g in
                Section(g.title ?? "") {
                    ForEach(g.tasks) { t in
                        TaskRow(task: t, toggle: { toggle(t) })
                            .contentShape(Rectangle())
                            .onTapGesture { selected = t }
                            .swipeActions {
                                Button(role: .destructive) { remove(t) } label: { Label("Eliminar", systemImage: "trash") }
                            }
                    }
                }
            }
        }
        .listStyle(.insetGrouped)
        .navigationTitle(title)
        .searchable(text: $query, prompt: "Buscar")
        .onChange(of: query) { _, _ in Task { await load() } }
        .refreshable { await load() }
        .task { await load() }
        .safeAreaInset(edge: .bottom) { quickAdd }
        .sheet(item: $selected) { t in
            TaskDetailView(task: t) { Task { await load() } }
        }
        .alert("Error", isPresented: Binding(get: { error != nil }, set: { if !$0 { error = nil } })) {
            Button("OK") { error = nil }
        } message: { Text(error ?? "") }
    }

    var quickAdd: some View {
        HStack(spacing: 8) {
            TextField("Nueva tarea… (mañana 18:00 p1 +Proyecto)", text: $draft)
                .focused($focused)
                .submitLabel(.send)
                .onSubmit(add)
                .textFieldStyle(.roundedBorder)
            Button(action: add) { Image(systemName: "plus.circle.fill").font(.title) }
                .disabled(draft.trimmingCharacters(in: .whitespaces).isEmpty)
        }
        .padding(.horizontal).padding(.vertical, 8)
        .background(.bar)
    }

    func load() async {
        guard let api = store.api else { return }
        do {
            groups = try await api.tasks(view: view, query: query).groups
        } catch is CancellationError {
        } catch { self.error = error.localizedDescription }
    }

    func add() {
        let text = draft.trimmingCharacters(in: .whitespaces)
        guard !text.isEmpty, let api = store.api else { return }
        draft = ""
        Task {
            do {
                _ = try await api.add(text, project: project)
                await load()
                await store.syncReminders()
            } catch { self.error = error.localizedDescription }
        }
    }

    func toggle(_ t: TaskItem) {
        guard let id = t.id, let api = store.api else { return }
        Task {
            do {
                _ = try await api.toggle(id)
                try? await Task.sleep(nanoseconds: 250_000_000)
                await load()
                await store.syncReminders()
            } catch { self.error = error.localizedDescription }
        }
    }

    func remove(_ t: TaskItem) {
        guard let id = t.id, let api = store.api else { return }
        Task {
            do {
                _ = try await api.delete(id)
                await load()
                await store.syncReminders()
            } catch { self.error = error.localizedDescription }
        }
    }
}

struct TaskRow: View {
    let task: TaskItem
    let toggle: () -> Void

    var priorityColor: Color {
        switch task.priority { case 1: .red; case 2: .orange; case 3: .blue; default: .secondary }
    }

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            Button(action: toggle) {
                Image(systemName: task.done ? "checkmark.circle.fill" : "circle")
                    .font(.title3).foregroundStyle(priorityColor)
            }
            .buttonStyle(.plain)
            VStack(alignment: .leading, spacing: 3) {
                Text(task.text).strikethrough(task.done).foregroundStyle(task.done ? .secondary : .primary)
                HStack(spacing: 8) {
                    if let d = Fmt.date(task.due) {
                        Label(d + (task.time.map { " " + $0 } ?? ""), systemImage: "calendar")
                            .foregroundStyle(Fmt.isOverdue(task.due) ? .red : .secondary)
                    }
                    if let d = Fmt.date(task.deadline) { Label(d, systemImage: "flag").foregroundStyle(.orange) }
                    if task.recurrence != nil { Image(systemName: "repeat") }
                    if task.hasDescription { Image(systemName: "text.alignleft") }
                    if !task.notes.isEmpty { Label("\(task.notes.count)", systemImage: "bubble.left") }
                    ForEach(task.labels, id: \.self) { Text("#" + $0) }
                }
                .font(.caption).foregroundStyle(.secondary).labelStyle(.titleAndIcon)
            }
        }
    }
}
