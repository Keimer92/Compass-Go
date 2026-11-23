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
  } from './lib/bridge'
  import { notifyRepoChanged } from './lib/repoChanged'
  import { svg } from './lib/icons'
  import Card from './components/Card.svelte'
  import Explorer from './components/Explorer.svelte'
  import Git from './components/Git.svelte'
  import History from './components/History.svelte'
  import Mascot from './lib/Mascot.svelte'
  import { loadGitInfo } from './lib/gitCache'
  import { warmExplorer, warmLog } from './lib/panelCache'
  import type { ProjectInfo, MenuItemInfo, WorkspaceInfo, ThemeInfo, Settings, Subcard, TerminalInfo, CleanupTarget, CleanupPurgeResult, SoundCatalog, SoundEventInfo } from './lib/bridge'

  interface Tab {
    key: number
    label: string
    ptyId: string | null
    exited: boolean
    project: string
  }

  let workspace = $state<WorkspaceInfo>({ projects: [], active: 0 })
  let menu = $state<MenuItemInfo[]>([])
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
  let moodText = $derived(
    mood === 'working' ? 'Trabajando…'
      : mood === 'success' ? 'Terminó sin errores'
      : mood === 'error' ? 'Algo falló'
      : 'En reposo'
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
  })
  // Catalogo de eventos/presets de sonido para la seccion "Sonidos" de Opciones.
  let soundCatalog = $state<SoundCatalog | null>(null)
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
  // Contenedor del terminal: un ResizeObserver reajusta xterm cuando el panel
  // cambia de alto/ancho (al cambiar de proyecto cambian las cards y el alto
  // disponible, y la ultima fila -el input del chat- quedaba cortada abajo).
  let termStageEl: HTMLElement | null = null
  let fitRaf = 0

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
      })
      optionsOpen = false
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

    const first = tabs.find((t) => t.project === projectPath)
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
    menu = await getMenu()
  }

  // Boton "+" de la barra lateral: selector nativo de carpetas. El backend
  // emite workspace:changed, que ya refresca workspace y menu.
  async function addProject() {
    try {
      workspace = await pickProjectDialog()
      // El menu y la primera terminal no bloquean el repintado del proyecto.
      const menuP = getMenu()
      const first = tabs.find((t) => t.project === (workspace.projects[workspace.active]?.path ?? ''))
      if (first) await activate(first.key)
      else activeKey = null
      menu = await menuP
    } catch (e) {
      lastError = 'ERROR al abrir el proyecto: ' + (e as Error).message
    }
  }

  async function pickProject(i: number) {
    // No se cierran las pestañas: cada proyecto conserva sus terminales
    // abiertos y se puede volver a ellas al cambiar de proyecto.
    //
    // Se resalta la seleccion de inmediato (optimista) y se solapa la carga
    // del menu con la activacion de la terminal: el cambio se siente al
    // instante aunque el backend tarde en re-detectar el proyecto.
    if (i !== workspace.active) workspace = { ...workspace, active: i }
    workspace = await selectProject(i)
    const menuP = getMenu()
    // Activa la primera pestaña del proyecto recién seleccionado (si tiene).
    const first = tabs.find((t) => t.project === (workspace.projects[workspace.active]?.path ?? ''))
    if (first) await activate(first.key)
    else activeKey = null
    menu = await menuP
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
    term.onData((data) => {
      const tab = tabs.find((t) => t.key === key)
      if (tab?.ptyId) writeTerminal(tab.ptyId, bytesToB64(new TextEncoder().encode(data)))
    })
    // Copiar/pegar: Ctrl+Shift+C copia siempre; Ctrl+C copia solo si hay
    // seleccion (si no, envia ^C). Ctrl+V pega desde el portapapeles.
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
      if (mod && k === 'v') {
        navigator.clipboard.readText().then((txt) => { if (txt) term.paste(txt) }).catch(() => {})
        return false
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

  // fitTerm ajusta el xterm al host y corrige el ancho util real.
  // FitAddon mide el host, pero el ancho disponible de verdad lo marca el
  // viewport, que descuenta su barra de scroll. Si la ultima columna sobresale
  // (por el ancho de la barra o por el redondeo de la celda segun el DPR), se
  // recorta el numero de columnas para que no quede texto oculto a la derecha.
  function fitTerm(key: number) {
    const entry = terms.get(key)
    if (!entry) return
    // Guarda la posicion del historial: `fit()` redimensiona y xterm puede
    // saltar al final. Al volver a una pestaña desde otro proyecto se perdia
    // el scroll y la consola quedaba pegada abajo.
    const before = entry.term.buffer.active
    const wasAtBottom = before.viewportY >= before.baseY
    const line = before.viewportY
    entry.fit.fit()
    const el = document.getElementById(`term-${key}`)
    const screen = el?.querySelector('.xterm-screen') as HTMLElement | null
    const viewport = el?.querySelector('.xterm-viewport') as HTMLElement | null
    if (screen && viewport && entry.term.cols) {
      // Ancho de celda tal como lo pinta el renderer (ya redondeado al DPR).
      const cell = screen.offsetWidth / entry.term.cols
      if (cell) {
        // Solo se toca si el contenido sobresale del ancho util: asi no se
        // pierde una columna por un simple redondeo cuando ya cabe.
        const overflow = screen.offsetWidth - viewport.clientWidth
        if (overflow > 0) {
          const cols = Math.max(2, entry.term.cols - Math.ceil(overflow / cell))
          entry.term.resize(cols, entry.term.rows)
        }
      }
    }
    if (wasAtBottom) {
      // Si estaba al final, se mantiene ahi tras el reflow.
      entry.term.scrollToBottom()
    } else {
      // Se restaura la misma linea (acotada al nuevo scrollback) para que el
      // usuario siga leyendo donde estaba.
      entry.term.scrollToLine(Math.min(line, entry.term.buffer.active.baseY))
    }
  }

  function resizePty(tab: Tab, entry: { term: Terminal }) {
    if (!tab.ptyId) return
    const sig = `${entry.term.cols}x${entry.term.rows}`
    if (lastPtySize.get(tab.key) === sig) return
    lastPtySize.set(tab.key, sig)
    resizeTerminal(tab.ptyId, entry.term.cols, entry.term.rows)
  }

  // Reajusta la pestaña activa al nuevo tamano del panel, coalesciendo los
  // avisos del ResizeObserver en un frame.
  function scheduleFit() {
    if (fitRaf) return
    fitRaf = requestAnimationFrame(() => {
      fitRaf = 0
      if (activeKey == null) return
      const entry = terms.get(activeKey)
      const tab = activeTab()
      if (!entry) return
      fitTerm(activeKey)
      if (tab) resizePty(tab, entry)
    })
  }

  async function activate(key: number) {
    activeKey = key
    await tick()
    const entry = terms.get(key)
    const tab = tabs.find((t) => t.key === key)
    if (!entry) return
    fitTerm(key)
    if (tab) resizePty(tab, entry)
    entry.term.focus()
  }

  function toggleRightPanel() {
    // El ResizeObserver del stage detecta el nuevo ancho y reajusta xterm/PTY.
    rightCollapsed = !rightCollapsed
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
      entry.term.dispose()
      terms.delete(key)
    }
    mounted.delete(key)
    lastPtySize.delete(key)
    tabs = tabs.filter((t) => t.key !== key)
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
      <div class="cards-area">
        <div class="cards">
          {#each menu.filter((m) => m.section !== 'CFG') as item}
            <Card {item} accent={cardAccent(item)} onOpen={() => openTask(item)} />
          {/each}
        </div>

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
            {#if !visibleTabs.length}
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
        <div class="right-foot" title={moodText}>
          <div class="right-foot-label {mood}">
            <span class="mood-dot"></span>
            <span class="mascot-text">{moodText}</span>
          </div>
          <div class="mascot-track">
            <span class="mascot-runner">
              <Mascot size={40} {mood} walk />
            </span>
          </div>
        </div>
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
