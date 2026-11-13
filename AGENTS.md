# Compass Go — Guía para agentes y desarrolladores

## ¿Qué es?

`Compass Go` es el **port a Go** de *Compass* (originalmente en Rust): una
**TUI para Windows** que automatiza tareas de desarrollo, compilación y
configuración para proyectos **Tauri (Rust)**, **Flutter (Dart)**,
**Kotlin Multiplatform (Compose)**, **Go + Wails v3**, **Go puro**,
**Python** y **Rust puro**. Detecta el tipo de proyecto y adapta comandos,
rutas y menús.

Además del menú, es un **contenedor de terminal propio**: cada card abre una
pestaña **ConPTY** (PowerShell) dentro de la misma ventana de Compass en lugar
de lanzar consolas externas. También registra en Windows el **menú contextual
Compass**, la acción **`.gitignore`** y la cascada **Vibe Coding**.

El producto se distribuye como `compass-go.exe` y con un instalador NSIS
autónomo (`dist/CompassGo-Setup.exe`).

## Requisitos

| Requisito | Para qué | Cómo |
|-----------|----------|------|
| **Go** (toolchain reciente; `go.mod` declara `go 1.26.0`) | compilar | https://go.dev/dl/ |
| **Windows** | toda la shell/registro/ConPTY es específica de Windows | — |
| **PowerShell** | menú contextual y varias tareas lo invocan | viene con Windows |
| **go-winres** *(opcional)* | embeber icono + versión + manifest en el `.exe` | `go install github.com/tc-hib/go-winres@latest` |
| **NSIS** *(opcional)* | empaquetar el instalador `CompassGo-Setup.exe` | https://nsis.sourceforge.io/ |

Si `go-winres` no está instalado, `install.bat` / `dist.bat` compilan igualmente
pero **sin** los recursos de Windows embebidos (el icono del `.exe` será el
genérico).

## Estructura del repositorio

```
Compass Go/
├─ cmd/compass/main.go        # entrypoint: flags, dispatch global/local
├─ internal/
│  ├─ tui/                    # TUI con tcell: model, render, submenu, glow (halo hover)
│  ├─ container/              # contenedor ConPTY: sesión, encode teclado/ratón, render vt10x→tcell
│  ├─ tasks/                  # ejecutores por plataforma + menús contextuales Windows + global tasks
│  │  └─ assets/              # plantillas PowerShell (env-backup, opencode)
│  ├─ theme/                  # 20 temas + persistencia
│  ├─ project/                # detección de tipo de proyecto
│  ├─ shell/                  # envoltorio PowerShell / cmd
│  ├─ lock/                   # instancia única por proyecto (LockFileEx, Windows)
│  ├─ window/                 # trae al frente la ventana de otra instancia abierta
│  ├─ console/                # Ctrl-C handler y bloqueo de cierre (Windows)
│  ├─ prompts/  ui/           # prompts y helpers de UI
├─ assets/icon/               # Icon.ico, Default.ico, opencode.ico
├─ dist/                      # GENERADO por dist.bat (payload, installer.nsi, setup); desechable
├─ bin/                       # GENERADO por install.bat (binario de trabajo); desechable
├─ install.bat                # compilar + instalar en el sistema
├─ dist.bat                   # compilar + empaquetar el instalador en dist/
├─ uninstall.bat              # desinstalar de la máquina
├─ go-winres.json             # metadatos del .exe (nombre "Compass Go")
├─ corekit/                  # core kits canónicos que viajan dentro de Compass (p.ej. whatsapp)
│  └─ whatsapp/               #   motor de WhatsApp: engine.js, bundle.mjs, node.exe, node_modules
└─ PLAN.md                    # plan de migración original
```

Módulo Go: `compass` (imports `compass/internal/...`).

## Scripts `.bat` (Windows) — uso

Hay **tres scripts**, separados por velocidad de uso: `install.bat` compila e
instala en la máquina (el ciclo corto de desarrollo), `dist.bat` compila y
empaqueta el instalador autónomo en `dist\` (solo para distribuir) y
`uninstall.bat` desinstala.

### 1. `install.bat` — compilar + instalar (tipo: *install*)

Compila a `bin\compass-go.exe` (integrado: `go-winres` + `go build`), lo instala
con los iconos en `%LOCALAPPDATA%\Programs\Keimer` (añadido al PATH de usuario),
elimina la copia antigua de `%USERPROFILE%\.cargo\bin`, registra el **menú
contextual Compass** y la acción **`.gitignore`**, y configura **Vibe Coding**
la primera vez. **No** genera nada en `dist\` ni ejecuta NSIS: es el camino
rápido para probar cambios en tu propia máquina.

```bat
install.bat              # compilar + instalar
install.bat --force      # rehace también menús y la cascada Vibe Coding
install.bat --vibe       # reconfigura Vibe Coding (detecta CLIs que falten)
```

- Cierra `compass-go.exe` en ejecución antes de sobrescribir (Windows no
  deja sobreescribir un `.exe` en uso).
- El menú contextual queda en `HKCU\...\shell\zz_CompassGo` y
  `zz_CompassGitignore`; Vibe Coding en `zz_VibeCoding`.
- Requiere ejecutarlo **desde la raíz del proyecto** (necesita `go.mod`).

### 2. `dist.bat` — generar el instalador en `dist\` (tipo: *package*)

Compila a `dist\payload\compass-go.exe` (con iconos), regenera
`dist\installer.nsi` y, si NSIS está instalado, produce el instalador autónomo
`dist\CompassGo-Setup.exe`. Todo lo generado vive en `dist\` y es desechable.

```bat
dist.bat                 # compila + genera dist\CompassGo-Setup.exe
dist.bat --no-installer  # solo prepara dist\payload (sin NSIS)
```

- Si falta NSIS, avisa y deja `dist\payload` e `installer.nsi` listos.
- Requiere ejecutarlo **desde la raíz del proyecto** (necesita `go.mod`).

### 3. `uninstall.bat` — desinstalar de la máquina (tipo: *uninstall*)

Quita los menús contextuales (Compass + `.gitignore` + cascada Vibe Coding)
vía el propio binario (`--task context-menu-uninstall` / `opencode-uninstall`)
y borra `compass-go.exe` e iconos de `%LOCALAPPDATA%\Programs\Keimer` (más
cualquier resto en `%USERPROFILE%\.cargo\bin`), quitando esa carpeta del PATH
del usuario.

```bat
uninstall.bat
```

- **No** desinstala las CLIs de Vibe Coding que hayas instalado (opencode,
  cline, …); solo limpia las entradas de registro de Compass.
- Refresca la caché de iconos del Explorador (`ie4uinit.exe -show`).

> Nota: `install.bat` / `dist.bat` / `uninstall.bat` son para **desarrollo /
> uso en tu propia máquina**. Para **distribuir** a terceros usa el instalador
> NSIS que genera `dist.bat` (`dist\CompassGo-Setup.exe`), que no requiere Go
> ni scripts sueltos.

## Uso del binario (CLI)

```bat
compass-go.exe                      # detecta el proyecto y abre el TUI
compass-go.exe -p "C:\ruta\proyecto"  # abre el TUI sobre una carpeta concreta
compass-go.exe --pick               # elige la carpeta con un navegador
compass-go.exe -t windows-dev       # ejecuta una tarea directamente (sin TUI)
compass-go.exe --task context-menu-install
compass-go.exe --trace              # registra eventos del TUI en <tmp>/compass-tui.log
```

Flags:

| Flag | Alias | Descripción |
|------|-------|-------------|
| `--task` | `-t` | Ejecuta una tarea global directamente (ver abajo) |
| `--path` | `-p` | Ruta del proyecto (autodetecta si se omite) |
| `--pick` | — | Navegador de carpeta para elegir proyecto |
| `--trace`| — | Log de eventos de la TUI |

### Tareas globales (`--task`)

Se ejecutan **antes** de la detección de proyecto y no dependen de un
proyecto concreto:

| Tarea | Qué hace |
|-------|----------|
| `context-menu` | Menú interactivo para instalar/desinstalar menús |
| `context-menu-install` | Registra `zz_CompassGo` + `zz_CompassGitignore` (también limpia la clave antigua `zz_Compass`) |
| `context-menu-uninstall` | Quita las claves de Compass (incluye las antiguas `zz_Compass` / `zz_VibeGitignore`) |
| `context-gitignore` | Genera/mezcla un `.gitignore` en la carpeta dada (`--path`) |
| `vibe-setup` | Configura la cascada Vibe Coding (pregunta por las CLIs que falten) |
| `opencode` / `opencode-configure` | Menú / configuración de OpenCode |
| `opencode-install` / `opencode-uninstall` | Alta/baja de la entrada Vibe Coding de OpenCode |
| `cline-install` / `cline-uninstall` | Ídem para Cline |
| `kilo-install` / `kilo-uninstall` | Ídem para Kilo Code |
| `antigravity-install` / `antigravity-uninstall` | Ídem para Antigravity |

## Core kits (motores canónicos)

Compass lleva dentro ciertos **core kits** —motores o runtimes compartidos— en
`corekit/`, para que viajen con el binario y no dependan de rutas externas. El
primer kit es el **motor de WhatsApp** (`corekit/whatsapp/`).

- `corekit/whatsapp/` contiene `engine.js`, `bundle.mjs`, `package.json`,
  `node_modules/`, `node.exe` y el bundle generado `whatsapp-engine-bundle/`.
  `bundle.mjs` deja el bundle en la raíz del kit (junto a `node.exe`).
- **Resolución de ruta** (`internal/tasks/whatsapp_engine.go`):
  1. env `COMPASS_WHATSAPP_ENGINE` (override explícito);
  2. `<dir del exe>/corekit/whatsapp` (instalación / binario compilado);
  3. `<cwd>/corekit/whatsapp` (desarrollo con `go run`).
- **Opt-in por proyecto**: un proyecto solo recibe el runtime si tiene el
  marcador `use-whatsapp-engine` en su raíz (o una carpeta `whatsapp_engine`
  local). Sin él, Compass no etaja nada.
- **Etapado en build**: `stageWhatsappEngine` (llamado desde `windowsArtifacts`
  en el release de escritorio) reconstruye el bundle si `engine.js` cambió y
  copia `node.exe` + `whatsapp-engine-bundle` al `bin/resources/` del proyecto
  (o al `resources/` de Tauri/Flutter/Kotlin según tipo).
- **Tarea `whatsapp`**: compila el motor canónico (`node bundle.mjs`) y lo etaja.
- `install.bat` / `dist.bat` copian `corekit/` junto al binario/instalador.

## Desarrollo

```bat
go build -o dist/payload/compass-go.exe ./cmd/compass   # compilar sin instalar
go vet ./...                  # análisis estático
go test ./...                 # tests unitarios (sin TUI real)
COMPASS_E2E=1 go test ./...   # tests e2e (requieren Windows + ejecutar el binario real)
```

- Tests de TUI/container usan `tcell` `SimulationScreen` y no necesitan
  terminal real.
- Los tests e2e (`internal/container/e2e_test.go`) están **tras la variable
  de entorno `COMPASS_E2E=1`** porque arrancan `compass-go.exe` de verdad.
- Recursos de Windows: `go-winres.json` + `assets/icon/*.ico` → `.syso` en
  `cmd/compass/rsrc` (generados al compilar con `install.bat` / `dist.bat`).

## Notas de arquitectura / diferencias con el Rust original

- **TUI**: `tcell/v2` en vez de `ratatui` + `crossterm`. El halo de hover
  animado usa un `cellBuf` intermedio porque tcell no permite leer celdas ya
  pintadas.
- **Terminal embebida**: `go-pty` (ConPTY) + `vt10x` (parser VT) en vez de
  `portable-pty`. **vt10x no tiene scrollback** (solo pantalla visible).
- **Instancia única por proyecto**: `internal/lock` con
  `x/sys/windows.LockFileEx` sobre un fichero por proyecto en
  `%LOCALAPPDATA%\compass\instances` (no se escribe nada en la carpeta del
  proyecto). Si ya hay un Compass abierto para esa carpeta, la segunda
  instancia no arranca: `internal/window` trae al frente su ventana de consola
  (conhost o Windows Terminal) y, si no la encuentra, avisa y sale. El conteo
  de instancias previas usa el nombre real del exe (`os.Args[0]`), así que
  renombrar el binario no rompe el conteo.
- **Regex**: RE2 (`regexp`) en vez de la crate `regex` de Rust — **sin
  lookahead/lookbehind**.
- **Color de pestaña** (`tab_accent`): determinista por ruta de proyecto
  (FNV-1a → 12 tonos → rotación áurea por índice de instancia entre Compass
  vivos vía `CreateToolhelp32Snapshot`).
- **Menú contextual**: claves `zz_CompassGo` / `zz_CompassGitignore` /
  `zz_VibeCoding` en `HKCU`, gestionadas por PowerShell desde Go.
- **Plataformas soportadas**: solo **Windows** como aplicación de escritorio;
  Android sigue como destino de compilación para los proyectos compatibles.
