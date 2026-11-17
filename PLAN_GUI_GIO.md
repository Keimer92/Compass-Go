# Plan: GUI nativa para Compass Go (Gio)

> Documento de diseño / hoja de ruta. Complementa a `AGENTS.md` y `PLAN.md`.
> Estado: **fase 0 ejecutada** — spike funcional, pendiente de validación visual.

## Estado de la fase 0 (spike)

- [x] `gioui.org v0.10.3` fijado en `go.mod`; `go mod tidy` aplicado.
- [x] `cmd/compass-gui/main.go` — entrada aislada, no sustituye a `cmd/compass`.
- [x] `internal/gui/app.go` — ventana Gio, sidebar, cabecera y bucle de eventos.
- [x] `internal/gui/terminal.go` — pinta `vt10x.View` con Gio: colores ANSI,
      emoji (color glyphs), cursor, foco de teclado y `Resize` en vivo.
- [x] ConPTY real reutilizado (`container.Spawn`) sin tocar el motor.
- [x] `go build ./...` y `go vet ./...` limpios (la TUI antigua sigue intacta).
- [x] Ejecutable generado: **11,9 MB**, proceso respondiendo, ~66 MB en uso.
- [ ] Validación visual del usuario (colores, emoji, teclado, redimensionar).

### Correcciones tras la primera prueba del usuario

El usuario reportó que no se podía escribir y que el texto salía truncado.
Causas y arreglos aplicados:

1. **Foco de teclado perdido**: en Gio v0.10.3 las ops se reinician cada frame y
   un `FocusCmd` sin `event.Op` en ese frame se descarta. Ahora se registran
   `event.Op` + `key.FocusCmd` **en cada frame**, y además se reenvían los
   `key.EditEvent` (que algunos IME usan en vez de `EventKey`).
2. **Texto truncado/desplazado**: el pintado agrupaba celdas en "runs" y las
   posiciones las marcaba el shaper, con lo que un glifo ancho desplazaba el
   resto. Ahora se pinta **celda a celda en su coordenada exacta de rejilla**,
   con caché de glifos por rune.
3. **Espacio y mayúsculas**: `NameSpace` no tiene longitud 1 (se manejaba mal) y
   Gio entrega las letras en mayúscula; se corrige según `ModShift`.
4. **Duplicación de caracteres**: Gio entrega cada tecla por `EventKey` **y** por
   `EditEvent`; enviar ambos duplicaba cada pulsación. Se ignora `EditEvent`.
5. **Pintado celda a celda**: se eliminó el agrupado en "runs" que desplazaba
   los glifos; ahora cada celda se pinta en su coordenada exacta de rejilla.

### Resultado verificado (5:29 PM)

- Escribir `dir` + Enter dentro de la ventana ejecuta el comando y muestra el
  listado completo del directorio (comprobado con captura de pantalla).
- El texto se ve completo y alineado; el "truncado" inicial era únicamente el
  glifo del emoji, que Gio no pinta todavía.
- `go build ./...` y `go vet ./...` limpios; binario ~11,4 MB.

### Causa raíz de la sidebar invisible (resuelta)

`Terminal.Layout` llamaba a `paint.Fill` **antes** de empujar `clip.Rect`. Sin
clip activo, `PaintOp` pintaba toda la ventana con el color del terminal y
tapaba sidebar y cabecera. Al invertir el orden (clip → fill) todo aparece.

## Fase 1 — ejecutada

- [x] `internal/gui/theme.go` — los 20 temas de `internal/theme` traducidos a
      `color.NRGBA` (sin tocar el paquete original).
- [x] `internal/gui/cards.go` — rejilla de cards con acento por `MenuItem`,
      badges, descripción y clics; filtrada por tipo de proyecto y pestana.
- [x] `internal/gui/app.go` — ventana con sidebar, cabecera con pestañas
      "Inicio/Config" clicables, area de contenido y gestor de teclado unificado.
- [x] Selector de temas con clic: panel con los 20 temas, aplicacion en vivo y
      persistencia via `theme.Save` (verificado: Dracula quedó guardado).
- [x] `cmd/compass-gui/main.go` — añadido `--task` para que las cards abran la
      tarea real en una ConPTY (flujo completo extremo a extremo).
- [x] Fondo del area de contenido pintado con el color del tema (antes quedaba
      blanco).
- [x] `go build ./...` / `go vet ./...` limpios.

### Verificado visualmente

- Home: cards con colores de acento, sidebar con proyecto/tipo, cabecera con
  tema activo.
- Clic en "Temas" abre el panel con los 20 temas; clic en "Dracula" cambia
  toda la paleta y escribe `Dracula` en `%LOCALAPPDATA%\compass\theme`.

### Pendiente / conocido

- El emoji 🚀 sigue sin pintarse: Gio no resuelve el color glyph ni por nombre
  (`Segoe UI Emoji`) ni cargando `seguiemj.ttf`. Los badges de las cards usan
  letras en su lugar. Investigar `Shaper.Bitmaps` / CBDT más adelante.
- Los atajos de teclado (Ctrl+T, flechas) son poco fiables bajo `SendKeys` en
  este entorno; la GUI es totalmente usable con raton. Mantener como extra.

## Fase 2 — ejecutada

- [x] `internal/container/scrollback.go` — expuesto `NewScrollback` (aditivo;
      sin tocar la logica del motor).
- [x] `internal/gui/pane.go` — `Pane` con ConPTY + `Scrollback` propios, barra
      de pestanas clicable, boton × y marca ✓ al terminar.
- [x] `internal/gui/terminal.go` — pinta pantalla viva **o historia** segun
      `Back.AtBottom()`, resaltado de seleccion y extraccion de texto.
- [x] Rueda del raton → `Scrollback.Scroll` (3 lineas por paso).
- [x] Arrastre con raton → seleccion de celdas y copia al portapapeles
      (`clipboard.WriteCmd`) al soltar.
- [x] `reapFinished()` cierra pestanas que piden cierre y marca el resto ✓.
- [x] Eventos de raton enrutados por el tag global de la aplicacion, con area
      de impacto limitada al terminal para no tapar cards/pestanas/temas.

### Verificado

- Clic en "Run" abre una pestana "Run", ejecuta la tarea y conserva la salida
  con ✓ cuando el comando falla (exit != 0), como en la TUI antigua.
- Arrastre sobre el texto copia al portapapeles (comprobado con `Get-Clipboard`).
- `go build ./...` / `go vet ./...` limpios.

### Pendiente de la fase 2 / diferido

- Scrollback con salida muy larga y seleccion multilinea probadas solo a nivel
  de codigo (la tarea de prueba imprime pocas lineas).
- Aviso sonoro de fin de build (`internal/sound`) y "abrir carpeta de salida"
  quedan para la fase de paridad (plan original, fase 4/5).

## Fase 3 — completada

Requisito añadido por el usuario: **el terminal se abre abajo, como un panel de
IDE**, sin sustituir a las cards; y **eliminar Rust y Tauri** del producto.

- [x] `internal/workspace/workspace.go` — lista de proyectos persistida en
      `%LOCALAPPDATA%\compass\projects.json`, alta/baja/cambio de activo.
- [x] `internal/workspace/ipc.go` — token de instancia unica en
      `gui-instance.json` + servidor TCP loopback: una segunda invocacion
      entrega la carpeta a la ventana abierta y sale.
- [x] `cmd/compass-gui/main.go` — `workspace.TryForward(dir)` antes de abrir
      ventana (el menu contextual apila proyectos).
- [x] GUI: sidebar con proyectos apilados y clicables, estado (cards/pestanas)
      por proyecto, cambio con `switchProject` (guardar/cargar separados).
- [x] GUI: panel de terminal **inferior** (~42 % del area de contenido) con
      barra de pestañas propia; las cards permanecen arriba. Boton
      "Terminal ▾/▴" en la cabecera para mostrar/ocultar.
- [x] **Eliminacion total de Rust + Tauri**: borrados `rust.go`,
      `android_tauri.go` y `windows.go` (este ultimo era 100 % Tauri); quitados
      los tipos `TypeTauri`/`TypeRust` del enum y todas sus ramas en
      `internal/tasks`, `internal/tui`, `internal/project`, tests y documentacion
      (`AGENTS.md`, `PLAN.md`). La GUI agora solo ofrece Flutter, Kotlin,
      Python, Go y Go+Wails.

### Verificado

- `go build ./...`, `go vet ./...` y `go test ./...` en verde.
- Panel inferior: al abrir "Run", las cards siguen visibles arriba y el
  terminal con su barra de pestañas aparece abajo.
- Abrir una segunda carpeta con otra instancia la **apila en la sidebar** sin
  abrir otra ventana (JSON con 2 proyectos, activo el nuevo).
- Cambiar de proyecto en la sidebar recupera sus pestañas; cambiar a uno sin
  pestañas oculta el panel.

## Fase 4 — paridad final (completada)

- [x] **Aviso sonoro**: ya lo emite el proceso hijo (`shell.PauseFor` /
      `shell.RunBuildAndRevealWithEnv` → `internal/sound`), así que funciona
      igual dentro de una ConPTY de la GUI. No ha hecho falta código nuevo.
- [x] **Abrir carpeta de salida**: lo gestiona la capa `tasks`/`shell` leyendo
      `internal/settings`. La GUI añade el modal de opciones (botón "Opciones"
      en la cabecera o Ctrl+O) con la casilla persistida en
      `%LOCALAPPDATA%\compass\settings.json`.
- [x] **Atajos**: `ctrlLetter` reconoce Ctrl+T/Ctrl+O aunque Gio entregue el
      nombre como carácter de control (problemas vistos con IME/SendKeys).
- [x] Sondas de diagnóstico (test de emoji, logs temporales) eliminadas.

### Verificado

- Modal de opciones visible, casillamutable y persistida (`open_output_dir:
  true` tras el clic).
- `go build ./...`, `go vet ./...` y `go test ./...` en verde.

### Limitación conocida (aceptada)

- Los glifos de emoji/banner de las tareas no siempre se pintan con fidelidad
  en Gio: Segoe UI Emoji es una fuente de bitmap (CBDT/COLR) y en algunos
  casos salen fragmentados. No afecta a la funcionalidad. Se puede revisar
  más adelante con otra fuente o un rasterizador dedicado.

## Ajustes finales de interfaz (tras las 4 fases)

Petición del usuario: **barra nativa de Windows** (se descartó la barra macOS
propia), **interior estilo macOS**, **cards modernas con hover** y **panel
derecho con árbol de archivos + cambios Git**.

- [x] Maximizado al arrancar (`app.Maximized.Option()`), sin `app.Size` (que
      fuerza modo Windowed).
- [x] Barra de título nativa de Windows restaurada (sin `frameless` ni
      semáforos dibujados).
- [x] Interior estilo macOS: tipografía Segoe UI, sidebars con tono traslúcido,
      separadores finos, cards con esquinas redondeadas.
- [x] Hover en cards: al pasar el ratón la card se aclara con su color de
      acento (y la seleccionada se intensifica).
- [x] `internal/gui/explorer.go`: panel derecho con `git status --porcelain`,
      códigos M/A/D/? coloreados, árbol de archivos plegable (hasta 3 niveles,
      omite `.git`, `node_modules`, `bin`, `dist`...), botón de refresco y
      actualización automática cada ~5 s.
- [x] El panel de terminal sigue abajo, sin solapar el explorador derecho.

## Mejoras de interfaz añadidas después

- [x] **Iconos reales por card**: `internal/gui/icons.go` mapea los 18
      `MenuItem` a iconos vectoriales Material Design (`widget.Icon`), en vez
      de la inicial del titulo. Cada card tiene ademas hover que aclara su
      fondo con el color de acento.
- [x] **Pestañas superiores separadas**: `pillButton` dibuja cada pestaña como
      una pastilla redondeada (Inicio, Config, Terminal, Opciones) con hover y
      estado activo.
- [x] **Refresco dinamico del explorador Git**: cada ~2 s y al recuperar el
      foco de la ventana (`ConfigEvent.Focused`), ademas del boton manual ↻.
      El refresco se pide por canal al bucle de la UI para evitar carreras.
- [x] **Panel derecho con pestañas**: "Explorador" y "Commit", con el mismo
      estilo de pastillas redondeadas que la cabecera central.
- [x] **Commit asistido por OpenCode** (`internal/gui/commit.go`), en la
      pestaña Commit del panel derecho:
      - Mensaje editable, boton "Generar con OpenCode" (usa `opencode run`
        con un resumen de `git diff`/`git status`) y "Commit"
        (`git add -A` + `git commit -m`).
      - Si opencode falla se muestra el error real y se rellena un mensaje de
        reserva (`chore: actualizar N archivos`) que el usuario puede editar.
      - Solo se intenta opencode (no otros agentes), segun pidió el usuario.

### Incidencia detectada con opencode

`opencode v2.2.6` esta instalado en `C:\Users\Admin\AppData\Roaming\npm`, pero
`opencode run` falla con **HTTP 401: invalid auth token** (proveedor de
`deepseek-v4.1-flash`). `opencode auth list` solo muestra "Ollama Cloud".
No es un fallo de Compass: hay que reautenticar con `opencode auth login`
(o cambiar el modelo con `opencode run -m <proveedor/modelo>`). Mientras tanto
el panel de commit sigue siendo usable con el mensaje de reserva.

## Últimos ajustes (pestañas derechas, modelo free y subcards nativas)

- [x] Panel derecho con pestañas **Explorador | Commit** (estilo pastillas como
      la cabecera central).
- [x] **Modelo gratuito para commits**: `opencode run -m opencode/hy4-flash-free`
      (verifyado en esta máquina). Se puede cambiar en
      `%LOCALAPPDATA%\compass\settings.json` → `commit_model`.
      - Los modelos `ollama-cloud/*` fallan por falta de créditos; el proveedor
        `opencode/*-free` funciona sin auth adicional.
- [x] **Subcards de release nativas**: al pulsar "Win Release" o los builds
      Android se abre un diálogo con las 4 opciones como cards clicables, en vez
      del submenú de tcell dibujado en la ConPTY. Se añadieron tareas
      `windows-release-{x64,x86,arm64,all}` y `android-release-{arm64,armv7,universal,all}`
      en `internal/tasks/release.go` (`ReleaseWindowsArch`/`ReleaseAndroidArch`).
- [x] Resize de la ConPTY propagado en cada frame y barra de pestañas del
      terminal reducida a 26 dp para evitar que el TUI hijo pierda filas
      superiores.

## Estado del proyecto

Compass Go dispone de GUI nativa (Gio) con barra Windows maximizada, sidebar
multi-proyecto, panel de terminal estilo IDE, pestañas por tarea, scrollback,
selección/copiar, 20 temas, opciones, avisos, explorador Git dinamico y commit
asistido por OpenCode. La TUI clásica de tcell sigue disponible como respaldo.

Notas técnicas del spike:
- Gio v0.10.3 usa la API nueva: `text.Shaper` stateful (`LayoutString` +
  `NextGlyph`), `gtx.Execute(key.FocusCmd{...})` para el foco, y `app.Main()`
  con el bucle de ventana en una goroutine.
- El repintado se fuerza a ~30 fps con `w.Invalidate()` porque la salida de la
  ConPTY llega en segundo plano (igual que hacía el `frame` de la TUI).
- `key.FocusOp` ya no existe en v0.10.3; se usa `key.FocusCmd`.

## 1. Objetivo

Sustituir la TUI actual de `tcell` por una **interfaz gráfica nativa** con aspecto de
herramienta moderna (estilo opencode desktop), manteniendo todo el motor de
Compass Go: tareas, ConPTY, temas, sonido, menú contextual y corekits.

## 2. Decisiones cerradas

| Decisión | Elección |
|----------|----------|
| Distribución | **Sidebar izquierda** (proyectos apilados) + **terminal/área de trabajo a la derecha** |
| Modelo de proyectos | **Varios proyectos en una sola ventana** (cambia el lock actual de "uno por proyecto") |
| Framework | **Gio** (`gioui.org`) — dibujo GPU nativo, sin WebView, sin cgo |
| TUI anterior | **Conservada como respaldo** bajo `COMPASS_LEGACY_TUI=1` durante la migración |
| Plataforma | Solo Windows (igual que hoy); Gio usa Direct3D 11 en Windows |

### Por qué Gio

- Se reproduce con el requisito "dibujar nativamente al sistema": ventana del SO,
  Direct3D 11, texto con contornos, sin motor de navegador ni HTML/CSS/JS.
- `go.mod` ya declara `go 1.26.0`, compatible sin cambios.
- Binario pequeño (~10 MB), arranque rápido, 60 fps, sin cgo.
- Usado en producción por Tailscale, gotraceui, godcr.
- Alternativa valorada y descartada: **GOUI** encaja aún más literalmente (Win32 +
  DirectWrite, trae sidebar/pestañas hechas) pero es v0.8.0 recién estrenado;
  **Fyne** es más fácil pero impone su propio look Material y compilador C.

## 3. Inventario de código

### Se conserva íntegro

- `internal/tasks/**` — todos los ejecutores por plataforma, tareas globales,
  menú contextual de Windows, WhatsApp, generador de `.gitignore`, instaladores.
- `internal/project`, `internal/theme` (20 temas), `internal/settings`,
  `internal/sound`, `internal/shell`, `internal/window`, `internal/console`,
  `internal/prompts`, `internal/corekit`.

### Se conserva con adaptación mínima

- `internal/container/session.go` — `Spawn`, `Write`, `Resize`, `Close`,
  `CloseRequested`, `PaneScript`. El ConPTY (`go-pty`) y `COMPASS_EMBEDDED=1`
  siguen exactamente igual. Solo cambia el destino de la salida.
- `internal/container/scrollback.go`, `selection.go`, `palette.go`, `encode.go` —
  lógica VT ya en Go; se reutiliza desde el nuevo widget.
- `internal/lock` — pasa de lock por proyecto a **lock global** (ver §5).

### Se sustituye

- `internal/tui/**` (`tui.go`, `model.go`, `render.go`, `glow.go`, `cellbuf.go`,
  `submenu.go`, `settings_modal.go`, tests) → `internal/gui/**`.
- `internal/container/render.go` y todo el pintado con `tcell` → `gui/terminal.go`.
- Dependencias que salen de `go.mod`: `github.com/gdamore/tcell/v2`,
  `github.com/gdamore/encoding`, `github.com/lucasb-eyer/go-colorful`.
- Se mantiene `github.com/rivo/uniseg` (clusters gráficos → ancho de celda).

### Nuevo

- `internal/gui/**` — capa visual en Gio.
- `internal/workspace/**` — gestión de N proyectos, persistencia e IPC.

## 4. Arquitectura objetivo

```
internal/gui/                    ← reemplaza a internal/tui
  app.go          ventana Gio, bucle FrameEvent, layout sidebar + main
  sidebar.go      proyectos apilados, selección, botón "añadir proyecto"
  menu.go         cards derivadas del actual MenuItem enum (model.go)
  terminal.go     pinta celdas de vt10x.View + Scrollback + Selection
  theme.go        los 20 temas → color.NRGBA
  modals.go       selector de temas, opciones, confirmación de salida

internal/workspace/              ← NUEVO
  workspace.go    N proyectos, pestañas por proyecto, estado activo
  store.go        persistencia en %LOCALAPPDATA%\compass\projects.json
  ipc.go          servidor IPC; recibe "añade esta carpeta"

internal/container/              ← motor, intacto salvo el pintado
  Session, Scrollback, Selection, Palette, encode  (se quedan)
  render.go / cellbuf.go                          (se eliminan)
```

### Flujo del terminal (sin puente Go↔JS)

```
container.Session (ConPTY, igual que hoy)
   ├─ goroutine de lectura ──▶ s.term.Write(buf) ──▶ gui/terminal.go pinta celdas
   ├─ gui/terminal.go (teclas) ──▶ s.Write(bytes)
   ├─ ResizeObserver / FrameEvent ──▶ s.Resize(cols, rows)
   └─ ExitCode ──▶ marca ✓ o cierra la pestaña
```

Todo son llamadas directas en el mismo proceso: desaparecen bindings, eventos y
flow-control entre lenguajes.

### Notas Gio concretas

- Bucle: `app.NewWindow`, `for e := range w.Events()`, `system.FrameEvent`,
  `gtx := layout.NewContext(&ops, e)`, `e.Frame(gtx.Ops)`.
- Layout: `layout.Flex` / `layout.Grid` para sidebar + main; `widget.List` para
  las listas (scrollback, proyectos) y `widget.Clickable` para cards y pestañas.
- Texto: `text.Shaper` + `paint.Text` agrupando celdas con el mismo estilo
  (color de frente/fondo, negrita, subrayado) para no pintar carácter a carácter.
- Fuentes: `gofont.Collection()` más una fuente emoji de Windows
  (`Segoe UI Emoji`) — la TUI actual dibujaba emoji vía `uniseg`; hay que
  preservar ese comportamiento.
- HiDPI: usar `unit.Dp` para todos los tamaños.

## 5. Modelo multi-proyecto e IPC

Hoy `cmd/compass/main.go` adquiere un lock por proyecto y, si ya hay una
instancia para esa carpeta, llama a `reuseOpenInstance`. Con N proyectos:

1. El lock de `internal/lock` pasa a ser **único y global** (una ventana Compass).
2. Al arrancar sin instancia previa: se crea la ventana con el proyecto inicial.
3. Si ya hay una instancia: el CLI se conecta por IPC y envía
   `{"op":"addProject","path":"..."}`, entonces sale.
4. El menú contextual (`compass -p "<carpeta>"`) sigue funcionando igual desde
   el punto de vista del usuario, pero ahora **añade la carpeta a la sidebar**.
5. Transporte IPC propuesto: named pipe (`go-winio`) o TCP loopback con token en
   `%LOCALAPPDATA%\compass\instance.json`. Elegir en la fase 3.
6. Cada proyecto mantiene sus pestañas de tareas y su acento de color
   determinista (`shell.TabAccent`).

## 6. Fases

| Fase | Duración | Entregable | Criterio de aceptación |
|------|----------|-----------|------------------------|
| **0. Spike** | 2–3 d | Demo Gio: ventana + sidebar + un ConPTY con `dir` y teclado | Arranca, pinta colores/emoji, redimensiona; medir `.exe` y arranque |
| **1. Núcleo GUI** | 1 sem | `internal/gui`: layout, bucle, 20 temas, cards del menú | Navegar cards y cambiar tema (aún sin terminal) |
| **2. Widget terminal** | 1–1.5 sem | Render de celdas, scrollback, selección/copiar, resize, pestañas, ✓ | Correr un `windows-release` completo dentro de la ventana |
| **3. Multi-proyecto + IPC** | 1 sem | Lock global, `projects.json`, `--add-path`, menú contextual | Dos clics del Explorador → dos entradas, una sola ventana |
| **4. Paridad y pulido** | 3–5 d | Beep, "abrir carpeta de salida", acentos, F12/Ctrl+T/Ctrl+O, modales | Checklist de paridad de `AGENTS.md` |
| **5. Empaquetado y transición** | 3–5 d | Build, NSIS, iconos, flag legacy | `dist.bat`/`install.bat` funcionan; `COMPASS_LEGACY_TUI=1` abre la TUI vieja |

**Total estimado: 4–5 semanas** para una persona.

## 7. Riesgos y mitigación

| Riesgo | Mitigación |
|--------|-----------|
| No existe widget de terminal en Gio — es el único trabajo realmente nuevo | `vt10x` + `Scrollback` + `Selection` ya están escritos; se adapta `render.go`, no se empieza de cero |
| Widgets de Gio escasos (tablas/árboles) | El menú son cards y pestañas, no tablas; `widget.List` cubre scrollback y sidebar |
| Emoji / fuentes | Añadir `Segoe UI Emoji` al `text.Shaper`; verificar en el spike |
| API de Gio pre-1.0 (v0.10.x) | Fijar versión exacta en `go.mod` y revisar changelog al actualizar |
| Cambio de modelo de instancia | Flag legacy y fase 3 aislada; el CLI headless (`--task`) no se toca |

## 8. Criterios de paridad (checklist final)

- [ ] Todas las cards de `MenuItem` abren su tarea correcta por tipo de proyecto.
- [ ] Pestañas de tarea: cambiar, cerrar, F12 (última), ✓ al terminar, cierre automático.
- [ ] Terminal: colores ANSI, scrollback, selección con ratón, copiar al portapapeles.
- [ ] 20 temas con preview en vivo y persistencia.
- [ ] `Ctrl+T` temas, `Ctrl+O` opciones, "abrir carpeta de salida", beep de build.
- [ ] Menú contextual Compass / `.gitignore` / Vibe Coding añade proyectos a la sidebar.
- [ ] Acento de pestaña determinista por ruta.
- [ ] WhatsApp engine, instaladores de proyectos y corekits intactos.
- [ ] `install.bat`, `dist.bat`, `uninstall.bat` y NSIS actualizados.
- [ ] `COMPASS_LEGACY_TUI=1` sigue dando la TUI actual.
