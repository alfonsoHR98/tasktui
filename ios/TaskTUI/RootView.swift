import SwiftUI

struct RootView: View {
    @EnvironmentObject var store: Store
    @State private var path: [Route] = []

    var body: some View {
        NavigationStack(path: $path) {
            HomeView()
                .navigationDestination(for: Route.self) { TaskListView(route: $0) }
        }
        .task { await store.syncReminders() }
        .onOpenURL(perform: open)
    }

    /// Enlaces del widget: `tasktui://hoy` abre Hoy; `tasktui://tarea/<id>` abre además la tarea.
    private func open(_ url: URL) {
        guard url.scheme == "tasktui" else { return }
        path = [.smart(.today)]
        if url.host == "tarea", let id = url.pathComponents.last, id != "/" {
            store.openTaskID = id
        }
    }
}

/// Pantalla principal: tarjetas de listas inteligentes y «Mis listas».
struct HomeView: View {
    @EnvironmentObject var store: Store
    @Environment(\.dynamicTypeSize) private var typeSize
    @Environment(\.scenePhase) private var scenePhase
    @State private var newTask = false
    @State private var editingList: ListSheet.Mode?

    private var columns: [GridItem] {
        // Con tamaños de accesibilidad, una tarjeta por fila para que el texto quepa.
        Array(repeating: GridItem(.flexible(), spacing: 12), count: typeSize.isAccessibilitySize ? 1 : 2)
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 24) {
                LazyVGrid(columns: columns, spacing: 12) {
                    ForEach(SmartList.allCases) { list in
                        NavigationLink(value: Route.smart(list)) {
                            SmartCard(list: list, count: store.count(list))
                        }
                        .buttonStyle(.plain)
                    }
                }

                VStack(alignment: .leading, spacing: 8) {
                    Text("Mis listas")
                        .font(.title2.bold())
                        .accessibilityAddTraits(.isHeader)
                    if store.userLists.isEmpty {
                        Text("Aún no tienes listas. Crea una con «Añadir lista».")
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                            .padding(.vertical, 8)
                    } else {
                        listsCard
                    }
                }
            }
            .padding()
        }
        .background(Color(.systemGroupedBackground))
        .navigationTitle("Listas")
        .toolbar {
            // Como en Recordatorios: sin título visible en la pantalla principal.
            ToolbarItem(placement: .principal) { Text("") }
            ToolbarItem(placement: .topBarTrailing) {
                Menu {
                    Button("Cerrar sesión", systemImage: "rectangle.portrait.and.arrow.right", role: .destructive) {
                        store.logout()
                    }
                } label: {
                    Label("Más", systemImage: "ellipsis.circle")
                }
            }
            ToolbarItemGroup(placement: .bottomBar) {
                Button { newTask = true } label: {
                    // En la barra inferior `Label` solo muestra el ícono; se arma a mano.
                    HStack(spacing: 6) {
                        Image(systemName: "plus.circle.fill").font(.title2)
                        Text("Nueva tarea").font(.headline)
                    }
                    .foregroundStyle(Color.accentColor)
                }
                Spacer()
                Button("Añadir lista") { editingList = .create }
            }
        }
        .refreshable { await store.refreshHome() }
        .task { await store.refreshHome() }
        // Al volver a la app (p. ej. tras completar algo en el widget).
        .onChange(of: scenePhase) { _, phase in
            if phase == .active { Task { await store.refreshHome() } }
        }
        .sheet(isPresented: $newTask) {
            TaskEditorView(mode: .create(TaskDefaults())) { Task { await store.refreshHome() } }
        }
        .sheet(item: $editingList) { mode in
            ListSheet(mode: mode)
        }
        .alert("Error", isPresented: Binding(get: { store.error != nil }, set: { if !$0 { store.error = nil } })) {
            Button("OK") { store.error = nil }
        } message: { Text(store.error ?? "") }
    }

    private var listsCard: some View {
        VStack(spacing: 0) {
            ForEach(Array(store.userLists.enumerated()), id: \.element.id) { i, p in
                if i > 0 { Divider().padding(.leading, 56) }
                NavigationLink(value: Route.project(p.name)) {
                    ListRow(name: p.name, style: store.style(for: p.name), count: p.open)
                }
                .buttonStyle(.plain)
                .contextMenu {
                    Button("Información de la lista", systemImage: "info.circle") { editingList = .edit(p.name) }
                }
            }
        }
        .background(Color(.secondarySystemGroupedBackground), in: RoundedRectangle(cornerRadius: 12))
    }
}

/// Tarjeta de lista inteligente: ícono de color, contador y título.
struct SmartCard: View {
    let list: SmartList
    let count: Int?

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .top) {
                ListIcon(style: list.style, size: 32)
                Spacer()
                Text(count.map(String.init) ?? "–")
                    .font(.title.bold())
                    .monospacedDigit()
            }
            Text(list.title)
                .font(.headline)
                .foregroundStyle(.secondary)
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Color(.secondarySystemGroupedBackground), in: RoundedRectangle(cornerRadius: 12))
        .contentShape(RoundedRectangle(cornerRadius: 12))
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(list.title)
        .accessibilityValue(count.map { "\($0) pendientes" } ?? "")
        .accessibilityAddTraits(.isButton)
    }
}

struct ListRow: View {
    let name: String
    let style: ListStyle
    let count: Int

    var body: some View {
        HStack(spacing: 12) {
            ListIcon(style: style)
            Text(name)
                .foregroundStyle(.primary)
            Spacer()
            Text("\(count)")
                .foregroundStyle(.secondary)
                .monospacedDigit()
            Image(systemName: "chevron.right")
                .font(.footnote.weight(.semibold))
                .foregroundStyle(.tertiary)
                .accessibilityHidden(true)
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
        .contentShape(Rectangle())
        .accessibilityElement(children: .combine)
    }
}

/// Crear una lista o cambiar su color e ícono.
struct ListSheet: View {
    enum Mode: Identifiable, Hashable {
        case create
        case edit(String)
        var id: String { if case .edit(let n) = self { n } else { "" } }
    }

    @EnvironmentObject var store: Store
    @Environment(\.dismiss) private var dismiss
    let mode: Mode

    @State private var name = ""
    @State private var style = ListStyle(tint: .azul, icon: "list.bullet")
    @State private var error: String?
    @State private var busy = false

    private var isNew: Bool { mode == .create }
    private let grid = [GridItem(.adaptive(minimum: 44), spacing: 12)]

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    VStack(spacing: 16) {
                        ListIcon(style: style, size: 80)
                        TextField("Nombre de la lista", text: $name)
                            .font(.title2.bold())
                            .multilineTextAlignment(.center)
                            .foregroundStyle(style.color)
                            .padding(10)
                            .background(Color(.tertiarySystemFill), in: RoundedRectangle(cornerRadius: 10))
                            .disabled(!isNew)
                    }
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 8)
                }
                Section("Color") {
                    LazyVGrid(columns: grid, spacing: 12) {
                        ForEach(ListTint.allCases) { t in
                            Button { style.tint = t } label: {
                                Circle().fill(t.color)
                                    .frame(width: 34, height: 34)
                                    .padding(4)
                                    .overlay(Circle().strokeBorder(style.tint == t ? Color.secondary : .clear, lineWidth: 3))
                            }
                            .buttonStyle(.plain)
                            .accessibilityLabel(t.name)
                            .accessibilityAddTraits(style.tint == t ? .isSelected : [])
                        }
                    }
                    .padding(.vertical, 6)
                }
                Section("Ícono") {
                    LazyVGrid(columns: grid, spacing: 12) {
                        ForEach(ListStyle.icons, id: \.self) { icon in
                            Button { style.icon = icon } label: {
                                Image(systemName: icon)
                                    .font(.title3)
                                    .frame(width: 42, height: 42)
                                    .foregroundStyle(style.icon == icon ? .white : .primary)
                                    .background(style.icon == icon ? style.color : Color(.tertiarySystemFill), in: Circle())
                            }
                            .buttonStyle(.plain)
                            .accessibilityLabel(ListStyle.iconName(icon))
                            .accessibilityAddTraits(style.icon == icon ? .isSelected : [])
                        }
                    }
                    .padding(.vertical, 6)
                }
                if let error { Text(error).foregroundStyle(.red) }
            }
            .navigationTitle(isNew ? "Nueva lista" : "Información de la lista")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Cancelar") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) {
                    Button(isNew ? "Añadir" : "OK") { save() }
                        .disabled(name.trimmingCharacters(in: .whitespaces).isEmpty || busy)
                }
            }
        }
        .onAppear {
            if case .edit(let n) = mode {
                name = n
                style = store.style(for: n)
            } else {
                style = .automatic(for: UUID().uuidString)
            }
        }
    }

    private func save() {
        let n = name.trimmingCharacters(in: .whitespaces)
        guard isNew else {
            store.setStyle(style, for: n)
            dismiss()
            return
        }
        busy = true
        Task {
            do {
                try await store.createProject(n)
                store.setStyle(style, for: n)
                dismiss()
            } catch {
                self.error = error.localizedDescription
            }
            busy = false
        }
    }
}
