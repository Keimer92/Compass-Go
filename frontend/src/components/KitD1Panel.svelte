<script lang="ts">
  import { onMount, tick } from 'svelte'
  import {
    kitD1Info,
    kitD1Whoami,
    kitD1Databases,
    kitD1Actions,
    kitD1SetPath,
    kitD1PickPath,
    kitD1Script,
    type KitD1Info,
    type KitD1Databases,
    type KitD1ActionDef,
  } from '../lib/bridge'

  let {
    onClose,
    onRun,
  }: { onClose: () => void; onRun: (label: string, script: string) => void } = $props()

  let box: HTMLDivElement | undefined = $state()

  let info = $state<KitD1Info>({
    path: '', exists: false, goVersion: '', gitBranch: '', gitDirty: false,
    lastTag: '', instances: [], consumers: [], error: '',
  })
  let actions = $state<KitD1ActionDef[]>([])
  let whoami = $state('')
  let databases = $state<KitD1Databases>({ account: '', databases: [], error: '' })
  let loading = $state(true)
  let whoLoading = $state(false)
  let dbLoading = $state(false)
  let error = $state('')
  let selected = $state('')
  let version = $state('')
  // Confirmación de dos pasos para acciones peligrosas: guarda el id armado.
  let armed = $state('')
  let armTimer: ReturnType<typeof setTimeout> | undefined

  const groups: { id: string; label: string }[] = [
    { id: 'worker', label: 'Worker' },
    { id: 'nube', label: 'Nube / Cloudflare' },
    { id: 'claves', label: 'Claves y tenants' },
    { id: 'kit', label: 'Kit' },
  ]

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose()
  }

  async function load() {
    loading = true
    error = ''
    try {
      info = await kitD1Info()
      if (!selected && info.instances.length) selected = info.instances[0].name
    } catch (e) {
      error = (e as Error).message
    }
    try {
      actions = await kitD1Actions()
    } catch {
      actions = []
    }
    loading = false
    if (info.exists) {
      void refreshWhoami()
      void loadDatabases()
    }
  }

  async function loadDatabases() {
    dbLoading = true
    try {
      databases = await kitD1Databases()
    } catch (e) {
      databases = { account: '', databases: [], error: (e as Error).message }
    } finally {
      dbLoading = false
    }
  }

  // Tamaño legible como el dashboard (512 B, 12.4 kB, 3.1 MB).
  function fmtBytes(n: number): string {
    if (!n) return '—'
    const units = ['B', 'kB', 'MB', 'GB', 'TB']
    let v = n
    let i = 0
    while (v >= 1024 && i < units.length - 1) {
      v /= 1024
      i++
    }
    return `${i === 0 ? v : v.toFixed(1)} ${units[i]}`
  }

  // Contadores del dashboard: 1.2k, 3.4M…
  function fmtNum(n: number): string {
    if (n < 1000) return String(n)
    if (n < 1e6) return `${(n / 1e3).toFixed(1)}k`
    if (n < 1e9) return `${(n / 1e6).toFixed(1)}M`
    return `${(n / 1e9).toFixed(1)}G`
  }

  async function refreshWhoami() {
    whoLoading = true
    whoami = ''
    try {
      whoami = await kitD1Whoami()
    } catch (e) {
      whoami = (e as Error).message
    } finally {
      whoLoading = false
    }
  }

  async function pickPath() {
    try {
      info = await kitD1PickPath()
      if (!selected && info.instances.length) selected = info.instances[0].name
      if (info.exists) {
        void refreshWhoami()
        void loadDatabases()
      }
    } catch (e) {
      error = (e as Error).message
    }
  }

  async function clearPath() {
    try {
      info = await kitD1SetPath('')
      whoami = ''
      databases = { account: '', databases: [], error: '' }
      if (info.exists) {
        void refreshWhoami()
        void loadDatabases()
      }
    } catch (e) {
      error = (e as Error).message
    }
  }

  function arm(id: string): boolean {
    if (armed === id) {
      clearTimeout(armTimer)
      armed = ''
      return true
    }
    armed = id
    clearTimeout(armTimer)
    armTimer = setTimeout(() => (armed = ''), 4000)
    return false
  }

  async function run(a: KitD1ActionDef) {
    error = ''
    if (a.needsInstance && !selected) {
      error = 'Elige una instancia primero.'
      return
    }
    if (a.dangerous && !arm(a.id)) return
    try {
      const script = await kitD1Script(a.id, selected, version)
      onRun(`Kit D1 · ${a.label}`, script)
      if (a.id === 'tag') version = ''
    } catch (e) {
      error = (e as Error).message
    }
  }

  onMount(async () => {
    void load()
    await tick()
    box?.focus()
  })
</script>

<div class="modal-backdrop" onclick={onClose} role="presentation">
  <div
    class="modal kitd1"
    bind:this={box}
    tabindex="-1"
    role="dialog"
    aria-modal="true"
    aria-label="Kit D1"
    onclick={(e) => e.stopPropagation()}
    onkeydown={onKey}
  >
    <div class="modal-head">
      <span>Kit D1</span>
      <button onclick={onClose} aria-label="Cerrar">×</button>
    </div>

    {#if loading}
      <p class="hint">Leyendo el kit…</p>
    {:else}
      <section class="sec">
        <div class="sec-title">Ubicación</div>
        <div class="path-row">
          <span class="path" title={info.path || '(sin fijar)'}>
            {info.path || '(autodetectar)'}
          </span>
          <button class="pill" onclick={pickPath}>Elegir ruta</button>
          {#if info.path}<button class="pill" onclick={clearPath}>Autodetectar</button>{/if}
        </div>
        {#if !info.exists}
          <p class="warn">{info.error || 'No se encontró el kit en esa ruta.'}</p>
        {:else}
          <div class="badges">
            <span class="badge">módulo Go {info.goVersion || '?'}</span>
            {#if info.gitBranch}<span class="badge">{info.gitBranch}{info.gitDirty ? ' *' : ''}</span>{/if}
            {#if info.lastTag}<span class="badge">tag {info.lastTag}</span>{/if}
            <span class="badge">{info.instances.length} instancias</span>
          </div>
        {/if}
      </section>

      {#if info.exists}
        <section class="sec">
          <div class="sec-title">
            Wrangler
            <button class="lnk" onclick={refreshWhoami} disabled={whoLoading}>
              {whoLoading ? 'consultando…' : 'Actualizar'}
            </button>
          </div>
          <pre class="who">{whoami || 'Sin sesión de Wrangler (usa Wrangler login).'}</pre>
        </section>

        <section class="sec">
          <div class="sec-title">
            Bases D1
            <button class="lnk" onclick={loadDatabases} disabled={dbLoading}>
              {dbLoading ? 'consultando…' : 'Actualizar'}
            </button>
          </div>
          {#if databases.error}
            <p class="warn">{databases.error}</p>
          {:else if dbLoading && !databases.databases.length}
            <p class="hint">Consultando Cloudflare…</p>
          {:else if !databases.databases.length}
            <p class="hint">La cuenta no tiene bases D1.</p>
          {:else}
            <div class="db-list">
              <div class="db-head">
                <span>Base</span>
                <span>Tamaño</span>
                <span>Queries 24 h</span>
                <span>Rows 24 h</span>
              </div>
              {#each databases.databases as db (db.id)}
                <div class="db" class:missing={db.missing}>
                  <span class="db-name" title={db.id}>
                    {db.name}
                    {#if db.missing}<span class="tag bad">no existe</span>{/if}
                    <span class="db-map">
                      {#if db.instance}{db.instance}{:else}sin instancia{/if}
                      {#if db.worker} · {db.worker}{/if}
                    </span>
                  </span>
                  <span class="num">{db.missing ? '—' : fmtBytes(db.fileSize)}</span>
                  <span class="num" title="lecturas / escrituras">
                    {#if db.analytics}{fmtNum(db.readQueries)} / {fmtNum(db.writeQueries)}{:else}—{/if}
                  </span>
                  <span class="num" title="rows leídas / escritas">
                    {#if db.analytics}{fmtNum(db.rowsRead)} / {fmtNum(db.rowsWritten)}{:else}—{/if}
                  </span>
                </div>
              {/each}
            </div>
            <p class="hint">
              Métricas de las últimas 24 h de la cuenta {databases.account || '(sin identificar)'}, igual que el dashboard de Cloudflare.
            </p>
          {/if}
        </section>

        <section class="sec">
          <div class="sec-title">Instancias del worker</div>
          {#if info.instances.length}
            <div class="inst-grid">
              {#each info.instances as ins (ins.name)}
                <button
                  class="inst"
                  class:sel={selected === ins.name}
                  onclick={() => (selected = ins.name)}
                >
                  <span class="inst-name">{ins.name}</span>
                  <span class="inst-worker">{ins.workerName || '(sin worker)'}</span>
                  <span class="inst-url" title={ins.workerUrl}>{ins.workerUrl || 'sin URL'}</span>
                  {#if ins.databaseName}
                    <span class="inst-db" title={ins.databaseId}>base {ins.databaseName}</span>
                  {/if}
                  <span class="tags">
                    {#if ins.hasPackage}<span class="tag">pkg</span>{/if}
                    {#if ins.hasSchema}<span class="tag">schema</span>{/if}
                    {#if ins.hasKeys}<span class="tag">keys</span>{/if}
                  </span>
                </button>
              {/each}
            </div>
          {:else}
            <p class="hint">No hay instancias en worker/instances.</p>
          {/if}
        </section>

        <section class="sec">
          <div class="sec-title">Consumidores del workspace</div>
          {#if info.consumers.length}
            <div class="consumer-list">
              {#each info.consumers as c (c.path)}
                <div class="consumer">
                  <span class="consumer-name">{c.name}</span>
                  <span class="consumer-meta">
                    {#if c.engine === 'go'}
                      módulo Go {c.version || ''}{c.local ? ' · local (go.work)' : ''}
                    {:else}
                      Flutter (worker)
                    {/if}
                  </span>
                </div>
              {/each}
            </div>
          {:else}
            <p class="hint">No se detectaron proyectos que usen el kit.</p>
          {/if}
        </section>

        <section class="sec actions">
          {#each groups as g (g.id)}
            {@const inGroup = actions.filter((a) => a.group === g.id)}
            {#if inGroup.length}
              <div class="sec-title">{g.label}</div>
              <div class="action-grid">
                {#each inGroup as a (a.id)}
                  {#if a.id === 'tag'}
                    <div class="tag-row">
                      <input
                        class="set-input ver"
                        bind:value={version}
                        placeholder="v0.1.0"
                        spellcheck="false"
                      />
                      <button
                        class="pill"
                        class:danger={a.dangerous && armed === a.id}
                        title={a.description}
                        onclick={() => run(a)}
                      >
                        {a.dangerous && armed === a.id ? '¿Seguro? pulsa de nuevo' : a.label}
                      </button>
                    </div>
                  {:else}
                    <button
                      class="pill act"
                      class:danger={a.dangerous && armed === a.id}
                      title={a.description}
                      onclick={() => run(a)}
                      disabled={a.needsInstance && !selected}
                    >
                      {a.dangerous && armed === a.id ? '¿Seguro? pulsa de nuevo' : a.label}
                    </button>
                  {/if}
                {/each}
              </div>
            {/if}
          {/each}
          <p class="hint">
            Las acciones peligrosas piden confirmación dos veces. Se ejecutan en una pestaña del terminal de Compass.
          </p>
        </section>
      {/if}

      {#if error}<div class="msg error">{error}</div>{/if}
    {/if}
  </div>
</div>

<style>
  :global(.modal.kitd1) { width: 620px; max-width: 95vw; max-height: 88vh; overflow-y: auto; }
  .hint { margin: 6px 0 0; font-size: 10px; line-height: 1.45; color: var(--muted); }
  .warn { margin: 8px 0 0; font-size: 11px; color: var(--yellow); }
  .sec { margin-top: 14px; padding-top: 12px; border-top: 1px solid var(--border); }
  .sec:first-child { margin-top: 4px; padding-top: 0; border-top: none; }
  .sec-title {
    display: flex; align-items: center; justify-content: space-between;
    font-size: 11px; font-weight: 700; color: var(--accent);
  }
  .lnk {
    border: none; background: none; color: var(--muted);
    font: inherit; font-size: 10px; cursor: pointer;
  }
  .lnk:hover { color: var(--accent); }
  .path-row { display: flex; align-items: center; gap: 6px; margin-top: 8px; }
  .path {
    flex: 1 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis;
    white-space: nowrap; font-size: 11px; color: var(--text);
    padding: 5px 8px; border: 1px solid var(--border); border-radius: 7px;
    background: color-mix(in srgb, var(--panel) 60%, transparent);
  }
  .badges { display: flex; flex-wrap: wrap; gap: 5px; margin-top: 8px; }
  .badge {
    padding: 2px 7px; border: 1px solid var(--border); border-radius: 20px;
    font-size: 10px; color: var(--muted);
  }
  .who {
    margin: 8px 0 0; padding: 8px 10px; border: 1px solid var(--border);
    border-radius: 8px; background: color-mix(in srgb, var(--panel) 60%, transparent);
    font-size: 10px; line-height: 1.5; white-space: pre-wrap; word-break: break-word;
    max-height: 120px; overflow-y: auto; color: var(--muted);
  }
  .inst-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 7px; margin-top: 8px; }
  .inst {
    display: flex; flex-direction: column; gap: 3px; text-align: left;
    padding: 7px 9px; border: 1px solid var(--border); border-radius: 8px;
    background: color-mix(in srgb, var(--panel) 60%, transparent);
    color: var(--text); font: inherit; cursor: pointer;
  }
  .inst:hover { border-color: var(--accent); }
  .inst.sel { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 10%, transparent); }
  .inst-name { font-size: 11px; font-weight: 700; }
  .inst-worker { font-size: 10px; color: var(--muted); }
  .inst-url { font-size: 10px; color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .inst-db { font-size: 9px; color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .db-list {
    margin-top: 8px; border: 1px solid var(--border); border-radius: 8px; overflow: hidden;
  }
  .db-head, .db {
    display: grid; grid-template-columns: 1.9fr .8fr 1.1fr 1.2fr;
    gap: 8px; align-items: center; padding: 5px 9px;
  }
  .db-head {
    font-size: 9px; text-transform: uppercase; letter-spacing: .04em;
    color: var(--muted); background: color-mix(in srgb, var(--panel) 80%, transparent);
    border-bottom: 1px solid var(--border);
  }
  .db { font-size: 11px; border-bottom: 1px solid var(--border); }
  .db:last-child { border-bottom: none; }
  .db.missing { opacity: .7; }
  .db-name {
    display: flex; flex-wrap: wrap; align-items: center; gap: 5px;
    min-width: 0; font-weight: 600;
  }
  .db-map { flex-basis: 100%; font-size: 9px; font-weight: 400; color: var(--muted); }
  .num { font-size: 10px; color: var(--muted); font-variant-numeric: tabular-nums; white-space: nowrap; }
  .tag.bad { color: var(--red); }
  .tags { display: flex; gap: 4px; }
  .tag { font-size: 9px; color: var(--accent); opacity: 0.85; }
  .consumer-list { margin-top: 8px; }
  .consumer {
    display: flex; align-items: center; justify-content: space-between; gap: 8px;
    padding: 5px 8px; border-bottom: 1px solid var(--border); font-size: 11px;
  }
  .consumer-name { font-weight: 600; }
  .consumer-meta { color: var(--muted); font-size: 10px; }
  .action-grid { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 8px; margin-bottom: 6px; }
  .act { font-size: 11px; }
  .danger { color: var(--red) !important; border-color: var(--red) !important; }
  .tag-row { display: flex; gap: 6px; align-items: center; margin-top: 8px; }
  .set-input {
    padding: 6px 8px; border: 1px solid var(--border); border-radius: 7px;
    background: var(--panel); color: var(--text); font: inherit; font-size: 11px; outline: none;
  }
  .set-input:focus { border-color: var(--accent); }
  .set-input.ver { width: 100px; }
  .msg { margin: 8px 0 0; padding: 6px 9px; border-radius: 8px; font-size: 11px; white-space: pre-wrap; }
  .msg.error { color: var(--red); background: color-mix(in srgb, var(--red) 12%, transparent); }
</style>
