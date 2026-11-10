# App de iPhone (SwiftUI)

`ios/` contiene una app nativa que usa la misma API del servidor: vistas Hoy,
Próximos, Inbox y Proyectos, búsqueda, captura rápida, completar, editar con
descripción y notas, y avisos locales según fecha/hora (sin hora: 9:00).

```sh
brew install xcodegen
cd ios && xcodegen            # genera TaskTUI.xcodeproj
open TaskTUI.xcodeproj        # elige tu iPhone o un simulador y ejecuta
```

Al abrir pide la URL del servidor y el token (se guarda en el Llavero). Con un
Apple ID gratuito, la app instalada con Xcode caduca a los 7 días.


## Antes de compilar con tu cuenta

En `ios/project.yml` cambia `bundleIdPrefix`, `PRODUCT_BUNDLE_IDENTIFIER` (app y
widget) y el App Group `group.dev.madaradev.tasktui` (también en
`ios/Support/*.entitlements` y en `ios/TaskTUI/API.swift`) por los tuyos: los
identificadores de Apple son únicos y los de este repo ya están registrados por
su autor. Después, `xcodegen` y firma con tu equipo en Xcode.
