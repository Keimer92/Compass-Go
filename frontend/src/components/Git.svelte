<script lang="ts">
  import { onMount } from 'svelte'
  import {
    gitPush,
    gitSync,
    gitSetRemote,
    gitInitRepo,
    gitRemoteStatus,
    gitGhInfo,
    gitCreateRemote,
    openExternal,
    setAutoCommit,
    autoCommitStatus,
    onAutoCommit,
    onGitChanged,
    onPushProgress,
    type GitInfo,
    type AutoCommitState,
    type RemoteStatus,
    type GhInfo,
    type PushProgress,
  } from '../lib/bridge'
  import { draftFor, runFor, setMessage, generateFor, cancelFor, commitFor } from '../lib/commitStore.svelte'
  import { onRepoChanged, notifyRepoChanged } from '../lib/repoChanged'
  import { emptyGitInfo, peekGitInfo, loadGitInfo } from '../lib/gitCache'
  import { svg } from '../lib/icons'

  let { projectPath }: { projectPath: string } = $props()

  // Si ya se visito esta pestana antes, se arranca con los datos cacheados:
  // sin "Leyendo…" ni parpadeo de "no es un repositorio".
  const cached = peekGitInfo(projectPath)
  let info = $state<GitInfo>(cached ?? emptyGitInfo())
  let refreshing = $state(false)
  let refreshQueued = false
  // `loaded` solo pasa a true con una respuesta real de Go: mientras tanto no
  // se puede afirmar que la carpeta no sea un repo.
  let loaded = $state(cached !== null)
  let busy = $state(false)
  // Que accion esta en curso, para poner el spinner solo en su boton.
  let action = $state<'' | 'push' | 'sync'>('')
  let error = $state('')
  let notice = $state('')
  let connectOpen = $state(false)
  let remoteInput = $state('')
  let auto = $state<AutoCommitState>({
    enabled: false, idleMin: 10, pending: false, idleSec: 0, busy: false, lastMsg: '', lastError: ''
  })
  let autoBusy = $state(false)

  // Estado del remoto: se comprueba con `git ls-remote` (una sola vez por
  // proyecto+URL cada pocos minutos) para detectar repositorios borrados.
  let remote = $state<RemoteStatus>({ state: 'none', detail: '', url: '' })
  let remoteKey = ''
  let remoteAt = 0

  // Asistente para vincular otro repositorio cuando el actual ya no existe.
  let wizardOpen = $state(false)
  let wizStep = $state<'choose' | 'create' | 'link'>('choose')
  let wizBusy = $state(false)
  let wizError = $state('')
  let wizPush = $state(true)
  let gh = $state<GhInfo>({ available: false, account: '' })
  let newName = $state('')
  let newPrivate = $state(true)
  let linkUrl = $state('')
  // Progreso del push del asistente. Lo emite Go mientras `git push
  // --progress` trabaja; `wizStatus` cubre las fases que no reportan avance
  // (crear el repositorio con gh).
  let pushProg = $state<PushProgress | null>(null)
  let wizStatus = $state('')

  // El borrador del commit vive en el store, como en la antigua pestaña:
  // sobrevive al cambio de pestaña y a los refrescos del panel.
  let draft = $derived(draftFor(projectPath))
  let state = $derived(runFor(projectPath))

  async function refresh() {
    // Si llega un aviso mientras ya se esta consultando (guardar y guardar
    // otra vez), se encola UNA repeticion: descartarlo dejaria el panel con
    // el estado de la primera consulta.
    if (refreshing) {
      refreshQueued = true
      return
    }
    refreshing = true
    try {
      info = await loadGitInfo(projectPath)
      error = ''
      loaded = true
      void checkRemote()
    } catch (e) {
      error = (e as Error).message
    } finally {
      refreshing = false
      if (refreshQueued) {
        refreshQueued = false
        void refresh()
      }
    }
  }

  // checkRemote consulta al remoto real. Se cachea por proyecto+URL unos
  // minutos: refresh() corre con cada guardado de archivo y no conviene lanzar
  // una peticion de red cada vez.
  async function checkRemote(force = false) {
    if (!info.remoteUrl) {
      remote = { state: 'none', detail: '', url: '' }
      return
    }
    const key = `${projectPath}|${info.remoteUrl}`
    if (!force && key === remoteKey && Date.now() - remoteAt < 5 * 60_000) return
    remoteKey = key
    remoteAt = Date.now()
    try {
      const st = await gitRemoteStatus()
      // Descarta la respuesta si el remoto cambio mientras se comprobaba.
      if (st.url === info.remoteUrl) remote = st
    } catch {
      /* la comprobacion es informativa: no rompe el panel */
    }
  }

  function openWizard() {
    wizStep = 'choose'
    wizError = ''
    wizBusy = false
    wizPush = true
    newPrivate = true
    pushProg = null
    wizStatus = ''
    const base = projectPath.split(/[\\/]/).filter(Boolean).pop() ?? 'mi-repo'
    newName = base.replace(/[^A-Za-z0-9._-]+/g, '-')
    linkUrl = ''
    wizardOpen = true
    gitGhInfo().then((g) => (gh = g)).catch(() => (gh = { available: false, account: '' }))
  }

  function closeWizard() {
    if (wizBusy) return
    wizardOpen = false
    pushProg = null
    wizStatus = ''
  }

  async function wizardCreate() {
    wizBusy = true
    wizError = ''
    pushProg = null
    try {
      wizStatus = 'Creando el repositorio en GitHub…'
      let msg = await gitCreateRemote(newName.trim(), newPrivate)
      if (wizPush) {
        wizStatus = 'Preparando la subida…'
        try {
          msg += '\n' + (await gitPush())
        } catch (e) {
          msg += '\nNo se pudo subir todavía: ' + (e as Error).message
        }
      }
      wizardOpen = false
      done(msg)
    } catch (e) {
      wizError = (e as Error).message
    } finally {
      wizBusy = false
      wizStatus = ''
    }
  }

  async function wizardLink() {
    wizBusy = true
    wizError = ''
    pushProg = null
    try {
      const url = linkUrl.trim()
      if (!url) {
        wizError = 'Escribe la URL del repositorio.'
        return
      }
      // Verifica que el destino exista antes de tocar la configuracion.
      wizStatus = 'Comprobando el repositorio…'
      const st = await gitRemoteStatus(url)
      if (st.state === 'missing' || st.state === 'none' || st.state === 'error') {
        wizError = st.detail || 'Ese repositorio no existe o no es accesible.'
        return
      }
      let msg = await gitSetRemote(url)
      if (wizPush) {
        wizStatus = 'Preparando la subida…'
        try {
          msg += '\n' + (await gitPush())
        } catch (e) {
          msg += '\nNo se pudo subir todavía: ' + (e as Error).message
        }
      }
      wizardOpen = false
      done(msg)
    } catch (e) {
      wizError = (e as Error).message
    } finally {
      wizBusy = false
      wizStatus = ''
    }
  }

  async function loadAuto() {
    try {
      auto = await autoCommitStatus()
    } catch {
      // El estado del vigilante es informativo: un fallo aqui no rompe el panel.
    }
  }

  async function toggleAuto() {
    autoBusy = true
    try {
      await setAutoCommit(!auto.enabled)
      await loadAuto()
      error = ''
    } catch (e) {
      fail(e)
    } finally {
      autoBusy = false
    }
  }

  function fmtSecs(s: number) {
    if (s < 60) return `${s} s`
    const m = Math.floor(s / 60)
    if (m < 60) return `${m} min ${s % 60} s`
    return `${Math.floor(m / 60)} h ${m % 60} min`
  }

  // Rutas de Windows: comparar sin distinguir mayusculas ni el tipo de barra.
  function samePath(a: string, b: string) {
    if (!a || !b) return false
    return a.replace(/\\/g, '/').toLowerCase() === b.replace(/\\/g, '/').toLowerCase()
  }

  onMount(() => {
    refresh()
    loadAuto()
    const offRepo = onRepoChanged(refresh)
    const offAuto = onAutoCommit(() => {
      refresh()
      loadAuto()
    })
    // Aviso instantaneo del vigilante de archivos: el panel se repinta en
    // cuanto se guarda algo, sin esperar al sondeo de 5 s.
    const offChanged = onGitChanged((e) => {
      if (!samePath(e.dir, projectPath)) return
      refresh()
      loadAuto()
    })
    // Progreso del push: solo interesa mientras el asistente esta abierto
    // (los push del panel se lanzan desde ahi mismo o desde Sincronizar).
    const offPush = onPushProgress((p) => {
      if (!wizardOpen) return
      pushProg = p
    })
    const timer = setInterval(loadAuto, 5000)
    return () => {
      offRepo()
      offAuto()
      offChanged()
      offPush()
      clearInterval(timer)
    }
  })

  function done(msg: string) {
    notice = msg
    error = ''
    connectOpen = false
    remoteInput = ''
    notifyRepoChanged()
    refresh().then(() => checkRemote(true))
  }

  function fail(e: unknown) {
    error = (e as Error).message
    notice = ''
  }

  async function initRepo() {
    busy = true
    try { done(await gitInitRepo()) } catch (e) { fail(e) } finally { busy = false }
  }

  async function connectRemote() {
    busy = true
    try { done(await gitSetRemote(remoteInput)) } catch (e) { fail(e) } finally { busy = false }
  }

  async function push() {
    busy = true
    action = 'push'
    try { done(await gitPush()) } catch (e) { fail(e) } finally { busy = false; action = '' }
  }

  async function sync() {
    busy = true
    action = 'sync'
    try {
      // Pull --rebase + push en una sola llamada al backend: comparte la
      // deteccion de GitInfo y evita un viaje extra por la red de Wails.
      done(await gitSync())
    } catch (e) { fail(e) } finally { busy = false; action = '' }
  }

  function openWeb() {
    if (!info.webUrl) return
    openExternal(info.webUrl).catch(fail)
  }

  let canPush = $derived(
    info.isRepo && info.hasCommits && !!info.remoteUrl && remote.state !== 'missing' &&
    !info.branch.startsWith('detached') && !busy
  )
  let canSync = $derived(canPush && !!info.upstream)

  // ── Progreso del push del asistente ──
  // En MB (la unidad que se pidio) cuando hay volumen suficiente; en KB/B para
  // pushes diminutos, donde "0.0 MB de 0.0 MB" no diria nada.
  function humanBytes(bytes: number) {
    const n = Math.max(0, bytes)
    if (n >= 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`
    if (n >= 1024) return `${Math.round(n / 1024)} KB`
    return `${Math.round(n)} B`
  }
  // Con total conocido la barra es determinada; si no, queda animada.
  let pushKnown = $derived(!!pushProg && pushProg.total > 0)
  let pushPct = $derived(pushProg ? Math.max(0, Math.min(100, pushProg.percent)) : 0)
  let pushLeft = $derived(pushKnown ? Math.max(0, (pushProg as PushProgress).total - (pushProg as PushProgress).uploaded) : 0)
  let pushLabel = $derived.by(() => {
    if (!wizBusy) return ''
    if (!pushProg) return wizStatus
    switch (pushProg.stage) {
      case 'counting': return 'Preparando los objetos…'
      case 'compressing': return 'Comprimiendo…'
      case 'writing':
        return pushProg.speed > 0 ? `Subiendo · ${humanBytes(pushProg.speed)}/s` : 'Subiendo…'
      case 'resolving': return 'Confirmando en GitHub…'
      case 'done': return 'Subida completa'
    }
    return wizStatus
  })
</script>

<div class="head">
  <span>GIT</span>
  <div class="head-actions">
    <button
      class="auto-btn"
      class:on={auto.enabled}
      onclick={toggleAuto}
      disabled={autoBusy || !info.isRepo}
      title={auto.enabled
        ? `Commit automático activo: commitea tras ${auto.idleMin} min sin cambios`
        : 'Commit automático tras un rato sin cambios'}
    >Auto</button>
    <button
      class="refresh"
      class:spin={refreshing}
      onclick={refresh}
      disabled={refreshing}
      title="Refrescar estado de git"
      aria-label="Refrescar estado de git"
    >{@html svg('reload', 14)}</button>
  </div>
</div>
{#if error}<div class="msg error">{error}</div>{/if}
{#if notice}<div class="msg ok">{notice}</div>{/if}
{#if auto.enabled}
  <div class="auto-line">
    {#if auto.busy}
      Commiteando cambios…
    {:else if auto.pending}
      Quieto hace {fmtSecs(auto.idleSec)} · commitea a los {auto.idleMin} min
    {:else}
      Vigilando cambios…
    {/if}
  </div>
  {#if auto.lastMsg}<div class="auto-last">Último automático: {auto.lastMsg}</div>{/if}
  {#if auto.lastError}<div class="auto-err">{auto.lastError}</div>{/if}
{/if}

{#if !loaded}
  <div class="loading">
    {#if error}
      <p>No se pudo leer el estado de git.</p>
      <button class="pill ghost" onclick={refresh}>Reintentar</button>
    {:else}
      Leyendo el estado de git…
    {/if}
  </div>
{:else if !info.isRepo}
  <div class="empty">
    <div class="empty-ico">{@html svg('git', 26)}</div>
    <p>Esta carpeta aún no es un repositorio.</p>
    <button class="pill primary" onclick={initRepo} disabled={busy}>
      {busy ? 'Inicializando...' : 'Inicializar repositorio'}
    </button>
    <span class="hint">Crea la rama main y hace el primer commit.</span>
  </div>
{:else}
  {#if info.branch}
    <div class="branch" title="Rama actual">
      <span class="branch-ico">{@html svg('branch', 13)}</span>
      <span class="branch-name">{info.branch}</span>
      {#if info.ahead > 0}<span class="ahead" title="Commits por subir">↑{info.ahead}</span>{/if}
      {#if info.behind > 0}<span class="behind" title="Commits por bajar">↓{info.behind}</span>{/if}
    </div>
    {#if info.upstream}
      <div class="upstream" title="Rama de seguimiento">{info.upstream}</div>
    {/if}
  {/if}

  <div class="section">REMOTO</div>
  {#if info.remoteUrl}
    <div class="remote">
      <div class="remote-row">
        <span class="host">{info.isGitHub ? 'GitHub' : 'Remoto'}</span>
        <div class="remote-actions">
          {#if info.webUrl}
            <button class="link" onclick={openWeb} title="Abrir en el navegador">Abrir ↗</button>
          {/if}
          <button class="link" onclick={openWizard} title="Vincular otro repositorio">Cambiar</button>
        </div>
      </div>
      <div class="remote-url" title={info.remoteUrl}>{info.remoteUrl}</div>
      {#if remote.state === 'missing' || remote.state === 'auth' || remote.state === 'unverifiable'}
        <div class="remote-warn">
          {remote.detail}
          <button class="warn-cta" onclick={openWizard}>Vincular otro repositorio</button>
        </div>
      {/if}
    </div>
  {:else if connectOpen}
    <div class="connect">
      <input
        class="url-input"
        placeholder="https://github.com/usuario/repo.git"
        bind:value={remoteInput}
        onkeydown={(e) => { if (e.key === 'Enter') connectRemote(); if (e.key === 'Escape') connectOpen = false }}
      />
      <div class="connect-actions">
        <button class="pill ghost" onclick={() => (connectOpen = false)} disabled={busy}>Cancelar</button>
        <button class="pill primary" onclick={connectRemote} disabled={busy || !remoteInput.trim()}>
          {busy ? 'Guardando...' : 'Conectar'}
        </button>
      </div>
    </div>
  {:else}
    <button class="pill ghost connect-btn" onclick={() => (connectOpen = true)}>
      Conectar con remoto
    </button>
  {/if}

  {#if info.hasCommits}
    <div class="section">CAMBIOS</div>
    <div class="changes-line">
      {info.changes === 0 ? 'Sin cambios pendientes.' : `${info.changes} cambio${info.changes === 1 ? '' : 's'} pendiente${info.changes === 1 ? '' : 's'}.`}
    </div>
    <div class="actions">
      <button class="pill primary" onclick={push} disabled={!canPush} title={remote.state === 'missing' ? 'El remoto ya no existe: vincula otro repositorio' : info.remoteUrl ? 'Subir rama actual' : 'Configura un remoto primero'}>
        {#if action === 'push'}<span class="spin" aria-hidden="true"></span>{/if}
        {action === 'push' ? 'Subiendo…' : 'Push'}
      </button>
      <button class="pill ghost" onclick={sync} disabled={!canSync} title={info.upstream ? 'Pull --rebase + Push' : 'La rama no tiene upstream'}>
        {#if action === 'sync'}<span class="spin" aria-hidden="true"></span>{/if}
        {action === 'sync' ? 'Sincronizando…' : 'Sincronizar'}
      </button>
    </div>
    {#if action === 'sync'}
      <div class="sync-line">Pull --rebase y push con {info.upstream || 'el remoto'}…</div>
    {:else if action === 'push'}
      <div class="sync-line">Subiendo {info.branch || 'la rama'} a {info.upstream || 'origin'}…</div>
    {/if}
  {/if}

  <div class="commit-box">
    <div class="commit-head">Commit con IA</div>
    <div class="commit-body">
        <div class="commit-input">
          <textarea
            placeholder="Mensaje del commit..."
            value={draft.message}
            oninput={(e) => setMessage(projectPath, e.currentTarget.value)}
          ></textarea>
          {#if draft.message}
            <button
              class="clear-msg"
              onclick={() => setMessage(projectPath, '')}
              title="Limpiar el mensaje"
              aria-label="Limpiar el mensaje"
            >×</button>
          {/if}
        </div>
        <div class="commit-actions">
          <button
            class="pill ghost"
            onclick={() => generateFor(projectPath)}
            disabled={state.loading || !!draft.message.trim()}
          >
            {state.loading ? 'Generando...' : 'Generar'}
          </button>
          <button
            class="pill primary"
            onclick={() => commitFor(projectPath)}
            disabled={state.committing || !draft.message.trim()}
          >
            {state.committing ? 'Commiteando...' : 'Commit'}
          </button>
        </div>
        {#if state.loading}
          <div class="gen-row">
            <div class="progress"><div class="bar"></div></div>
            <span class="elapsed">{(state.elapsedMs / 1000).toFixed(1)} s</span>
          </div>
          <button class="pill ghost" onclick={() => cancelFor(projectPath)}>Cancelar</button>
        {:else if draft.lastMs > 0}
          <div class="elapsed done">Última: {(draft.lastMs / 1000).toFixed(1)} s</div>
        {/if}
        {#if state.error}<div class="msg error">{state.error}</div>{/if}
        {#if state.ok}<div class="msg ok">{state.ok}</div>{/if}
        <div class="hint">Se añadirán todos los cambios (git add -A).</div>
      </div>
    </div>
{/if}

{#if wizardOpen}
  <div class="wiz-backdrop" onclick={closeWizard} role="presentation">
    <div class="wiz" tabindex="-1" onclick={(e) => e.stopPropagation()} role="dialog" aria-label="Vincular repositorio">
      <div class="wiz-head">
        <span>Vincular repositorio</span>
        <button class="wiz-x" onclick={closeWizard} disabled={wizBusy} aria-label="Cerrar">×</button>
      </div>

      {#if wizStep === 'choose'}
        <p class="wiz-text">
          {remote.state === 'missing'
            ? 'El remoto configurado ya no existe en GitHub. Elige cómo continuar:'
            : 'Elige el nuevo destino para este repositorio:'}
        </p>
        {#if remote.state === 'unverifiable'}
          <p class="wiz-hint">No se pudo confirmar el remoto actual (GitHub devolvió 404). Puedes seguir usando este repositorio o vincular otro.</p>
        {/if}
        <button class="wiz-opt" onclick={() => (wizStep = 'create')} disabled={!gh.available}>
          <span class="wiz-opt-title">Crear uno nuevo en GitHub</span>
          <span class="wiz-opt-sub">
            {gh.available
              ? (gh.account ? `Se creará en la cuenta ${gh.account}` : 'Usa el CLI de GitHub (gh)')
              : 'Requiere el CLI de GitHub (gh) instalado'}
          </span>
        </button>
        <button class="wiz-opt" onclick={() => (wizStep = 'link')}>
          <span class="wiz-opt-title">Vincular uno existente</span>
          <span class="wiz-opt-sub">Pega la URL del repositorio de GitHub</span>
        </button>
        {#if !gh.available}
          <p class="wiz-hint">
            Para crear repositorios desde aquí instala <code>gh</code> y ejecuta
            <code>gh auth login</code>.
          </p>
        {/if}
      {:else if wizStep === 'create'}
        <p class="wiz-text">Se creará el repositorio y se enlazará como <b>origin</b>.</p>
        <label class="wiz-col">
          <span class="wiz-label">Nombre</span>
          <input class="wiz-input" bind:value={newName} placeholder="mi-proyecto" />
        </label>
        <div class="wiz-col">
          <span class="wiz-label">Visibilidad</span>
          <div class="wiz-seg">
            <button class:on={newPrivate} onclick={() => (newPrivate = true)}>Privado</button>
            <button class:on={!newPrivate} onclick={() => (newPrivate = false)}>Público</button>
          </div>
        </div>
        <label class="wiz-check">
          <input type="checkbox" bind:checked={wizPush} />
          <span>Subir la rama actual al terminar</span>
        </label>
      {:else}
        <p class="wiz-text">Pega la URL del repositorio al que quieres enlazar este proyecto.</p>
        <label class="wiz-col">
          <span class="wiz-label">URL del remoto</span>
          <input
            class="wiz-input"
            bind:value={linkUrl}
            placeholder="https://github.com/usuario/repo.git"
            onkeydown={(e) => { if (e.key === 'Enter') wizardLink() }}
          />
        </label>
        <label class="wiz-check">
          <input type="checkbox" bind:checked={wizPush} />
          <span>Subir la rama actual al terminar</span>
        </label>
        <p class="wiz-hint">Se comprobará que el repositorio exista antes de enlazarlo.</p>
      {/if}

      {#if wizError}<div class="wiz-err">{wizError}</div>{/if}

      {#if wizBusy}
        <div class="wiz-progress" aria-live="polite">
          <div class="wiz-bar" class:indet={!pushKnown}>
            <div class="wiz-fill" style={`width:${pushKnown ? pushPct : 35}%`}></div>
          </div>
          <div class="wiz-meta">
            <span>{pushLabel}</span>
            {#if pushProg && (pushProg.uploaded > 0 || pushKnown)}
              <span class="wiz-bytes">
                {humanBytes(pushProg.uploaded)} subidos{#if pushKnown} de {humanBytes(pushProg.total)}{/if}{#if pushLeft > 0} · faltan {humanBytes(pushLeft)}{/if}
              </span>
            {/if}
          </div>
        </div>
      {/if}

      <div class="wiz-actions">
        {#if wizStep === 'choose'}
          <button class="pill ghost" onclick={closeWizard} disabled={wizBusy}>Cancelar</button>
        {:else}
          <button class="pill ghost" onclick={() => { wizStep = 'choose'; wizError = '' }} disabled={wizBusy}>Atrás</button>
          {#if wizStep === 'create'}
            <button class="pill primary" onclick={wizardCreate} disabled={wizBusy || !newName.trim()}>
              {wizBusy ? 'Creando...' : 'Crear y vincular'}
            </button>
          {:else}
            <button class="pill primary" onclick={wizardLink} disabled={wizBusy || !linkUrl.trim()}>
              {wizBusy ? 'Vinculando...' : 'Vincular'}
            </button>
          {/if}
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  .head { display:flex; align-items:center; justify-content:space-between; padding:14px 14px 8px; color:var(--accent); font-weight:700; font-size:11px; letter-spacing:.08em; }
  .head-actions { display:flex; align-items:center; gap:6px; }
  .auto-btn { padding:0 10px; height:26px; border:1px solid var(--border); border-radius:8px; background:color-mix(in srgb, var(--panel) 50%, transparent); color:var(--muted); font:inherit; font-size:10px; font-weight:700; letter-spacing:.06em; cursor:pointer; transition:color .15s ease, border-color .15s ease, background .15s ease; }
  .auto-btn:hover:not(:disabled) { color:var(--text); border-color:color-mix(in srgb, var(--accent) 55%, var(--border)); }
  .auto-btn.on { color:var(--text); background:color-mix(in srgb, var(--green) 30%, var(--bg)); border-color:color-mix(in srgb, var(--green) 55%, var(--border)); }
  .auto-btn:disabled { opacity:.45; cursor:default; }
  .auto-line { margin:0 14px 4px; font-size:10px; color:var(--green); font-variant-numeric:tabular-nums; }
  .auto-last, .auto-err { margin:0 14px 4px; font-size:10px; color:var(--muted); white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
  .auto-err { color:var(--yellow); }
  .loading { padding:26px 18px; text-align:center; color:var(--muted); font-size:11px; }
  .loading p { margin:0 0 8px; }
  .refresh { display:grid; place-items:center; width:26px; height:26px; padding:0; border:1px solid var(--border); border-radius:8px; background:color-mix(in srgb, var(--panel) 50%, transparent); color:var(--muted); cursor:pointer; }
  .refresh:hover:not(:disabled) { color:var(--accent); border-color:color-mix(in srgb, var(--accent) 55%, var(--border)); }
  .refresh:disabled { opacity:.65; cursor:default; }
  .refresh.spin :global(svg) { animation:spin .8s linear infinite; }
  @keyframes spin { to { transform:rotate(360deg); } }
  .msg { margin:2px 14px 6px; padding:6px 9px; border-radius:8px; font-size:11px; white-space:pre-wrap; word-break:break-word; }
  .msg.error { color:var(--red); background:color-mix(in srgb, var(--red) 12%, transparent); }
  .msg.ok { color:var(--green); background:color-mix(in srgb, var(--green) 12%, transparent); }
  .section { padding:12px 14px 6px; color:var(--muted); font-size:10px; letter-spacing:.08em; }
  .branch { display:flex; align-items:center; gap:7px; margin:2px 12px 0; padding:5px 9px; border-radius:9px; background:color-mix(in srgb, var(--brand) 14%, transparent); border:1px solid color-mix(in srgb, var(--brand) 28%, transparent); color:var(--text); font-size:11px; }
  .branch-ico { display:grid; place-items:center; color:var(--brand); flex-shrink:0; }
  .branch-name { flex:1; font-family:'Cascadia Mono', Consolas, monospace; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
  .ahead, .behind { padding:1px 6px; border-radius:9px; font-size:10px; font-variant-numeric:tabular-nums; flex-shrink:0; }
  .ahead { color:var(--green); background:color-mix(in srgb, var(--green) 18%, transparent); }
  .behind { color:var(--yellow); background:color-mix(in srgb, var(--yellow) 18%, transparent); }
  .upstream { margin:4px 14px 0; color:var(--muted); font-size:10px; font-family:'Cascadia Mono', Consolas, monospace; }
  .remote { margin:0 12px; padding:8px 10px; border:1px solid var(--border); border-radius:9px; background:color-mix(in srgb, var(--panel) 45%, transparent); }
  .remote-row { display:flex; align-items:center; justify-content:space-between; }
  .host { color:var(--brand); font-weight:700; font-size:11px; }
  .link { border:none; background:none; color:var(--accent); font-size:11px; cursor:pointer; }
  .link:hover { text-decoration:underline; }
  .remote-url { margin-top:3px; color:var(--muted); font-size:10px; font-family:'Cascadia Mono', Consolas, monospace; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
  .connect { margin:0 12px; }
  .url-input { width:100%; box-sizing:border-box; padding:7px 9px; border:1px solid var(--border); border-radius:8px; background:var(--panel); color:var(--text); font:inherit; font-size:11px; outline:none; }
  .url-input:focus { border-color:var(--accent); }
  .connect-actions, .actions, .commit-actions { display:flex; gap:8px; margin-top:8px; }
  .connect-btn { margin:0 12px; }
  .changes-line { margin:0 14px; color:var(--text); font-size:11px; }
  .pill { padding:7px 12px; border:none; border-radius:14px; font:inherit; font-size:11px; cursor:pointer; }
  .pill.primary { background:color-mix(in srgb, var(--accent) 40%, var(--bg)); color:var(--text); }
  .pill.primary:hover:not(:disabled) { background:color-mix(in srgb, var(--accent) 60%, var(--bg)); }
  .pill.ghost { background:color-mix(in srgb, var(--border) 55%, transparent); color:var(--text); }
  .pill:disabled { opacity:.5; cursor:default; }
  .actions .pill, .commit-actions .pill { flex:1; }
  .actions .pill { display:inline-flex; align-items:center; justify-content:center; gap:6px; }
  .spin { flex:0 0 auto; width:11px; height:11px; border:2px solid currentColor; border-right-color:transparent; border-radius:50%; animation:spin .7s linear infinite; }
  @keyframes spin { to { transform:rotate(360deg); } }
  @media (prefers-reduced-motion: reduce) { .spin { animation-duration:1.8s; } }
  .sync-line { margin-top:6px; font-size:10px; color:var(--muted); }
  .empty { padding:26px 18px; text-align:center; color:var(--text); }
  .empty-ico { color:var(--brand); }
  .empty p { font-size:12px; line-height:1.5; }
  .empty .pill { margin-top:4px; }
  .hint { display:block; margin-top:8px; color:var(--muted); font-size:10px; line-height:1.4; }
  .commit-box { margin:14px 12px 4px; border:1px solid var(--border); border-radius:10px; background:color-mix(in srgb, var(--panel) 40%, transparent); }
  .commit-head { padding:9px 11px; color:var(--accent); font-size:11px; font-weight:700; }
  .commit-body { padding:0 11px 11px; }
  .commit-input { position:relative; }
  .commit-input textarea { padding-right:26px; }
  .clear-msg { position:absolute; top:4px; right:4px; width:18px; height:18px; display:flex; align-items:center; justify-content:center; border:none; border-radius:50%; background:color-mix(in srgb, var(--border) 70%, transparent); color:var(--muted); font-size:13px; line-height:1; cursor:pointer; padding:0; }
  .clear-msg:hover { background:color-mix(in srgb, var(--accent) 45%, var(--bg)); color:var(--text); }
  textarea { width:100%; box-sizing:border-box; height:82px; resize:vertical; background:var(--panel); color:var(--text); border:1px solid var(--border); border-radius:8px; padding:8px; font:inherit; font-size:11px; outline:none; }
  textarea:focus { border-color:var(--accent); }
  .gen-row { display:flex; align-items:center; gap:8px; margin-top:8px; }
  .progress { flex:1; height:4px; border-radius:2px; background:color-mix(in srgb, var(--border) 55%, transparent); overflow:hidden; }
  .progress .bar { width:35%; height:100%; background:var(--accent); animation:indeterminate 1.1s ease-in-out infinite; }
  @keyframes indeterminate { 0% { transform:translateX(-110%); } 100% { transform:translateX(320%); } }
  .elapsed { font-size:10px; color:var(--muted); font-variant-numeric:tabular-nums; }
  .elapsed.done { display:block; margin-top:6px; }

  .remote-actions { display:flex; align-items:center; gap:8px; }
  .remote-warn { margin-top:7px; padding:6px 8px; border-radius:7px; background:color-mix(in srgb, var(--yellow) 13%, transparent); color:var(--yellow); font-size:10px; line-height:1.45; }
  .warn-cta { display:block; margin-top:5px; padding:0; border:none; background:none; color:var(--accent); font:inherit; font-size:10px; font-weight:700; text-decoration:underline; cursor:pointer; }

  .wiz-backdrop { position:fixed; inset:0; z-index:60; display:grid; place-items:center; background:rgba(0,0,0,.6); }
  .wiz { width:400px; max-width:92vw; max-height:88vh; overflow-y:auto; box-sizing:border-box; padding:16px; border:1px solid var(--border); border-radius:var(--r-lg); background:var(--bg); box-shadow:var(--shadow-3); }
  .wiz-head { display:flex; align-items:center; justify-content:space-between; margin-bottom:10px; color:var(--accent); font-size:12px; font-weight:700; }
  .wiz-x { width:22px; height:22px; padding:0; border:none; border-radius:6px; background:none; color:var(--muted); font-size:16px; line-height:1; cursor:pointer; }
  .wiz-x:hover:not(:disabled) { color:var(--text); background:color-mix(in srgb, var(--border) 60%, transparent); }
  .wiz-text { margin:0 0 12px; color:var(--text); font-size:11px; line-height:1.5; }
  .wiz-opt { display:flex; flex-direction:column; gap:3px; width:100%; box-sizing:border-box; margin-bottom:8px; padding:10px 11px; border:1px solid var(--border); border-radius:10px; background:color-mix(in srgb, var(--panel) 45%, transparent); color:var(--text); font:inherit; text-align:left; cursor:pointer; }
  .wiz-opt:hover:not(:disabled) { border-color:color-mix(in srgb, var(--accent) 55%, var(--border)); background:color-mix(in srgb, var(--accent) 10%, transparent); }
  .wiz-opt:disabled { opacity:.5; cursor:default; }
  .wiz-opt-title { font-size:11px; font-weight:700; }
  .wiz-opt-sub { color:var(--muted); font-size:10px; }
  .wiz-col { display:flex; flex-direction:column; gap:4px; margin-bottom:10px; }
  .wiz-label { color:var(--muted); font-size:10px; }
  .wiz-input { width:100%; box-sizing:border-box; padding:7px 9px; border:1px solid var(--border); border-radius:8px; background:var(--panel); color:var(--text); font:inherit; font-size:11px; outline:none; }
  .wiz-input:focus { border-color:var(--accent); }
  .wiz-seg { display:flex; gap:6px; }
  .wiz-seg button { flex:1; padding:6px 8px; border:1px solid var(--border); border-radius:8px; background:color-mix(in srgb, var(--panel) 45%, transparent); color:var(--muted); font:inherit; font-size:11px; cursor:pointer; }
  .wiz-seg button.on { border-color:color-mix(in srgb, var(--accent) 60%, var(--border)); background:color-mix(in srgb, var(--accent) 22%, transparent); color:var(--text); }
  .wiz-check { display:flex; align-items:center; gap:7px; margin-bottom:10px; color:var(--text); font-size:11px; cursor:pointer; }
  .wiz-check input { accent-color:var(--accent); }
  .wiz-hint { margin:0 0 10px; color:var(--muted); font-size:10px; line-height:1.45; }
  .wiz-hint code { padding:1px 4px; border-radius:4px; background:color-mix(in srgb, var(--border) 55%, transparent); font-family:'Cascadia Mono', Consolas, monospace; }
  .wiz-err { margin-bottom:10px; padding:6px 9px; border-radius:8px; background:color-mix(in srgb, var(--red) 12%, transparent); color:var(--red); font-size:11px; white-space:pre-wrap; word-break:break-word; }
  .wiz-progress { margin-bottom:10px; }
  .wiz-bar { height:6px; border-radius:3px; background:color-mix(in srgb, var(--border) 60%, transparent); overflow:hidden; }
  .wiz-fill { height:100%; border-radius:3px; background:var(--accent); transition:width .18s linear; }
  .wiz-bar.indet .wiz-fill { animation:indeterminate 1.1s ease-in-out infinite; }
  .wiz-meta { display:flex; align-items:baseline; justify-content:space-between; gap:8px; margin-top:5px; font-size:10px; color:var(--muted); }
  .wiz-bytes { color:var(--text); font-variant-numeric:tabular-nums; white-space:nowrap; }
  @media (prefers-reduced-motion: reduce) {
    .wiz-fill { transition:none; }
    .wiz-bar.indet .wiz-fill { animation-duration:2.4s; }
  }
  .wiz-actions { display:flex; gap:8px; }
  .wiz-actions .pill { flex:1; }
</style>
