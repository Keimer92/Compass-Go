# Compass Go — Guía para agentes y desarrolladores

## ¿Qué es?

`Compass Go` es el **port a Go** de *Compass* (originalmente en Rust): una
**GUI de escritorio para Windows** (Wails v3 + Svelte 5) que automatiza tareas
de desarrollo, compilación y configuración para proyectos **Flutter (Dart)**,
**Kotlin Multiplatform (Compose)**, **Go + Wails v3**, **Go puro**,
**Python** y los **corekits** compartidos (**motor de WhatsApp** y
**Cloudflare D1**). Detecta el tipo de proyecto y adapta comandos, rutas y
menús.

Además del menú, es un **contenedor de terminal propio**: cada card abre una
pestaña **ConPTY** (PowerShell) dentro de la misma ventana de Compass en lugar
de lanzar consolas externas, y la salida se pinta con **xterm.js** en el
frontend. También registra en Windows el **menú contextual Compass**, la acción
**`.gitignore`** y la cascada **Vibe Coding**.

El producto se distribuye como `compass-go.exe` (construido desde
`cmd/compass-web`) y con un instalador NSIS autónomo
(`dist/CompassGo-Setup.exe`).

## Requisitos

| Requisito | Para qué | Cómo |
|-----------|----------|------|
| **Go** (toolchain reciente; `go.mod` declara `go 1.26.0`) | compilar | https://go.dev/dl/ |
| **Windows** | toda la shell/registro/ConPTY es específica de Windows | — |
| **PowerShell** | menú contextual y varias tareas lo invocan | viene con Windows |
| **go-winres** *(opcional)* | embeber icono + versión + manifest en el `.exe` | `go install github.com/tc-hib/go-winres@latest` |
| **NSIS** (necesario para `install.bat` / `dist.bat`) | empaquetar el instalador `CompassGo-Setup.exe` | https://nsis.sourceforge.io/ |

Si `go-winres` no está instalado, `install.bat` / `dist.bat` compilan igualmente
pero **sin** los recursos de Windows embebidos (el icono del `.exe` será el
genérico).

## Estructura del repositorio

```
Compass Go/
├─ cmd/compass-web/main.go    # entrypoint GUI (Wails v3): flags, dispatch --task, IPC
├─ cmd/compass-web/frontend/dist/  # bundle Svelte embebido con //go:embed
├─ frontend/                  # proyecto Svelte 5 + Vite + TS (fuente de la UI)
│  └─ src/
│     ├─ App.svelte           # layout: sidebar, cards, pestañas ConPTY, paneles
│     ├─ components/          # Card, Explorer (git status + árbol), Git (panel VSCode), History
│     └─ lib/bridge.ts        # Call.ByName + suscripción a eventos pty:*/workspace:changed
├─ internal/
│  ├─ webui/                  # puente Go↔JS: Bridge, terminal, menú, temas, git
│  ├─ menu/                   # modelo de las cards (MenuItem, Label, Icon, Section, SubItem)
│  ├─ container/              # contenedor ConPTY: sesión, scrollback, parser vt10x
│  ├─ tasks/                  # ejecutores por plataforma + menús contextuales Windows + global tasks
│  │  └─ assets/              # plantillas PowerShell (env-backup, opencode)
│  ├─ theme/                  # 20 temas + persistencia
│  ├─ settings/               # opciones globales persistidas (settings.json)
│  ├─ sound/                  # aviso sonoro de fin de build (Beep de kernel32)
│  ├─ project/                # detección de tipo de proyecto
│  ├─ shell/                  # envoltorio PowerShell / cmd
│  ├─ workspace/              # lista de proyectos abiertos + IPC multi-proyecto
│  ├─ lock/                   # instancia única por proyecto (LockFileEx, Windows)
│  ├─ window/                 # trae al frente la ventana de otra instancia abierta
│  ├─ console/                # Ctrl-C handler y bloqueo de cierre (Windows)
│  ├─ prompts/  ui/           # prompts y helpers de UI
├─ assets/icon/               # CompassGo.ico, VibeCoding.ico, Git.ico (mismos nombres, contenido reemplazable)
├─ packaging/                 # plantilla del instalador NSIS + generador de cabecera
│  ├─ installer.nsi           #   asistente MUI2 (icono, componentes, ejecutar al final)
│  ├─ make_icons.ps1          #   convierte un .png en .ico multi-tamano (16..256)
│  └─ make_header.ps1         #   cabecera 150x57 BMP a partir de assets/icon/CompassGo.ico
├─ dist/                      # GENERADO por dist.bat (payload/ + CompassGo-Setup.exe); desechable
├─ dev.bat                    # Vite en 2º plano (HMR de UI) + dev-run.ps1 (Go)
├─ dev-run.ps1                # compila/arranca el backend y reinicia al guardar Go (R/Q)
├─ frontend/dev-vite.bat      # arranca Vite con salida a %LOCALAPPDATA%\compass\vite-dev.log
├─ dev-reset.ps1              # cierra la sesión de desarrollo anterior al relanzar dev.bat
├─ install.bat                # dist.bat + instalación silenciosa con el Setup
├─ dist.bat                   # compilar (frontend + Go) y empaquetar el instalador en dist/
├─ uninstall.bat              # desinstalar de la máquina (usa Uninstall.exe si existe)
├─ go-winres.json             # metadatos del .exe (nombre "Compass Go")
├─ Kit Whatsapp/              # motor de WhatsApp canónico (engine.js, bundle.mjs)
│  ├─ whatsapp-engine-bundle/ #   bundle de ncc para Go/Wails
│  └─ single/                 #   bundle autocontenido para Flutter
├─ Kit-D1/                    # stack de sincronización Cloudflare D1 (subtree)
│  ├─ go/syncengine/          #   módulo Go reutilizable
│  ├─ protocol/fixtures/      #   fixture canónico del contrato wire
│  └─ worker/instances/       #   instancias del Worker por proyecto
├─ PLAN_SVELTE.md             # estado y plan de la migración a la GUI web
└─ PLAN.md                    # plan de migración original
```

Módulo Go: `compass` (imports `compass/internal/...`).

## Scripts `.bat` (Windows) — uso

Hay **cuatro scripts**, separados por velocidad de uso: `dev.bat` compila y
abre la app en modo desarrollo con HMR (Vite); `install.bat` compila y
genera el instalador, y lo ejecuta en silencio para instalar todo en la
máquina (el ciclo corto de desarrollo); `dist.bat` compila y empaqueta el
instalador autónomo en `dist\` (solo para distribuir); `uninstall.bat`
desinstala.

### 1. `install.bat` — compilar + instalar (tipo: *install*)

Llama a `dist.bat --fast` (compila con `go-winres` + `go build` y genera
`dist\CompassGo-Setup.exe` con compresión zlib) y después ejecuta ese Setup con
`/S` para que el instalador NSIS haga la instalación real: binario, iconos,
corekits, PATH del usuario, menú contextual, accesos directos y (best-effort)
anclaje a la barra de tareas. Al terminar, `install.bat` define
`COMPASS_WHATSAPP_ENGINE` y configura **Vibe Coding** la primera vez.

```bat
install.bat              # compilar + instalar (sin node.exe; usa Node 20+ del sistema)
install.bat --force      # rehace también la cascada Vibe Coding
install.bat --vibe       # reconfigura Vibe Coding (detecta CLIs que falten)
install.bat --with-node  # vuelve a empaquetar node.exe (sidecar)
```

- **No cierra `compass-go.exe`**: si hay una instancia en ejecución, `install.bat`
  aborta con un aviso para que la cierres tú mismo. Así no se matan las pestañas
  ni las CLIs abiertas dentro de Compass. La comprobación se hace dos veces (al
  empezar y justo antes de instalar, porque `dist.bat` tarda minutos y puedes
  abrir Compass mientras compila).
- **Purga los kits antes de instalar**: `packaging\installer.nsi` hace
  `RMDir /r` de `$INSTDIR\Kit-D1` y `$INSTDIR\Kit Whatsapp` justo antes de
  `File /r "payload\*"`. NSIS sobrescribe pero no borra, así que sin esta purga
  los archivos retirados del repo quedarían para siempre en la instalación.
- Al terminar hace `pause` para que el resumen quede a la vista.
- Borra el `dist\CompassGo-Setup.exe` anterior antes de compilar: si el
  empaquetado falla, no se instala un Setup viejo.
- **Requiere NSIS** (si falta, `install.bat` aborta y sugiere
  `winget install NSIS.NSIS`).
- En modo silencioso no se ejecuta la app al terminar y el anclaje a la barra
  de tareas suele quedar bloqueado por Windows; ancla a mano desde el menú
  Inicio.
- **Caché de iconos**: al final ejecuta `packaging\refresh_icons.ps1` con
  `-IconPath %BINDIR%\CompassGo.ico` y `-StampPath
  %LOCALAPPDATA%\compass\icon-stamp.txt`. Si el SHA-256 del `.ico` cambió
  respecto a la última instalación, reconstruye la caché (reinicia Explorer y
  borra `iconcache_*.db`/`thumbcache_*.db`); si no, solo hace un refresco suave
  (`SHChangeNotify` + `ie4uinit -show`). Evita que el acceso del escritorio,
  del menú Inicio o el anclado a la barra de tareas siga pintando el icono
  viejo después de cambiar la imagen.
- El menú contextual queda en `HKCU\...\shell\zz_CompassGo` y
  `zz_VibeGitignore` (entrada "Add gitignore"); Vibe Coding en
  `zz_VibeCoding`.
- Requiere ejecutarlo **desde la raíz del proyecto** (necesita `go.mod`).

### 2. `dist.bat` — generar el instalador en `dist\` (tipo: *package*)

Compila a `dist\payload\compass-go.exe` (con iconos), copia la plantilla
`packaging\installer.nsi` a `dist\installer.nsi` y genera la cabecera del
asistente en `dist\installer_header.bmp`; con NSIS instalado produce el
instalador autónomo `dist\CompassGo-Setup.exe`. `dist\` es solo un área de
trabajo temporal: al terminar `makensis` se borran el `.nsi`, el `.bmp` y
`payload\`, de modo que `dist\` queda **únicamente** con
`CompassGo-Setup.exe` (el instalador lo lleva todo dentro). Con
`--no-installer` sí se conserva `dist\payload` (es el único artefacto).

Al copiar `Kit Whatsapp/` al payload se excluyen los artefactos de build del motor
(`node_modules`, `out`, `examples`) y, **por defecto, también `node.exe`**: el
runtime usa el **Node 20+ instalado en el sistema** (PATH) y el bundle ya viaja
compilado. El payload baja a ~13 MB / ~35 archivos y la instalación silenciosa
ronda los 5 s. El instalador además borra esos artefactos y cualquier `node.exe`
de instalaciones anteriores antes de extraer.

Con `--with-node` se vuelve a incluir el sidecar `node.exe` (~70 MB, payload
~83 MB), copiado del Node 20+ que haya en el PATH (el repo ya no guarda uno).
Es la opción para distribuir a terceros que no tengan Node instalado; si no hay
Node en el PATH, `dist.bat` avisa y deja el payload sin sidecar.
En ambos modos los proyectos consumidores resuelven Node en cascada —sidecar si
existe → `node` del PATH— y exigen **Node >= 20** (lo impone Baileys).

El asistente usa Modern UI 2 con la marca de Compass Go (icono y cabecera,
como el instalador de Firefox) y ofrece, además de la instalación base, acceso
directo en el menú Inicio, acceso en el escritorio, anclaje a la barra de
tareas (best-effort: Windows 10 1903+ puede bloquearlo y se avisa al usuario)
y la casilla **Ejecutar Compass Go** al terminar.

```bat
dist.bat                 # compila + genera dist\CompassGo-Setup.exe (LZMA, sin node.exe)
dist.bat --fast          # compresión zlib: instalador mayor pero mucho más rápido
dist.bat --no-installer  # solo prepara dist\payload (sin NSIS)
dist.bat --with-node     # incluye node.exe (sidecar) en el payload
```

- `--fast` (zlib) tarda ~40 s y produce ~57 MB; sin él, LZMA sólido tarda
  ~3 min y produce ~38 MB. `install.bat` usa siempre `--fast`; para distribuir,
  compila sin el flag.
- El anclaje a la barra de tareas se intenta con
  `Shell.Application.InvokeVerb` desde PowerShell, no con `ExecShell`: si
  Windows no expone el verbo, `ExecShell` cae en el verbo `open` y abriría la
  app durante la instalación.

- Si falta NSIS, avisa y deja `dist\payload` listo (no es un error para
  `dist.bat`; `install.bat` sí aborta en ese caso).
- Si `makensis` falla, `dist.bat` termina con código 1.
- Para recompilar el instalador a mano hay que regenerar los temporales:
  `powershell -File packaging\make_header.ps1 -Icon assets\icon\CompassGo.ico -Out dist\installer_header.bmp`,
  `copy packaging\installer.nsi dist\` y luego
  `makensis /DAPPICON=payload\CompassGo.ico dist\installer.nsi` desde `dist\`.
- Requiere ejecutarlo **desde la raíz del proyecto** (necesita `go.mod`).

### 3. `uninstall.bat` — desinstalar de la máquina (tipo: *uninstall*)

Quita los menús contextuales (Compass + `.gitignore` + cascada Vibe Coding)
vía el propio binario (`--task context-menu-uninstall` / `opencode-uninstall`);
si existe `%LOCALAPPDATA%\Programs\Keimer\Uninstall.exe` ejecuta además el
desinstalador NSIS (`/S`), que borra accesos directos, PATH, entradas de
registro y la carpeta de instalación. Después limpia a mano lo que quede
(binario, iconos, corekits, restos en `%USERPROFILE%\.cargo\bin`) y elimina
`COMPASS_WHATSAPP_ENGINE`.

```bat
uninstall.bat
```

- **No** desinstala las CLIs de Vibe Coding que hayas instalado (opencode,
  cline, …); solo limpia las entradas de registro de Compass.
- Refresca la caché de iconos del Explorador con
  `packaging\refresh_icons.ps1` (borra el stamp del icono para que la próxima
  instalación reconstruya la caché completa).

> Nota: `install.bat` / `dist.bat` / `uninstall.bat` son para **desarrollo /
> uso en tu propia máquina**. Para **distribuir** a terceros usa el instalador
> NSIS que genera `dist.bat` (`dist\CompassGo-Setup.exe`), que no requiere Go
> ni scripts sueltos.

## Uso del binario (CLI)

```bat
compass-go.exe                      # detecta el proyecto y abre la ventana
compass-go.exe -p "C:\ruta\proyecto"  # abre la ventana sobre una carpeta concreta
compass-go.exe -t windows-dev       # ejecuta una tarea directamente (sin ventana)
compass-go.exe --task context-menu-install
```

> `--pick` y `--trace` pertenecían a la TUI retirada; hoy no están soportados
> por `cmd/compass-web`.

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
| `context-menu-install` | Registra `zz_CompassGo` + `zz_VibeGitignore` (también limpia la clave antigua `zz_Compass` y la anterior `zz_CompassGitignore`) |
| `context-menu-uninstall` | Quita las claves de Compass (incluye las antiguas `zz_Compass` / `zz_CompassGitignore`) |
| `context-gitignore` | Genera/mezcla un `.gitignore` en la carpeta dada (`--path`) |
| `vibe-setup` | Configura la cascada Vibe Coding (pregunta por las CLIs que falten) |
| `opencode` / `opencode-configure` | Menú / configuración de OpenCode |
| `opencode-install` / `opencode-uninstall` | Alta/baja de la entrada Vibe Coding de OpenCode |
| `cline-install` / `cline-uninstall` | Ídem para Cline |
| `kilo-install` / `kilo-uninstall` | Ídem para Kilo Code |
| `antigravity-install` / `antigravity-uninstall` | Ídem para Antigravity |

### Opciones globales y aviso sonoro

- **Aviso sonoro de fin de build (siempre activo)**: al terminar cualquier
  compilación release (`windows-release`, `android-release`, `android-aab`,
  `windows-build`, …) suena una triada ascendente; si el build falla suenan dos
  tonos graves. Se emite desde `shell.PauseFor` (éxito) y
  `shell.RunBuildAndRevealWithEnv` / `runReleaseMenu` (fallo), usando `Beep` de
  `kernel32` (`internal/sound`), sin WAV ni PowerShell. No se puede desactivar.
- **Cerrar la pestaña al terminar un build (opcional, desactivado por
  defecto)**: en el modal **Opciones** (`Ctrl+O`) hay una casilla que activa la
  cuenta atrás de 10 s y el cierre automático de la pestaña del terminal. Por
  defecto está **desactivada**: `shell.PauseFor` solo imprime "Build completado.
  La pestaña permanece abierta." y no emite `ExitCloseTab`, así que la salida
  queda visible para leerla.
- **Abrir carpeta de salida (opcional)**: en el menú principal, `Ctrl+O` abre
  el modal **Opciones** con la casilla "Abrir la carpeta de salida al terminar
  un build". Se guarda al instante en
  `%LOCALAPPDATA%\compass\settings.json` (`internal/settings`) y afecta a
  `shell.ShowArtifact`, `revealArtifact`, `finishRelease` y al build de
  escritorio de Wails. Desactivada, el artefacto se lista pero el Explorer no
  se abre.

## Subcards con modal GUI y ruta de backups

- **Limpieza selectiva en modal (sin terminal)**: la subcard "Elegir qué borrar"
  de la card Clean (`clean-do-choose`) **no** abre una pestaña ConPTY: abre un
  modal en `frontend/src/App.svelte`. El puente expone
  `Bridge.ListCleanupCandidates()` y `Bridge.PurgeCleanupCandidates(paths)`,
  respaldados por `ListCleanupTargets` / `PurgeCleanupTargets`
  (`internal/tasks/cleanup.go`). La GUI lista los directorios existentes con
  tamaño y marca `deps`, marca los que quiere borrar y confirma; el resultado
  (espacio liberado/errores) se muestra dentro del modal y la lista se refresca.
  **Seguridad**: `ResolveCleanupTargets` valida cada path recibido contra la
  lista dinámica `selectableTargets(ctx)` del proyecto activo (normaliza con
  `filepath.Clean` y compara sin distinción de mayúsculas, propio de Windows);
  un path ajeno/relativo/con `..` rechaza toda la operación (todo-o-nada) y
  nunca se borra un string que venga del frontend. El borrado real comparte
  `purgeWithError` con el CLI. La versión por consola (`chooseAndPurge`) sigue
  disponible vía `--task clean-do-choose`.
- **Destino de backups en Documents**: `defaultBackupRoot`
  (`internal/tasks/environment.go`) apunta por defecto a
  `%USERPROFILE%\Documents\Compass Go\Entorno de Desarrollo` (helper
  `documentsDir()` en `internal/tasks/helpers.go`, con fallback a `$HOME` y, si
  faltan ambos, al antiguo `%LOCALAPPDATA%\compass\env-backup\Backup`). Afecta
  de forma coherente a Backup, Restaurar/Validar y Abrir respaldos. Los scripts
  `.ps1` siguen materializándose en `%LOCALAPPDATA%\compass\env-backup` (área
  de trabajo, no destino). No se migran backups antiguos automáticamente.

### Toggle "Incluir portable" del release de Windows

- **Apagado por defecto**: el modal **Win Release** muestra un interruptor
  encima de las cards cuando el submenú trae release de escritorio
  (`windows-release-*` o `flutter-portable`). Cada vez que se abre se reinicia a
  apagado; encendido, el release deja además el `.exe` portable.
- **Viaja como `--portable`**: la tarea corre en otro proceso
  (`compass-web.exe --task …`), así que `App.svelte` guarda el estado con
  `Bridge.SetReleasePortable(bool)` y `Bridge.TaskScript` añade `'--portable'`
  solo a las tareas `windows-release*`. `main.go` lo lee con
  `flag.Bool("portable", …)` y llama a `tasks.SetIncludePortable`.
- **Qué hace cada tipo**: en Go y Go+Wails, `dropPortable`
  (`internal/tasks/release.go`) quita el `.exe` de la lista de artefactos y lo
  borra de `bin/`, dejando el instalador NSIS; si no hay instalador (NSIS falló
  o la variante solo produce el exe) se conserva el exe para no terminar sin
  nada. En Go+Wails **x86** solo existe el portable (`wails3 task build
  ARCH=386`), así que con el toggle apagado esa arquitectura se omite con un
  aviso (afecta a "Todos"). En Flutter el portable es una card aparte
  (`flutter-portable`) y se oculta del modal mientras el toggle está apagado.
- **Consola**: `compass-web.exe --task windows-release-x64` sin `--portable`
  tampoco genera el portable; el flag es la única forma de activarlo.

## Panel Git lateral (tipo VSCode)

- **Git vive en el panel derecho, no en una card**: la card Git y sus subcards
  (`git-do-*`) se retiraron; `menu.Git` queda en `hiddenMenuItems` para no mover
  los índices de las demás cards. La pestaña "Git" reemplaza a la antigua
  pestaña "Commit": el generador de commit con IA queda siempre desplegado y,
  si la carpeta aún no es un repo, ofrece **Inicializar repositorio** (rama
  `main` + primer commit). Toda la configuración (commit automático, modelo de
  OpenRouter y su API key) vive en el modal **Ajustes de
  Git**, que se abre desde un botón del pie del sidebar justo debajo de
  "Opciones" (`GitSettings.svelte`). Las pestañas del panel derecho son ahora
  **Explorador / Git / Historial**. No hay barra superior (topbar): el título lo
  aporta el sidebar y los botones "Tema" y "Opciones" viven en un pie
  (`sidebar-foot`) al final del sidebar, de modo que cards y terminal ganan esos
  52 px de alto. El modal "Opciones" conserva "Herramientas" (cards de Config:
  Terminal, Entorno) y "General".
- **Panel de Kit D1**: el botón "Kit D1" del pie del sidebar abre el panel de
  administración del kit. Está documentado completo en la sección
  [Core Kits → Kit D1](#kit-d1), junto al motor de WhatsApp.
- **Fecha y hora en el Historial**: `GitLog` y `CommitDetail` usan
  `--date=format:%Y-%m-%d %H:%M`, así que cada commit muestra el día y la hora
  exacta (sin segundos ni zona horaria, que solo alargaban la fila). El formato
  está fijado en `internal/webui/git.go` y cubierto por `git_test.go`.
- **Cuenta de GitHub en la UI** (`internal/webui/github_auth.go`): el modal
  Ajustes de Git lista las cuentas de Git Credential Manager
  (`GitHubAccounts`), permite añadir una (`GitHubLogin`, abre el navegador,
  plazo 180 s) y borrarla (`GitHubLogout`, con confirmación de dos clics). Son
  las mismas acciones que `git credential-manager github login/logout`, pero
  sin consola; Compass no guarda tokens. Los nombres se validan contra
  `[A-Za-z0-9][A-Za-z0-9-]{0,38}` antes de pasarlos como argumento.
- **Mascota** (`frontend/src/lib/Mascot.svelte`): brújula 3D con brazos en SVG
  puro (sin assets, hereda `currentColor` del tema). El bisel metalico se logra
  con una base `currentColor` y capas de brillo/sombra; la esfera es oscura y
  la aguja roja. Vive en el badge de la marca (40 px en un tile de 46 px).
  Estados `idle` / `working` / `success` / `error`: la aguja se balancea o gira,
  los brazos saludan, los ojos parpadean y miran alrededor, y hay saltos o
  tambaleo con gota de sudor. `App.svelte` destella `success` o `error` 2.6 s al
  recibir `pty:exit` con `code` 0 o >0 (un código negativo es un cierre forzado
  y no cuenta). Respeta `prefers-reduced-motion`.
  - **Rendimiento**: el flotado/bote se anima en la envoltura HTML `.float`
    (transform compuesto, `will-change`) y no en grupos SVG; dentro del SVG solo
    se mueven piezas pequeñas (aguja, brazos, ojos). Así la mascota no repinta
    el sidebar a 60 fps ni ralentiza el cambio de proyecto.
  - **Cambio de proyecto**: `pickProject` resalta el proyecto de forma optimista
    antes de esperar al backend y solapa `getMenu` con la activación de la
    terminal; `SelectProject`/`AddProject` arman el vigilante fsnotify fuera del
    lock (`go b.ensureWatch(dir)`), porque recorrer el árbol bloqueaba la
    respuesta en proyectos grandes.
  - **Pestaña recordada por proyecto**: `lastTabByProject` guarda la última
    pestaña activa de cada proyecto (se actualiza en `activate`) y
    `tabForProject` la devuelve al volver; si se cerró, cae en la primera del
    proyecto. Evita que cambiar de proyecto siempre reabra la primera consola.
- **API del puente** (`internal/webui/git.go`): `Bridge.GitInfo()` devuelve un
  DTO con `isRepo`, `hasCommits`, `branch`, `upstream`, `remoteUrl`, `webUrl`,
  `isGitHub`, `ahead`, `behind`, `changes`. Nunca falla: lo que no se puede
  leer queda vacío. `Bridge.GitPush()` hace `git push` con upstream o
  `git push -u origin HEAD` la primera vez; `Bridge.GitPull()` hace
  `git pull --rebase`; `Bridge.GitSync()` encadena pull --rebase + push en una
  sola llamada y reutiliza **una** detección de `GitInfo` (siete procesos git),
  que era el coste evitable de sincronizar; el botón muestra un spinner y la
  rama/upstream mientras corre (`action` en `Git.svelte`).
  `Bridge.GitSetRemote(url)` agrega/actualiza `origin`; `Bridge.GitInitRepo()`
  inicializa, fija rama `main` y hace el primer commit; `Bridge.OpenExternal(url)`
  abre el enlace del repo en el navegador del sistema. `GitPush`/`GitPull`/`GitSync` usan
  `runGitTimeout` (90 s) para no congelar la promesa Wails si git espera
  credenciales. `@{u}` es revisión fija de git, no texto del usuario.
- **403 al hacer push** (`internal/gitmsg`): cuando `git push` falla por
  permisos, `gitmsg.ParseDenied` extrae cuenta y repo del
  `Permission to <repo>.git denied to <cuenta>` de GitHub (y `IsForbidden` el
  403 genérico). El panel Git muestra el aviso y remite a **Ajustes de Git →
  Cuenta de GitHub**; la tarea CLI (`pushCurrentBranch`) sigue imprimiendo los
  comandos de `git credential-manager`. La cuenta la elige el credential helper
  del sistema, no Compass: el backend solo ejecuta `git push`.
- **Remoto caído y asistente de re-vinculación** (`internal/webui/git_remote.go`):
  `Bridge.GitRemoteStatus(url)` comprueba el remoto con `git ls-remote
  --exit-code` (25 s) y devuelve `state` = `ok` / `empty` / `missing` / `auth` /
  `unreachable` / `none` / `error`. Con URL vacía comprueba `origin`; con URL
  comprueba el candidato del asistente. El frontend solo lo consulta una vez
  por proyecto+URL cada 5 minutos (el panel se refresca con cada guardado). Si
  el estado es `missing`, el panel avisa y bloquea Push, y ofrece el asistente
  (`Git.svelte`, modal `wiz`): crear un repo nuevo con `Bridge.GitCreateRemote(name,
  private)` (usa `gh repo create` y luego enlaza `origin`; valida el nombre con
  `^[A-Za-z0-9._-]{1,100}$`) o vincular uno existente validando la URL antes de
  `GitSetRemote`. `Bridge.GitGhInfo()` indica si `gh` está instalado y con qué
  cuenta. El asistente puede hacer push al terminar.
- **Progreso del push** (`internal/webui/git_push.go`): `pushBranch` (usado por
  `GitPush` y `GitSync`) ejecuta `git push --progress` y reenvía el avance por el
  evento `git:pushprogress` (`PushProgress{stage, percent, uploaded, total,
  speed}`). Antes del push, `pushTransferSize` suma el tamaño en disco de los
  objetos que faltan en el upstream (`rev-list --objects --no-object-names` +
  `cat-file --batch-check=%(objectsize:disk)`, 20 s) para poder decir cuántos MB
  faltan; es una estimación (el pack real añade cabeceras y deltas). El
  porcentaje global se pondera por fase (counting 0-10, compressing 10-30,
  writing 30-95, resolving 95-100) porque git reinicia el tanto por ciento en
  cada una. El proceso va con `LC_ALL=C`/`LANG=C`: las fases y los errores se
  reconocen por texto en inglés. El asistente `wiz` pinta barra determinada
  (o animada mientras no hay total) y «X subidos de Y · faltan Z».
- **Seguridad**: `GitSetRemote` valida la URL (no empieza por guion, sin CRLF,
  formato https/git@/ssh) antes de pasarla a git; `OpenExternal` solo acepta
  http/https. Los helpers `gitHost`, `gitWebURL`, `gitLooksLikeURL` y
  `gitHostIsGitHub` viven en `internal/webui/git.go` (duplicados a propósito de
  los de consola en `internal/tasks/git.go`, que no están expuestos a la GUI).
  El panel se refresca con `onRepoChanged`/`notifyRepoChanged` (`repoChanged.ts`)
  y, al instante, con `onGitChanged`.
- **Detección instantánea de cambios** (`internal/webui/watch.go`): un
  vigilante `fsnotify` por proyecto (activo + los que tienen Auto) escucha el
  árbol de trabajo, agrupa las ráfagas de un guardado (200 ms), recalcula
  `autoSig` y emite `git:changed` (`{dir, changes, conflict}`). El panel Git se
  repinta en cuanto se guarda, sin esperar al sondeo de 5 s, y el reloj del
  auto-commit arranca en el momento del guardado. Se excluyen `node_modules`,
  `dist`, `build`, `vendor`, `target`, `.git`, `bin`, `obj`, etc. (tope de
  3000 carpetas) y los temporales de editor; las carpetas nuevas se añaden al
  vuelo. Tras un commit se olvida la firma (`watchForget`) porque `.git` no
  genera eventos del árbol. El ticker de 15 s sigue como red de seguridad si
  se pierde un evento.
- **Auto-commit por inactividad** (`internal/webui/autocommit.go`): el botón
  "Auto" del panel Git activa un vigilante **por proyecto** que corre en el
  backend (no en el componente, para que siga vivo al cambiar de pestaña).
  Cada 15 s calcula la firma del árbol (`autoSig`): códigos de
  `git status --porcelain -z` **más tamaño y mtime de cada archivo cambiado**,
  de modo que volver a guardar un archivo ya modificado también reinicia el
  reloj (con solo los códigos, la segunda edición pasaba desapercibida). Si la
  firma no cambia durante `settings.AutoCommitIdleMin` minutos (5 por defecto,
  ajustable en Git → Ajustes, rango 1–240), genera el mensaje (OpenRouter; si
  no hay API key, un `chore: cambios automaticos <fecha>`) y commitea. Nunca
  hace push. Se salta árboles con conflictos (`isConflict`), reinicia el reloj
  con cada cambio nuevo y emite el evento `git:auto` para que la UI se
  refresque. El
  umbral se relee en cada pasada, así que cambiarlo en Git → Ajustes aplica sin
  reiniciar el botón. El estado **persiste por proyecto** en
  `settings.AutoCommitProjects`: al arrancar, `resumeAuto()` rearma los
  vigilantes de los proyectos que siguen en el workspace, así que reiniciar
  Compass ya no apaga el botón. Sacar un proyecto del workspace lo apaga
  (`stopAuto`) para que no commitee en segundo plano. API:
  `Bridge.SetAutoCommit(bool)` y `Bridge.AutoCommitStatus()`.
- **Proveedor de mensajes de commit**: solo OpenRouter (`internal/webui/openrouter.go`).
  El CLI local de GitHub Copilot se retiró como proveedor; la configuración
  `commit_provider`/`copilot_model` de ficheros antiguos se ignora y desaparece
  al siguiente `Save`. `GenerateCommit` exige API key y modelo, y cae en el
  gratuito más rápido si no hay modelo elegido.
- **API key de OpenRouter**: `GetSettings` nunca devuelve la clave (ni
  enmascarada); `Bridge.OpenRouterKeyConfigured()` dice si hay una guardada.
  El modal muestra "API configurada" y solo abre el input al pulsar
  "Introducir nueva key"; `SetSettings` conserva la anterior si la nueva viene
  vacía.
- **Test de velocidad de modelos**: `Bridge.TestModelSpeeds(models, endpoint, key)`
  mide una petición real (`Reply with exactly: OK`) a cada modelo con 3 workers
  y timeout de 12 s, y devuelve `[]ModelSpeedResult` ordenado por ms (fallos al
  final). El modal lo pinta en una lista y permite fijar el ganador como modelo.
- **Modal de Opciones**: `App.svelte` lo pinta como rejilla de dos columnas de
  tarjetas (`.opt-grid`/`.opt-card`, 780 px, una columna por debajo de 720 px)
  con secciones Herramientas y General. La tarjeta Git solo remite a la pestaña
  Git; los ajustes de commit/OpenRouter se editan allí (`Git.svelte`).
- **Sin parpadeo "no es un repositorio"**: `Git.svelte` guarda el último
  `GitInfo` por proyecto en `frontend/src/lib/gitCache.ts` y arranca con ese
  valor, así al cambiar de pestaña pinta datos reales al instante (Go lanza
  varios procesos git y tardaba lo suficiente para verse el estado inicial).
  `App.svelte` precalienta el cache con un `$effect` sobre `projectPath`.
  `loaded` solo pasa a `true` con una respuesta real de Go, así que el estado
  vacío nunca se pinta antes de saber si hay repo.
- **Explorador e Historial también usan cache** (`frontend/src/lib/panelCache.ts`):
  guardan por proyecto el último snapshot del árbol (`changes`, `branch`,
  `roots`, `children`, `expanded`) y la lista de commits. Al cambiar de pestaña
  se pinta el snapshot y se refresca en segundo plano. Además `Explorer`
  resuelve `gitStatus` + `gitBranch` + `listDir` en paralelo (antes eran
  secuenciales y sumaban ~2 s). `App.svelte` precalienta ambos con
  `warmExplorer`/`warmLog` en el mismo `$effect` que `loadGitInfo`.

## Core Kits (motores canónicos): WhatsApp y D1

Compass lleva dentro dos **core kits** —motores o runtimes compartidos— en
carpetas propias de la raíz, para que viajen con el binario y no dependan de
rutas externas. Ambos se copian al instalador y Compass los gestiona; el Kit D1
además se administra desde un panel propio.

| Kit | Carpeta | Qué es | Se gestiona desde |
|-----|---------|--------|-------------------|
| **WhatsApp** | `Kit Whatsapp/` | motor de envíos Baileys compartido (Go/Wails, Flutter, Kotlin) | etapado automático en cada release de Windows |
| **D1** | `Kit-D1/` | stack de sincronización Cloudflare D1 (módulo Go + Worker) | panel "Kit D1" del sidebar |

### Kit WhatsApp

- `Kit Whatsapp/` contiene `engine.js`, `bundle.mjs`, `package.json`,
  `node_modules/` y el bundle generado `whatsapp-engine-bundle/`. **No guarda
  ningún `node.exe`**: el runtime usa el **Node 20+ del sistema** (PATH) y
  `dist.bat --with-node` copia ese Node del PATH al payload cuando se necesita
  el sidecar para distribuir a máquinas sin Node.
- **Resolución de ruta** (`internal/tasks/whatsapp_engine.go`):
  1. env `COMPASS_WHATSAPP_ENGINE` (override explícito);
  2. `<dir del exe>/Kit Whatsapp` (instalación / binario compilado);
  3. `<cwd>/Kit Whatsapp` (desarrollo con `go run`).
- **Detección de uso (opt-in + automática)** (`projectUsesWhatsappEngine` /
  `projectImportsWhatsapp` en `internal/tasks/whatsapp_engine.go`): un proyecto
  recibe el runtime si se cumple **cualquiera** de estos criterios:
  1. tiene una carpeta `whatsapp_engine` local **con `engine.js`** (motor propio
     del proyecto que hay que compilar);
  2. tiene el marcador `use-whatsapp-engine` en su raíz (opt-in explícito);
  3. en **Flutter**, `windows/CMakeLists.txt` o el código Dart de `lib/`
     mencionan `whatsapp_engine`;
  4. **escaneo automático**: sus fuentes Go importan un paquete cuya ruta
     referencia `whatsapp` (p.ej. `miproyecto/internal/whatsapp`). El escaneo
     recorre recursivamente los `.go` y omite `node_modules`, `vendor`, `.git`,
     `bin`, `resources`, `out` y `Kit Whatsapp` (para no rallar ni falsar positivos
     dentro del propio Compass); para en cuanto encuentra una coincidencia.
  El marcador `use-whatsapp-engine` es el mecanismo **canónico y
  multiplataforma**: sirve igual para Flutter, Kotlin, Python, Go y Go+Wails y
  sobrevive a refactors del código. Los criterios 3 y 4 son atajos heredados.
  La carpeta `whatsapp_engine/` **no** hace falta en el repo: Compass la genera
  al compilar (con solo `index.js` no cuenta como motor propio).
- **Auto-instalación de dependencias**: solo cuando hay que reconstruir el
  bundle (falta `index.js` o `engine.js` es más nuevo), y si `package.json` es
  más reciente que `node_modules`, Compass ejecuta `npm install` en el kit. La
  copia instalada no lleva `node_modules` (el bundle ya viene compilado); si
  faltara y hubiera que reconstruir, se avisa y se usa el bundle existente.
- **Etapado en build**: `stageWhatsappEngine` (llamado desde `windowsArtifacts`
  en el release de escritorio) reconstruye el bundle si `engine.js` cambió y
  copia **solo `whatsapp-engine-bundle`** al `bin/resources/` del proyecto (o al
  `resources/` de Flutter/Kotlin según tipo). En **Flutter** el destino es
  `whatsapp_engine/` (index.js `--single`). No copia `node.exe` y, si quedó uno
  de un staging anterior, lo elimina para forzar el Node del PATH. La copia solo
  se reescribe si el contenido cambió (`copyFileIfChanged`), para no invalidar
  builds incrementales.
- **Convención única en dev (Go y Flutter)**: `flutter run` usa
  `prepareWhatsappEngineDev`, que **no copia** el motor al proyecto cuando
  `COMPASS_WHATSAPP_ENGINE` está definida y el kit tiene `single/index.js`; el
  servicio Dart lo lee directo del kit. Si la variable no existe (o el bundle
  falta) se etaja como siempre, que es el único modo de funcionar sin Compass
  instalado. El **release siempre lleva su copia**, porque en la máquina del
  usuario final no hay variable ni kit.
- **Node en runtime (cascada)**: los consumidores buscan primero un sidecar
  `node.exe` **del propio proyecto** (staging/release anterior) y, si no,
  `node` en el PATH; exigen **Node >= 20** (Baileys 6.7.x). No usan el
  `node.exe` del engine canónico (`COMPASS_WHATSAPP_ENGINE`), para que el
  runtime dependa del Node del sistema aunque el kit conserve el binario.
  GymPro lo hace en `internal/whatsapp/node.go` (`resolveNode` +
  `checkNodeVersion`, con `findFirstProject`); ELA Group en
  `WhatsappService._findNode` (sidecar → `where node` → rutas comunes → PATH,
  validando la versión). Error típico si falta o es viejo:
  «Node 20+ requerido».
- **Flutter: staging efímero**: tras un release correcto (`FlutterBuildWindows`)
  se llama a `cleanupStagedWhatsappResources`, que borra `whatsapp_engine/` del
  proyecto porque CMake ya instaló su copia en el bundle del release. El motor
  no queda duplicado en el repo; solo existe durante el build. No se limpia si
  el build falló ni si falta la regla `install()` (el release no tendría copia).
- **Flutter: cierre de motores huérfanos antes del build**: `closeProjectWhatsappEngines`
  (`whatsapp_process_windows.go`, llamado antes de `prepareWhatsappEngine` en los
  builds `windows-build` / `flutter-portable` / `flutter-installer`) enumera los
  `node.exe` con `CreateToolhelp32Snapshot`, obtiene su ruta con
  `QueryFullProcessImageName` y termina solo los que viven bajo la carpeta del
  proyecto (`whatsapp_engine/` o `resources/whatsapp-engine-bundle/`). Windows no
  deja sobrescribir un `.exe` en uso: si quedó un motor vivo de una prueba
  anterior, CMake fallaba al copiar `node.exe` al Release con `Permission denied`
  (envuelto en el `MSB3073` de MSBuild). Nunca toca los `node.exe` del sistema ni
  de otros proyectos; si no puede cerrar uno avisa con `[WARN]`. No hace falta
  compilador alternativo: Flutter Windows solo usa MSVC.
- **Flutter: regla CMake automática**: si el proyecto usa el motor y
  `windows/CMakeLists.txt` no menciona `whatsapp_engine`, Compass inyecta el
  bloque `install(DIRECTORY "${CMAKE_SOURCE_DIR}/../whatsapp_engine" ...)` al
  final (`ensureFlutterWhatsappCmakeRule`, idempotente y respetando CRLF/LF).
  Así el motor viaja al release sin que el proyecto tenga que mantener la regla.
- **Sin tarea manual**: el motor se compila y etaja solo en cada release de
  Windows (`prepareWhatsappEngine` → `ensureWhatsappNpm` + `stageWhatsappEngine`,
  que reconstruye el bundle si `engine.js` es más nuevo). La antigua card
  "WhatsApp" y las tareas `whatsapp` / `whatsapp-portable` se retiraron: el
  portable lo genera el propio release para los proyectos que usan el motor.
- `install.bat` / `dist.bat` copian `Kit Whatsapp/` junto al binario/instalador
  (sin los artefactos de build del motor: `node_modules`, `out`, `examples`, y
  sin `node.exe` salvo con `--with-node`).
- **Variable de entorno en la instalación**: `install.bat` define
  `COMPASS_WHATSAPP_ENGINE` (ámbito Usuario) apuntando al kit instalado
  (`%LOCALAPPDATA%\Programs\Keimer\Kit Whatsapp`), para que los proyectos
  que consumen el motor (p.ej. GymPro en modo desarrollo) lo reutilicen en vivo
  sin copiarlo dentro del proyecto. `uninstall.bat` elimina esa variable.
  El modo desarrollo de Compass (`windows-dev` → `wails3 task dev`) no hace
  staging: la ventana nueva hereda el entorno de Compass, así que basta con
  reiniciar Compass tras la instalación para que el proyecto vea el engine.

#### Protocolo stdin/stdout del motor

Contrato que deben respetar todos los consumidores (Go/Wails, Flutter, Tauri,
Python). El motor corre como proceso hijo de `node` y habla **una línea JSON por
mensaje**. `engine.js` es la única fuente de verdad; los bundles son artefactos
generados y **nunca** se editan a mano.

##### Entrada (app → motor, por stdin)

```json
{ "type": "config", "browser": ["GymPro","Chrome","121.0.6167.160"], "authDir": "whatsapp_auth",
  "connectTimeoutMs": 60000,
  "rate": { "sendDelayMs": 5000, "maxWindowSends": 20, "windowMs": 60000 } }
{ "type": "send", "number": "50588888888", "text": "Hola" }
{ "type": "send_image", "number": "50588888888", "imagePath": "C:/tmp/recibo.png", "caption": "Su recibo" }
{ "type": "pair", "phoneNumber": "50588888888" }
```

`config` es opcional y puede llegar **en cualquier momento** (antes o después
de conectar); los campos no enviados conservan su default. `send` y
`send_image` pasan por la cola serial con rate-limit.

##### Salida (motor → host, por stdout con prefijos)

| Prefijo | Significado |
|---|---|
| `STATUS:init` / `STATUS:starting` / `STATUS:auth_ready` | ciclo de arranque |
| `STATUS:socket_creating` / `STATUS:connecting_socket` | conexión con servidores |
| `STATUS:restarting` / `STATUS:disconnected:<reason>` / `STATUS:error:<msg>` | reconexión y errores |
| `STATUS:connected:<client>\|\|<id>\|\|<pic>` | conectado (separador `\|\|` triple) |
| `STATUS:session_invalid` / `STATUS:session_cleared` | sesión inválida / limpiada |
| `QR_DATA_URL:data:image/png;base64,...` | QR listo para la UI |
| `SENDING:<n>` / `SENT_CONFIRM:<n>` / `SENT_RETRY_OK:<n>` / `SEND_ERROR:<msg>` / `SEND_RETRY_FAILED:<msg>` | envío de texto |
| `IMAGE_SENT_CONFIRM:<n>` / `IMAGE_ERROR:<msg>` | envío de imagen |
| `PAIR_CODE:<code>` / `PAIR_ERROR:<msg>` | código de vinculación |
| `CONFIG_RECEIVED:ok` / `ENGINE_ERROR:<msg>` | config aceptada / error interno |
| *(stderr)* | logs de depuración (`WA_DEBUG_*`, `WA_ENGINE_UNCAUGHT_*`) |

**No cambiar** el `emit()` del motor: usa `fs.writeSync(1, ...)` porque en
Windows, cuando stdout es un pipe, `console.log` puede quedar bufferizado y
llegar tarde (o perder eventos).

##### Config por app

Precedencia: **mensaje `CONFIG` > env `WHATSAPP_AUTH_DIR` > defaults**.

| Parámetro | Default | Descripción |
|---|---|---|
| `browser` | `['WhatsApp Engine','Chrome','121.0.6167.160']` | navegador que ve WhatsApp |
| `authDir` | env `WHATSAPP_AUTH_DIR` o `whatsapp_auth` | carpeta de la sesión |
| `connectTimeoutMs` | `60000` | watchdog del QR/conexión |
| `rate.sendDelayMs` | `5000` | intervalo mínimo entre envíos (ms) |
| `rate.maxWindowSends` | `20` | máximo de envíos por ventana |
| `rate.windowMs` | `60000` | ventana de tiempo (ms) |

Cada app debe usar un `authDir` distinto para evitar "logged in from another
device" entre apps propias.

##### Rate limit anti-ban y sesión

El motor centralizó esto tras un ban por ráfaga de 12 recibos al reconectar:

1. **Cola serial** (`sendChain`): los envíos van **uno a la vez**, nunca en
   paralelo. El bug original era `rl.on('line')` con handlers `async` que
   disparaban todos a la vez al reconectar.
2. **`SEND_DELAY_MS`**: espera tras cada envío (default 5000 ms).
3. **Ventana deslizante**: máximo 20 envíos por 60 s; el exceso se **encola**,
   no se descarta.
4. **Flag `isSocketOpen`**: solo envía con socket `open`; sin conexión los
   mensajes quedan bufferizados (máx 30 s).

| Código | Significado | Acción |
|---|---|---|
| `401` / `loggedOut` | sesión expirada | limpia `authDir` + pide QR nuevo |
| `403` | cuenta **baneada/suspendida** | igual que logout: limpia sesión + QR |
| `Connection Failure` | IP marcada por WhatsApp (no la cuenta) | reintenta (máx 5); **no es fix de código**, cambiar red/IP |

Evitar regenerar QR en bucle mientras la IP esté marcada: cada intento
resetea el contador.

##### Build y seguridad

- `node bundle.mjs --asset` → `whatsapp-engine-bundle/` (carpeta con `index.js`
  + `file*.js` + `worker*.js`; hay que copiar **toda** la carpeta).
  `node bundle.mjs --single` → `out/single/index.js` (un solo archivo, Flutter).
- **Baileys pinneado a `6.7.24`** (rama legacy segura; resuelve
  `GHSA-qvv5-jq5g-4cgg`). `npm audit` reporta 0 vulnerabilidades. No saltar a
  `7.x` sin migration test: es un major.
- `NODE_TLS_REJECT_UNAUTHORIZED=0` se usa para evadir inspección SSL de
  antivirus corporativos; reduce la seguridad TLS de terceros.
- **Nunca commitear** `whatsapp_auth` ni el `authDir` de una app.

#### Guía de integración del WhatsApp Engine (para agentes de IA)

Cómo hacer que un proyecto consumidor (p.ej. **GymPro**) use el motor de
WhatsApp canónico de Compass, de forma **dinámica y robusta a cambios de ruta**
(mover Compass o el proyecto no debe romper la integración). El protocolo
stdin/stdout está documentado arriba, en
[Protocolo stdin/stdout del motor](#protocolo-stdinstout-del-motor).

##### Modelo mental
- El motor **canónico** vive en `Kit Whatsapp/` dentro de Compass Go: única
  fuente de verdad (`engine.js`, `bundle.mjs`, `node_modules/`). El repo no
  guarda `node.exe`: el runtime usa Node 20+ del sistema.
- Cada proyecto consumidor recibe **su propia copia** del bundle generado
  (`whatsapp-engine-bundle/`) en su carpeta de recursos. **Nunca** se edita a
  mano el bundle dentro del proyecto: se regenera desde el canónico.
- El motor corre como proceso hijo `node` y se comunica por stdin/stdout.

##### Resolución dinámica de la ruta del motor (sin hardcodear disco)
`whatsappEngineSource()` en `internal/tasks/whatsapp_engine.go` prueba en orden:
1. env `COMPASS_WHATSAPP_ENGINE` (override explícito);
2. `<dir del exe>/Kit Whatsapp`;
3. `<cwd>/Kit Whatsapp`.

Ninguna capa usa una letra de unidad fija, por tanto:
- Si **instalas** Compass, `install.bat` fija `COMPASS_WHATSAPP_ENGINE` a
  `%LOCALAPPDATA%\Programs\Keimer\Kit Whatsapp` (ruta **relativa a la
  máquina**: sobrevive a mover el repo).
- Si trabajas **solo desde el repo** (`go run` / `wails dev`), la variable puede
  estar ausente y la resolución cae a `<cwd>/Kit Whatsapp` (encuentra el
  motor dentro del repo).
- **Nunca** apuntes la env var a una ruta absoluta del repo
  (`D:\...\Compass Go\Kit Whatsapp`): se rompería al volver a mover la
  carpeta. Usa siempre la ruta de instalación relativa o deja que Compass la
  resuelva por cwd/exe.

##### Detección automática en el proyecto consumidor
Un proyecto recibe el runtime si cumple **cualquiera**:
1. tiene carpeta `whatsapp_engine/` local (motor propio);
2. tiene el marcador `use-whatsapp-engine` en su raíz;
3. **escaneo automático**: sus `.go` importan un paquete cuya ruta contiene
   `whatsapp` (p.ej. `internal/whatsapp`). Es el caso normal de GymPro.

Para integrar un proyecto nuevo basta con que importe el paquete `whatsapp`;
Compass etapará el motor solo. No hace falta marcador manual.

##### Pasos operativos (integrar / actualizar)
1. **Editar el motor canónico** solo en `Kit Whatsapp/engine.js` (y
   `bundle.mjs` si cambia el empaquetado). No tocar los bundles generados.
2. **Regenerar el bundle** (lo hace Compass al ejecutar la tarea `whatsapp`, o
   manual):
   ```bat
   cd "Kit Whatsapp"
   npm install
   node bundle.mjs --asset      # Go/Wails          → whatsapp-engine-bundle/
   node bundle.mjs --single     # Flutter           → out/single/index.js
   ```
3. **Etapar al proyecto**: ya no hay tarea manual. El staging ocurre solo al
   compilar un release de Windows desde Compass
   (`prepareWhatsappEngine` → `stageWhatsappEngine`), que copia
   `whatsapp-engine-bundle/` a `resources/whatsapp-engine-bundle/` del proyecto
   (Go/Wails) o a la ruta de recursos según el tipo. No copia `node.exe`: el
   runtime usa Node 20+ del PATH.
4. **Dev en vivo** (sin re-etapar en cada cambio): asegúrate de que
   `COMPASS_WHATSAPP_ENGINE` apunte a la instalación (`install.bat` lo deja
   listo). El modo `windows-dev` hereda el entorno y el proyecto ve el engine.
   En Flutter, `flutter run` hace lo mismo: el servicio Dart resuelve
   `COMPASS_WHATSAPP_ENGINE/single` y Compass no copia nada al proyecto.
   Tras mover Compass, re-ejecuta `install.bat` para re-fijar la variable.

##### Por framework (dónde va el bundle)
| Framework | Destino del bundle | Resolución en runtime |
|---|---|---|
| Go + Wails v3 | `resources/whatsapp-engine-bundle/` | `COMPASS_WHATSAPP_ENGINE` → cwd de la app |
| Flutter | `whatsapp_engine/index.js` (modo single) | `COMPASS_WHATSAPP_ENGINE/single` → cwd → exe dir → app docs |
| Kotlin | carpeta de recursos del framework | según launcher |

##### Ejemplo concreto: GymPro (Go + Wails v3)

Extracto fiel y simplificado de `GymPro/internal/whatsapp` (el paquete real
vive en `supervisor.go`, `exec.go`, `instance_windows.go`, `console_windows.go`
y `logger.go`). La integración se resume en: (a) resolver `node.exe` y el
bundle, (b) lanzar `node <bundle>` con los streams conectados y cwd en el
directorio de datos, (c) mandar `CONFIG` por stdin y (d) parsear stdout por
prefijos.

```go
// internal/whatsapp/engine.go
package whatsapp

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// findFirst devuelve la 1ª ruta existente entre los candidatos relativos al
// cwd de la app y al engine canónico (COMPASS_WHATSAPP_ENGINE).
func findFirst(rel ...string) string {
	if eng := strings.TrimSpace(os.Getenv("COMPASS_WHATSAPP_ENGINE")); eng != "" {
		for _, r := range rel {
			if p := filepath.Join(eng, r); fileExists(p) {
				return p
			}
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		for _, r := range rel {
			if p := filepath.Join(cwd, r); fileExists(p) {
				return p
			}
		}
	}
	return ""
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// resolveEngine localiza Node (sidecar o PATH) y el bundle generado, y valida
// que Node sea >= 20 (lo exige Baileys).
func resolveEngine() (node, bundle string, err error) {
	bundle = findFirst("whatsapp-engine-bundle/index.js",
		"resources/whatsapp-engine-bundle/index.js")
	if bundle == "" {
		return "", "", errors.New("no se encontró el bundle del motor")
	}
	if p := findFirstProject("node.exe", "resources/node.exe", "bin/node.exe",
		"resources/node-x86_64-pc-windows-msvc.exe"); p != "" {
		node = p
	} else if p, lookErr := exec.LookPath("node"); lookErr == nil {
		node = p
	} else {
		return "", "", errors.New("instala Node 20+ o vuelve a etapar el motor con su node.exe")
	}
	if err := checkNodeVersion(node); err != nil {
		return "", "", err
	}
	return node, bundle, nil
}

// startEngine lanza `node <bundle>` con los 3 streams conectados y cwd en el
// directorio de datos (donde el motor crea `whatsapp_auth`).
func startEngine(node, bundle, workDir string) (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
	cmd := exec.Command(node, bundle)
	cmd.Dir = workDir
	hideConsoleWindow(cmd) // Windows: evita que node.exe abra su propia consola
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, nil, nil, err
	}
	return cmd, stdin, stdout, stderr, nil
}

// Run arranca el motor, lo configura y procesa sus eventos.
func Run() error {
	node, bundle, err := resolveEngine()
	if err != nil {
		return err
	}
	// workDir = directorio de datos de la app (allí el motor guarda whatsapp_auth)
	appData, _ := os.UserConfigDir()

	cmd, stdin, stdout, _, err := startEngine(node, bundle, appData)
	if err != nil {
		return err
	}
	defer cmd.Process.Kill()

	// (1) CONFIG por stdin: browser, authDir y rate-limit anti-ban.
	cfg, _ := json.Marshal(map[string]any{
		"type":    "config",
		"browser": []string{"GymPro", "Chrome", "121.0.6167.160"},
		"authDir": "whatsapp_auth",
		"rate":    map[string]any{"sendDelayMs": 5000, "maxWindowSends": 20, "windowMs": 60000},
	})
	if _, err := stdin.Write(append(cfg, '\n')); err != nil {
		return err
	}

	// (2) Eventos por stdout (una línea JSON o con prefijo STATUS:/QR_DATA_URL:/...).
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "QR_DATA_URL:"):
			showQR(strings.TrimPrefix(line, "QR_DATA_URL:")) // p.ej. mostrar en la UI
		case strings.HasPrefix(line, "STATUS:connected:"):
			onConnected(strings.TrimPrefix(line, "STATUS:connected:"))
		case strings.HasPrefix(line, "STATUS:disconnected:"):
			onDisconnected()
		case strings.HasPrefix(line, "SENT_CONFIRM:"):
			onSent(strings.TrimPrefix(line, "SENT_CONFIRM:"))
		}
	}
	return nil
}

// SendText despacha un mensaje; la cola serial + rate-limit las gestiona el motor.
func SendText(stdin io.Writer, number, text string) error {
	msg, _ := json.Marshal(map[string]string{"type": "send", "number": number, "text": text})
	_, err := stdin.Write(append(msg, '\n'))
	return err
}
```

> Notas de GymPro reales que un agente debe respetar:
> - `instance_windows.go` adquiere un mutex con nombre
>   (`Local\GymPro-WhatsApp-Engine`) para que **dos procesos de GymPro no
>   lancen el motor sobre la misma carpeta de sesión** (evita "Bad MAC" y envíos
>   colgados). Mantener ese candado.
> - `logger.go` auto-genera el bundle con `node bundle.mjs` **dentro del engine
>   canónico** (`COMPASS_WHATSAPP_ENGINE`) si falta. Usa el `node.exe` del
>   engine si existe; si no, se omite (el bundle ya viene compilado).
> - `resources/whatsapp-engine-bundle/` de GymPro es la copia etapada por
>   Compass; **no se edita a mano**.

Para (re)etapar el motor en GymPro tras cambiar `engine.js` basta con lanzar un
release de Windows desde Compass: `stageWhatsappEngine` detecta que `engine.js`
es más nuevo y regenera el bundle antes de copiarlo al `resources/` del proyecto.

##### Errores que un agente NO debe cometer
- ❌ Hardcodear `D:\...` en código o en la env var. Usa rutas relativas /
  `%LOCALAPPDATA%`.
- ❌ Editar el bundle dentro de `resources/whatsapp-engine-bundle/` del
  proyecto. Regenera desde el canónico.
- ❌ Asumir que en producción el engine se lee desde la máquina de dev. Cada
  instalador debe embeber su propia copia del bundle; el runtime exige Node
  20+ (sidecar solo si se compila con `--with-node`).
- ❌ Dejar `COMPASS_WHATSAPP_ENGINE` apuntando a una instalación vieja tras
  mover Compass. Re-ejecuta `install.bat`.

### Kit D1

El kit de sincronización **Cloudflare D1** vive **DENTRO de Compass** en
`Kit-D1/` (raíz del repo; sin espacios para que la ruta sea un module path de Go
válido), incorporado con
`git subtree add --prefix="Kit-D1" ../Core Station/Kit D1 main --squash`. Ya no
es un checkout separado: el antiguo repo `Keimer92/Kit-D1` se retira. Contiene
el módulo Go `github.com/Keimer92/Compass-Go/Kit-D1/go/syncengine`, el protocolo
y su fixture canónico (`protocol/fixtures/wire-session.json`), los scripts de
provisión y el `worker/template/` del que se ensamblan las instancias por
proyecto.

- **Resolución de ruta** (`kitD1Root()` en `internal/webui/kitd1.go`): se fija en
  `settings.kit_d1_path` o `COMPASS_KIT_D1`; si no, **primero** la copia
  empaquetada junto al exe (`<exe>/Kit-D1`, instalación) y **después** junto al
  proyecto activo (`Kit-D1/`, o `<padre>/Compass Go UX/Kit-D1` si el activo es
  GymPro/School Tracker). Así el binario instalado usa sus propios kits y basta
  con reinstalar Compass Go cuando cambie el kit; en desarrollo (`go run`) el exe
  vive en un temporal sin `Kit-D1` y se cae a las rutas del proyecto. Se conservan
  como fallback los nombres anteriores (`Kit D1/`, `Core Station/Kit D1`).
- **Panel de administración** (`frontend/src/components/KitD1Panel.svelte` +
  `internal/webui/kitd1.go`): botón "Kit D1" en el pie del sidebar, al lado de
  "Ajustes de Git". El kit es **global**, no por proyecto (como Git), y el panel
  muestra ruta, versión del módulo Go, rama/tag/working tree, sesión de Wrangler
  (`whoami`), instancias de `worker/instances/*` y consumidores del workspace
  (módulo Go con `go.work` local, o Flutter).
- **Acciones**: ensamblar, typecheck, dev, deploy, provision dry-run/real,
  whoami/login, listar tenants, exportar/rotar clave e instalar dependencias.
  Se arman en `KitD1Script` con **whitelist** de acción + validación de
  instancia contra el disco (sin interpolación libre) y se ejecutan en una
  pestaña propia del terminal de Compass (`runKitD1` en `App.svelte`). Deploy,
  provision real y rotar clave piden **confirmación de dos pasos**. La acción
  `tag` se retiró del panel: ahora los tags se crean en el repo de Compass con
  `git tag Kit-D1/go/syncengine/vX.Y.Z`, y `git describe` filtra por
  `--match Kit-D1/go/syncengine/v*`.
- **Instalador**: `dist.bat` copia `Kit-D1/` al payload excluyendo
  `node_modules`, `.wrangler`, `dist`, `out` y `.git`; por eso la acción
  **Instalar dependencias** (`npm install`, acción `bootstrap`) precede a
  typecheck/dev/deploy en una máquina nueva.
- **Consumidores**: en desarrollo los `go.work` de GymPro y School Tracker
  apuntan con ruta relativa a `../Compass Go UX/Kit-D1/go/syncengine`; en un
  release fijan `require github.com/Keimer92/Compass-Go/Kit-D1/go/syncengine vX.Y.Z`.
  El fixture del protocolo (`Kit-D1/protocol/fixtures/wire-session.json`) es la
  fuente única para el replay Go de School Tracker (`internal/wiretest`).
- Cobertura en `internal/webui/kitd1_test.go` (fixture + detección dentro de
  Compass).

#### Contrato HTTP (protocolo v1)

Contrato entre un cliente (Go `syncengine`, cliente Dart o los tests del
Worker) y el Worker de Cloudflare de cada proyecto. Cada byte fue capturado de
una sesión real contra el worker (`wrangler dev --local`, D1/KV simulados) y se
reproduce en `TestReplayDelContratoWire` de School Tracker. La grabación
canónica es `protocol/fixtures/wire-session.json`.

- **Transporte**: `POST <workerUrl>/<path>` con cuerpo `application/json`
  (siempre un objeto, aunque sea `{}`). Toda petición lleva la cabecera
  `X-School-Key: <key>`; el Worker la enruta por la KV `SCHOOL_ROUTING` a la D1
  del proyecto (binding `DB`). Una clave = un namespace dentro de una D1.
- Las respuestas son objetos con `ok: true`. Los errores pueden venir como
  4xx/5xx **o** como HTTP 200 con `ok: false`: el cliente debe tratar **ambas**
  formas como fallo y leer `error`.
- Timestamps de fila: `YYYY-MM-DD HH:MM:SS` (estilo SQLite, texto UTC-ish
  local). `/hello.serverTime` es ISO-8601 con milisegundos.

| Endpoint | Request | Response / uso |
|---|---|---|
| `/hello` | `{}` | `{ok,databaseId,serverTime}` — prueba de conexión y enrutado |
| `/stats` | `{}` | `{ok,total,tombstones,porTabla}` — diagnóstico; nombres del manifiesto |
| `/ids` | `{deviceId,blocks:{tabla:n}}` | `{ok,blocks:{tabla:{start,end}}}` — reserva de bloques de ids **antes** de escribir, para que dispositivos offline no colisionen |
| `/push` | `{since,deviceId,data:{tabla:[fila]},deletions:[{tabla,pk}]}` | `{ok,applied,deletionsApplied}` — upsert por PK canónico sellando `updated_at` al reloj del server; los deletes se guardan como tombstone `(tabla,pk,updated_at,device_id)` |
| `/pull` | `{deviceId,since}` | `{ok,rows,deleted,serverTime}` — filas con columnas calculadas resueltas por `LEFT JOIN`; excluye las filas con `device_id` del propio cliente (anti-eco) |
| `/reset` | `{}` | `{ok,borradas}` — vacía el namespace (incluye tombstones) |

`/usage` (cuota) y las rutas no-JSON (`/ws`, `/conectar`, `/release.json`,
`/updates/latest`) son opcionales/diagnóstico; `/usage` no aparece en el
fixture porque necesita la API de Analytics, inexistente en `wrangler dev
--local`.

**Invariantes del cliente**:

- **Dos relojes independientes**: el *pull cursor* es `serverTime` y solo se
  guarda tras un pull exitoso (arranca en `1970-01-01 00:00:00`); el *push
  cursor* es reloj local (`last_push_sync`) y solo avanza tras un push real y
  exitoso. Un push fallido **nunca** lo avanza. Se reinician en
  `wipe_remote`, `reset_remote`, `restore_from_cloud` o un push `full=true`.
- **Anti-eco**: el Worker sella `updated_at`, el `/pull` excluye las filas del
  propio `deviceId` y `sync_recent_pulls` (TTL 86 400 s) completa el mecanismo.
- **Freno de reintento**: tras pushes fallidos consecutivos, backoff
  exponencial de 5 → 240 min (`push_fail_count`, `push_retry_after`). Mientras
  está armado se sigue **pulling** pero no se hace push; `force=true` lo
  salta solo para botones explícitos (cuota free tier ≈100k filas/día; un push
  completo ≈8k).
- **Identidad canónica**: la PK es la lista ordenada del manifiesto (tombstones
  y anti-eco la usan tal cual). `CloudPK` sustituye las columnas calculadas por
  su `fk` almacenado, para borrar filas remotas durante un wipe. Clave anti-eco:
  `tabla + "|" + pkPart + "|" + updated_at`, con cada valor PK en JSON y
  **barra final**, p. ej. `asistencia|"0035"|"2026-10-21"||2026-10-21 08:00:00`.

#### Especificación del manifiesto de schema (v1)

Cada proyecto declara su schema sincronizable en un manifiesto. El motor Go lo
consume como `[]syncengine.TableSpec` (ver
`go/syncengine/example/school/schema.go`); el Worker lo consume como JSON
generado a partir del registry. Ambos describen **el mismo** contrato.

**Contrato mínimo de la SQLite local**: la tabla debe existir con el mismo
nombre que en D1; tener `updated_at TEXT` mantenido en cada escritura (es la
única ordenación que conoce el protocolo); poder registrar un tombstone por PK
canónica en la **misma transacción** que el `DELETE`; y contar con las tablas de
infraestructura `d1_config`, `sync_recent_pulls`, `sync_pending_deletions`
(+ las dedicadas por tabla) y `d1_id_blocks`.

| Campo `TableSpec` | Tipo | Significado |
|---|---|---|
| `name` | ident | Nombre compartido por SQLite local y D1 |
| `pk` | ident[] | **Identidad canónica (cloud)**, en orden de tombstone; puede incluir columnas calculadas |
| `strategy` | enum | Cómo aplicar las filas bajadas (default `replace`) |
| `business_key` | ident[] | Columnas reconciliadas en vez del id (lo exige `catalog`) |
| `computed` | ComputedFK[] | Columnas solo de protocolo resueltas por joins con padres |
| `upsert` | Upsert | Objetivo/config de conflicto para `key-upsert` |
| `tombstone_table` | ident | Tabla dedicada de borrados pendientes (si no, la genérica) |
| `orphan` | OrphanPurge | Purga local de huérfanos (nunca se tombstonan) |
| `restored` | bool | La borra un restore-from-cloud autoritativo |
| `wire_cols` | ident[] | Columnas que el Worker almacena; espejo del registry, validado en build |

- **ComputedFK**: `column` (columna wire con el valor de negocio),
  `parent` (tabla dueña de la clave), `parent_key` (default `id`), `fk`
  (columna local que referencia al padre y se reescribe al aplicar), `target`
  (columna de negocio del padre, estable entre dispositivos), `code_col`.
  El Worker **no las almacena**: las adjunta en push (join local) y las
  resuelve en pull (`LEFT JOIN`). Existen porque los rowids locales divergen
  entre dispositivos.
- **Upsert**: `conflict` (columnas objetivo del `ON CONFLICT`), `set`
  (columnas a actualizar; vacío = todas salvo el objetivo) e `include_id`.
- **Estrategias**: `replace` (`INSERT OR REPLACE`, default) · `catalog`
  (reconcilia por `business_key` conservando el id local) · `fk-rewrite`
  (resuelve cada FK calculada al id local del padre; descarta filas sin padre)
  · `key-upsert` (reescribe FKs y `ON CONFLICT DO UPDATE` para conservar una
  sola fila por identidad de negocio) · `validate-only` (solo aplica si el
  código adjunto resuelve a la misma fila local). Escape hatch: un
  `UpsertHook` inyectable por app; si dos proyectos lo necesitan, pasa a ser
  una estrategia del kit.
- **Validación en build** (`syncengine.NewRegistry`): todos los identificadores
  deben cumplir `^[A-Za-z_][A-Za-z0-9_]*$` (llegan a SQL por interpolación; el
  allowlist es la guardia anti-inyección), sin nombres vacíos ni duplicados ni
  PK vacía; cada componente de `pk` debe ser calculada o estar en `wire_cols`;
  se valida la matriz estrategia/requisitos y se rechazan estrategias
  desconocidas; `orphan` exige `fk` y `parent`.
- **Orden**: la lista del spec **es** el orden de payload/aplicación. Los
  padres deben ir antes que las tablas reconciliadas por su clave de negocio.

El manifiesto completo (22 tablas) está en
`go/syncengine/example/school/schema.go` (+ DDL en `ddl.go`) y se prueba desde
la app en School Tracker (`internal/syncschema/schema_test.go`).

#### Worker template y operaciones

El Worker es un servidor D1 genérico gobernado por el manifiesto
(`sync.schema.json`, generado desde el registry Go). **Nunca editar a mano**
`worker/instances/<project>/src/**`: es copia de `template/src` y
`scripts/build-instance.mjs` la sobrescribe.

- **Layout**: `template/src/` con `index.ts` (router que monta módulos según
  bindings), `engine.ts` (endpoints), y los opcionales `hub.ts` (`/ws`),
  `releases.ts` (feed de updates), `auth.ts` (Google/email, `/admin/*`) y
  `provision.ts` (`/ping`, `/conectar`). Cada instancia lleva su
  `sync.schema.json`, `sync.config.json`, `wrangler.toml`, `package.json`,
  `sync-project.json` (config de ops), `schema.sql`, `tenants.json` y
  `sync-keys.json` (credencial, gitignored).
- **Montaje por binding**: `engine` requiere `DB` + `SCHOOL_ROUTING`;
  `provision` siempre ( `/conectar` solo con `deepLinkScheme`); `releases`
  requiere KV `RELEASES`; `auth` requiere KV `USERS`; `hub` requiere el DO
  `SYNC_HUB`. Un proyecto que solo quiere sincronizar declara `DB` +
  `SCHOOL_ROUTING` y obtiene el worker mínimo.
- **Feed de updates**: con `RELEASES` KV, `GET /updates/latest?version=<v>&platform=<os>`
  devuelve el manifiesto (`200`), `204` si la versión publicada no es más
  nueva, `404` si no hay nada publicado (la app lo trata como fallo, no como
  "estás al día"). Artefactos en `file:<name>`, descargables desde
  `/release/<name>`. Se publica con
  `node scripts/publish-update.mjs --dir worker/instances/<p> --manifest update.json --file <artifact>`.
- **Cómo el manifiesto gobierna el engine**: `conflict` (objetivo del
  `ON CONFLICT`), `set_exclude_id` (el UPDATE nunca reescribe el `id` stored),
  `server_assigns_id` (INSERT omite el id; la identidad es la clave de
  negocio), `computed[].canonize` (reescribe la FK al id canónico del padre
  mediante `COALESCE((SELECT …), …)`). `ensureSchema` añade las columnas
  `sk`/`device_id`, el índice `idx_<t>_sk` y, donde hace falta, el UNIQUE
  `idx_<t>_conflict`.
- **Checklist para proyecto nuevo**: (1) escribir el registry Go y generar
  `sync.schema.json`; (2) copiar una instancia existente para las formas de
  `wrangler.toml`/`package.json`, con worker/D1/KVs propios; (3) añadir
  `sync.config.json`; (4) `node scripts/build-instance.mjs <p>` y
  `npx tsc --noEmit`; (5) `node scripts/provision.mjs --dir worker/instances/<p>`
  y probar con `npx wrangler dev --local`; (6) emitir claves con
  `generate-key.mjs` y operar con `admin.mjs` (requiere `SYNC_ADMIN_ID_TOKEN`).
  Nada del lado cloud debe vivir en el repo de la app.
- **Paridad**: `scripts/parity-check.mjs` compara una captura fresca contra el
  fixture dorado: paths/statuses estrictos, requests byte a byte (se aceptan
  solo diferencias de orden de array) y respuestas como JSON normalizado de
  timestamps e insensible al orden. `school-tracker` pasó los 14 intercambios.
- **Credenciales**: `*.key`, `sync-keys.json`, `school-keys.json`, `.dev.vars`,
  `.wrangler/`, `*.db`/`*.sqlite` están gitignored. `wrangler.toml` guarda
  identificadores (account id, KV/D1 ids, emails), no secretos: el token vive
  en `wrangler secret`.

## Desarrollo

```bat
REM Frontend (Svelte) → cmd/compass-web/frontend/dist (embebido con //go:embed)
cd frontend && npm install && npm run build && cd ..

go build -o dist/payload/compass-go.exe ./cmd/compass-web   # compilar sin instalar
go vet ./...                  # análisis estático
go test ./...                 # tests unitarios (sin ventana real)
COMPASS_E2E=1 go test ./...   # tests e2e (requieren Windows + ejecutar el binario real)
```

- Los tests de `internal/webui` cubren detección de proyecto, fallback al
  workspace, `git status`, árbol de archivos y subcards; no abren ventana.
- Los tests e2e (`internal/container/e2e_test.go`) están **tras la variable
  de entorno `COMPASS_E2E=1`** porque arrancan `compass-go.exe` de verdad.
- Recursos de Windows: `go-winres.json` + `assets/icon/*.ico` → `.syso` en
  `cmd/compass-web/rsrc` (generados al compilar con `install.bat` / `dist.bat`).
- Si el frontend no se recompila, el `.exe` embebe el bundle **anterior**:
  `dist.bat` siempre ejecuta `npm run build` antes de `go build`.

### Desarrollo con HMR (`dev.bat`)

`dev.bat` levanta Vite **en segundo plano dentro de la misma consola**
(`start /b` sobre `frontend/dev-vite.bat`, puerto 5173, `strictPort`) y luego
deja el backend en manos de `dev-run.ps1`, que compila y arranca
`build\dev\compass-web.exe --dev`. No se abre una segunda ventana: la salida de
Vite se escribe en `%LOCALAPPDATA%\compass\vite-dev.log` para no mezclarse con
la del backend, y al cerrar la consola Vite muere con ella (sin huérfanos). El
flag `--dev` exporta
`FRONTEND_DEVSERVER_URL=http://127.0.0.1:5173`; con ese valor Wails v3 sirve la
UI desde Vite en lugar del bundle embebido (rama `!production` de
`internal/assetserver`), de modo que los cambios de Svelte/CSS/TS se aplican en
caliente.

**Backend sin cerrar la consola**: Go es un binario compilado, así que no hay
hot reload real. `dev-run.ps1` vigila `*.go`, `go.mod` y `go.sum` con un
`FileSystemWatcher` y, tras 600 ms de silencio (para agrupar varios guardados),
recompila y reinicia la ventana. También acepta teclas en la consola: `R`
reinicia en el acto y `Q` cierra Compass. La compilación va a
`build\dev\compass-web.next.exe` y solo si termina bien se cierra la instancia
actual y se renombra a `compass-web.exe`; así un error de Go deja el Compass
abierto (solo se ven los errores) y el siguiente guardado lo reintenta. El
`.exe` en ejecución no se puede sobrescribir en Windows, de ahí el nombre
intermedio. El cierre es ordenado (`CloseMainWindow`) para que Wails termine
sus ConPTY; a los 5 s se mata el árbol con `taskkill /T /F`. Los eventos del
watcher se encolan en un hashtable sincronizado, de modo que un guardado
durante un rebuild no se pierde: se aplica en la vuelta siguiente.

```bat
dev.bat              # Vite (HMR de UI) + backend con R = recompilar/reiniciar
```

- **Reinicio limpio** (`dev-reset.ps1`): antes de arrancar, `dev.bat` cierra la
  sesión de desarrollo anterior para que no se acumulen ventanas/Compass. El
  script mata el proceso `compass-web.exe`/`compass-go.exe` cuyo `--dev` está en
  su línea de comandos (la versión instalada no lleva `--dev` y no se toca) y
  cierra la terminal con título `Compass Go - desarrollo`, excluyéndose a sí
  mismo. Como Vite ahora comparte esa consola, muere con ella; `dev.bat` deja
  1 s de margen y vuelve a sondear el puerto antes de arrancar otro (evita el
  choque con el `strictPort` mientras el node anterior suelta el socket). El
  título se pone en el batch **después** de llamar al reset, por eso el
  script puede distinguir la terminal vieja de la nueva.
- `frontend/vite.config.ts` fija `server.hmr` a `ws://127.0.0.1:5173`: la
  página se sirve bajo el esquema `wails://wails:5173`, así que el cliente HMR
  no puede deducir el host del servidor y hay que indicárselo.
- **`server.host = '127.0.0.1'` es obligatorio**: por defecto Vite 6 escucha
  solo en `::1` (IPv6), mientras que el proxy de Wails y el sondeo de `dev.bat`
  van por IPv4. Sin esto el puerto parece cerrado, el batch se queda esperando
  y la ventana quedaría en blanco.
- **Los `.bat` deben estar en CRLF en disco** (`core.autocrlf=true`). Si se
  editan con herramientas que escriben solo LF, `cmd.exe` no encuentra los
  labels (`call :probe` / `goto :wait`) y sale `The system cannot find the batch
  label specified - probe`. Para convertir: leer el archivo, reemplazar
  `\r?\n` por `\r\n` y reescribir.
- **`assets/icon/CompassGo.ico` es obligatorio**: `assets/embed.go` lo embebe
  con `//go:embed icon/CompassGo.ico` y `go-winres.json` / `dist.bat` apuntan al
  mismo nombre (sin espacios). Renombrarlo (p. ej. a `Compass Go.ico`) rompe
  `go run`/`go build` con `pattern icon/CompassGo.ico: no matching files found`
  y `dev.bat` nunca abre la ventana. Si se regenera el icono, se conserva esse
  nombre exacto.
- En modo dev se omite `workspace.TryForward`, para que una instancia instalada
  abierta no se quede el foco ni absorba la carpeta.
- **Las ConPTY sobreviven a la recarga de la página**: al guardar un `.svelte`
  Vite recarga el WebView y el frontend pierde su estado, pero
  `TerminalService` conserva las sesiones y los últimos 256 KB de salida de
  cada una (`terminal.go`). Al montar, `restoreTerminals` llama a
  `ListTerminals()` y reconstruye las pestañas con el mismo id, repintando el
  búfer, así que un build en curso no se interrumpe al editar la UI.
- **Hosts de terminal keyed y siempre medibles**: el `{#each tabs}` que pinta
  los `.term-host` usa clave `(tab.key)`. Sin clave, Svelte reutilizaba los
  nodos al cerrar una pestaña y el DOM de xterm se quedaba en el host de otra
  pestaña (se veía el contenido de una terminal en otra). Las pestañas ocultas
  usan `visibility: hidden` (no `display: none`) para que el host conserve
  tamaño y `FitAddon` no mida 0. Al volver a una pestaña se restaura el scroll:
  `activate()` anota si la terminal que se deja estaba al final y hace
  `scrollToBottom()` en la que se abre.
- **Ancho útil del terminal (`fitTerm`)**: `FitAddon` mide el host y resta una
  barra de scroll supuesta (15 px cuando no puede medirla), así que su ancho se
  queda corto. El ancho real es `viewport.clientWidth`, que ya descuenta la
  barra de 10 px del CSS global; `fitTerm` calcula las columnas con el ancho de
  celda que pinta el renderer y usa **todo** ese ancho. Reservar una celda extra
  dejaba una franja muerta a la derecha y las TUI (el chat de Codebuddy trunca
  sus líneas al ancho de la PTY) cortaban su marco y su texto una columna antes
  del borde. Solo se llama a `resize()` si el tamaño cambia, y
  `.term-host .xterm-viewport` lleva `overflow-x: hidden` para que no aparezca
  una barra horizontal que robe una fila.
- `dist.bat` compila con `-tags production`, así que el binario distribuido
  ignora `FRONTEND_DEVSERVER_URL` y siempre sirve el bundle embebido.

## Notas de arquitectura / diferencias con el Rust original

- **UI**: Wails v3 + Svelte 5 en vez de `ratatui`/Gio. El puente vive en
  `internal/webui`; el frontend llama `Call.ByName("compass/internal/webui.Bridge.<Metodo>")`.
- **Sin TUI**: la interfaz de terminal se retiró por completo (`internal/tui` y
  el render de `internal/container` ya no existen). Lo que queda es el *modelo*
  de las cards en `internal/menu` (enum `MenuItem` + etiqueta/icono/color por
  tipo de proyecto) y los ejecutores de `internal/tasks`. Las tareas que antes
  abrían un submenú interactivo (`clean`, `git`, `signing`, `environment`,
  `windows-release`, `android-release`) ahora piden la opción por consola con
  `prompts.Select`; la GUI usa siempre las subcards (`<prefijo>-do-<clave>`).
  Gracias a eso el binario ya no depende de `tcell` ni de `gioui`.
- **Cards de Android condicionadas al proyecto**: `GetMenu` oculta toda card
  con `MenuItem.IsAndroid(pt)` (And. Dev, APK Release, AAB, Install, Debug Andr
  y Signing) cuando `ProjectContext.HasAndroidProject` es falso. Se detecta con
  la presencia de `build/android/gradlew.bat` (Wails), `android/gradlew.bat`
  (Flutter) o `gradlew.bat` en la raíz (Kotlin). Un proyecto Wails sin
  `wails3 update build-assets` ya no ofrece builds de Android que abortarían.
- **Cards de escritorio de Wails condicionadas al Taskfile**: `GetMenu` oculta
  Windows (Win Dev) y Win Release cuando el proyecto es Go+Wails y no tiene
  `Taskfile.yml`/`.yaml` (`ProjectContext.HasTaskfile` falso). Esas cards lanzan
  `wails3 task dev|build|package`, que sin Taskfile aborta con *"No Taskfile
  found"*. `MenuItem.NeedsWailsTasks()` marca qué cards dependen del CLI de
  Wails (en Go puro las mismas usan `go run`/`go build` y no se ocultan). En
  Compass Go, que se compila con `go build` + `dist.bat` y no trae Taskfile,
  desaparecen; en un proyecto Wails de plantilla siguen funcionando.
- **Corekits como proyecto (`TypeWhatsappKit`, `TypeKitD1`)**: los repos de los
  kits compartidos se abren en Compass igual que una app, para editarlos y
  versionarlos (auto-commit incluido), aunque no se compilan ni se publican.
  - `ClassifyDir` reconoce el **motor de WhatsApp** con `engine.js` +
    `bundle.mjs` + `package.json` en la raíz, y el **Kit D1** con
    `go/syncengine/go.mod` + `worker/template/` + `scripts/provision.mjs`
    (los tres, para no confundirlo con un worker suelto de Cloudflare).
  - `ProjectType.IsKit()` centraliza la distinción. Los kits solo reciben las
    tareas genéricas (`clean`, `git`, `terminal`, `environment`); `appTypes()`
    es la lista sin kits que usan release y firma. `GetMenu` oculta además la
    card Signing para ellos.
  - El Kit D1 ya no necesita abrirse como proyecto externo: vive en `Kit-D1/`
    dentro de este repo (subtree) y se gestiona desde el panel. `ClassifyDir`
    sigue reconociéndolo (por si se abre esa carpeta directamente), pero
    `isWailsV3` se evalúa antes, así que abrir la raíz de Compass NO lo clasifica
    como kit.
  - `.gitignore`: `gitignoreContent` tiene plantillas propias. La de WhatsApp
    ignora `node_modules/`, `dist/`, el bundle de ncc (`whatsapp-engine-bundle/`,
    `single/`, `node.exe`) y las sesiones (`whatsapp_auth/`, `.auth/`,
    `auth_info*/`). La de Kit D1 ignora `.wrangler/`, `.dev.vars`, las
    credenciales (`sync-keys.json`, `*-keys.json`, `*.key`), bases locales
    (`*.db`, `*.sqlite*`) y artefactos de Go. Los kits **no** reciben las reglas
    del motor WhatsApp etajado (`resources/whatsapp-engine-bundle/`): esas solo
    aplican a las apps que lo consumen.
- **Terminal embebida**: `go-pty` (ConPTY) + `vt10x` (parser VT) en Go, y
  xterm.js en el frontend. La salida se reparte con `container.SpawnTap` entre
  el parser y el evento `pty:data` (nunca dos lectores sobre la misma ConPTY).
  **vt10x no tiene scrollback**; el scrollback lo aporta xterm.js.
  - **Scroll y resize**: `fitTerm` guarda `viewportY` antes de `FitAddon.fit()`
    y lo restaura (o baja al final si ya estaba abajo); el PTY solo recibe
    `ResizeTerminal` cuando `cols x rows` cambia (`lastPtySize`), para no enviar
    SIGWINCH innecesarios al cambiar de proyecto (las TUI redibujaban y perdian
    el scroll). Al salir un proceso se limpian los modos de mouse
    (`?1000l/?1002l/?1003l/?1005l/?1006l`).
  - **ResizeObserver del stage**: al cambiar de proyecto cambian las cards y el
    alto disponible del terminal; sin reajustar, la ultima fila (el input del
    chat) quedaba cortada abajo y no se podia bajar mas. El observer de
    `.term-stage` coalesce el reflow en un `requestAnimationFrame` y vuelve a
    hacer `fitTerm` + `resizePty` de la pestaña activa.
  - **Ancho util y barra de scroll**: `fitTerm` calcula el tamano final con
    `fit.proposeDimensions()` y lo acota a las columnas que caben en
    `viewport.clientWidth` (que ya descuenta la barra), llamando a `resize()`
    **una sola vez** y solo si el tamano cambia. Llamar a `fit()` y luego
    recortar columnas redimensionaba dos veces en cada cambio de pestaña y ese
    reflow desplazaba la consola. Un segundo `ResizeObserver` (`vpRo`) observa
    el viewport de cada pestaña para reajustar cuando aparece la barra de
    scrollback.
  - **Reparto cards/terminal**: `.terminal-panel` usa
    `flex: 0 0 var(--term-pct, 60%)`, de modo que el alto no depende del numero
    de cards (cambiar de proyecto no dispara SIGWINCH). El porcentaje se ajusta
    arrastrando `.term-splitter` (o con flechas/Inicio si tiene el foco), se
    acota a 25-80 y se persiste en `terminal_panel_pct`. Durante el arrastre
    `scheduleFit` no hace nada: se reajusta una sola vez al soltar para no
    redibujar la TUI en cada pixel.
  - **Rueda en TUI**: si la app activa mouse tracking en el buffer normal,
    `attachCustomWheelEventHandler` desplaza el scrollback de xterm en vez de
    mandar la rueda a la CLI; en el buffer alterno (vim, less, htop) se respeta
    el mouse de la aplicacion.
  - **Av Pág / Re Pág**: xterm no las mapea al scrollback por defecto, así que
    `attachCustomKeyEventHandler` intercepta `PageUp`/`PageDown` y llama a
    `term.scrollPages(-1|1)`. Solo actúa en el buffer normal (sin Ctrl/Alt); en
    el alterno el par se envía a la TUI, que es quien pagina.
  - **Panel derecho plegable**: el boton `⤢` de la barra de pestañas oculta
    `.right` (`class:collapsed`) para dar todo el ancho al terminal cuando un
    diff o una tabla no caben; al plegar/desplegar se reajusta el xterm y el PTY.
- **Sonidos de eventos**: `internal/sound` genera los avisos con `Beep` de
  kernel32 (sin WAV ni PowerShell) y ofrece cinco presets (Clasico, Suave,
  Brillante, Grave, Silencio) para seis eventos: build correcto/fallido,
  commit correcto/fallido y tarea de terminal correcta/fallida. La seccion
  "Sonidos" de Opciones permite elegir un preset por evento y escucharlo con
  ▶ (`GetSoundCatalog`/`PreviewSound`). Los presets se guardan en
  `sound_presets` de `settings.json`; `sound.Normalize` rellena claves
  ausentes o desconocidas. Los eventos de terminal llegan en `none` porque un
  build ya suena desde su propio proceso y encender ambos duplicaria el aviso.
- **CLIs de Vibe Coding en la barra lateral**: la seccion "Vibe Coding" de
  Opciones lista todas las CLIs (`GetVibeTools`, incluidas las no instaladas)
  con un interruptor. La barra lateral pinta las **habilitadas e instaladas**
  en una rejilla de dos columnas (`vibeSide`), solo con el nombre; al hacer
  hover/foco un tooltip muestra la ruta y la accion. Al pulsar se lanza
  `vibe-run-<clave>` (`RunTask` devuelve ese id tal cual porque no tiene indice
  de menu). Las no instaladas solo se ven en Opciones, donde tambien viven
  "Instalar faltantes" y "Configurar OpenCode" (antes subcards de la card
  retirada; el enum `VibeCoding` se conserva oculto para no desplazar indices).
  Se guarda la lista de deshabilitadas (no la de habilitadas) para que una CLI
  nueva aparezca activa sin migrar el fichero;
  `settings.NormalizeVibeDisabled` recorta espacios y duplica sin distinguir
  mayusculas. La cascada del Explorador se escribe en el registro al instalarse,
  asi que `SetSettings` llama (en segundo plano y solo si la lista cambio) a
  `tasks.SyncVibeContextMenu`, que reescribe las sub-entradas con
  `resolveVibeTools` (mismo filtro) y no crea el menu si el usuario nunca lo
  instalo.
- **CodeBuddy y HIGH/CRITICAL**: `vibeEnv` fija variables de entorno por CLI
  antes de lanzarla. CodeBuddy usa `CODEBUDDY_IS_SANDBOX=1`, porque
  `--dangerously-skip-permissions` (bypassPermissions) aun pide confirmacion en
  las acciones HIGH/CRITICAL; con esa variable de proceso se aceptan por
  defecto (full pass). `vibeEnvPrefix` devuelve el prefijo `$env:NOMBRE='v'; `
  que se inserta **antes** del `&` de invocacion, tanto en la ConPTY de la GUI
  (`VibeCommand`, que ya devuelve la linea completa) como en la cascada del
  Explorador (`vibeContextCommand`). Debe ser variable de proceso: no vale
  inyectarla desde `settings.json`. El resto de CLIs no lleva prefijo.
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
- **Menú contextual**: claves `zz_CompassGo` / `zz_VibeGitignore` /
  `zz_VibeCoding` en `HKCU`, gestionadas por PowerShell desde Go. Cada entrada
  usa su propio `.ico` (`CompassGo.ico`, `VibeCoding.ico`, `Git.ico`) generado
  desde los `.png` de `assets/icon` con `packaging/make_icons.ps1`.
  `refreshContextIcons` reescribe el `Icon` de las entradas existentes en cada
  `context-menu-install`, de modo que mover la instalación no deja rutas
  absolutas obsoletas (que el Explorador pinta como "sin icono").
- **Instalador de proyectos**: los releases de Windows de **Go puro** generan
  además `<Nombre> v<ver> [arch] Setup.exe` (plantilla NSIS embebida en
  `internal/tasks/assets/app-installer.nsi`, compilada con `makensis`). La
  página final ofrece "Crear un acceso directo en el escritorio" y "Ejecutar
  ahora". Go+Wails sigue usando el instalador nativo de su framework
  (necesario para WebView2).
- **Releases de Flutter**: la tarjeta Windows Build ofrece tres subcards —
  build del host, **portable** (`flutter-portable`) e **instalador**
  (`flutter-installer`). Flutter no compila por arquitectura (no acepta
  `--target-platform`), así que solo se ofrece la del equipo (`HostArchLabel`).
  - `internal/tasks/folder_artifacts.go`: `writeFolderArchive` empaqueta la
    carpeta Release completa en `payload.tar.gz` (gzip `BestCompression`,
    renombrando el exe real a `app.exe` para esquivar espacios en ldflags);
    `buildFolderPortable` genera un stub Go (`assets/portable-launcher.go.txt`)
    que extrae a `%LOCALAPPDATA%\<App>\<hash>\`, marca con `.compass-ready` y
    lanza el exe con cwd ahí, heredando args y propagando el exit code;
    `buildFolderInstaller` usa `assets/app-folder-installer.nsi` (`File /r` de
    todo el Release). Salidas en `<proyecto>/bin`.
  - Un release de ~136 MB (Flutter + `whatsapp_engine/`) da un portable de
    ~48 MB y un Setup NSIS de ~32 MB. El portable tarda ~2-4 s la primera vez
    en extraer; el stub precisó ~46 MB de payload comprimido en la prueba real.
  - Tanto `FlutterBuildPortable`/`FlutterBuildInstaller` como `flutterArtifacts`
    limpian el `whatsapp_engine/` etajado al terminar (si la regla `install()`
    del CMake existe), ya que CMake copió su propia copia al Release.
- **Plataformas soportadas**: solo **Windows** como aplicación de escritorio;
  Android sigue como destino de compilación para los proyectos compatibles.
