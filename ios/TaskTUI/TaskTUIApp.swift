import SwiftUI

@main
struct TaskTUIApp: App {
    @StateObject private var store = Store()

    var body: some Scene {
        WindowGroup {
            Group {
                if store.api == nil { LoginView() } else { RootView() }
            }
            .environmentObject(store)
        }
    }
}
