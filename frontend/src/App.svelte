<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { Terminal } from '@xterm/xterm'
  import { FitAddon } from '@xterm/addon-fit'
  import {
    getWorkspace, selectProject, removeProject, getMenu, runTask,
    onPtyData, onPtyExit, openTerminal, writeTerminal, resizeTerminal, taskScript,
    closeTerminal, focusWindow, getDebug, getThemes, getTheme, setTheme, getSettings, setSettings,
    subcards, onWorkspaceChanged, pickProject as pickProjectDialog, listTerminals,
    listCleanupCandidates, purgeCleanupCandidates, getSoundCatalog, previewSound,
    getVibeTools,
  } from './lib/bridge'
  import { notifyRepoChanged } from './lib/repoChanged'
  import { svg } from './lib/icons'
  import Card from './components/Card.svelte'
  import Explorer from './components/Explorer.svelte'
  import Git from './components/Git.svelte'
  import History from './components/History.svelte'
  import System from './components/System.svelte'
  import Mascot from './lib/Mascot.svelte'
  import { loadGitInfo } from './lib/gitCache'
  import { warmExplorer, warmLog } from './lib/panelCache'
  import type { ProjectInfo, MenuItemInfo, WorkspaceInfo, ThemeInfo, Settings, Subcard, TerminalInfo, CleanupTarget, CleanupPurgeResult, SoundCatalog, SoundEventInfo, VibeToolInfo } from './lib/bridge'

  interface Tab {
    key: number
    label: string
    ptyId: string | null
    exited: boolean
    project: string
  }

  let workspace = $state<WorkspaceInfo>({ projects: [], active: 0 })
  let menu = $state<MenuItemInfo[]>([])
  // Numero de la ultima peticion de menu: si dos cambios de proyecto se
  // solapan, solo la respuesta mas reciente puede pintar las cards.
  let menuSeq = 0
  let tabs = $state<Tab[]>([])
  let activeKey = $state<number | null>(null)
  // Mascota: reposo, trabajando (hay una tarea corriendo) y destellos de
  // exito/error al terminar un proceso. El destello manda sobre el resto.
  let moodFlash = $state<'success' | 'error' | null>(null)
  let moodTimer: ReturnType<typeof setTimeout> | undefined
  let running = $derived(tabs.some((t) => !t.exited && !!t.ptyId))
  let mood = $derived<'idle' | 'working' | 'success' | 'error'>(
    moodFlash ?? (running ? 'working' : 'idle')
  )
  function flashMood(next: 'success' | 'error') {
    moodFlash = next
    clearTimeout(moodTimer)
    moodTimer = setTimeout(() => (moodFlash = null), 2600)
  }
  let rightTab = $state<'explorer' | 'git' | 'log'>('explorer')
  // El panel derecho se puede plegar para darle todo el ancho al terminal
  // (diffs anchos de las CLI, tablas, etc.). No se desmonta: solo se oculta.
  let rightCollapsed = $state(false)
  let themeMenu = $state(false)
  let optionsOpen = $state(false)
  let lastError = $state('')
  let themes = $state<ThemeInfo[]>([])
  let theme = $state<ThemeInfo | null>(null)
  let previewName = $state<string | null>(null)
  let settings = $state<Settings>({
    open_output_dir: true,
    close_tab_on_build_end: false,
    openrouter_endpoint: 'https://openrouter.ai/api/v1',
    openrouter_model: '',
    openrouter_api_key: '',
    auto_commit_idle_min: 5,
    commit_provider: 'openrouter',
    copilot_model: '',
    sound_presets: {},
    terminal_panel_pct: 60,
    vibe_disabled: [],
  })
  // Reparto cards/terminal en % del area central. Se cambia arrastrando el
  // separador y se guarda en settings; al no depender del numero de cards, el
  // terminal conserva su alto al cambiar de proyecto (sin SIGWINCH).
  let termPct = $state(60)
  let splitDrag = $state(false)
  // Catalogo de eventos/presets de sonido para la seccion "Sonidos" de Opciones.
  let soundCatalog = $state<SoundCatalog | null>(null)
  // CLIs de Vibe Coding con su interruptor para la seccion "Vibe Coding" de
  // Opciones. La card solo muestra las que quedan habilitadas.
  let vibeTools = $state<VibeToolInfo[]>([])
  // CLIs que se muestran en la barra lateral: habilitadas en Opciones e
  // instaladas (una card que solo mostrara un aviso no aporta nada).
  let vibeSide = $derived(vibeTools.filter((t) => t.found && t.enabled))
  let subMenu = $state<{ title: string; items: Subcard[] } | null>(null)

  // Modal "Elegir que borrar" de la card Clean. cleanupSelected usa el path
  // absoluto devuelto por Go como clave; Go vuelve a validarlo antes de borrar.
  let cleanupOpen = $state(false)
  let cleanupItems = $state<CleanupTarget[]>([])
  let cleanupSelected = $state<Record<string, boolean>>({})
  let cleanupBusy = $state(false)
  let cleanupResult = $state<CleanupPurgeResult | null>(null)

  let seq = 0
  // Etiqueta por carga de pagina: las sesiones restauradas conservan su id
  // antiguo, y las nuevas se crean con este prefijo para no colisionar con
  // ellas aunque `seq` se reinicie.
  const sessionTag = Math.random().toString(36).slice(2, 8)
  const terms = new Map<number, { term: Terminal; fit: FitAddon }>()
  const mounted = new Set<number>()
  // Ultimo tamano enviado a cada ConPTY. Al cambiar de proyecto se evita
  // reenviar el mismo resize: un SIGWINCH innecesario hace que las TUI
  // (Codebuddy, vim...) redibujen y pierdan el scroll o corten el contenido.
  const lastPtySize = new Map<number, string>()
  // Ultima posicion de lectura de cada pestana, medida en lineas desde el
  // final (no como linea absoluta): mientras la pestana esta oculta su PTY
  // sigue escribiendo y `baseY` crece, asi que una linea absoluta apuntaria a
  // otro contenido y la consola "saltaria" al volver al proyecto.
  const scrollMemo = new Map<number, { fromBottom: number; bottom: boolean }>()
  // Ultima pestana activa de cada proyecto. Al volver a un proyecto se
  // reabre esa (y no siempre la primera), que es lo que el usuario dejo
  // seleccionado antes de cambiar.
  const lastTabByProject = new Map<string, number>()
  // Mientras se activa una pestana se ignoran los `onScroll`: son producto del
  // re-sync de xterm, no del usuario.
  let activating = false
  // Contenedor del terminal: un ResizeObserver reajusta xterm cuando el panel
  // cambia de alto/ancho (al cambiar de proyecto cambian las cards y el alto
  // disponible, y la ultima fila -el input del chat- quedaba cortada abajo).
  let termStageEl: HTMLElement | null = null
  let fitRaf = 0
  // El viewport de cada xterm cambia de ancho util cuando aparece o desaparece
  // su barra de scroll, sin que cambie el tamano del stage. Observarlo evita
  // que la ultima columna quede tapada al crecer el scrollback.
  const vpRo = new ResizeObserver(() => scheduleFit())

  const activeTab = () => tabs.find((t) => t.key === activeKey) ?? null

  // Solo se muestran las pestanas del proyecto activo; las de los demas
  // siguen vivas en `tabs` (y sus xterm montados) pero ocultas.
  let projectPath = $derived(workspace.projects[workspace.active]?.path ?? '')
  let visibleTabs = $derived(tabs.filter((t) => t.project === projectPath))

  // Precalienta el estado de git del proyecto activo: cuando el usuario abre
  // la pestaña Git ya hay datos cacheados y no ve el estado de "cargando".
  $effect(() => {
    const p = projectPath
    if (!p) return
    loadGitInfo(p).catch(() => {})
    warmExplorer(p).catch(() => {})
    warmLog(p).catch(() => {})
  })
  // Cards de Config (Terminal, Entorno) que antes vivian en la pestaña CFG y
  // ahora se lanzan desde la sección "Herramientas" del modal de Opciones.
  // Git ya no es una card: todo vive en la pestaña Git del panel derecho.
  let cfgTools = $derived(menu.filter((m) => m.section === 'CFG'))

  // applyTheme solo pinta (CSS vars + terminales); no toca el tema guardado.
  // Asi se puede llamar en hover para previsualizar sin persistir nada.
  function applyTheme(t: ThemeInfo) {
    const root = document.documentElement.style
    root.setProperty('--bg', t.bg)
    root.setProperty('--panel', t.panel)
    root.setProperty('--sidebar', t.sidebar)
    root.setProperty('--text', t.text)
    root.setProperty('--muted', t.muted)
    root.setProperty('--border', t.border)
    root.setProperty('--brand', t.brand)
    root.setProperty('--accent', t.accent)
    for (const entry of terms.values()) {
      entry.term.options.theme = { background: t.panel, foreground: t.text }
    }
  }

  // Tema que se esta mostrando: el previsualizado si hay hover, si no el guardado.
  const shownTheme = () => (previewName ? themes.find((t) => t.name === previewName) ?? theme : theme)

  function hoverTheme(t: ThemeInfo) {
    previewName = t.name
    applyTheme(t)
  }

  function endPreview() {
    previewName = null
    if (theme) applyTheme(theme)
  }

  function closeThemeMenu() {
    endPreview()
    themeMenu = false
  }

  async function pickTheme(name: string) {
    theme = await setTheme(name)
    previewName = null
    applyTheme(theme)
    themeMenu = false
  }

  async function openOptions() {
    optionsOpen = true
    // Se relee lo guardado para no mostrar un estado obsoleto de otra sesion.
    try {
      settings = await getSettings()
    } catch {
      /* se mantiene lo que ya habia */
    }
    try {
      soundCatalog = await getSoundCatalog()
    } catch {
      /* la seccion Sonidos no se muestra */
    }
    await loadVibeTools()
  }

  function toggleVibeTool(key: string, enabled: boolean) {
    vibeTools = vibeTools.map((t) => (t.key === key ? { ...t, enabled } : t))
  }

  // Carga el catalogo de CLIs para las cards de la barra lateral. Se repite al
  // guardar Opciones y al arrancar; si falla, la seccion no se muestra.
  async function loadVibeTools() {
    try {
      vibeTools = await getVibeTools()
    } catch {
      /* sin catalogo no se pinta la seccion */
    }
  }

  // Lanza una CLI en una pestana del proyecto activo.
  async function launchVibe(tool: VibeToolInfo) {
    try {
      const t = await runTask('vibe-run-' + tool.key)
      if (!t) {
        lastError = `Sin tarea para ${tool.title}`
        return
      }
      lastError = ''
      await runNamedTask(tool.title, t)
    } catch (e) {
      lastError = 'ERROR: ' + (e as Error).message
    }
  }

  // Acciones de mantenimiento de Vibe Coding, que antes vivian en el submenu
  // de su card: se lanzan como cualquier tarea, en una pestana del proyecto.
  async function runVibeTask(task: string, label: string) {
    optionsOpen = false
    try {
      const t = await runTask(task)
      if (!t) {
        lastError = `Sin tarea para ${label}`
        return
      }
      lastError = ''
      await runNamedTask(label, t)
    } catch (e) {
      lastError = 'ERROR: ' + (e as Error).message
    }
  }

  // Preset elegido para un evento (si aun no hay valor guardado, el de fabrica).
  function presetFor(ev: SoundEventInfo): string {
    return settings.sound_presets?.[ev.id] || ev.default_preset
  }

  function setPreset(ev: SoundEventInfo, preset: string) {
    settings.sound_presets = { ...settings.sound_presets, [ev.id]: preset }
  }

  function previewEvent(ev: SoundEventInfo) {
    previewSound(ev.id, presetFor(ev)).catch(() => {})
  }

  async function saveSettings() {
    // El modal solo edita las opciones generales; se parte de lo guardado para
    // no pisar los ajustes de Git que ahora vive en la pestana Git.
    try {
      const cur = await getSettings()
      await setSettings({
        ...cur,
        open_output_dir: settings.open_output_dir,
        close_tab_on_build_end: settings.close_tab_on_build_end,
        sound_presets: settings.sound_presets,
        // Si el catalogo no llego (backend antiguo o fallo), se conserva la
        // lista guardada en vez de habilitar todo por accidente.
        vibe_disabled: vibeTools.length
          ? vibeTools.filter((t) => !t.enabled).map((t) => t.key)
          : cur.vibe_disabled,
      })
      optionsOpen = false
      // La barra lateral refleja los interruptores al instante.
      await loadVibeTools()
    } catch (e) {
      lastError = (e as Error).message
    }
  }

  // Lanza una card de Config desde el modal de Opciones: se cierra Opciones
  // primero para que no se solapen los dos modales, y se reutiliza openTask
  // (que ya decide entre modal de subcards y pestaña ConPTY).
  async function launchCfgTool(item: MenuItemInfo) {
    optionsOpen = false
    await openTask(item)
  }

  const cardAccent = (item: MenuItemInfo) => shownTheme()?.cards?.[item.color] ?? shownTheme()?.accent ?? '#cba6f7'

  function b64ToBytes(b64: string): Uint8Array {
    const bin = atob(b64)
    const out = new Uint8Array(bin.length)
    for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
    return out
  }
  function bytesToB64(bytes: Uint8Array): string {
    let bin = ''
    for (const b of bytes) bin += String.fromCharCode(b)
    return btoa(bin)
  }
  const colors = ['#89b4fa', '#94e2d5', '#a6e3a1', '#f9e2af', '#f38ba8', '#cba6f7']

  onMount(() => {
    onPtyData((d) => {
      const tab = tabs.find((t) => t.ptyId === d.id)
      const entry = tab && terms.get(tab.key)
      if (entry) entry.term.write(b64ToBytes(d.data))
    })
    onPtyExit((d) => {
      const tab = tabs.find((t) => t.ptyId === d.id)
      const entry = tab && terms.get(tab.key)
      if (entry) {
        // Una TUI que muere sin desactivar el mouse deja la rueda capturada:
        // al cerrar el proceso se restauran los modos para poder hacer scroll.
        entry.term.write('\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1005l\x1b[?1006l')
        entry.term.writeln('\r\n\x1b[2m─ proceso terminado ─\x1b[0m')
      }
      if (tab) {
        tab.exited = true
        tab.ptyId = null
      }
      // La mascota celebra o se lamenta. Un codigo negativo es un cierre
      // forzado (cerrar la pestana): no es un fallo de la tarea.
      if (typeof d.code === 'number') {
        if (d.code === 0) flashMood('success')
        else if (d.code > 0) flashMood('error')
      }
      // El comando/tarea pudo tocar el repo (git, builds, generacion de
      // codigo): que el explorador vuelva a leer el estado.
      notifyRepoChanged()
    })
    // El alto/ancho del terminal cambia al cargar las cards de otro proyecto,
    // al plegar el panel derecho o al redimensionar la ventana. El observer
    // reajusta xterm y la ConPTY para que no quede la ultima fila ni la
    // ultima columna ocultas.
    const ro = new ResizeObserver(() => scheduleFit())
    if (termStageEl) ro.observe(termStageEl)
    // Cambios hechos fuera de Compass (otra terminal, IDE, Explorer): al volver
    // a la ventana se refresca el estado git del explorador.
    window.addEventListener('focus', notifyRepoChanged)
    document.addEventListener('visibilitychange', () => {
      if (document.visibilityState === 'visible') notifyRepoChanged()
    })
    loadTheme()
    // Catalogo de CLIs para la seccion Vibe Coding de la barra lateral.
    loadVibeTools()
    // El reparto elegido con el separador se restaura al arrancar.
    getSettings().then((s) => { termPct = s.terminal_panel_pct }).catch(() => {})
    onWorkspaceChanged(() => {
      refresh()
      notifyRepoChanged()
    })
    // Las ConPTY sobreviven a la recarga: se reconstruyen sus pestanas con la
    // salida reciente. Se hace despues de refresh() porque hay que conocer el
    // proyecto activo para decidir cual queda seleccionada.
    refresh().then(restoreTerminals).catch(() => {})
    return () => {
      ro.disconnect()
      vpRo.disconnect()
      if (fitRaf) cancelAnimationFrame(fitRaf)
    }
  })

  // restoreTerminals reengancha la UI a las sesiones que ya estaban vivas.
  //
  // En modo desarrollo (dev.bat) Vite recarga el WebView al guardar un
  // .svelte; antes se cerraban todas las ConPTY al montar y los builds en
  // curso morian en cada guardado. Ahora el backend las conserva y aqui se
  // repintan: mismo id, misma salida, mismo proceso.
  async function restoreTerminals() {
    let list: TerminalInfo[] = []
    try {
      list = await listTerminals()
    } catch {
      return
    }
    if (!list.length) return

    for (const s of list) {
      if (tabs.some((t) => t.ptyId === s.id)) continue
      const key = ++seq
      // ptyId se fija DESPUES de pintar el bufer: si se fijara antes, un
      // chunk que llegue mientras se monta el xterm se escribiria antes que
      // la salida historica y el orden quedaria invertido.
      tabs.push({ key, label: s.label || 'Terminal', ptyId: null, exited: s.exited, project: s.project })
      mountTerm(key)
      await attachTerm(key)
      const entry = terms.get(key)
      if (entry && s.output) entry.term.write(b64ToBytes(s.output))
      if (s.exited) {
        if (entry) entry.term.writeln('\r\n\x1b[2m─ proceso terminado ─\x1b[0m')
      } else {
        tabs.find((t) => t.key === key)!.ptyId = s.id
      }
    }

    const first = tabForProject(projectPath)
    if (first) await activate(first.key)
  }

  async function loadTheme() {
    try {
      themes = await getThemes()
      theme = await getTheme()
      applyTheme(theme)
      settings = await getSettings()
    } catch (e) {
      lastError = 'ERROR cargando tema: ' + (e as Error).message
    }
  }

  async function refresh() {
    workspace = await getWorkspace()
    const seq = ++menuSeq
    const m = await getMenu()
    if (seq === menuSeq) menu = m
  }

  // Boton "+" de la barra lateral: selector nativo de carpetas. El backend
  // emite workspace:changed, que ya refresca workspace y menu.
  async function addProject() {
    try {
      workspace = await pickProjectDialog()
      // El menu y la primera terminal no bloquean el repintado del proyecto.
      // El dialogo ya cambio el proyecto en el backend, asi que getMenu
      // devuelve las cards correctas.
      const seq = ++menuSeq
      const menuP = getMenu()
      const first = tabForProject(workspace.projects[workspace.active]?.path ?? '')
      if (first) await activate(first.key)
      else activeKey = null
      const next = await menuP
      if (seq === menuSeq) menu = next
    } catch (e) {
      lastError = 'ERROR al abrir el proyecto: ' + (e as Error).message
    }
  }

  async function pickProject(i: number) {
    // No se cierran las pestañas: cada proyecto conserva sus terminales
    // abiertos y se puede volver a ellas al cambiar de proyecto.
    //
    // El proyecto Y su pestaña se cambian en el mismo frame. Si solo se
    // adelanta el proyecto y se espera a que el backend responda, durante esa
    // ventana `projectPath` ya es el nuevo pero `activeKey` apunta al
    // anterior: no hay pestañas visibles, desaparecen la barra de pestañas y
    // el terminal, y luego vuelven a pintarse (el "doble render" al cambiar
    // de proyecto).
    if (i !== workspace.active) workspace = { ...workspace, active: i }
    const first = tabForProject(workspace.projects[i]?.path ?? '')
    if (first) activating = true
    activeKey = first ? first.key : null
    workspace = await selectProject(i)
    // El menu se pide DESPUES de confirmar el proyecto: getMenu lo construye
    // con el proyecto activo del backend, y pedirlo antes devolvia las cards
    // del proyecto anterior (se veian las de A al entrar en B).
    const seq = ++menuSeq
    const menuP = getMenu()
    // El backend confirma el proyecto activo; la pestaña se revalida por si
    // cambio el indice.
    const confirmed = tabForProject(workspace.projects[workspace.active]?.path ?? '')
    if (confirmed) await activate(confirmed.key)
    else {
      activeKey = null
      activating = false
    }
    const next = await menuP
    if (seq === menuSeq) menu = next
  }

  async function dropProject(i: number) {
    const removed = workspace.projects[i]?.path
    const wasActive = i === workspace.active
    workspace = await removeProject(i)
    // Solo se cierran las terminales del proyecto eliminado.
    if (removed) {
      for (const t of tabs.filter((t) => t.project === removed)) await closeTab(t.key)
    }
    if (wasActive) menu = await getMenu()
  }

  // mountTerm crea el xterm de una pestana y lo engancha a su host.
  function mountTerm(key: number): Terminal {
    const term = new Terminal({
      fontFamily: 'Cascadia Mono, Consolas, monospace',
      fontSize: 13,
      cursorBlink: true,
      theme: { background: theme?.panel ?? '#11111b', foreground: theme?.text ?? '#cdd6f4' },
      allowProposedApi: true,
      scrollback: 5000,
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    terms.set(key, { term, fit })
    term.onScroll(() => {
      // Solo se memoriza lo que hace el usuario en la pestana visible: el
      // reajuste interno de xterm al mostrar un host oculto (o al activar una
      // pestana) no debe pisar la posicion de lectura guardada.
      if (activating || activeKey !== key) return
      const b = term.buffer.active
      scrollMemo.set(key, { fromBottom: b.baseY - b.viewportY, bottom: b.viewportY >= b.baseY })
    })
    term.onData((data) => {
      const tab = tabs.find((t) => t.key === key)
      if (tab?.ptyId) writeTerminal(tab.ptyId, bytesToB64(new TextEncoder().encode(data)))
    })
    // Copiar: Ctrl+Shift+C copia siempre; Ctrl+C copia solo si hay seleccion
    // (si no, envia ^C). NO interceptamos Ctrl+V: xterm.js ya escucha el
    // evento `paste` nativo del navegador y lo inyecta en el PTY. Llamar
    // ademas a term.paste() desde aqui pegaba el texto DOS veces.
    term.attachCustomKeyEventHandler((e) => {
      if (e.type !== 'keydown') return true
      const mod = e.ctrlKey || e.metaKey
      const k = e.key.toLowerCase()
      if (mod && k === 'c') {
        const sel = term.getSelection()
        if (sel) {
          navigator.clipboard.writeText(sel).catch(() => {})
          return false
        }
        if (e.shiftKey) return false
      }
      return true
    })
    // Las CLI con TUI (por ejemplo el chat de Codebuddy) activan mouse
    // tracking: la rueda se envia a la aplicacion y el historial de xterm
    // queda inmovil. En el buffer normal con scrollback la desplazamos
    // nosotros; en el buffer alterno (vim, less, htop) se respeta la app.
    term.attachCustomWheelEventHandler((ev) => {
      const buf = term.buffer.active
      if (buf.type !== 'normal' || buf.baseY === 0 || term.modes.mouseTrackingMode === 'none') {
        return true
      }
      let lines = ev.deltaY
      if (ev.deltaMode === 0) lines = ev.deltaY / 24
      else if (ev.deltaMode === 2) lines = ev.deltaY * term.rows
      const n = lines > 0 ? Math.max(1, Math.round(lines)) : Math.min(-1, Math.round(lines))
      if (n) term.scrollLines(n)
      ev.preventDefault()
      return false
    })
    return term
  }

  // attachTerm pinta el xterm en su host una vez que el DOM existe.
  async function attachTerm(key: number) {
    await tick()
    const el = document.getElementById(`term-${key}`)
    const entry = terms.get(key)
    if (!el || !entry || mounted.has(key)) return
    entry.term.open(el)
    const vp = el.querySelector('.xterm-viewport')
    if (vp) vpRo.observe(vp)
    // Clic derecho sobre la terminal: copia la seleccion. Se detiene la
    // propagacion para que el menu contextual de Wails no lo intercepte.
    el.addEventListener('contextmenu', (ev) => {
      ev.preventDefault()
      ev.stopPropagation()
      const sel = entry.term.getSelection()
      if (sel) {
        navigator.clipboard.writeText(sel).catch(() => {})
        entry.term.clearSelection()
      }
    }, { capture: true })
    mounted.add(key)
    fitTerm(key)
  }

  // fitTerm ajusta el xterm al host sin proponer nunca un tamano que no quepa.
  // FitAddon mide el host, pero el ancho util de verdad lo marca el viewport
  // (descuenta su barra de scroll). Antes se llamaba a `fit()` y luego se
  // recortaban columnas: eso redimensionaba DOS veces (ancho y luego menos
  // ancho) en cada cambio de pestaña, y cada reflow desplazaba el contenido.
  // Ahora se calcula el tamano final y solo se llama a `resize()` si de verdad
  // cambia, de modo que cambiar de proyecto no toca la geometria del terminal.
  function fitTerm(key: number) {
    const entry = terms.get(key)
    if (!entry) return
    const el = document.getElementById(`term-${key}`)
    const screen = el?.querySelector('.xterm-screen') as HTMLElement | null
    const viewport = el?.querySelector('.xterm-viewport') as HTMLElement | null
    if (!screen || !viewport || !entry.term.cols) return
    const dims = entry.fit.proposeDimensions()
    if (!dims) return
    // Ancho de celda tal como lo pinta el renderer (ya redondeado al DPR).
    const cell = screen.offsetWidth / entry.term.cols
    if (!cell) return
    // Reserva una celda al borde derecho: con el redondeo de Windows/DPI algunas
    // TUI pintaban la ultima columna fuera del viewport y quedaba recortada.
    const cols = Math.max(2, Math.min(dims.cols, Math.floor((viewport.clientWidth - cell - 1) / cell)))
    const rows = Math.max(1, dims.rows)
    if (cols === entry.term.cols && rows === entry.term.rows) return
    // Guarda la posicion del historial: `resize()` refluye y xterm puede saltar
    // al final. Se mide en lineas desde el final porque el reflow cambia el
    // numero absoluto de linea.
    const before = entry.term.buffer.active
    const wasAtBottom = before.viewportY >= before.baseY
    const fromBottom = before.baseY - before.viewportY
    entry.term.resize(cols, rows)
    if (wasAtBottom) {
      entry.term.scrollToBottom()
    } else {
      const b = entry.term.buffer.active
      entry.term.scrollToLine(Math.max(0, b.baseY - fromBottom))
    }
  }

  function resizePty(tab: Tab, entry: { term: Terminal }) {
    if (!tab.ptyId) return
    const sig = `${entry.term.cols}x${entry.term.rows}`
    if (lastPtySize.get(tab.key) === sig) return
    lastPtySize.set(tab.key, sig)
    resizeTerminal(tab.ptyId, entry.term.cols, entry.term.rows)
  }

  // Reajusta solo la pestaña activa al nuevo tamaño del panel. Si otro
  // proyecto no tiene pestañas y oculta `.term-tabs`, las terminales ocultas
  // de otros proyectos no se redimensionan ni reciben SIGWINCH al ir y volver.
  function scheduleFit() {
    if (fitRaf) return
    fitRaf = requestAnimationFrame(() => {
      fitRaf = 0
      // Durante el arrastre del separador se ignora: el fit se hace al soltar.
      if (splitDrag) return
      if (activeKey == null || !mounted.has(activeKey)) return
      const entry = terms.get(activeKey)
      if (!entry) return
      fitTerm(activeKey)
      const tab = tabs.find((t) => t.key === activeKey)
      if (tab) resizePty(tab, entry)
    })
  }

  // Pestana a mostrar al entrar en un proyecto: la ultima que estuvo activa
  // alli y siga existiendo; si no, la primera del proyecto.
  function tabForProject(path: string): Tab | undefined {
    const remembered = lastTabByProject.get(path)
    if (remembered != null) {
      const t = tabs.find((x) => x.key === remembered && x.project === path)
      if (t) return t
    }
    return tabs.find((t) => t.project === path)
  }

  async function activate(key: number) {
    // La posicion se lee ANTES de tocar `activeKey`: al mostrar el host oculto
    // xterm puede re-sincronizar su viewport y disparar `onScroll`, que
    // sobrescribiria la posicion buena con la saltada.
    const memo = scrollMemo.get(key)
    activating = true
    activeKey = key
    // Se recuerda por proyecto: al volver se reabre esta pestana y no la
    // primera, aunque se haya cambiado de proyecto con otra activa.
    const current = tabs.find((t) => t.key === key)
    if (current) lastTabByProject.set(current.project, key)
    await tick()
    const entry = terms.get(key)
    if (!entry) {
      activating = false
      return
    }
    // Si la ventana o el separador cambiaron mientras la pestaña estaba oculta,
    // se ajusta ahora (no-op si el tamaño no cambió).
    if (mounted.has(key)) {
      fitTerm(key)
      if (current) resizePty(current, entry)
    }
    const buf = entry.term.buffer.active
    const wasAtBottom = memo ? memo.bottom : buf.viewportY >= buf.baseY
    const fromBottom = memo ? memo.fromBottom : buf.baseY - buf.viewportY
    entry.term.focus()
    if (!wasAtBottom) {
      // Se reaplica la distancia al final, no una linea absoluta: la salida
      // que llego mientras la pestana estaba oculta no debe mover la vista.
      const restore = () => {
        const b = entry.term.buffer.active
        entry.term.scrollToLine(Math.max(0, b.baseY - fromBottom))
      }
      restore()
      // Segundo intento en el frame siguiente: xterm puede re-sincronizar el
      // viewport al mostrar el host oculto.
      requestAnimationFrame(() => {
        restore()
        activating = false
      })
    } else {
      activating = false
    }
  }

  function toggleRightPanel() {
    // El ResizeObserver del stage detecta el nuevo ancho y reajusta xterm/PTY.
    rightCollapsed = !rightCollapsed
  }

  // Separador cards/terminal. Mientras se arrastra NO se reajusta el xterm:
  // cada pixel cambiaria el PTY y la TUI se redibujaria sin parar. Al soltar
  // se hace un unico fit (y se guarda el reparto para la proxima sesion).
  function splitDown(e: PointerEvent) {
    const handle = e.currentTarget as HTMLElement
    const area = handle.parentElement
    if (!area) return
    e.preventDefault()
    handle.setPointerCapture(e.pointerId)
    splitDrag = true
    const move = (ev: PointerEvent) => {
      const r = area.getBoundingClientRect()
      if (!r.height) return
      const pct = ((r.bottom - ev.clientY) / r.height) * 100
      termPct = Math.min(80, Math.max(25, Math.round(pct)))
    }
    const up = () => {
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', up)
      splitDrag = false
      scheduleFit()
      persistSplit()
    }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', up)
  }

  function splitKey(e: KeyboardEvent) {
    const step = e.shiftKey ? 10 : 3
    if (e.key === 'ArrowUp') termPct = Math.min(80, termPct + step)
    else if (e.key === 'ArrowDown') termPct = Math.max(25, termPct - step)
    else if (e.key === 'Home') termPct = 60
    else return
    e.preventDefault()
    scheduleFit()
    persistSplit()
  }

  async function persistSplit() {
    try {
      const cur = await getSettings()
      await setSettings({ ...cur, terminal_panel_pct: termPct })
    } catch {
      /* el reparto queda solo en esta sesion */
    }
  }

  async function startPty(tab: Tab, task: string) {
    const entry = terms.get(tab.key)
    if (!entry) return
    entry.term.clear()
    fitTerm(tab.key)
    const script = await taskScript(task)
    // El id se fija ANTES de abrir: la salida de la tarea puede llegar antes
    // de que la promesa resuelva y hay que poder casarla ya.
    const id = `tab-${sessionTag}-${tab.key}`
    tab.ptyId = id
    tab.exited = false
    const opened = await openTerminal(id, tab.label, script, entry.term.rows, entry.term.cols)
    if (!opened) {
      tab.ptyId = null
      tab.exited = true
      lastError = 'ERROR: no se pudo abrir la ConPTY'
      return
    }
    lastError = ''
    // La ConPTY ya nace con este tamano: no hace falta reenviarlo al activar.
    lastPtySize.set(tab.key, `${entry.term.cols}x${entry.term.rows}`)
    entry.term.focus()
    reclaimFocus()
  }

  // La app que se lanza en modo dev (o cualquier ventana hija) puede robar el
  // foco al abrirse. Solo reafirmamos el foco si REALMENTE se perdio: llamar a
  // Focus() con la ventana ya enfocada provoca un parpadeo del WebView.
  function reclaimFocus() {
    for (const ms of [150, 600, 1500, 3000, 6000]) {
      setTimeout(() => {
        if (!document.hasFocus()) focusWindow().catch(() => {})
      }, ms)
    }
  }

  async function openTask(item: MenuItemInfo) {
    // No se usa un mensaje "Abriendo..." transitorio: hacia aparecer y
    // desaparecer el banner y desplazaba el layout en cada clic (parpadeo).
    try {
      // Release / Git / Entorno abren un modal con subcards (como en la GUI Gio).
      const subs = await subcards(item.id)
      if (subs?.length) {
        subMenu = { title: item.label, items: subs }
        return
      }
      const task = await runTask(item.id)
      if (!task) {
        const dbg = await getDebug()
        lastError = `Sin tarea para ${item.label} (ctx=${dbg.ctxDir || 'nil'}, tipo=${dbg.type || '?'})`
        return
      }
      lastError = ''
      await runNamedTask(item.label, task)
    } catch (e) {
      lastError = 'ERROR: ' + (e as Error).message
    }
  }

  async function pickSubcard(sub: Subcard) {
    subMenu = null
    // "Elegir que borrar" se resuelve en un modal GUI, no en una ConPTY.
    if (sub.task === 'clean-do-choose') {
      await openCleanupModal()
      return
    }
    await runNamedTask(sub.label, sub.task)
  }

  async function openCleanupModal() {
    cleanupResult = null
    cleanupBusy = true
    try {
      cleanupItems = await listCleanupCandidates()
      // Nada marcado por defecto: el usuario elige explicitamente que borrar.
      cleanupSelected = {}
      lastError = ''
      cleanupOpen = true
    } catch (e) {
      lastError = 'ERROR al listar destinos: ' + (e as Error).message
    } finally {
      cleanupBusy = false
    }
  }

  function toggleCleanup(path: string) {
    cleanupSelected = { ...cleanupSelected, [path]: !cleanupSelected[path] }
  }

  function selectedCount() {
    return cleanupItems.filter((i) => cleanupSelected[i.path]).length
  }

  async function confirmCleanup() {
    if (cleanupBusy) return
    const paths = cleanupItems.filter((i) => cleanupSelected[i.path]).map((i) => i.path)
    if (!paths.length) return
    cleanupBusy = true
    try {
      cleanupResult = await purgeCleanupCandidates(paths)
      lastError = ''
      // Refrescar: los directorios borrados desaparecen de la lista.
      cleanupItems = await listCleanupCandidates()
      const live = new Set(cleanupItems.map((i) => i.path))
      cleanupSelected = Object.fromEntries(
        Object.entries(cleanupSelected).filter(([p]) => live.has(p)),
      )
      notifyRepoChanged()
    } catch (e) {
      lastError = 'ERROR al borrar: ' + (e as Error).message
    } finally {
      cleanupBusy = false
    }
  }

  async function runNamedTask(label: string, task: string) {
    // Reutiliza la pestana de esa card solo si es del MISMO proyecto; asi dos
    // proyectos con una card "Windows" no comparten terminal.
    const project = workspace.projects[workspace.active]?.path ?? ''
    const existing = tabs.find((t) => t.label === label && t.project === project)
    if (existing) {
      if (existing.ptyId) {
        await activate(existing.key)
        return
      }
      await activate(existing.key)
      await startPty(existing, task)
      return
    }

    const key = ++seq
    tabs.push({ key, label, ptyId: null, exited: false, project })
    mountTerm(key)
    await attachTerm(key)
    await activate(key)
    await startPty(tabs.find((t) => t.key === key)!, task)
  }

  async function closeTab(key: number) {
    const tab = tabs.find((t) => t.key === key)
    if (tab?.ptyId) await closeTerminal(tab.ptyId)
    const entry = terms.get(key)
    if (entry) {
      const vp = document.getElementById(`term-${key}`)?.querySelector('.xterm-viewport')
      if (vp) vpRo.unobserve(vp)
      entry.term.dispose()
      terms.delete(key)
    }
    mounted.delete(key)
    lastPtySize.delete(key)
    scrollMemo.delete(key)
    tabs = tabs.filter((t) => t.key !== key)
    // Si la pestana cerrada era la recordada del proyecto, se olvida para que
    // al volver se elija otra en vez de un key inexistente.
    if (tab && lastTabByProject.get(tab.project) === key) lastTabByProject.delete(tab.project)
    if (activeKey === key) {
      // Siguiente pestaña del MISMO proyecto (no salta a otro proyecto).
      const sameProject = tabs.filter((t) => t.project === tab?.project)
      const next = sameProject.find((t) => t.key > key) ?? sameProject[sameProject.length - 1] ?? null
      activeKey = next ? next.key : null
      if (next) await activate(next.key)
    }
  }

  async function closeAllTabs() {
    for (const t of [...tabs]) await closeTab(t.key)
    activeKey = null
  }
</script>

<div class="app">
  <aside class="sidebar">
    <div class="brand">
      <span class="logo" aria-hidden="true"><Mascot size={40} {mood} /></span>
      <span class="wordmark">Compass <b>Go</b></span>
    </div>
    {#if workspace.projects.length}
      <div class="project-name">{workspace.projects[workspace.active]?.name}</div>
      <div class="project-type">{workspace.projects[workspace.active]?.type}</div>
    {/if}
    <div class="section-title">
      <span>PROYECTOS</span>
      <button
        class="add-project"
        onclick={addProject}
        title="Añadir proyecto"
        aria-label="Añadir proyecto"
      >{@html svg('plus', 13)}</button>
    </div>
    <div class="projects">
      {#each workspace.projects as p, i}
        <div class:active={i === workspace.active} class="project">
          <button class="project-main" onclick={() => pickProject(i)} title={p.path}>
            <span class="dot" style="background:{colors[i % colors.length]}"></span>
            <span class="pname">{p.name}</span>
          </button>
          <button class="project-close" onclick={() => dropProject(i)} aria-label="Cerrar proyecto" title="Cerrar proyecto">×</button>
        </div>
      {/each}
    </div>
    {#if vibeSide.length}
      <div class="section-title"><span>VIBE CODING</span></div>
      <div class="vibe-grid">
        {#each vibeSide as tool (tool.key)}
          <button
            class="vibe-card"
            onclick={() => launchVibe(tool)}
            aria-label={`Abrir ${tool.title} en el proyecto activo`}
          >
            <span class="vibe-name">{tool.title}</span>
            <span class="vibe-tip" role="tooltip">
              <span class="vibe-tip-title">{tool.title}</span>
              <span class="vibe-tip-path">{tool.path}</span>
              <span class="vibe-tip-hint">Clic para abrir en el proyecto activo</span>
            </span>
          </button>
        {/each}
      </div>
    {/if}
    <div class="sidebar-foot">
      <button class="theme-btn" onclick={() => (themeMenu = !themeMenu)}>
        <span>◈</span><span class="theme-btn-text">{theme?.name ?? 'Tema'}</span>
      </button>
      <button class="theme-btn" onclick={openOptions}>
        <span>⚙</span><span class="theme-btn-text">Opciones</span>
      </button>
    </div>
  </aside>

  <main class="main">
    {#if lastError}<div class="debug-line">{lastError}</div>{/if}
    <section class="content">
      <div class="cards-area" class:dragging={splitDrag} style={`--term-pct: ${termPct}%`}>
        <div class="cards">
          {#each menu.filter((m) => m.section !== 'CFG') as item}
            <Card {item} accent={cardAccent(item)} onOpen={() => openTask(item)} />
          {/each}
        </div>

        <button
          type="button"
          class="term-splitter"
          class:dragging={splitDrag}
          aria-label={`Ajustar el alto del terminal (${termPct}%). Flechas arriba/abajo, Inicio para 60%`}
          title="Arrastra para ajustar el alto del terminal"
          onpointerdown={splitDown}
          onkeydown={splitKey}
        ></button>

        <div class="terminal-panel">
          {#if visibleTabs.length}
            <div class="term-tabs">
              {#each visibleTabs as tab (tab.key)}
                <div
                  class="term-tab"
                  class:active={tab.key === activeKey}
                  role="tab"
                  tabindex="0"
                  title={tab.project}
                  aria-selected={tab.key === activeKey}
                  onclick={() => activate(tab.key)}
                  onkeydown={(e) => e.key === 'Enter' && activate(tab.key)}
                >
                  <span class="term-dot" class:dead={tab.exited}></span>
                  <span class="term-label">{tab.label}</span>
                  <button
                    class="term-close"
                    title="Cerrar pestaña"
                    onclick={(e) => {
                      e.stopPropagation()
                      closeTab(tab.key)
                    }}>×</button>
                </div>
              {/each}
              <button
                class="term-wide"
                title={rightCollapsed ? 'Mostrar panel derecho' : 'Ampliar terminal'}
                aria-label={rightCollapsed ? 'Mostrar panel derecho' : 'Ampliar terminal'}
                onclick={(e) => {
                  e.stopPropagation()
                  toggleRightPanel()
                }}
              >{rightCollapsed ? '⤡' : '⤢'}</button>
            </div>
          {/if}
          <div class="term-stage" bind:this={termStageEl}>
            {#each tabs as tab (tab.key)}
              <div class="term-host" class:hidden={tab.key !== activeKey} id={`term-${tab.key}`}></div>
            {/each}
            {#if activeKey == null}
              <div class="term-empty">
                <p class="muted">Abre una card para lanzar su tarea en una pestaña ConPTY.</p>
              </div>
            {/if}
          </div>
        </div>
      </div>

      <aside class="right" class:collapsed={rightCollapsed}>
        <div class="right-tabs">
          <button class:active={rightTab === 'explorer'} onclick={() => (rightTab = 'explorer')}>Explorador</button>
          <button class:active={rightTab === 'git'} onclick={() => (rightTab = 'git')}>Git</button>
          <button class:active={rightTab === 'log'} onclick={() => (rightTab = 'log')}>Historial</button>
        </div>
        <div class="right-body">
          {#if rightTab === 'explorer'}
            {#key workspace.active}
              <Explorer projectPath={projectPath} />
            {/key}
          {:else if rightTab === 'git'}
            {#key workspace.active}
              <Git projectPath={projectPath} />
            {/key}
          {:else if rightTab === 'log'}
            {#key workspace.active}
              <History projectPath={projectPath} />
            {/key}
          {/if}
        </div>
        <System />
      </aside>
    </section>
  </main>
</div>

{#if subMenu}
  <div class="modal-backdrop" onclick={() => (subMenu = null)} role="presentation">
    <div class="modal wide" onclick={(e) => e.stopPropagation()} role="dialog" aria-label={subMenu.title}>
      <div class="modal-head"><span>{subMenu.title}</span><button onclick={() => (subMenu = null)}>×</button></div>
      <div class="sub-grid">
        {#each subMenu.items as sub}
          <button class="sub-card" onclick={() => pickSubcard(sub)} title={`${sub.label} — ${sub.description}`}>
            <span class="sub-icon">{@html svg(sub.icon, 18)}</span>
            <span class="sub-text">
              <span class="sub-label">{sub.label}</span>
              <span class="sub-desc">{sub.description}</span>
            </span>
          </button>
        {/each}
      </div>
    </div>
  </div>
{/if}

{#if cleanupOpen}
  <div class="modal-backdrop" onclick={() => (cleanupOpen = false)} role="presentation">
    <div class="modal wide" onclick={(e) => e.stopPropagation()} role="dialog" aria-label="Elegir qué borrar">
      <div class="modal-head"><span>Elegir qué borrar</span><button onclick={() => (cleanupOpen = false)}>×</button></div>
      {#if cleanupBusy && !cleanupItems.length}
        <p class="cleanup-empty">Calculando tamaños…</p>
      {:else if !cleanupItems.length}
        <p class="cleanup-empty">No hay nada que borrar: no existe ningún directorio de build.</p>
      {:else}
        <p class="cleanup-hint">Marca lo que quieras eliminar. Los elementos <b>deps</b> tardan en reinstalarse.</p>
        <div class="cleanup-list">
          {#each cleanupItems as item (item.path)}
            <label class="cleanup-row">
              <input type="checkbox" checked={!!cleanupSelected[item.path]} onchange={() => toggleCleanup(item.path)} />
              <span class="cleanup-label">{item.label}</span>
              <span class="cleanup-size">{item.size}</span>
              {#if item.deps}<span class="cleanup-badge">deps</span>{/if}
            </label>
          {/each}
        </div>
      {/if}
      {#if cleanupResult}
        <div class="cleanup-result">
          <div>Liberado: <b>{cleanupResult.freed_label}</b></div>
          {#if cleanupResult.errors.length}
            <ul class="cleanup-errors">
              {#each cleanupResult.errors as err}<li>{err}</li>{/each}
            </ul>
          {/if}
        </div>
      {/if}
      <div class="modal-actions">
        <button onclick={() => (cleanupOpen = false)}>Cerrar</button>
        <button class="btn-primary" disabled={cleanupBusy || selectedCount() === 0} onclick={confirmCleanup}>
          {cleanupBusy ? 'Borrando…' : `Borrar (${selectedCount()})`}
        </button>
      </div>
    </div>
  </div>
{/if}

{#if themeMenu}
  <div class="modal-backdrop" onclick={closeThemeMenu} role="presentation">
    <div class="modal wide" onclick={(e) => e.stopPropagation()} role="dialog" aria-label="Temas"
         onmouseleave={endPreview}>
      <div class="modal-head"><span>Temas</span><button onclick={closeThemeMenu}>×</button></div>
      <div class="theme-grid">
        {#each themes as t}
          <button class="theme-swatch" class:active={t.name === shownTheme()?.name}
                  onmouseenter={() => hoverTheme(t)} onclick={() => pickTheme(t.name)}>
            <span class="swatch-colors">
              <i style="background:{t.bg}"></i>
              <i style="background:{t.panel}"></i>
              <i style="background:{t.accent}"></i>
              <i style="background:{t.brand}"></i>
            </span>
            <span class="swatch-name">{t.name}</span>
          </button>
        {/each}
      </div>
      <div class="theme-hint">Pasa el cursor para previsualizar · clic para aplicar</div>
    </div>
  </div>
{/if}

{#if optionsOpen}
  <div class="modal-backdrop" onclick={() => (optionsOpen = false)} role="presentation">
    <div class="modal options" onclick={(e) => e.stopPropagation()} role="dialog" aria-label="Opciones">
      <div class="modal-head"><span>Opciones</span><button onclick={() => (optionsOpen = false)}>×</button></div>

      <div class="opt-grid">
        {#if cfgTools.length}
          <section class="opt-card">
            <h3 class="opt-card-title">Herramientas</h3>
            <div class="opt-tools">
              {#each cfgTools as tool}
                <button class="opt-tool" onclick={() => launchCfgTool(tool)} title={tool.description}>
                  <span class="opt-tool-ico">{@html svg(tool.icon, 17)}</span>
                  <span class="opt-tool-text">
                    <span class="opt-tool-label">{tool.label}</span>
                    <span class="opt-tool-desc">{tool.description}</span>
                  </span>
                  <span class="opt-tool-arrow">›</span>
                </button>
              {/each}
            </div>
          </section>
        {/if}

        <section class="opt-card">
          <h3 class="opt-card-title">General</h3>
          <label class="opt-check">
            <input type="checkbox" bind:checked={settings.open_output_dir} />
            <span>Abrir la carpeta de salida al terminar un build</span>
          </label>
          <label class="opt-check">
            <input type="checkbox" bind:checked={settings.close_tab_on_build_end} />
            <span>Cerrar la pestaña del terminal al terminar un build</span>
          </label>
        </section>

        {#if vibeTools.length}
          <section class="opt-card opt-vibe">
            <h3 class="opt-card-title">Vibe Coding</h3>
            <p class="opt-hint">
              Elige qué CLIs aparecen en la card y en el menú contextual de Windows.
            </p>
            <div class="vibe-list">
              {#each vibeTools as tool}
                <label class="vibe-row">
                  <span class="vibe-text">
                    <span class="sound-label">{tool.title}</span>
                    <span class="sound-desc">{tool.found ? tool.path : 'No instalada'}</span>
                  </span>
                  <input
                    type="checkbox"
                    checked={tool.enabled}
                    onchange={(e) => toggleVibeTool(tool.key, e.currentTarget.checked)}
                    aria-label={`Mostrar ${tool.title} en la card Vibe Coding`}
                  />
                </label>
              {/each}
            </div>
            <div class="vibe-actions">
              <button
                class="opt-tool"
                onclick={() => runVibeTask('vibe-setup', 'Instalar CLIs')}
                title="Detecta e instala las CLIs de Vibe Coding que falten"
              >
                <span class="opt-tool-ico">{@html svg('install', 17)}</span>
                <span class="opt-tool-text">
                  <span class="opt-tool-label">Instalar faltantes</span>
                  <span class="opt-tool-desc">Detecta e instala las CLIs que falten</span>
                </span>
                <span class="opt-tool-arrow">›</span>
              </button>
              <button
                class="opt-tool"
                onclick={() => runVibeTask('opencode-configure', 'OpenCode')}
                title="Configura el proveedor y el plugin de OpenCode"
              >
                <span class="opt-tool-ico">{@html svg('environment', 17)}</span>
                <span class="opt-tool-text">
                  <span class="opt-tool-label">Configurar OpenCode</span>
                  <span class="opt-tool-desc">Proveedor InferX y plugin de la TUI</span>
                </span>
                <span class="opt-tool-arrow">›</span>
              </button>
            </div>
          </section>
        {/if}

        {#if soundCatalog}
          <section class="opt-card opt-sounds">
            <h3 class="opt-card-title">Sonidos</h3>
            <p class="opt-hint">
              Elige el sonido de cada evento. Usa ▶ para escucharlo antes de guardar.
            </p>
            <div class="sound-list">
              {#each soundCatalog.events as ev}
                <div class="sound-row">
                  <div class="sound-text">
                    <span class="sound-label">{ev.label}</span>
                    <span class="sound-desc">{ev.description}</span>
                  </div>
                  <select
                    class="opt-input sound-select"
                    value={presetFor(ev)}
                    onchange={(e) => setPreset(ev, e.currentTarget.value)}
                    aria-label={`Sonido para ${ev.label}`}
                  >
                    {#each soundCatalog.presets as p}
                      <option value={p.id} title={p.description}>{p.name}</option>
                    {/each}
                  </select>
                  <button
                    type="button"
                    class="sound-preview"
                    onclick={() => previewEvent(ev)}
                    title="Escuchar"
                    aria-label={`Escuchar ${ev.label}`}
                  >▶</button>
                </div>
              {/each}
            </div>
          </section>
        {/if}
      </div>

      <div class="modal-actions">
        <button class="btn-primary" onclick={saveSettings}>Guardar</button>
      </div>
    </div>
  </div>
{/if}
