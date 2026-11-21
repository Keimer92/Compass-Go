# Compass Go — Guía para agentes y desarrolladores

## ¿Qué es?

`Compass Go` es el **port a Go** de *Compass* (originalmente en Rust): una
**GUI de escritorio para Windows** (Wails v3 + Svelte 5) que automatiza tareas
de desarrollo, compilación y configuración para proyectos **Flutter (Dart)**,
**Kotlin Multiplatform (Compose)**, **Go + Wails v3**, **Go puro** y
**Python**. Detecta el tipo de proyecto y adapta comandos, rutas y menús.

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
├─ dev.bat                    # Vite en 2º plano (HMR de UI) + go run --dev
├─ frontend/dev-vite.bat      # arranca Vite con salida a %LOCALAPPDATA%\compass\vite-dev.log
├─ dev-reset.ps1              # cierra la sesión de desarrollo anterior al relanzar dev.bat
├─ install.bat                # dist.bat + instalación silenciosa con el Setup
├─ dist.bat                   # compilar (frontend + Go) y empaquetar el instalador en dist/
├─ uninstall.bat              # desinstalar de la máquina (usa Uninstall.exe si existe)
├─ go-winres.json             # metadatos del .exe (nombre "Compass Go")
├─ corekit/                  # core kits canónicos que viajan dentro de Compass (p.ej. whatsapp)
│  └─ whatsapp/               #   motor de WhatsApp: engine.js, bundle.mjs (sin node.exe)
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

- Cierra `compass-go.exe` en ejecución antes de sobrescribir (Windows no
  deja sobreescribir un `.exe` en uso).
- **Si se lanza desde una pestaña de Compass** (las ConPTY exportan
  `COMPASS_EMBEDDED=1`), `install.bat` no continúa ahí: se relanza a sí mismo
  en una consola aparte (`--detached`, limpia la marca) y sale. De lo
  contrario el `taskkill` de arriba cerraría la app, la consola y el agente
  que la lanzó, abortando la instalación a medias. Esa consola aparte hace
  `pause` al terminar (o al fallar) para que el resumen quede a la vista.
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

Al copiar `corekit/` al payload se excluyen los artefactos de build del motor
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

## Panel Git lateral (tipo VSCode)

- **Git vive en el panel derecho, no en una card**: la card Git y sus subcards
  (`git-do-*`) se retiraron; `menu.Git` queda en `hiddenMenuItems` para no mover
  los índices de las demás cards. La pestaña "Git" reemplaza a la antigua
  pestaña "Commit": el generador de commit con IA queda siempre desplegado y,
  si la carpeta aún no es un repo, ofrece **Inicializar repositorio** (rama
  `main` + primer commit). Toda la configuración (commit automático, proveedor
  de mensajes, modelo de Copilot y OpenRouter) vive en el desplegable **Ajustes
  de Git** del propio panel. Las pestañas del panel derecho son ahora
  **Explorador / Git / Historial**. No hay barra superior (topbar): el título lo
  aporta el sidebar y los botones "Tema" y "Opciones" viven en un pie
  (`sidebar-foot`) al final del sidebar, de modo que cards y terminal ganan esos
  52 px de alto. El modal "Opciones" conserva "Herramientas" (cards de Config:
  Terminal, Entorno) y "General".
- **API del puente** (`internal/webui/git.go`): `Bridge.GitInfo()` devuelve un
  DTO con `isRepo`, `hasCommits`, `branch`, `upstream`, `remoteUrl`, `webUrl`,
  `isGitHub`, `ahead`, `behind`, `changes`. Nunca falla: lo que no se puede
  leer queda vacío. `Bridge.GitPush()` hace `git push` con upstream o
  `git push -u origin HEAD` la primera vez; `Bridge.GitPull()` hace
  `git pull --rebase` (el frontend compone "Sincronizar" = pull + push);
  `Bridge.GitSetRemote(url)` agrega/actualiza `origin`; `Bridge.GitInitRepo()`
  inicializa, fija rama `main` y hace el primer commit; `Bridge.OpenExternal(url)`
  abre el enlace del repo en el navegador del sistema. `GitPush`/`GitPull` usan
  `runGitTimeout` (90 s) para no congelar la promesa Wails si git espera
  credenciales. `@{u}` es revisión fija de git, no texto del usuario.
- **Seguridad**: `GitSetRemote` valida la URL (no empieza por guion, sin CRLF,
  formato https/git@/ssh) antes de pasarla a git; `OpenExternal` solo acepta
  http/https. Los helpers `gitHost`, `gitWebURL`, `gitLooksLikeURL` y
  `gitHostIsGitHub` viven en `internal/webui/git.go` (duplicados a propósito de
  los de consola en `internal/tasks/git.go`, que no están expuestos a la GUI).
  El panel se refresca con `onRepoChanged`/`notifyRepoChanged` (`repoChanged.ts`).
- **Auto-commit por inactividad** (`internal/webui/autocommit.go`): el botón
  "Auto" del panel Git activa un vigilante **por proyecto** que corre en el
  backend (no en el componente, para que siga vivo al cambiar de pestaña).
  Cada 15 s lee `git status --porcelain`; si la firma del estado no cambia
  durante `settings.AutoCommitIdleMin` minutos (5 por defecto, ajustable en
  Git → Ajustes, rango 1–240), genera el mensaje (OpenRouter; si no hay API
  key, un `chore: cambios automaticos <fecha>`) y commitea. Nunca hace push. Se
  salta árboles con conflictos (`hasConflict`), reinicia el reloj con cada
  cambio nuevo y emite el evento `git:auto` para que la UI se refresque. El
  umbral se relee en cada pasada, así que cambiarlo en Git → Ajustes aplica sin
  reiniciar el botón. API: `Bridge.SetAutoCommit(bool)` y
  `Bridge.AutoCommitStatus()`.
- **Proveedor de mensajes de commit** (`settings.CommitProvider`):
  `openrouter` (por defecto, API con key) o `copilot` (CLI local de GitHub
  Copilot). El segundo vive en `internal/webui/copilot.go`: localiza el binario
  (`%LOCALAPPDATA%\GitHubCopilotCLI\copilot.exe`, luego PATH; si es el shim
  `.cmd` de npm lo lanza vía `cmd /c`) y ejecuta
  `copilot -s --deny-tool=shell --deny-tool=write -p <prompt>` con timeout de
  120 s. No usa API: consume la suscripción del usuario y tarda ~18 s por
  mensaje. `Bridge.CopilotAvailable()` permite avisar en Git → Ajustes si falta el
  CLI. El prompt (resumen de git acotado a ~9 KB de diff) viaja como argv, muy
  por debajo del límite de Windows.
- **Modelo de Copilot** (`settings.CopilotModel` + `Bridge.CopilotModels()`):
  el catálogo curado son los perfiles Auto (`""` = equilibrado,
  `auto-intelligence`) y los modelos pequeños del catálogo (haiku/flash/luna),
  que son los más rápidos y baratos. `copilotModelArgs` traduce la elección a
  `--model`/`--auto-tier` y **cae en Auto ante cualquier id desconocido**, así
  que nunca se pasa texto arbitrario al CLI. Ojo: qué modelos están disponibles
  depende del plan de GitHub; con Copilot Free solo responde `auto` y el resto
  devuelve `Model "X" is not available`.
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

## Core kits (motores canónicos)

Compass lleva dentro ciertos **core kits** —motores o runtimes compartidos— en
`corekit/`, para que viajen con el binario y no dependan de rutas externas. El
primer kit es el **motor de WhatsApp** (`corekit/whatsapp/`).

- `corekit/whatsapp/` contiene `engine.js`, `bundle.mjs`, `package.json`,
  `node_modules/` y el bundle generado `whatsapp-engine-bundle/`. **No guarda
  ningún `node.exe`**: el runtime usa el **Node 20+ del sistema** (PATH) y
  `dist.bat --with-node` copia ese Node del PATH al payload cuando se necesita
  el sidecar para distribuir a máquinas sin Node.
- **Resolución de ruta** (`internal/tasks/whatsapp_engine.go`):
  1. env `COMPASS_WHATSAPP_ENGINE` (override explícito);
  2. `<dir del exe>/corekit/whatsapp` (instalación / binario compilado);
  3. `<cwd>/corekit/whatsapp` (desarrollo con `go run`).
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
     `bin`, `resources`, `out` y `corekit` (para no rallar ni falsar positivos
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
  de un staging anterior, lo elimina para forzar el Node del PATH.
- **Node en runtime (cascada)**: los consumidores buscan primero un sidecar
  `node.exe` **del propio proyecto** (staging/release anterior) y, si no,
  `node` en el PATH; exigen **Node >= 20** (Baileys 6.7.x). No usan el
  `node.exe` del engine canónico (`COMPASS_WHATSAPP_ENGINE`), para que el
  runtime dependa del Node del sistema aunque el corekit conserve el binario.
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
- `install.bat` / `dist.bat` copian `corekit/` junto al binario/instalador
  (sin los artefactos de build del motor: `node_modules`, `out`, `examples`, y
  sin `node.exe` salvo con `--with-node`).
- **Variable de entorno en la instalación**: `install.bat` define
  `COMPASS_WHATSAPP_ENGINE` (ámbito Usuario) apuntando al corekit instalado
  (`%LOCALAPPDATA%\Programs\Keimer\corekit\whatsapp`), para que los proyectos
  que consumen el motor (p.ej. GymPro en modo desarrollo) lo reutilicen en vivo
  sin copiarlo dentro del proyecto. `uninstall.bat` elimina esa variable.
  El modo desarrollo de Compass (`windows-dev` → `wails3 task dev`) no hace
  staging: la ventana nueva hereda el entorno de Compass, así que basta con
  reiniciar Compass tras la instalación para que el proyecto vea el engine.

## Guía de integración del WhatsApp Engine (para agentes de IA)

Cómo hacer que un proyecto consumidor (p.ej. **GymPro**) use el motor de
WhatsApp canónico de Compass, de forma **dinámica y robusta a cambios de ruta**
(mover Compass o el proyecto no debe romper la integración). La referencia del
protocolo stdin/stdout está en `corekit/whatsapp/AGENTS.md`.

### Modelo mental
- El motor **canónico** vive en `corekit/whatsapp/` dentro de Compass Go: única
  fuente de verdad (`engine.js`, `bundle.mjs`, `node_modules/`). El repo no
  guarda `node.exe`: el runtime usa Node 20+ del sistema.
- Cada proyecto consumidor recibe **su propia copia** del bundle generado
  (`whatsapp-engine-bundle/`) en su carpeta de recursos. **Nunca** se edita a
  mano el bundle dentro del proyecto: se regenera desde el canónico.
- El motor corre como proceso hijo `node` y se comunica por stdin/stdout.

### Resolución dinámica de la ruta del motor (sin hardcodear disco)
`whatsappEngineSource()` en `internal/tasks/whatsapp_engine.go` prueba en orden:
1. env `COMPASS_WHATSAPP_ENGINE` (override explícito);
2. `<dir del exe>/corekit/whatsapp`;
3. `<cwd>/corekit/whatsapp`.

Ninguna capa usa una letra de unidad fija, por tanto:
- Si **instalas** Compass, `install.bat` fija `COMPASS_WHATSAPP_ENGINE` a
  `%LOCALAPPDATA%\Programs\Keimer\corekit\whatsapp` (ruta **relativa a la
  máquina**: sobrevive a mover el repo).
- Si trabajas **solo desde el repo** (`go run` / `wails dev`), la variable puede
  estar ausente y la resolución cae a `<cwd>/corekit/whatsapp` (encuentra el
  motor dentro del repo).
- **Nunca** apuntes la env var a una ruta absoluta del repo
  (`D:\...\Compass Go\corekit\whatsapp`): se rompería al volver a mover la
  carpeta. Usa siempre la ruta de instalación relativa o deja que Compass la
  resuelva por cwd/exe.

### Detección automática en el proyecto consumidor
Un proyecto recibe el runtime si cumple **cualquiera**:
1. tiene carpeta `whatsapp_engine/` local (motor propio);
2. tiene el marcador `use-whatsapp-engine` en su raíz;
3. **escaneo automático**: sus `.go` importan un paquete cuya ruta contiene
   `whatsapp` (p.ej. `internal/whatsapp`). Es el caso normal de GymPro.

Para integrar un proyecto nuevo basta con que importe el paquete `whatsapp`;
Compass etapará el motor solo. No hace falta marcador manual.

### Pasos operativos (integrar / actualizar)
1. **Editar el motor canónico** solo en `corekit/whatsapp/engine.js` (y
   `bundle.mjs` si cambia el empaquetado). No tocar los bundles generados.
2. **Regenerar el bundle** (lo hace Compass al ejecutar la tarea `whatsapp`, o
   manual):
   ```bat
   cd corekit\whatsapp
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
   Tras mover Compass, re-ejecuta `install.bat` para re-fijar la variable.

### Por framework (dónde va el bundle)
| Framework | Destino del bundle | Resolución en runtime |
|---|---|---|
| Go + Wails v3 | `resources/whatsapp-engine-bundle/` | `internal/whatsapp` desde el cwd de la app |
| Flutter | `whatsapp_engine/index.js` (modo single) | cwd → exe dir → app docs |
| Kotlin | carpeta de recursos del framework | según launcher |

### Ejemplo concreto: GymPro (Go + Wails v3)

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

### Errores que un agente NO debe cometer
- ❌ Hardcodear `D:\...` en código o en la env var. Usa rutas relativas /
  `%LOCALAPPDATA%`.
- ❌ Editar el bundle dentro de `resources/whatsapp-engine-bundle/` del
  proyecto. Regenera desde el canónico.
- ❌ Asumir que en producción el engine se lee desde la máquina de dev. Cada
  instalador debe embeber su propia copia del bundle; el runtime exige Node
  20+ (sidecar solo si se compila con `--with-node`).
- ❌ Dejar `COMPASS_WHATSAPP_ENGINE` apuntando a una instalación vieja tras
  mover Compass. Re-ejecuta `install.bat`.

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
`go run ./cmd/compass-web --dev`. No se abre una segunda ventana: la salida de
Vite se escribe en `%LOCALAPPDATA%\compass\vite-dev.log` para no mezclarse con
la de `go run`, y al cerrar la consola Vite muere con ella (sin huérfanos). El
flag `--dev` exporta
`FRONTEND_DEVSERVER_URL=http://127.0.0.1:5173`; con ese valor Wails v3 sirve la
UI desde Vite en lugar del bundle embebido (rama `!production` de
`internal/assetserver`), de modo que los cambios de Svelte/CSS/TS se aplican en
caliente. **Solo hay HMR del frontend**: los cambios de Go requieren cerrar y
volver a ejecutar `dev.bat`.

```bat
dev.bat              # Vite (HMR de UI) + go run --dev
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
- **Terminal embebida**: `go-pty` (ConPTY) + `vt10x` (parser VT) en Go, y
  xterm.js en el frontend. La salida se reparte con `container.SpawnTap` entre
  el parser y el evento `pty:data` (nunca dos lectores sobre la misma ConPTY).
  **vt10x no tiene scrollback**; el scrollback lo aporta xterm.js.
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
