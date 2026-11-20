<script lang="ts">
  import { onMount } from 'svelte'
  import {
    gitPush,
    gitPull,
    gitSetRemote,
    gitInitRepo,
    openExternal,
    setAutoCommit,
    autoCommitStatus,
    onAutoCommit,
    type GitInfo,
    type AutoCommitState
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
  // `loaded` solo pasa a true con una respuesta real de Go: mientras tanto no
  // se puede afirmar que la carpeta no sea un repo.
  let loaded = $state(cached !== null)
  let busy = $state(false)
  let error = $state('')
  let notice = $state('')
  let connectOpen = $state(false)
  let remoteInput = $state('')
  let auto = $state<AutoCommitState>({
    enabled: false, idleMin: 10, pending: false, idleSec: 0, busy: false, lastMsg: '', lastError: ''
  })
  let autoBusy = $state(false)

  // El borrador del commit vive en el store, como en la antigua pestaña:
  // sobrevive al cambio de pestaña y a los refrescos del panel.
  let draft = $derived(draftFor(projectPath))
  let state = $derived(runFor(projectPath))

  async function refresh() {
    if (refreshing) return
    refreshing = true
    try {
      info = await loadGitInfo(projectPath)
      error = ''
      loaded = true
    } catch (e) {
      error = (e as Error).message
    } finally {
      refreshing = false
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

  onMount(() => {
    refresh()
    loadAuto()
    const offRepo = onRepoChanged(refresh)
    const offAuto = onAutoCommit(() => {
      refresh()
      loadAuto()
    })
    const timer = setInterval(loadAuto, 5000)
    return () => {
      offRepo()
      offAuto()
      clearInterval(timer)
    }
  })

  function done(msg: string) {
    notice = msg
    error = ''
    connectOpen = false
    remoteInput = ''
    notifyRepoChanged()
    refresh()
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
    try { done(await gitPush()) } catch (e) { fail(e) } finally { busy = false }
  }

  async function sync() {
    busy = true
    try {
      const pulled = await gitPull()
      const pushed = await gitPush()
      done([pulled, pushed].filter(Boolean).join('\n'))
    } catch (e) { fail(e) } finally { busy = false }
  }

  function openWeb() {
    if (!info.webUrl) return
    openExternal(info.webUrl).catch(fail)
  }

  let canPush = $derived(
    info.isRepo && info.hasCommits && !!info.remoteUrl &&
    !info.branch.startsWith('detached') && !busy
  )
  let canSync = $derived(canPush && !!info.upstream)
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
        {#if info.webUrl}
          <button class="link" onclick={openWeb} title="Abrir en el navegador">Abrir ↗</button>
        {/if}
      </div>
      <div class="remote-url" title={info.remoteUrl}>{info.remoteUrl}</div>
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
      <button class="pill primary" onclick={push} disabled={!canPush} title={info.remoteUrl ? 'Subir rama actual' : 'Configura un remoto primero'}>
        {busy ? 'Subiendo...' : 'Push'}
      </button>
      <button class="pill ghost" onclick={sync} disabled={!canSync} title={info.upstream ? 'Pull --rebase + Push' : 'La rama no tiene upstream'}>
        Sincronizar
      </button>
    </div>
  {/if}

  <div class="commit-box">
    <div class="commit-head">Commit con OpenRouter</div>
    <div class="commit-body">
        <textarea
          placeholder="Mensaje del commit..."
          value={draft.message}
          oninput={(e) => setMessage(projectPath, e.currentTarget.value)}
        ></textarea>
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
  .empty { padding:26px 18px; text-align:center; color:var(--text); }
  .empty-ico { color:var(--brand); }
  .empty p { font-size:12px; line-height:1.5; }
  .empty .pill { margin-top:4px; }
  .hint { display:block; margin-top:8px; color:var(--muted); font-size:10px; line-height:1.4; }
  .commit-box { margin:14px 12px 4px; border:1px solid var(--border); border-radius:10px; background:color-mix(in srgb, var(--panel) 40%, transparent); }
  .commit-head { padding:9px 11px; color:var(--accent); font-size:11px; font-weight:700; }
  .commit-body { padding:0 11px 11px; }
  textarea { width:100%; box-sizing:border-box; height:82px; resize:vertical; background:var(--panel); color:var(--text); border:1px solid var(--border); border-radius:8px; padding:8px; font:inherit; font-size:11px; outline:none; }
  textarea:focus { border-color:var(--accent); }
  .gen-row { display:flex; align-items:center; gap:8px; margin-top:8px; }
  .progress { flex:1; height:4px; border-radius:2px; background:color-mix(in srgb, var(--border) 55%, transparent); overflow:hidden; }
  .progress .bar { width:35%; height:100%; background:var(--accent); animation:indeterminate 1.1s ease-in-out infinite; }
  @keyframes indeterminate { 0% { transform:translateX(-110%); } 100% { transform:translateX(320%); } }
  .elapsed { font-size:10px; color:var(--muted); font-variant-numeric:tabular-nums; }
  .elapsed.done { display:block; margin-top:6px; }
</style>
