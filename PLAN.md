# Plan de migración: Compass (Rust TUI) → Go

> Proyecto origen: `D:\Mis Proyectos\Rust\Compass` (Rust, solo Windows, ~18.6k LOC en `src/`)
> Proyecto destino: `D:\Mis Proyectos\Rust\Compass Go` (Go, solo Windows)

## 1. Alcance y objetivo

Reimplementar Compass en Go conservando el **comportamiento observable**:

- Misma CLI: `compass --task <t> --path <p> --pick --trace`
- Misma TUI: grid de "cards" con hover-glow, pestañas (DEV·MNT / CFG), scroll, selector de temas, modales
- Terminal **incrustada** (pestañas ConPTY dentro de la ventana) — la pieza diferenciadora
- Detección de 5 tipos de proyecto, ~20 módulos de tareas, menú contextual de Windows, lock de instancia, temas persistidos
- Binario único `compass.exe` (Windows) con icono + info de versión + manifest

**Decisión de diseño:** paridad funcional, no copia línea a línea. Gran parte del código es envoltura sobre comandos externos (fácil de portar); concentrar el esfuerzo en el TUI y el ConPTY.

## 2. Mapa de dependencias (Rust → Go)

| Rust (crate) | Rol | Equivalente Go | Notas |
|---|---|---|---|
| `ratatui` + `crossterm` | TUI / render buffer | `github.com/gdamore/tcell/v2` | tcell da control por celda (equivale a `Frame`/`Buffer`). Alt: `bubbletea`+`lipgloss` (menos fiel al glow) |
| `portable-pty` | ConPTY | `golang.org/x/sys/windows` (API PseudoConsole) | **No hay drop-in**; se implementa a mano |
| `tui-term` + `vt100` | parser VT | parser propio / `github.com/walter3306/goterminal` | spike necesario (R1) |
| `anyhow` | errores | `errors` + `fmt` | |
| `clap` | CLI | `github.com/spf13/cobra` | recrear flags/defaults |
| `inquire` | prompts | `github.com/charmbracelet/huh` | selector de carpeta |
| `indicatif` | spinners/progress | `charmbracelet/bubbles` o propio | |
| `chrono` | fechas | `time` (std) | |
| `dirs` | rutas usuario | `os` + `x/sys/windows` (`LOCALAPPDATA`) | |
| `serde_json` | JSON | `encoding/json` | |
| `regex` | regex | `regexp` (RE2) | ⚠️ RE2 no soporta lookahead/backrefs (R4) |
| `embed-resource` + `build.rs` | icono/versión | `github.com/tc-hib/go-winres` | estándar en Go |

## 3. Estructura de módulos (Go)

```
compass/                      (module: compass)
├─ cmd/compass/main.go        CLI, detección, dispatch, lock
├─ internal/
│  ├─ project/                ProjectType + detect (context.rs)
│  ├─ shell/                  pwsh detect, run_ps, spawn window, console title/tab color, PATH registry, artifact (cmd.rs)
│  ├─ ui/                     banners, pause, countdown (ui.rs)
│  ├─ lock/                   single-instance por directorio (instance_lock.rs)
│  ├─ theme/                  14 temas + mix/hsl (theme.rs)
│  ├─ tui/                    menú, layout grid, cards, hover glow, tabs, picker, modal (menu.rs + submenu.rs)
│  ├─ container/              panes, ConPTY, parser VT, barra de pestañas, key/mouse SGR, kill tree (container.rs)
│  ├─ console/                console guard: SetConsoleCtrlHandler + SC_CLOSE (console_guard.rs)  // build tag windows
│  ├─ tasks/                  android, cleanup, contextmenu, environment, flutter, git, go,
│  │                          gowails, kotlin, opencode, python, release, signer,
│  │                          terminal, version, whatsapp, submenu
│  └─ prompts/                selector de carpeta (inquire)
├─ assets/                    icono, etc.
├─ go.mod · go-winres.json
└─ install.bat (equivalante)
```

## 4. Plan por fases

- **Fase 0 — Andamiaje.** `go.mod`, estructura, `go-winres` (icono + versión + manifest), script de build a `compass.exe`. ✅
- **Fase 1 — Núcleo headless** (sin TUI). `project`, `shell`, `ui`, `lock` → ya deja funcional `compass --task ...` sin menú. ✅
- **Fase 2 — Tareas (`tasks/*`).** Portar una a una; orden: go, flutter, python, kotlin, android, gowails, signer, git, cleanup, release, version, environment, terminal, contextmenu (registro), opencode, whatsapp. ✅
- **Fase 3 — TUI base (tcell).** setup/restore, loop de eventos, layout grid, cards, tabs, scrollbar, header, footer (sin animación). ✅
- **Fase 4 — Temas + hover glow.** 14 temas (tests de distinctness), modal Ctrl+T, glow por celda en tcell. ✅
- **Fase 5 — Submenús** (signer/git/release) reusando la grilla. `internal/tui/submenu.go` (genérico) ✅ · Git, Limpieza, Entorno, Firmas (11 cards) y Release Windows/Android (cards por arquitectura) conectados ✅.
- **Fase 6 — Contenedor ConPTY (CRÍTICO).** `internal/container` con `go-pty` + `vt10x`: sesión, parser, panes, barra de pestañas, render tcell, reenvío tecla/mouse SGR, kill tree, `ExitCloseTab`. ✅
- **Fase 7 — Glue Win32.** `internal/console` (SetConsoleCtrlHandler + SC_CLOSE), reclaim título. ✅ · `tab_accent` por snapshot de procesos pendiente (hoy se usa color de pestaña ANSI).
- **Fase 8 — Empaquetado.** `go-winres`, `install.bat`/`uninstall.bat`, tests e2e. Scripts ✅ · icono embebido y e2e pendientes.
- **Fase 9 — Paridad y pulido.** Pendiente.

## 5. Riesgos y mitigaciones

- **R1 (crítico) — ConPTY en Go.** No hay librería madura todo-en-uno → spike en Fase 6. *Fallback:* el código Rust ya tiene camino "ventana externa" (`embedded()==false` → `spawn_new_window_*`); se puede lanzar cada tarea en su propia ventana de PowerShell.
- **R2 — Glow por celda.** tcell da control por celda exacto; portar `perimeter_point`/`mix` es directo.
- **R3 — Win32 bindings.** `x/sys/windows` cubre casi todo; `PROCESSENTRY32W` se declara a mano.
- **R4 — `regex` → RE2.** Buscar lookaround/backrefs y reescribir.
- **R5 — CLI idéntica.** Recrear flags/defaults/aliases en cobra.
- **R6 — Tests de snapshot.** Recrear con `tcell` `SimulationScreen`.

## 6. Testing

- Portar tests unitarios existentes a Go (detección de proyecto, distinctness de temas, artifact discover/rename, `tab_accent` estable, center-hit de submenús, secuencias tecla/mouse).
- Tests de integración headless para tareas sobre proyectos fixture.
- `SimulationScreen` de tcell para layout/cards/modales.

## 7. Recomendación

Fases 1+2 cubren ~70% del código (tareas = pegamento) y entregan CLI headless pronto. La Fase 6 (ConPTY) es el cuello de botella: hacer un spike antes de comprometer su cronograma.
