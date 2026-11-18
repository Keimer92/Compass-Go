# Migración a GUI web (Wails v3 + Svelte) — estado y plan

> Este documento complementa a `PLAN_GUI_GIO.md` (versión Gio, ya retirada).
> Estado: **migración completa**. Menú, terminal ConPTY con pestañas, subcards
> de Release/Git/Entorno, 20 temas, opciones, commit asistido, explorador con
> `git status` y árbol, IPC multi-proyecto e instalador NSIS probado de punta a
> punta.

## 1. Lo que YA está hecho

### Estructura
- `cmd/compass-web/main.go` — entrypoint Wails v3. Arranca maximizado, sirve
  el frontend embebido y registra el servicio `webui.Bridge`.
- `internal/webui/bridge.go` — servicio enlazado: `GetWorkspace`,
  `SelectProject`, `GetMenu`, `RunTask`, `ExePath`, `OpenTerminal`,
  `WriteTerminal`, `ResizeTerminal`, settings.
- `internal/webui/menu.go` — mapeo `MenuItem` → tarea (idéntico al de la GUI
  anterior) y lista de cards.
- `internal/webui/terminal.go` — `TerminalService`: abre ConPTYs con
  `container.Spawn`, reenvía salida como eventos `pty:data` (base64) y
  `pty:exit`, expone `Resize` y `Write`.
- `internal/container/session.go` — añadidos métodos aditivos `ReadDirect`,
  `ExitCode` para el transporte web.
- `frontend/` — proyecto Svelte 5 + Vite + TypeScript:
  - `App.svelte` — layout completo (sidebar, topbar, cards, panel terminal,
    panel derecho con pestañas Explorador/Commit, modal de temas).
  - `components/Card.svelte` — cards con iconos SVG (Lucide) y hover.
  - `components/Explorer.svelte` — estados Git tipo VS Code (U/A/M/D).
  - `components/Commit.svelte` — formulario de commit con pills.
  - `lib/bridge.ts` — `Call.ByName` con FQN `compass/internal/webui.Bridge.*`
    y suscripciones a eventos `pty:data` / `pty:exit`.
  - `lib/icons.ts` — iconos vectoriales por clave de color.
- `dist.bat` — compila el frontend (`npm run build`) y luego
  `go build -o dist\payload\compass-go.exe ./cmd/compass-web`.
- Versión Gio eliminada: `internal/gui/` y `cmd/compass-gui/` borrados.
  `internal/tui/` se conserva solo como biblioteca de tipos (`MenuItem`).

### Verificado
- `npm run build` produce el bundle en `cmd/compass-web/frontend/dist`.
- `go build ./...`, `go vet ./...`, `go test ./...` en verde.
- La ventana Wails abre maximizada y se ve el layout Svelte completo.
- Los bindings funcionan: `GetWorkspace` devuelve los 3 proyectos guardados,
  las pestañas Explorador/Commit responden, xterm.js se inicializa.

## 2. Lo que FALTA (en orden)

### Fase A — Hacer que el menú aparezca (bloqueante) — HECHA
- [x] `NewBridge(dir)` deja `b.ctx` apuntando al proyecto activo: primero
      detecta la carpeta de lanzamiento y, si no es un proyecto, cae al
      proyecto activo persistido. `GetMenu` ya no devuelve `nil` (un `null` en
      JSON rompía el `menu.filter` del frontend).
- [x] `GetDebug()` expone `dir`, `ctxDir`, `type`, `projects` y `active`.
- [x] **Causa raíz del menú vacío**: el componente estaba en modo runes
      (`$state` en `lastError`) pero el resto de variables eran `let` legacy →
      Svelte no re-renderizaba al llegar `getMenu()`. Ahora `workspace`, `menu`,
      `activeTask`, `rightTab`, `themeMenu`, `optionsOpen` y `menuTab` usan
      `$state`.
- [x] **Card "Run" no ejecutaba nada**: `RunTask` usaba `item == 0` como
      centinela de "inválido", pero `tui.WindowsDev == 0`. `menuItem` ahora
      devuelve `(item, ok)` y `GetMenu` emite el índice como ID.

### Fase B — Conectar el terminal real — HECHA
- [x] `webui.SetApp(app)` se llama en `cmd/compass-web/main.go` (sin esto
      `Emit` era un no-op y no llegaba `pty:data`).
- [x] **Dos lectores sobre la misma ConPTY**: `Spawn` ya consumía la salida
      para vt10x, así que `ReadDirect` se quedaba sin bytes. Se añadió
      `container.SpawnTap(..., tap func([]byte))`: el tap se invoca desde el
      mismo lector y `ReadDirect` se eliminó.
- [x] `Session.WaitExit()` (canal `done`) sustituye al sondeo: el servicio web
      emite `pty:exit` con el código real.
- [x] `CloseTerminal(id)` en el bridge + botón × que cierra la sesión
      (`taskkill /T`).
- [x] Se importa `@xterm/xterm/css/xterm.css` (sin él el terminal salía
      invisible) y `term.write` recibe los bytes directamente (antes se
      decodificaba UTF-8 por trozos y rompía caracteres multibyte).
- [x] `fit.fit()` antes de abrir la PTY para que `rows`/`cols` sean los reales.


### Fase C — Paridad de funciones — HECHA
- [x] Pestañas de tareas: varias a la vez, activar/cerrar. Un `Terminal` de
      xterm por pestaña (`terms` map), hosts apilados y `display:none` para las
      inactivas; el bridge ya tenía N sesiones.
- [x] Subcards nativas de Release / Git / Entorno: `Bridge.Subcards(id)`
      devuelve las opciones (mismas que la GUI Gio) y el frontend las pinta en
      un modal de rejilla.
- [x] Selector de 20 temas: `GetThemes`/`GetTheme`/`SetTheme` serializan
      `internal/theme` a CSS (`--bg`, `--panel`, `--accent`, …) y al mapa de
      acentos por card; se persiste con `theme.Save`.
- [x] Modal de opciones conectado a `GetSettings`/`SetSettings`
      (`open_output_dir`, `commit_model`).
- [x] Commit asistido: `GenerateCommit` (opencode con prompt corto en inglés) y
      `Commit` (`git add -A` + `git commit`) portados de la GUI Gio.
- [x] Explorador real: `GitStatus` (`git status --porcelain`) y `FileTree`
      (dos niveles, plegable, omite `node_modules`/`dist`/…).
- [x] Sonido de fin de build: lo emite el proceso hijo (`internal/sound`).
- [x] IPC multi-proyecto: `workspace.Listen` se conecta al arrancar y
      `Bridge.AddProject` apila la carpeta y emite `workspace:changed`.

### Fase D — Instalación — HECHA
- [x] `dist.bat` compila el frontend y luego `./cmd/compass-web`; `go-winres`
      escribe los `.syso` en `cmd\compass-web\`.
- [x] **`-ldflags="-H=windowsgui"`** recuperado en `dist.bat`: sin él el `.exe`
      abría una consola. Verificado en el binario instalado (subsystem 2 = GUI).
- [x] `install.bat` borra `dist\` al terminar.
- [x] Ciclo completo probado: `install.bat` → instalación en
      `%LOCALAPPDATA%\Programs\Keimer`, PATH, menú contextual
      (`zz_CompassGo` → `compass-go.exe -p "%1"`), `COMPASS_WHATSAPP_ENGINE`
      y arranque del binario instalado sin consola.

### Fase E — Correcciones encontradas al probar la UI real
- [x] **La salida del terminal se perdía**: los eventos `pty:data` llegaban
      antes de que el JS conociera el `ptyId` (la promesa de `OpenTerminal`
      resuelve después de crear la ConPTY). Ahora **el id lo elige el
      frontend** (`tab-<n>`) y se fija antes de abrir la sesión; cubierto por
      `TestOpenTerminalUsesClientID`.
- [x] Los tests de `internal/webui` aíslan `LOCALAPPDATA`: antes apilaban
      proyectos de prueba en el `projects.json` real del usuario.
- [x] `.gitignore`: `frontend/node_modules/`, `frontend/bindings/`, `tmp/`.
- [x] Eliminados los e2e de `internal/container` que asumían la TUI retirada
      (`e2e_test.go`, `scroll_e2e_test.go`).

## 3. Notas técnicas para continuar

- El runtime de Wails v3 expone `Call.ByName('package.Struct.Method')`. El FQN
  correcto es `compass/internal/webui.Bridge.<Metodo>` (se descubrió tras
  probar `webui.Bridge.*`, que daba *unknown bound method name*).
- El plugin de Vite de Wails (`@wailsio/runtime/plugins/vite`) falló en el spike
  por falta de eventos tipados; se quitó y se usa `@wailsio/runtime` directo.
- No hace falta `wails3 generate bindings` para `Call.ByName`.
- Los eventos se emiten desde Go con `app.Event.Emit("pty:data", map[string]any{...})`
  y se leen en TS con `Events.On('pty:data', e => e.data.data)`.
- `app` en `internal/webui/bridge.go` se guarda con `SetApp`, y el entrypoint
  **ya llama a `webui.SetApp(app)`** (era el motivo de que no llegase la salida
  del terminal).
- Nunca abras un segundo lector sobre la ConPTY: `container.SpawnTap` reparte la
  salida entre el parser vt10x y el tap que emite `pty:data`.
- Verificación: `go test ./internal/webui/` cubre detección del proyecto,
  fallback al workspace, `git status`, árbol de archivos y subcards
  (`bridge_test.go`, `git_test.go`). El camino completo del terminal (binario
  real → ConPTY → tap) está en `terminal_e2e_test.go`, tras `COMPASS_E2E=1`.
- `cmd/compass-web/main.go` recuperó el despacho de la GUI anterior: tareas
  globales (`tasks.IsGlobalTask`), `context-gitignore`, `--pick`,
  `windialog.AttachParentConsole` (sin él el hijo `--task` no tiene consola y
  el terminal sale vacío) y el diálogo de error nativo.
- Pendiente menor: los e2e de `internal/container` (`TestCompassSubmenusE2E`,
  `TestTabSwitchE2E`, …) siguen asumiendo la TUI retirada; están tras
  `COMPASS_E2E=1` y hay que reescribirlos o borrarlos.
