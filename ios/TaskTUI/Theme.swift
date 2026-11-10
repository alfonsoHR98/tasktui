import SwiftUI

/// Colores disponibles para las listas (los mismos que ofrece Recordatorios).
enum ListTint: String, Codable, CaseIterable, Identifiable {
    case rojo, naranja, amarillo, verde, menta, cian, azul, indigo, morado, rosa, marron, gris

    var id: String { rawValue }

    var color: Color {
        switch self {
        case .rojo: .red
        case .naranja: .orange
        case .amarillo: .yellow
        case .verde: .green
        case .menta: .mint
        case .cian: .cyan
        case .azul: .blue
        case .indigo: .indigo
        case .morado: .purple
        case .rosa: .pink
        case .marron: .brown
        case .gris: .gray
        }
    }

    var name: String {
        switch self {
        case .rojo: "Rojo"
        case .naranja: "Naranja"
        case .amarillo: "Amarillo"
        case .verde: "Verde"
        case .menta: "Menta"
        case .cian: "Cian"
        case .azul: "Azul"
        case .indigo: "Índigo"
        case .morado: "Morado"
        case .rosa: "Rosa"
        case .marron: "Marrón"
        case .gris: "Gris"
        }
    }
}

/// Apariencia de una lista: color e ícono (SF Symbol). El servidor no guarda
/// colores, así que viven en el dispositivo.
struct ListStyle: Codable, Hashable {
    var tint: ListTint
    var icon: String

    var color: Color { tint.color }

    static let icons = [
        "list.bullet", "bookmark.fill", "mappin", "gift.fill", "birthday.cake.fill", "graduationcap.fill",
        "backpack.fill", "pencil.and.ruler", "doc.fill", "book.fill", "banknote.fill", "creditcard.fill",
        "dumbbell.fill", "figure.run", "fork.knife", "cart.fill", "house.fill", "building.2.fill",
        "briefcase.fill", "airplane", "car.fill", "heart.fill", "pills.fill", "laptopcomputer",
        "gamecontroller.fill", "music.note", "leaf.fill", "pawprint.fill", "star.fill", "sparkles",
    ]

    /// Nombre legible del ícono para VoiceOver.
    static func iconName(_ icon: String) -> String {
        let names: [String: String] = [
            "list.bullet": "Lista", "bookmark.fill": "Marcador", "mappin": "Ubicación", "gift.fill": "Regalo",
            "birthday.cake.fill": "Pastel", "graduationcap.fill": "Graduación", "backpack.fill": "Mochila",
            "pencil.and.ruler": "Lápiz y regla", "doc.fill": "Documento", "book.fill": "Libro",
            "banknote.fill": "Billete", "creditcard.fill": "Tarjeta", "dumbbell.fill": "Pesa",
            "figure.run": "Correr", "fork.knife": "Comida", "cart.fill": "Carrito", "house.fill": "Casa",
            "building.2.fill": "Edificio", "briefcase.fill": "Maletín", "airplane": "Avión", "car.fill": "Coche",
            "heart.fill": "Corazón", "pills.fill": "Medicinas", "laptopcomputer": "Computadora",
            "gamecontroller.fill": "Videojuegos", "music.note": "Música", "leaf.fill": "Hoja",
            "pawprint.fill": "Mascota", "star.fill": "Estrella", "sparkles": "Destellos",
        ]
        return names[icon] ?? icon
    }

    static let inbox = ListStyle(tint: .indigo, icon: "tray.fill")

    /// Color por defecto estable para un nombre (`hashValue` cambia en cada arranque).
    static func automatic(for name: String) -> ListStyle {
        let palette = ListTint.allCases.filter { $0 != .gris }
        var h: UInt32 = 5381
        for b in name.lowercased().utf8 { h = (h &* 33) &+ UInt32(b) }
        return ListStyle(tint: palette[Int(h % UInt32(palette.count))], icon: "list.bullet")
    }
}

/// Las listas inteligentes de la pantalla principal.
enum SmartList: String, CaseIterable, Hashable, Identifiable {
    case today, upcoming, inbox, all

    var id: String { rawValue }

    var title: String {
        switch self {
        case .today: "Hoy"
        case .upcoming: "Próximos"
        case .inbox: "Inbox"
        case .all: "Todas"
        }
    }

    var view: String { rawValue }

    var style: ListStyle {
        switch self {
        case .today: ListStyle(tint: .azul, icon: "calendar")
        case .upcoming: ListStyle(tint: .rojo, icon: "calendar.badge.clock")
        case .inbox: .inbox
        case .all: ListStyle(tint: .gris, icon: "tray.full.fill")
        }
    }
}

/// Destinos de navegación desde la pantalla principal.
enum Route: Hashable {
    case smart(SmartList)
    case project(String)
}

/// Círculo de color con un SF Symbol blanco, como el de las listas de Recordatorios.
struct ListIcon: View {
    let style: ListStyle
    var size: CGFloat = 30
    @ScaledMetric private var scale: CGFloat = 1

    var body: some View {
        Image(systemName: style.icon)
            .font(.system(size: size * scale * 0.48, weight: .semibold))
            .foregroundStyle(.white)
            .frame(width: size * scale, height: size * scale)
            .background(style.color.gradient, in: Circle())
            .accessibilityHidden(true)
    }
}

/// Color del círculo de una tarea según su prioridad (1 máxima … 4 sin prioridad).
enum Priority {
    static func color(_ p: Int) -> Color {
        switch p {
        case 1: .red
        case 2: .orange
        case 3: .blue
        default: Color(.tertiaryLabel)
        }
    }

    static func name(_ p: Int) -> String {
        switch p {
        case 1: "Alta"
        case 2: "Media"
        case 3: "Baja"
        default: "Ninguna"
        }
    }

    static func icon(_ p: Int) -> String {
        switch p {
        case 1: "exclamationmark.3"
        case 2: "exclamationmark.2"
        case 3: "exclamationmark"
        default: "minus"
        }
    }
}
