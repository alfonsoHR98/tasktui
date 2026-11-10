import SwiftUI

struct RootView: View {
    @EnvironmentObject var store: Store

    var body: some View {
        TabView {
            NavigationStack { TaskListView(title: "Hoy", view: "today") }
                .tabItem { Label("Hoy", systemImage: "sun.max") }
            NavigationStack { TaskListView(title: "Próximos", view: "upcoming") }
                .tabItem { Label("Próximos", systemImage: "calendar") }
            NavigationStack { TaskListView(title: "Inbox", view: "inbox") }
                .tabItem { Label("Inbox", systemImage: "tray") }
            NavigationStack { ProjectsView() }
                .tabItem { Label("Proyectos", systemImage: "folder") }
        }
        .task { await store.syncReminders() }
    }
}

struct ProjectsView: View {
    @EnvironmentObject var store: Store

    var body: some View {
        List {
            Section {
                NavigationLink("Todas") { TaskListView(title: "Todas", view: "all") }
            }
            Section("Proyectos") {
                ForEach(store.projects.filter { $0.name != "Inbox" }) { p in
                    NavigationLink {
                        TaskListView(title: p.name, view: "+" + p.name, project: p.name)
                    } label: {
                        HStack { Text(p.name); Spacer(); Text("\(p.open)").foregroundStyle(.secondary) }
                    }
                }
            }
            Section {
                Button("Cerrar sesión", role: .destructive) { store.logout() }
            }
        }
        .navigationTitle("Proyectos")
        .task { await store.refreshProjects() }
        .refreshable { await store.refreshProjects() }
    }
}
