import SwiftUI

struct LoginView: View {
    @EnvironmentObject var store: Store
    @State private var url = "https://tasks.madara.dev"
    @State private var token = ""
    @State private var error: String?
    @State private var busy = false

    var body: some View {
        NavigationStack {
            Form {
                Section("Servidor") {
                    TextField("https://tasks.madara.dev", text: $url)
                        .textInputAutocapitalization(.never).autocorrectionDisabled().keyboardType(.URL)
                    SecureField("Token", text: $token)
                        .textInputAutocapitalization(.never).autocorrectionDisabled()
                }
                if let error { Text(error).foregroundStyle(.red) }
                Button {
                    busy = true
                    Task {
                        error = await store.login(url: url, token: token)
                        busy = false
                    }
                } label: {
                    if busy { ProgressView() } else { Text("Conectar") }
                }
                .disabled(token.isEmpty || busy)
            }
            .navigationTitle("tasktui")
        }
    }
}
