# Compass Go — Guía para agentes y desarrolladores

## ¿Qué es?

`Compass Go` es el **port a Go** de *Compass* (originalmente en Rust): una
**TUI para Windows** que automatiza tareas de desarrollo, compilación y
configuración para proyectos **Flutter (Dart)**, **Kotlin Multiplatform
(Compose)**, **Go + Wails v3**, **Go puro** y **Python**. Detecta el tipo de
proyecto y adapta comandos, rutas y menús.

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
| **NSIS** (necesario para `install.bat` / `dist.bat`) | empaquetar el instalador `CompassGo-Setup.exe` | https://nsis.sourceforge.io/ |

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
│  ├─ settings/               # opciones globales persistidas (settings.json)
│  ├─ sound/                  # aviso sonoro de fin de build (Beep de kernel32)
│  ├─ project/                # detección de tipo de proyecto
│  ├─ shell/                  # envoltorio PowerShell / cmd
│  ├─ lock/                   # instancia única por proyecto (LockFileEx, Windows)
│  ├─ window/                 # trae al frente la ventana de otra instancia abierta
│  ├─ console/                # Ctrl-C handler y bloqueo de cierre (Windows)
│  ├─ prompts/  ui/           # prompts y helpers de UI
├─ assets/icon/               # Compass Go.png, Vibe Coding.png, Git.png (+ .ico generados), Default.ico
├─ packaging/                 # plantilla del instalador NSIS + generador de cabecera
│  ├─ installer.nsi           #   asistente MUI2 (icono, componentes, ejecutar al final)
│  ├─ make_icons.ps1          #   convierte un .png en .ico multi-tamano (16..256)
│  └─ make_header.ps1         #   cabecera 150x57 BMP a partir de assets/icon/CompassGo.ico
├─ dist/                      # GENERADO por dist.bat (payload, installer.nsi, setup); desechable
├─ install.bat                # dist.bat + instalación silenciosa con el Setup
├─ dist.bat                   # compilar + empaquetar el instalador en dist/
├─ uninstall.bat              # desinstalar de la máquina (usa Uninstall.exe si existe)
├─ go-winres.json             # metadatos del .exe (nombre "Compass Go")
├─ corekit/                  # core kits canónicos que viajan dentro de Compass (p.ej. whatsapp)
│  └─ whatsapp/               #   motor de WhatsApp: engine.js, bundle.mjs, node.exe, node_modules
└─ PLAN.md                    # plan de migración original
```

Módulo Go: `compass` (imports `compass/internal/...`).

## Scripts `.bat` (Windows) — uso

Hay **tres scripts**, separados por velocidad de uso: `install.bat` compila y
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
install.bat              # compilar + instalar
install.bat --force      # rehace también la cascada Vibe Coding
install.bat --vibe       # reconfigura Vibe Coding (detecta CLIs que falten)
```

- Cierra `compass-go.exe` en ejecución antes de sobrescribir (Windows no
  deja sobreescribir un `.exe` en uso).
- Borra el `dist\CompassGo-Setup.exe` anterior antes de compilar: si el
  empaquetado falla, no se instala un Setup viejo.
- **Requiere NSIS** (si falta, `install.bat` aborta y sugiere
  `winget install NSIS.NSIS`).
- En modo silencioso no se ejecuta la app al terminar y el anclaje a la barra
  de tareas suele quedar bloqueado por Windows; ancla a mano desde el menú
  Inicio.
- El menú contextual queda en `HKCU\...\shell\zz_CompassGo` y
  `zz_VibeGitignore` (entrada "Add gitignore"); Vibe Coding en
  `zz_VibeCoding`.
- Requiere ejecutarlo **desde la raíz del proyecto** (necesita `go.mod`).

### 2. `dist.bat` — generar el instalador en `dist\` (tipo: *package*)

Compila a `dist\payload\compass-go.exe` (con iconos), copia la plantilla
`packaging\installer.nsi` a `dist\installer.nsi` (generando antes la cabecera
del asistente desde el icono de la app) y, si NSIS está instalado, produce el
instalador autónomo `dist\CompassGo-Setup.exe`. Todo lo generado vive en
`dist\` y es desechable.

Al copiar `corekit/` al payload se excluyen los artefactos de build del motor
(`node_modules`, `out`, `examples`): en ejecución solo hacen falta `node.exe` +
`whatsapp-engine-bundle`. El payload pasa de ~165 MB / 2418 archivos a ~83 MB /
35 archivos, y la instalación silenciosa de ~15 s a ~5 s. El instalador además
borra esos artefactos de instalaciones anteriores antes de extraer.

El asistente usa Modern UI 2 con la marca de Compass Go (icono y cabecera,
como el instalador de Firefox) y ofrece, además de la instalación base, acceso
directo en el menú Inicio, acceso en el escritorio, anclaje a la barra de
tareas (best-effort: Windows 10 1903+ puede bloquearlo y se avisa al usuario)
y la casilla **Ejecutar Compass Go** al terminar.

```bat
dist.bat                 # compila + genera dist\CompassGo-Setup.exe (LZMA)
dist.bat --fast          # compresión zlib: instalador mayor pero mucho más rápido
dist.bat --no-installer  # solo prepara dist\payload (sin NSIS)
```

- `--fast` (zlib) tarda ~40 s y produce ~57 MB; sin él, LZMA sólido tarda
  ~3 min y produce ~38 MB. `install.bat` usa siempre `--fast`; para distribuir,
  compila sin el flag.
- El anclaje a la barra de tareas se intenta con
  `Shell.Application.InvokeVerb` desde PowerShell, no con `ExecShell`: si
  Windows no expone el verbo, `ExecShell` cae en el verbo `open` y abriría la
  app durante la instalación.

- Si falta NSIS, avisa y deja `dist\payload` e `installer.nsi` listos (no es
  un error para `dist.bat`; `install.bat` sí aborta en ese caso).
- Si `makensis` falla, `dist.bat` termina con código 1.
- Para recompilar el instalador a mano:
  `makensis /DAPPICON=payload\CompassGo.ico dist\installer.nsi` (añade
  `/DNOHEADER` si no existe `dist\installer_header.bmp`).
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
  `windows-build`, …) suena una triada ascendente justo antes de la cuenta
  atrás que cierra la pestaña; si el build falla suenan dos tonos graves.
  Se emite desde `shell.PauseFor` (éxito) y `shell.RunBuildAndRevealWithEnv` /
  `runReleaseMenu` (fallo), usando `Beep` de `kernel32` (`internal/sound`), sin
  WAV ni PowerShell. No se puede desactivar.
- **Abrir carpeta de salida (opcional)**: en el menú principal, `Ctrl+O` abre
  el modal **Opciones** con la casilla "Abrir la carpeta de salida al terminar
  un build". Se guarda al instante en
  `%LOCALAPPDATA%\compass\settings.json` (`internal/settings`) y afecta a
  `shell.ShowArtifact`, `revealArtifact`, `finishRelease` y al build de
  escritorio de Wails. Desactivada, el artefacto se lista pero el Explorer no
  se abre.

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
- **Detección de uso (opt-in + automática)** (`projectUsesWhatsappEngine` /
  `projectImportsWhatsapp` en `internal/tasks/whatsapp_engine.go`): un proyecto
  recibe el runtime si se cumple **cualquiera** de estos criterios:
  1. tiene una carpeta `whatsapp_engine` local (motor propio del proyecto);
  2. tiene el marcador `use-whatsapp-engine` en su raíz (opt-in explícito);
  3. **escaneo automático**: sus fuentes Go importan un paquete cuya ruta
     referencia `whatsapp` (p.ej. `miproyecto/internal/whatsapp`). El escaneo
     recorre recursivamente los `.go` y omite `node_modules`, `vendor`, `.git`,
     `bin`, `resources`, `out` y `corekit` (para no rallar ni falsar positivos
     dentro del propio Compass); para en cuanto encuentra una coincidencia.
  El criterio 3 es el principal: basta con que el código del proyecto importe el
  paquete WhatsApp para que Compass etaje el motor, sin marcadores manuales.
- **Auto-instalación de dependencias**: solo cuando hay que reconstruir el
  bundle (falta `index.js` o `engine.js` es más nuevo), y si `package.json` es
  más reciente que `node_modules`, Compass ejecuta `npm install` en el kit. La
  copia instalada no lleva `node_modules` (el bundle ya viene compilado); si
  faltara y hubiera que reconstruir, se avisa y se usa el bundle existente.
- **Etapado en build**: `stageWhatsappEngine` (llamado desde `windowsArtifacts`
  en el release de escritorio) reconstruye el bundle si `engine.js` cambió y
  copia `node.exe` + `whatsapp-engine-bundle` al `bin/resources/` del proyecto
  (o al `resources/` de Flutter/Kotlin según tipo).
- **Tarea `whatsapp`**: compila el motor canónico (`node bundle.mjs`) y lo etaja.
- `install.bat` / `dist.bat` copian `corekit/` junto al binario/instalador
  (sin los artefactos de build del motor: `node_modules`, `out`, `examples`).
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
  fuente de verdad (`engine.js`, `bundle.mjs`, `node.exe`, `node_modules/`).
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
3. **Etapar al proyecto** con la tarea de Compass:
   ```bat
   compass-go.exe -p "RUTA_DEL_PROYECTO" -t whatsapp
   ```
   Copia `node.exe` + `whatsapp-engine-bundle/` a `resources/whatsapp-engine-bundle/`
   del proyecto (Go/Wails), o a la ruta de recursos según el tipo.
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

// resolveEngine localiza node.exe y el bundle generado.
func resolveEngine() (node, bundle string) {
	node = findFirst("node.exe", "resources/node.exe", "bin/node.exe",
		"resources/node-x86_64-pc-windows-msvc.exe")
	bundle = findFirst("whatsapp-engine-bundle/index.js",
		"resources/whatsapp-engine-bundle/index.js")
	return
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
	node, bundle := resolveEngine()
	if node == "" || bundle == "" {
		return errors.New("motor no encontrado: define COMPASS_WHATSAPP_ENGINE o eta el bundle en resources/")
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
>   canónico** (`COMPASS_WHATSAPP_ENGINE`) si falta, usando el `node.exe` del
>   propio engine (no el del sistema).
> - `resources/whatsapp-engine-bundle/` de GymPro es la copia etapada por
>   Compass; **no se edita a mano**.

Para (re)etapar el motor en GymPro tras cambiar `engine.js`:

```bat
compass-go.exe -p "D:\Mis Proyectos\Go + Wails v3\GymPro" -t whatsapp
```

### Errores que un agente NO debe cometer
- ❌ Hardcodear `D:\...` en código o en la env var. Usa rutas relativas /
  `%LOCALAPPDATA%`.
- ❌ Editar el bundle dentro de `resources/whatsapp-engine-bundle/` del
  proyecto. Regenera desde el canónico.
- ❌ Asumir que en producción el engine se lee desde la máquina de dev. Cada
  instalador debe embeber su propia copia del bundle y `node.exe` como sidecar.
- ❌ Dejar `COMPASS_WHATSAPP_ENGINE` apuntando a una instalación vieja tras
  mover Compass. Re-ejecuta `install.bat`.

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
- **Plataformas soportadas**: solo **Windows** como aplicación de escritorio;
  Android sigue como destino de compilación para los proyectos compatibles.
