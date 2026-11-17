# WhatsApp Engine — Motor Canónico Centralizado (`agents.md`)

> **Propósito:** Servir como referencia única para integrar el motor de WhatsApp en **cualquier proyecto** (Tauri/Rust, Flutter/Dart, Node, Python...). Un solo `engine.js` = lógica centralizada; cada proyecto empaqueta y lanza su propio bundle.
> **Versión:** 2.0.0
> **Ubicación canónica:** `D:\Mis Proyectos\Go + Wails v3\Compass Go\corekit\whatsapp`
> **Dependencias:** Node.js (lado runtime) · `@whiskeysockets/baileys` · `pino` · `qr-image` · `@vercel/ncc` (build)

---

## 1. Qué es y por qué centralizado

Es un **sidecar de WhatsApp** basado en Baileys que:
- Enlaza un número real de WhatsApp (vía QR o código de vinculación `pair`).
- Envía **mensajes de texto** e **imágenes** (recibos, recordatorios).
- Corre como **proceso hijo** lanzado por la app (Tauri `shell().sidecar("node")` / Flutter `Process.start`).

La lógica vive **una sola vez** aquí. Cada proyecto:
1. Compila su bundle con el modo adecuado (`--asset` para Tauri, `--single` para Flutter).
2. Copia el bundle a su carpeta de recursos.
3. Lo lanza con `node <bundle>` y se comunica por **stdin/stdout**.

**Regla de edición:** todos los cambios al motor y a su protocolo empiezan en
`D:\Mis Proyectos\Go + Wails v3\Compass Go\corekit\whatsapp\engine.js`. Los bundles dentro de GymPro,
Tauri u otros proyectos son artefactos generados: no se editan manualmente.
Para GymPro Go/Wails, compila con `node bundle.mjs --asset` y copia el contenido
completo de `out\whatsapp-engine-bundle\` a
`D:\Mis Proyectos\Go + Wails v3\GymPro\resources\whatsapp-engine-bundle\`.

---

## 2. Estructura del directorio

```
D:\Mis Proyectos\Go + Wails v3\Compass Go\corekit\whatsapp\
├─ engine.js           ← MOTOR CANÓNICO (merge de GymPro + School Tracker)
├─ bundle.mjs          ← Compilador (modos --asset / --single)
├─ package.json        ← Dependencias (baileys, pino, qr-image, ncc)
├─ package-lock.json   ← Lock
├─ out/
│  ├─ whatsapp-engine-bundle/   ← bundle modo ASSET (para Tauri) [carpeta con *.js]
│  └─ single/index.js           ← bundle modo SINGLE (para Flutter) [un solo archivo]
├─ examples/
│  ├─ tauri_integration.rs      ← Snippet integración Rust
│  └─ flutter_integration.dart  ← Snippet integración Dart
└─ AGENTS.md           ← (este documento)
```

---

## 3. Consolidación del motor (merge de las 3 versiones)

| Característica | Fuente | Descripción |
|---|---|---|
| `emit()` con `fs.writeSync` | ST/Flutter | Flush real de stdout (crítico en pipes de Windows) |
| Watchdog de conexión (`connectTimeoutMs`) | ST/Flutter | Reinicia si el QR expira / no conecta |
| `qr-image` → `QR_DATA_URL:` | ST/Flutter | QR como `data:image/png;base64` listo para UI |
| Soporte `pair` (código de vinculación) | ST/Flutter | Vincular número en vez de escanear QR |
| `WHATSAPP_AUTH_DIR` por env | ST/Flutter | Sesión por proyecto (evita "logged in elsewhere") |
| **Rate-limit** (cola serial + ventana) | **GymPro** | Anti-burst: 1 envío a la vez, delay + tope/ventana |
| **`isSocketOpen`** flag real de conexión | **GymPro** | No envía si no hay socket abierto; bufferiza |
| **`403` como sesión inválida** | **GymPro** | Cuenta baneada/suspendida → limpia sesión + QR nuevo |
| `401` / `loggedOut` limpieza | todons | Sesión expirada → limpia y reinicia |
| Mensaje **`CONFIG`** por stdin | **nuevo** | Config por app (browser, authDir, rate, timeout) sin recompilar |

El motor resultante es un **superset**: los tres proyectos ganan funcionalidad (GymPro gana QR-data-url/watchdog/pair; ST/Flutter ganan rate-limit y manejo de 403).

---

## 4. Protocolo de comunicación

### 4.1 Entrada (app → motor, por `stdin`, **una línea JSON**)

```json
{ "type": "config", "browser": ["GymPro","Chrome","121.0.6167.160"], "authDir": "whatsapp_auth",
  "connectTimeoutMs": 60000,
  "rate": { "sendDelayMs": 5000, "maxWindowSends": 20, "windowMs": 60000 } }
{ "type": "send", "number": "50588888888", "text": "Hola" }
{ "type": "send_image", "number": "50588888888", "imagePath": "C:/tmp/recibo.png", "caption": "Su recibo" }
{ "type": "pair", "phoneNumber": "50588888888" }
```

- `config` es **opcional** y puede enviarse **en cualquier momento** (antes o después de conectar). Todos los campos son sobrescribibles; los que no se envíen mantienen su default.
- `send` / `send_image` pasan por la **cola serial** con rate-limit (ver §6).

### 4.2 Salida (motor → host, por `stdout` con prefijos)

| Prefijo | Significado |
|---|---|
| `STATUS:init` | Motor cargado |
| `STATUS:starting` | Iniciando |
| `STATUS:auth_ready` | Sesión/estado listo |
| `STATUS:socket_creating` | Create socket |
| `STATUS:connecting_socket` | Conectando con servidores |
| `STATUS:restarting` | Reintentando |
| `STATUS:connected:<clientName>\|\|<clientId>\|\|<profilePicUrl>` | Conectado + info cliente |
| `STATUS:disconnected:<reason>` | Desconectado (motivo) |
| `STATUS:error:<msg>` | Error crítico |
| `STATUS:session_invalid` | Sesión inválida (401/loggedOut/403) |
| `STATUS:session_cleared` | Sesión limpiada |
| `QR_DATA_URL:data:image/png;base64,...` | QR listo (data URL) |
| `SENDING:<number>` | Enviando texto |
| `SENT_CONFIRM:<number>` | Texto enviado |
| `SENT_RETRY_OK:<number>` | Texto enviado (tras reintento) |
| `SEND_ERROR:<msg>` / `SEND_RETRY_FAILED:<msg>` | Fallo envío |
| `IMAGE_SENT_CONFIRM:<number>` | Imagen enviada |
| `IMAGE_ERROR:<msg>` | Fallo imagen |
| `PAIR_CODE:<code>` | Código de vinculación listo |
| `PAIR_ERROR:<msg>` | Fallo vinculación |
| `CONFIG_RECEIVED:ok` | Config aceptada |
| `ENGINE_ERROR:<msg>` | Error interno del motor |
| *(stderr)* | Logs de depuración (`WA_DEBUG_*`, `WA_ENGINE_UNCAUGHT_*`) |

**Nota importante:** `STATUS:connected` usa el separador `||` triple en la info del cliente: `nombre||id||profilePic`.

### 4.3 Ojo con buffers de stdout
En Windows, cuando el stdout del hijo es un **pipe** (no terminal), `console.log`/`process.stdout.write` pueden ir bufferizados y llegar tarde. Por eso el motor usa `fs.writeSync(1, ...)` vía `emit()`. **No cambiar** eso.

---

## 5. Configuración por app (mensaje `CONFIG` + env)

Cada proyecto puede personalizarse **sin recompilar el bundle**. Precedencia: **mensaje `CONFIG` > env `WHATSAPP_AUTH_DIR` > defaults**.

| Parámetro | Default | Descripción |
|---|---|---|
| `browser` | `['WhatsApp Engine','Chrome','121.0.6167.160']` | Cadena de navegador que ve WhatsApp |
| `authDir` | env `WHATSAPP_AUTH_DIR` \|\| `whatsapp_auth` | Carpeta de la sesión |
| `connectTimeoutMs` | `60000` | Watchdog del QR/conexión |
| `rate.sendDelayMs` | `5000` | Intervalo mínimo entre envíos (ms) |
| `rate.maxWindowSends` | `20` | Máx de envíos por ventana |
| `rate.windowMs` | `60000` | Ventana de tiempo (ms) |

**Recomendación de sesión por proyecto:** define un `authDir` distinto en **cada** app (o el env `WHATSAPP_AUTH_DIR`). Esto evita "logged in from another device" entre tus propias apps.

Ejemplo de bootstrap en Rust (mando CONFIG tras arrancar el proceso):
```rust
child.write(b"{\"type\":\"config\",\"browser\":[\"GymPro\",\"Chrome\",\"121.0.6167.160\"],\"authDir\":\"whatsapp_auth\",\"rate\":{\"sendDelayMs\":5000}}\n")
```

---

## 6. Rate limiting anti-ban

Fue el mero que originó la centralización (cuenta baneada tras ráfaga de 12 recibos al reconectar). El motor implementa:

1. **Cola serial (`sendChain`, promise-chain):** los envíos se procesan **uno a la vez**, nunca en paralelo. El problema original era `rl.on('line')` corriendo handlers `async` que disparaban a la vez al reconectar.
2. **`SEND_DELAY_MS`:** espera tras **cada** envío antes del siguiente (default `5000 ms`).
3. **Ventana deslizante (`maxWindowSends`/`windowMs`):** máx `20` envíos por `60 s`. Si se excede, los siguientes **se encolan** (no se descartan) esperando cupo.
4. **Flag `isSocketOpen`:** solo envía con socket realmente `open`; sin conexión los mensajes quedan bufferizados (máx 30 s) en lugar de agruparse/fallar.

**Cómo esto lo protege:** al reconectar N recibos se envían SECUENCCIAL, espaciados 5 s, sin nunca pasar de 20/min → sin ráfaga → menor riesgo de ban.

---

## 7. Manejo de sesión y bloqueos

| Código | Significado | Acción del motor |
|---|---|---|
| `401` / `loggedOut` | Sesión expirada/cerrada | Limpia `authDir` + pide QR nuevo |
| **`403`** | Cuenta **baneada/suspendida** | Igual que logout: limpia sesión + QR nuevo |
| `Connection Failure` (sockets) | **IP marcado por WhatsApp** (no la cuenta) | Reintenta (máx 5). **No es fix de código**; cambiar de red/IP |

**Bloqueo por IP (`403` a nivel de socket antes de autenticar):** suele ser IP marcado por WhatsApp (por uso de spam). Soluciones: cambiar de red (hotspot/VPN), renovar IP pública del router, esperar 24-72 h. **Evitar** regenerar QR en bucle mientras la IP esté marcada (cada intento resurrena el contador).

---

## 8. Build y empaquetado

```bash
cd "D:\Mis Proyectos\Go + Wails v3\Compass Go\corekit\whatsapp"
npm install                     # solo la 1er vez
node bundle.mjs --asset         # → out/whatsapp-engine-bundle/   (Tauri: carpeta con assets)
node bundle.mjs --single        # → out/single/index.js           (Flutter: un solo archivo)
```

- **`--asset` / default:** `assetBuilds:true` → ncc separa los assets de media en varios `*.js`. Indispensable copiar **toda la carpeta** (`index.js` + `file*.js` + `worker*.js` + `*-index.js`). Usado por Tauri (`BaseDirectory::Resource`).
- **`--single`:** `assetBuilds:false` → **un solo** `index.js` autocontenido. Usado por Flutter (más rígido para empaquetar).

> Si cambias la **letra de la unidad** de la partición: los proyectos **en runtime** no dependen de `D:\` (resuelven el bundle relativo a la app instalada / ejecutable). Sólo debes **re-copiar** el bundle (tu herramienta de deploy) y re-apuntar rutas absolutas en tu config.

### 8.1 Copiar bundle por tipo de proyecto
- **Tauri:** copiar la **carpeta** `out/whatsapp-engine-bundle/` → `src-tauri/resources/whatsapp-engine-bundle/`
- **Flutter:** copiar `out/single/index.js` → `whatsapp_engine/index.js`
- **GymPro Go/Wails:** copiar la **carpeta completa** `out/whatsapp-engine-bundle/` → `resources/whatsapp-engine-bundle/`

> ⚠️ La herramienta de despliegue/copiado a los proyectos se gestiona **externa EMENTE** (fuera de este repo). Este directorio sólo mantiene `engine.js`, `bundle.mjs`, `examples/` y doc.

---

## 8.2 Distribución en producción (IMPORTANTE)

> **Decisión de arquitectura:** las apps que usan este motor **se distribuyen a otras máquinas en producción**. Por lo tanto, **NO se puede apuntar al directorio central en runtime** (ese path solo existe en la máquina de desarrollo). Cada proyecto debe llevar **su propia copia del bundle embebida** dentro del instalador/ejecutable.

### 8.2.1 Flujo de actualización por proyecto

1. Editar/mejorar `engine.js` **solo** en el repositorio central.
2. Compilar el/los bundle(s): `node bundle.mjs --asset` (Tauri) y/o `node bundle.mjs --single` (Flutter).
3. Copiar el bundle resultante a **cada proyecto** (con tu herramienta de deploy).
4. Recompilar/reempaquetar cada app (el bundle viaja embebido en el instalador).
5. Distribuir el nuevo instalador.

### 8.2.2 Ubicación del bundle por framework

En producción, cada proyecto necesita solo el bundle generado en la ruta que espera
su launcher. `engine.js`, `bundle.mjs` y `node_modules` permanecen en este repositorio
central y no se copian al proyecto consumidor.

**Aclaraciones por framework:**

| Framework | Dónde va el bundle | Ruta exacta que espera el launcher |
|---|---|---|
| **Tauri (Rust)** | `src-tauri/resources/whatsapp-engine-bundle/` | Resuelto con `BaseDirectory::Resource` en runtime (empaquetado en el instalador) |
| **Tauri (copia fuente de dev)** | `whatsapp_engine/` en la raíz del proyecto | — |
| **Flutter (Dart)** | `whatsapp_engine/` en la raíz del proyecto | `Process.start` con `WHATSAPP_AUTH_DIR`; busca `index.js` relativo a cwd → executable dir → app docs |
| **GymPro Go/Wails** | `resources/whatsapp-engine-bundle/` | Resuelto por `internal/whatsapp` desde el directorio de trabajo de la app |

> Para GymPro, la fuente canónica siempre es `D:\Mis Proyectos\Go + Wails v3\Compass Go\corekit\whatsapp`.
> Regenera el bundle desde allí y sincroniza la carpeta completa; nunca edites a mano
> el bundle generado dentro de `resources\whatsapp-engine-bundle`.

**Ejemplo concreto (GymPro y School Tracker):**
```
D:\Mis Proyectos\Go + Wails v3\GymPro\                 ← Go/Wails
└─ resources\
   └─ whatsapp-engine-bundle\                         ← bundle asset copiado del central

D:\Mis Proyectos\Tauri\GymPro\
└─ src-tauri\
   └─ resources\
      └─ whatsapp-engine-bundle\   ← (bundle modo ASSET, copiado del central)

D:\Mis Proyectos\Flutter\School Tracker Pro ELA\
└─ whatsapp_engine\                ← (carpeta requerida en la raíz)
   ├─ engine.js, bundle.mjs, package.json, node_modules
   └─ index.js                     ← (bundle modo SINGLE, copiado del central)
```

### 8.2.3 Nota sobre Tauri (`resources` / `externalBin`)

Para que el bundle se incluya en el instalador de una app **Tauri**, el `tauri.conf.json` debe declarar:

```json
{
  "bundle": {
    "resources": ["resources/whatsapp-engine-bundle/*"],
    "externalBin": ["bin/node"]
  }
}
```

- `resources` → incluye el bundle dentro de la app instalada.
- `externalBin` → empaqueta `node.exe` (sidecar), necesario para que la máquina del cliente pueda ejecutar el motor sin tener Node instalado.

> ⚠️ **GymPro actualmente NO declara `resources` ni `externalBin`** en su `tauri.conf.json` (a diferencia de School Tracker). Sin esto, el bundle **no se incluiría** en el instalador de GymPro. Antes de distribuir GymPro, hay que añadir ambas entradas.

---

## 9. Integración por framework

### 9.1 Tauri (Rust) — ver `examples/tauri_integration.rs`

Resumen del patrón:
1. Resolver bundle con `BaseDirectory::Resource` (NO con path del proyecto).
2. `app.shell().sidecar("node").args([bundle]).current_dir(app_data_dir).spawn()`.
3. Escuchar `CommandEvent::Stdout` para parsear eventos (`QR_DATA_URL:`, `STATUS:`, etc.).
4. Enviar órdenes con `child.write("{...}\n")`.
5. La sesión se guarda en `app_data_dir/whatsapp_auth` (relativo al working dir).

Registra los comandos en `lib.rs`: `start_whatsapp`, `logout_whatsapp`, `send_whatsapp_direct`, `send_whatsapp_image`.

### 9.2 Flutter (Dart) — Ver `examples/flutter_integration.dart`

1. Localiza el bundle (cwd → executable dir → app docs).
2. `Process.start(nodeBin, [engineJs], environment: {'WHATSAPP_AUTH_DIR': authDir})`.
3. Parsear stdout por líneas (`LineSplitter`) para eventos.
4. Escribir órdenes por `process.stdin.writeln(jsonEncode(...))`.

---

## 10. Versión / Historial de cambios

| Versión | Cambio |
|---|---|
| 2.0.0 | Consolidación: merge GymPro (rate-limit, `isSocketOpen`, 403) + ST/Flutter (emit, watchdog, qr-image, pair). Soporte mensaje `CONFIG`. `bundle.mjs` con modos `--asset`/`--single`. |
| 2.0.1 | **Seguridad:** Baileys `6.7.9` → `6.7.24` (resolve `GHSA-qvv5-jq5g-4cgg` y CVEs; `npm audit` 0 vulnerabilidades). |
| 2.0.2 | **Docs:** sección §8.2 "Distribución en producción" — cada proyecto copia su bundle embebido, carpeta `whatsapp_engine` obligatoria en la raíz, y notas `resources`/`externalBin` para Tauri. |
| (heredado) | Varias versiones dispersas en GymPro / School Tracker Pro ELA (Tauri y Flutter). |

---

## 11. Notas de seguridad

- **Baileys pinneado a `6.7.24`** (rama legacy segura). Resuelve el advisory crítico `GHSA-qvv5-jq5g-4cgg` (spoofing de mensajes / corrupción de app state) y otros CVEs. `npm audit` reporta **0 vulnerabilidades**.
  - Se eligió `6.7.24` (tag `legacy`) en lugar de `7.0.0-rc14` (latest) porque `6.x` es **API-compatible** con el `engine.js` actual y evita un migration test de major.
  - Para actualizar en el futuro: `node bundle.mjs --asset` y `--single`, luego re-copiar los bundles.
- `NODE_TLS_REJECT_UNAUTHORIZED=0` se usa para evadir inspección SSL de antivirus; es necesario en entornos Windows corporativos, pero reduce seguridad TLS de terceros.
- **Nunca commitar** `whatsapp_auth` (sesión) ni `authDir`.