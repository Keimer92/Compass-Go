# Compass Go

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go)](https://go.dev/)
[![Wails](https://img.shields.io/badge/Wails-v3-orange)](https://wails.io/)
[![Svelte](https://img.shields.io/badge/Svelte-5-FF3E00?logo=svelte)](https://svelte.dev/)
[![Platform](https://img.shields.io/badge/platform-Windows-0078D6?logo=windows)](https://www.microsoft.com/windows)

**Una brújula para tu día a día de desarrollo.** Compass Go es una GUI de
escritorio para Windows que detecta el tipo de proyecto y te da, en una sola
ventana, terminal embebida, control de Git, builds y releases, y los motores
compartidos del ecosistema (**WhatsApp** y **Cloudflare D1**).

Es el port a Go de Compass (originalmente en Rust), construido con
**Wails v3 + Svelte 5** y distribuido como un instalador NSIS autónomo.

---

## Qué incluye

- **Terminal propia, no consolas sueltas.** Cada card abre una pestaña
  **ConPTY (PowerShell)** dentro de la ventana, renderizada con xterm.js, con
  scrollback, splitter redimensionable y pestañas recordadas por proyecto.
- **Menú adaptado al proyecto.** Detecta si es Flutter, Kotlin Multiplatform,
  Go + Wails v3, Go puro, Python o un core kit, y muestra solo las cards
  válidas (por ejemplo, oculta Android si no hay `gradlew`).
- **Panel Git estilo VSCode.** Explorador / Git / Historial en el lateral
  derecho, con estado en vivo vía `fsnotify`, commits con IA (OpenRouter o
  Copilot CLI), push con barra de progreso, detección de remoto caído con
  asistente de re-vinculación y **auto-commit por inactividad**.
- **Builds y releases.** Windows / Android / Flutter (host, portable e
  instalador), aviso sonoro de fin de build y opción de abrir la carpeta de
  salida.
- **Integración con Windows.** Menú contextual, acción "Add gitignore", cascada
  **Vibe Coding** (OpenCode, Cline, Kilo Code, Antigravity), accesos directos,
  anclaje a la barra de tareas y caché de iconos.
- **Dos core kits embebidos**: el motor de WhatsApp y el stack de sincronización
  Cloudflare D1, con un panel propio para administrar este último.
- **Detalles de comodidad.** 20 temas persistentes, mascota brújula animada con
  estados, instancia única por proyecto y recarga HMR sin perder las terminales.

### Stacks soportados

| Stack | Detectado por | Builds |
|-------|---------------|--------|
| Flutter (Dart) | `pubspec.yaml` | host, portable, instalador, Android |
| Kotlin Multiplatform | `gradlew.bat` + `settings.gradle.kts` | APK / AAB, signing |
| Go + Wails v3 | `go.mod` con `wailsapp/wails` + Taskfile / `build/config.yml` | Windows (x64/x86), Android |
| Go puro | `go.mod` | Windows, instalador NSIS |
| Python | `pyproject.toml` con `[project]` o `[tool.poetry]` | tareas y terminal |
| Core kits | `engine.js`+`bundle.mjs`, o `go/syncengine`+`worker/template` | tareas genéricas |

---

## Core Kits

Compass incluye dos motores compartidos dentro del propio repositorio, para que
viajen con el instalador y no dependan de rutas externas. La guía completa para
agentes y desarrolladores está en [`AGENTS.md`](AGENTS.md).

| Kit | Carpeta | Para qué |
|-----|---------|----------|
| **WhatsApp** | [`Kit Whatsapp/`](Kit%20Whatsapp/) | motor Baileys de envíos, compartido por GymPro, ELA Group y proyectos Flutter |
| **D1** | [`kit-d1/`](kit-d1/) | sincronización Cloudflare D1: módulo Go, contrato wire y Worker por proyecto |

### Motor de WhatsApp

- Un único bundle canónico (`engine.js` → `whatsapp-engine-bundle/`), regenerado
  solo cuando cambia el código y etapado automáticamente en cada release de
  Windows. Los proyectos consumidores reciben **su copia**, nunca editan el
  bundle a mano.
- Resolución de ruta sin letras de unidad fijas: `COMPASS_WHATSAPP_ENGINE` →
  directorio del exe → directorio de trabajo.
- Runtime con **Node 20+ del sistema**; el sidecar `node.exe` solo se incluye si
  se compila con `dist.bat --with-node`.
- Integración dinámica: basta con importar un paquete `whatsapp` o añadir el
  marcador `use-whatsapp-engine` en la raíz del proyecto.

### Kit D1

- Vive dentro de Compass como `kit-d1/` (incorporado con `git subtree`), así que
  deja de depender de un checkout externo. El nombre sin espacios es obligatorio
  para que la ruta sirva como módulo Go.
- Expone el módulo Go `github.com/Keimer92/Compass-Go/kit-d1/go/syncengine`, el protocolo y
  su fixture canónico (`protocol/fixtures/wire-session.json`), y el template de
  Worker del que se ensambla cada instancia.
- Se administra desde el botón **Kit D1** del sidebar: instancias, consumidores,
  sesión de Wrangler y acciones (deploy, provisión, claves, typecheck…), con
  whitelist de comandos y confirmación de dos pasos para las acciones peligrosas.
- Los consumidores lo enlazan con un `go.work` local (no versionado), p. ej.
  `replace github.com/Keimer92/Compass-Go/kit-d1/go/syncengine => ../Compass Go UX/kit-d1/go/syncengine`. Las versiones publicadas se tagean en este repo como `kit-d1/go/syncengine/vX.Y.Z`.

---

## Instalación

### Requisitos

| Requisito | Para qué |
|-----------|----------|
| **Windows** | la shell, el registro y ConPTY son específicos de Windows |
| **Go 1.26+** | compilar (declarado en `go.mod`) |
| **Node 20+** | exigido por el motor de WhatsApp (Baileys) y los scripts del Kit D1 |
| **PowerShell** | menú contextual y varias tareas |
| **NSIS** *(para empaquetar)* | generar `CompassGo-Setup.exe` |
| **go-winres** *(opcional)* | embeber icono, versión y manifest en el `.exe` |

### Compilar e instalar en tu máquina

```bat
install.bat              :: compila + instala en silencio
install.bat --force      :: rehace también la cascada Vibe Coding
install.bat --vibe       :: reconfigura Vibe Coding
install.bat --with-node  :: incluye node.exe como sidecar
```

### Generar el instalador para distribuir

```bat
dist.bat                 :: dist\CompassGo-Setup.exe (LZMA, sin node.exe)
dist.bat --fast          :: compresión zlib, instalador mayor pero mucho más rápido
dist.bat --no-installer  :: solo prepara dist\payload
dist.bat --with-node     :: incluye node.exe (para máquinas sin Node)
```

El instalador resultante no requiere Go ni scripts sueltos: instala el binario,
los core kits, el PATH del usuario, el menú contextual y los accesos directos.

Para desinstalar:

```bat
uninstall.bat
```

> Los scripts `.bat` deben ejecutarse desde la raíz del repositorio.

---

## Uso del binario

```bat
compass-go.exe                        :: detecta el proyecto y abre la ventana
compass-go.exe -p "C:\ruta\proyecto"  :: abre sobre una carpeta concreta
compass-go.exe -t windows-dev         :: ejecuta una tarea global sin ventana
compass-go.exe --task context-menu-install
```

| Flag | Alias | Descripción |
|------|-------|-------------|
| `--task` | `-t` | Ejecuta una tarea global directamente |
| `--path` | `-p` | Ruta del proyecto (autodetectada si se omite) |

Dentro de la GUI, `Ctrl+O` abre el modal de opciones y el pie del sidebar da
acceso a **Temas**, **Opciones**, **Ajustes de Git** y **Kit D1**.

---

## Desarrollo

```bat
dev.bat                               :: Vite (HMR) + backend con R = reiniciar, Q = salir
```

A mano:

```bat
cd frontend && npm install && npm run build && cd ..
go build -o dist/payload/compass-go.exe ./cmd/compass-web
go vet ./...
go test ./...
```

Notas importantes:

- El frontend se embebe con `//go:embed`; si no se recompila, el `.exe` sirve el
  bundle anterior. `dist.bat` siempre ejecuta `npm run build` antes de `go build`.
- `dev.bat` levanta Vite en segundo plano y `dev-run.ps1` recompila el backend al
  guardar cambios de Go; los eventos en cola evitan perder guardados durante un
  rebuild.
- Los tests e2e (`internal/container/e2e_test.go`) están tras `COMPASS_E2E=1`
  porque arrancan el binario real.

### Estructura del repositorio

```text
Compass Go UX/
├─ cmd/compass-web/        entrypoint Wails (flags, dispatch --task, IPC)
├─ frontend/               Svelte 5 + Vite + TypeScript (fuente de la UI)
├─ internal/
│  ├─ webui/               puente Go↔JS: Git, terminal, menú, temas, kits
│  ├─ tasks/               ejecutores por plataforma + tareas globales
│  ├─ container/           ConPTY: sesión, scrollback, parser vt10x
│  ├─ menu/                modelo de cards por tipo de proyecto
│  └─ theme/ settings/ sound/ workspace/ ...   servicios transversales
├─ Kit Whatsapp/           motor de WhatsApp canónico + bundles
├─ kit-d1/                 stack D1 (Go + protocolo + Worker) [git subtree]
├─ assets/ packaging/      iconos y plantilla del instalador NSIS
├─ dev.bat / dist.bat / install.bat / uninstall.bat
└─ AGENTS.md               guía técnica completa y convenciones
```

---

## Documentación

- [`AGENTS.md`](AGENTS.md) — única guía técnica: arquitectura, convenciones,
  scripts, protocolo stdin/stdout de WhatsApp, contrato HTTP y manifiesto de D1.
  Secciones útiles: [Core Kits](AGENTS.md#core-kits-motores-canónicos-whatsapp-y-d1),
  [protocolo de WhatsApp](AGENTS.md#protocolo-stdinstout-del-motor) y
  [Kit D1](AGENTS.md#kit-d1).

---

Hecho para Windows, con Go, Wails y Svelte.
