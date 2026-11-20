<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { Terminal } from '@xterm/xterm'
  import { FitAddon } from '@xterm/addon-fit'
  import {
    getWorkspace, selectProject, removeProject, getMenu, runTask,
    onPtyData, onPtyExit, openTerminal, writeTerminal, resizeTerminal, taskScript,
    closeTerminal, focusWindow, getDebug, getThemes, getTheme, setTheme, getSettings, setSettings, listModels,
    subcards, onWorkspaceChanged, pickProject as pickProjectDialog, closeAllTerminals,
  } from './lib/bridge'
  import { notifyRepoChanged } from './lib/repoChanged'
  import { svg } from './lib/icons'
  import Card from './components/Card.svelte'
  import Explorer from './components/Explorer.svelte'
  import Commit from './components/Commit.svelte'
  import History from './components/History.svelte'
  import type { ProjectInfo, MenuItemInfo, WorkspaceInfo, ThemeInfo, Settings, Subcard, ModelOption } from './lib/bridge'

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
  let rightTab = $state<'explorer' | 'commit' | 'log'>('explorer')
  let themeMenu = $state(false)
  let optionsOpen = $state(false)
  let menuTab = $state<'DEV' | 'CFG'>('DEV')
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
  })
  let models = $state<ModelOption[]>([])
  let subMenu = $state<{ title: string; items: Subcard[] } | null>(null)

  let seq = 0
  // Etiqueta por carga de pagina: si la UI se recarga (HMR en desarrollo), los
  // ids de pestana vuelven a empezar en 1 y no deben casar con las sesiones
  // ConPTY de la carga anterior.
  const sessionTag = Math.random().toString(36).slice(2, 8)
  const terms = new Map<number, { term: Terminal; fit: FitAddon }>()
  const mounted = new Set<number>()

  const activeTab = () => tabs.find((t) => t.key === activeKey) ?? null

  // Solo se muestran las pestanas del proyecto activo; las de los demas
  // siguen vivas en `tabs` (y sus xterm montados) pero ocultas.
  let projectPath = $derived(workspace.projects[workspace.active]?.path ?? '')
  let visibleTabs = $derived(tabs.filter((t) => t.project === projectPath))

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
    // Se relee lo guardado: la key vuelve enmascarada y el modal nunca
    // muestra un estado obsoleto de una sesion anterior.
    try {
      settings = await getSettings()
    } catch {
      /* se mantiene lo que ya habia */
    }
    try {
      models = await listModels()
    } catch {
      models = []
    }
    // Si el modelo guardado no es gratuito (o esta vacio), preselecciona el
    // gratuito mas rapido de la lista.
    if (models.length && !models.some((m) => m.id === settings.openrouter_model)) {
      settings.openrouter_model = models[0].id
    }
  }

  async function saveSettings() {
    await setSettings({ ...settings })
    optionsOpen = false
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
    // Cualquier ConPTY de una carga anterior (recarga por HMR o fallo del
    // WebView) ya no tiene pestana que la lea: se cierra para no dejar
    // procesos huerfanos ni mezclar su salida con la de las nuevas.
    closeAllTerminals().catch(() => {})
    onPtyData((d) => {
      const tab = tabs.find((t) => t.ptyId === d.id)
      const entry = tab && terms.get(tab.key)
      if (entry) entry.term.write(b64ToBytes(d.data))
    })
    onPtyExit((d) => {
      const tab = tabs.find((t) => t.ptyId === d.id)
      const entry = tab && terms.get(tab.key)
      if (entry) entry.term.writeln('\r\n\x1b[2m─ proceso terminado ─\x1b[0m')
      if (tab) {
        tab.exited = true
        tab.ptyId = null
      }
      // El comando/tarea pudo tocar el repo (git, builds, generacion de
      // codigo): que el explorador vuelva a leer el estado.
      notifyRepoChanged()
    })
    window.addEventListener('resize', () => {
      const entry = activeKey != null ? terms.get(activeKey) : null
      const tab = activeTab()
      if (!entry) return
      entry.fit.fit()
      if (tab?.ptyId) resizeTerminal(tab.ptyId, entry.term.cols, entry.term.rows)
    })
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
    refresh()
  })

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
      menu = await getMenu()
      const first = tabs.find((t) => t.project === (workspace.projects[workspace.active]?.path ?? ''))
      if (first) await activate(first.key)
      else activeKey = null
    } catch (e) {
      lastError = 'ERROR al abrir el proyecto: ' + (e as Error).message
    }
  }

  async function pickProject(i: number) {
    // No se cierran las pestañas: cada proyecto conserva sus terminales
    // abiertos y se puede volver a ellas al cambiar de proyecto.
    workspace = await selectProject(i)
    menu = await getMenu()
    // Activa la primera pestaña del proyecto recién seleccionado (si tiene).
    const first = tabs.find((t) => t.project === (workspace.projects[workspace.active]?.path ?? ''))
    if (first) await activate(first.key)
    else activeKey = null
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
    entry.fit.fit()
  }

  async function activate(key: number) {
    activeKey = key
    await tick()
    const entry = terms.get(key)
    const tab = tabs.find((t) => t.key === key)
    if (!entry) return
    entry.fit.fit()
    if (tab?.ptyId) resizeTerminal(tab.ptyId, entry.term.cols, entry.term.rows)
    entry.term.focus()
  }

  async function startPty(tab: Tab, task: string) {
    const entry = terms.get(tab.key)
    if (!entry) return
    entry.term.clear()
    entry.fit.fit()
    const script = await taskScript(task)
    // El id se fija ANTES de abrir: la salida de la tarea puede llegar antes
    // de que la promesa resuelva y hay que poder casarla ya.
    const id = `tab-${sessionTag}-${tab.key}`
    tab.ptyId = id
    tab.exited = false
    const opened = await openTerminal(id, script, entry.term.rows, entry.term.cols)
    if (!opened) {
      tab.ptyId = null
      tab.exited = true
      lastError = 'ERROR: no se pudo abrir la ConPTY'
      return
    }
    lastError = ''
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
    await runNamedTask(sub.label, sub.task)
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
      <span class="logo" aria-hidden="true">{@html svg('compass', 19)}</span>
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
  </aside>

  <main class="main">
    {#if lastError}<div class="debug-line">{lastError}</div>{/if}
    <header class="topbar">
      <div class="tabs">
        <button class:active={menuTab === 'DEV'} onclick={() => (menuTab = 'DEV')}>Inicio</button>
        <button class:active={menuTab === 'CFG'} onclick={() => (menuTab = 'CFG')}>Config</button>
      </div>
      <div class="topbar-actions">
        <button class="theme-btn" onclick={() => (themeMenu = !themeMenu)}>◈ {theme?.name ?? 'Tema'} ▾</button>
        <button class="theme-btn" onclick={openOptions}>⚙ Opciones</button>
      </div>
    </header>

    <section class="content">
      <div class="cards-area">
        <div class="cards">
          {#each menu.filter((m) => (menuTab === 'CFG' ? m.section === 'CFG' : m.section !== 'CFG')) as item}
            <Card {item} accent={cardAccent(item)} onOpen={() => openTask(item)} />
          {/each}
        </div>

        <div class="terminal-panel">
          {#if visibleTabs.length}
            <div class="term-tabs">
              {#each visibleTabs as tab}
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
            </div>
          {/if}
          <div class="term-stage">
            {#each tabs as tab}
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

      <aside class="right">
        <div class="right-tabs">
          <button class:active={rightTab === 'explorer'} onclick={() => (rightTab = 'explorer')}>Explorador</button>
          <button class:active={rightTab === 'commit'} onclick={() => (rightTab = 'commit')}>Commit</button>
          <button class:active={rightTab === 'log'} onclick={() => (rightTab = 'log')}>Historial</button>
        </div>
        <div class="right-body">
          {#if rightTab === 'explorer'}
            {#key workspace.active}
              <Explorer />
            {/key}
          {:else if rightTab === 'commit'}
            <Commit projectPath={projectPath} />
          {:else}
            {#key workspace.active}
              <History />
            {/key}
          {/if}
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
          <button class="sub-card" onclick={() => pickSubcard(sub)}>
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
    <div class="modal" onclick={(e) => e.stopPropagation()} role="dialog" aria-label="Opciones">
      <div class="modal-head"><span>Opciones</span><button onclick={() => (optionsOpen = false)}>×</button></div>
      <label class="opt-row">
        <input type="checkbox" bind:checked={settings.open_output_dir} />
        <span>Abrir la carpeta de salida al terminar un build</span>
      </label>
      <label class="opt-row">
        <input type="checkbox" bind:checked={settings.close_tab_on_build_end} />
        <span>Cerrar la pestaña del terminal al terminar un build <small>(cuenta atrás de 10 s)</small></span>
      </label>
      <label class="opt-row column">
        <span>Endpoint de OpenRouter</span>
        <input class="opt-input" type="text" bind:value={settings.openrouter_endpoint} placeholder="https://openrouter.ai/api/v1" />
      </label>
      <label class="opt-row column">
        <span>Modelo para los commits <small>(gratuitos · los más rápidos primero)</small></span>
        <select class="opt-input" bind:value={settings.openrouter_model}>
          {#if settings.openrouter_model && !models.some((m) => m.id === settings.openrouter_model)}
            <option value={settings.openrouter_model}>{settings.openrouter_model} (actual)</option>
          {/if}
          {#each models as m}
            <option value={m.id}>{m.name} — {m.id}</option>
          {/each}
          {#if !models.length}
            <option value="">(no se pudieron cargar los modelos)</option>
          {/if}
        </select>
      </label>
      <label class="opt-row column">
        <span>API key de OpenRouter <small>{settings.openrouter_api_key === '********'
          ? '(guardada · escribe otra para cambiarla)'
          : '(se guarda en settings.json)'}</small></span>
        <input class="opt-input" type="password" bind:value={settings.openrouter_api_key} placeholder="********" autocomplete="off" />
      </label>
      <div class="modal-actions">
        <button class="btn-primary" onclick={saveSettings}>Guardar</button>
      </div>
    </div>
  </div>
{/if}
