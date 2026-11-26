<script lang="ts">
  import { onMount, tick } from 'svelte'
  import {
    getSettings,
    setSettings,
    listModels,
    openRouterKeyConfigured,
    testModelSpeeds,
    githubAccounts,
    githubLogin,
    githubLogout,
    type Settings,
    type ModelOption,
    type ModelSpeedResult,
  } from '../lib/bridge'
  import { notifyRepoChanged } from '../lib/repoChanged'

  let { onClose }: { onClose: () => void } = $props()

  let box: HTMLDivElement | undefined = $state()

  let cfg = $state<Settings>({
    open_output_dir: true,
    close_tab_on_build_end: false,
    openrouter_endpoint: 'https://openrouter.ai/api/v1',
    openrouter_model: '',
    openrouter_api_key: '',
    auto_commit_idle_min: 5,
    sound_presets: {},
    terminal_panel_pct: 60,
    vibe_disabled: [],
  })
  let models = $state<ModelOption[]>([])
  let keyConfigured = $state(false)
  let showKeyInput = $state(false)
  let newApiKey = $state('')
  let speeds = $state<ModelSpeedResult[]>([])
  let speedTesting = $state(false)
  let speedError = $state('')
  let loading = $state(true)
  let busy = $state(false)
  let saved = $state(false)
  let error = $state('')

  // Cuenta de GitHub: la guarda Git Credential Manager, no Compass.
  let accounts = $state<string[]>([])
  let ghLoading = $state(true)
  let ghBusy = $state(false)
  let ghUser = $state('')
  let ghMsg = $state('')
  let ghError = $state('')
  let confirmOut = $state('')
  let confirmTimer: ReturnType<typeof setTimeout> | undefined

  async function loadAccounts() {
    ghLoading = true
    try {
      accounts = await githubAccounts()
      ghError = ''
    } catch (e) {
      ghError = (e as Error).message
    } finally {
      ghLoading = false
    }
  }

  async function load() {
    try {
      cfg = await getSettings()
    } catch {
      /* se mantienen los valores por defecto */
    }
    try {
      keyConfigured = await openRouterKeyConfigured()
    } catch {
      keyConfigured = false
    }
    showKeyInput = !keyConfigured
    newApiKey = ''
    try {
      models = await listModels()
    } catch {
      models = []
    }
    // Si el modelo guardado no esta en el catalogo, se preselecciona el primero
    // (gratuito y rapido) para que el selector nunca quede en blanco.
    if (models.length && !models.some((m) => m.id === cfg.openrouter_model)) {
      cfg.openrouter_model = models[0].id
    }
    loading = false
    void loadAccounts()
  }

  async function save() {
    busy = true
    error = ''
    try {
      const min = Math.round(Number(cfg.auto_commit_idle_min))
      cfg.auto_commit_idle_min = Number.isFinite(min) ? Math.min(240, Math.max(1, min)) : 5
      const key = newApiKey.trim()
      await setSettings({ ...cfg, openrouter_api_key: key })
      if (key) {
        keyConfigured = true
        showKeyInput = false
        newApiKey = ''
      }
      saved = true
      setTimeout(() => (saved = false), 2500)
      // El panel Git relee el estado del commit automatico.
      notifyRepoChanged()
    } catch (e) {
      error = (e as Error).message
    } finally {
      busy = false
    }
  }

  // testSpeeds mide la respuesta real de cada modelo listado. La key recien
  // escrita se pasa sin guardar para poder probarla antes de dar a Guardar.
  async function testSpeeds() {
    if (!models.length || speedTesting) return
    if (!keyConfigured && !newApiKey.trim()) {
      speedError = 'Introduce la API key y guarda los ajustes antes de probar.'
      return
    }
    speedTesting = true
    speedError = ''
    speeds = []
    try {
      speeds = await testModelSpeeds(
        models.map((m) => m.id),
        cfg.openrouter_endpoint,
        newApiKey.trim(),
      )
    } catch (e) {
      speedError = (e as Error).message
    } finally {
      speedTesting = false
    }
  }

  async function login() {
    const user = ghUser.trim()
    if (!user || ghBusy) return
    ghBusy = true
    ghError = ''
    ghMsg = ''
    try {
      ghMsg = await githubLogin(user)
      ghUser = ''
      await loadAccounts()
      notifyRepoChanged()
    } catch (e) {
      ghError = (e as Error).message
    } finally {
      ghBusy = false
    }
  }

  async function logout(account: string) {
    if (ghBusy) return
    // Primer clic pide confirmacion; el segundo ejecuta.
    if (confirmOut !== account) {
      confirmOut = account
      clearTimeout(confirmTimer)
      confirmTimer = setTimeout(() => (confirmOut = ''), 4000)
      return
    }
    clearTimeout(confirmTimer)
    confirmOut = ''
    ghBusy = true
    ghError = ''
    ghMsg = ''
    try {
      ghMsg = await githubLogout(account)
      await loadAccounts()
      notifyRepoChanged()
    } catch (e) {
      ghError = (e as Error).message
    } finally {
      ghBusy = false
    }
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose()
  }

  onMount(async () => {
    void load()
    await tick()
    box?.focus()
  })
</script>

<div class="modal-backdrop" onclick={onClose} role="presentation">
  <div
    class="modal git-settings"
    bind:this={box}
    tabindex="-1"
    role="dialog"
    aria-modal="true"
    aria-label="Ajustes de Git"
    onclick={(e) => e.stopPropagation()}
    onkeydown={onKey}
  >
    <div class="modal-head">
      <span>Ajustes de Git</span>
      <button onclick={onClose} aria-label="Cerrar">×</button>
    </div>

    {#if loading}
      <p class="hint">Leyendo ajustes…</p>
    {:else}
      <label class="set-row">
        <span class="set-label">Commit automático tras</span>
        <input class="set-input num" type="number" min="1" max="240" step="1" bind:value={cfg.auto_commit_idle_min} />
        <span class="set-unit">min sin cambios</span>
      </label>

      <div>
        <label class="set-col">
          <span class="set-label">Endpoint de OpenRouter</span>
          <input class="set-input" type="text" bind:value={cfg.openrouter_endpoint} placeholder="https://openrouter.ai/api/v1" />
        </label>
        <label class="set-col">
          <span class="set-label">Modelo <small>(gratuitos · los más rápidos primero)</small></span>
          <select class="set-input" bind:value={cfg.openrouter_model}>
            {#if cfg.openrouter_model && !models.some((m) => m.id === cfg.openrouter_model)}
              <option value={cfg.openrouter_model}>{cfg.openrouter_model} (actual)</option>
            {/if}
            {#each models as m}
              <option value={m.id}>{m.name} — {m.id}</option>
            {/each}
            {#if !models.length}
              <option value="">(no se pudieron cargar los modelos)</option>
            {/if}
          </select>
        </label>

        <div class="speed-block">
          <div class="speed-head">
            <span class="set-label">Velocidad de modelos <small>(petición real · respuesta corta)</small></span>
            <button class="pill" onclick={testSpeeds} disabled={speedTesting || !models.length}>
              {speedTesting ? 'Probando…' : 'Probar velocidad'}
            </button>
          </div>
          {#if speedTesting}<p class="set-hint">Midiendo {models.length} modelos…</p>{/if}
          {#if speedError}<p class="set-warn">{speedError}</p>{/if}
          {#if speeds.length}
            <div class="speed-list">
              {#each speeds as s, i (s.id)}
                <div class="speed-row" class:best={i === 0 && s.ok}>
                  <span class="speed-rank">{i + 1}</span>
                  <span class="speed-name" title={s.id}>{s.name || s.id}</span>
                  {#if s.ok}
                    <span class="speed-ms">{s.ms} ms</span>
                    <button class="lnk" onclick={() => (cfg.openrouter_model = s.id)} disabled={cfg.openrouter_model === s.id}>
                      {cfg.openrouter_model === s.id ? 'activo' : 'usar'}
                    </button>
                  {:else}
                    <span class="speed-err" title={s.error}>error</span>
                  {/if}
                </div>
              {/each}
            </div>
            <p class="set-hint">Ordenado por tiempo de respuesta; el primero es el más rápido.</p>
          {/if}
        </div>

        <div class="set-col">
          <span class="set-label">API key de OpenRouter</span>
          {#if keyConfigured && !showKeyInput}
            <div class="key-ok">API configurada. La clave no se muestra por seguridad.</div>
            <button class="lnk" onclick={() => (showKeyInput = true)}>Introducir nueva key</button>
          {:else}
            <input
              class="set-input"
              type="password"
              bind:value={newApiKey}
              placeholder="sk-or-v1-…"
              autocomplete="off"
              spellcheck="false"
            />
            <p class="set-hint">Se guarda al pulsar «Guardar ajustes».</p>
            {#if keyConfigured}
              <button class="lnk" onclick={() => { showKeyInput = false; newApiKey = '' }}>Cancelar</button>
            {/if}
          {/if}
        </div>
      </div>

      <section class="gh">
        <h3 class="gh-title">Cuenta de GitHub</h3>
        <p class="hint">
          Es la cuenta que usa git al hacer push (Git Credential Manager). Compass no guarda tokens.
        </p>

        {#if ghLoading}
          <p class="hint">Leyendo cuentas…</p>
        {:else if accounts.length}
          {#each accounts as acc (acc)}
            <div class="gh-row">
              <span class="gh-name" title={acc}>{acc}</span>
              <button class="gh-out" class:confirm={confirmOut === acc} onclick={() => logout(acc)} disabled={ghBusy}>
                {confirmOut === acc ? '¿Seguro?' : 'Cerrar sesión'}
              </button>
            </div>
          {/each}
        {:else}
          <p class="hint">No hay ninguna cuenta guardada.</p>
        {/if}

        <div class="gh-login">
          <input
            class="set-input"
            bind:value={ghUser}
            placeholder="tu usuario de GitHub"
            autocomplete="off"
            spellcheck="false"
            onkeydown={(e) => {
              if (e.key === 'Enter') login()
            }}
          />
          <button class="pill primary" onclick={login} disabled={ghBusy || !ghUser.trim()}>
            {ghBusy ? 'Esperando…' : 'Iniciar sesión'}
          </button>
        </div>
        <p class="hint">
          {ghBusy
            ? 'Se abrió el navegador: autoriza la cuenta y vuelve aquí.'
            : 'Al iniciar sesión se abre el navegador para autorizar la cuenta.'}
        </p>

        {#if ghError}<div class="msg error">{ghError}</div>{/if}
        {#if ghMsg}<div class="msg ok">{ghMsg}</div>{/if}
      </section>

      {#if error}<div class="msg error">{error}</div>{/if}
      {#if saved}<div class="msg ok">Ajustes guardados.</div>{/if}

      <div class="modal-actions">
        <button class="pill primary" onclick={save} disabled={busy}>
          {busy ? 'Guardando…' : 'Guardar ajustes'}
        </button>
      </div>
    {/if}
  </div>
</div>

<style>
  :global(.modal.git-settings) { width: 460px; max-width: 94vw; }
  .hint { margin: 6px 0 0; font-size: 10px; line-height: 1.45; color: var(--muted); }
  .set-row { display: flex; align-items: center; gap: 7px; margin-top: 8px; }
  .set-col { display: flex; flex-direction: column; gap: 4px; margin-top: 8px; }
  .set-label { color: var(--muted); font-size: 10px; }
  .set-label small { color: var(--muted); opacity: 0.8; }
  .set-input {
    width: 100%;
    box-sizing: border-box;
    padding: 6px 8px;
    border: 1px solid var(--border);
    border-radius: 7px;
    background: var(--panel);
    color: var(--text);
    font: inherit;
    font-size: 11px;
    outline: none;
  }
  .set-input:focus { border-color: var(--accent); }
  .set-input.num { width: 62px; text-align: center; }
  .set-unit { color: var(--muted); font-size: 10px; }
  .set-hint, .set-warn { margin: 8px 0 0; font-size: 10px; line-height: 1.4; color: var(--muted); }
  .set-warn { color: var(--yellow); }
  .lnk {
    align-self: flex-start; border: none; background: none; color: var(--accent);
    font: inherit; font-size: 10px; cursor: pointer;
  }
  .lnk:hover { text-decoration: underline; }
  .lnk:disabled { color: var(--muted); cursor: default; text-decoration: none; }
  .key-ok {
    padding: 7px 9px; border: 1px solid var(--border); border-radius: 7px;
    background: color-mix(in srgb, var(--green) 10%, transparent);
    color: var(--green); font-size: 11px;
  }
  .speed-block { margin-top: 10px; padding-top: 10px; border-top: 1px solid var(--border); }
  .speed-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
  .speed-list { margin-top: 8px; max-height: 180px; overflow-y: auto; border: 1px solid var(--border); border-radius: 7px; }
  .speed-row {
    display: grid; grid-template-columns: 18px 1fr auto 34px; align-items: center; gap: 6px;
    padding: 4px 7px; border-bottom: 1px solid var(--border); font-size: 10px;
  }
  .speed-row:last-child { border-bottom: none; }
  .speed-row.best { background: color-mix(in srgb, var(--green) 10%, transparent); }
  .speed-rank { color: var(--muted); }
  .speed-name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .speed-ms { color: var(--text); font-variant-numeric: tabular-nums; }
  .speed-err { color: var(--red); }
  .msg { margin: 8px 0 0; padding: 6px 9px; border-radius: 8px; font-size: 11px; white-space: pre-wrap; word-break: break-word; }
  .msg.error { color: var(--red); background: color-mix(in srgb, var(--red) 12%, transparent); }
  .msg.ok { color: var(--green); background: color-mix(in srgb, var(--green) 12%, transparent); }

  .gh { margin-top: 16px; padding-top: 12px; border-top: 1px solid var(--border); }
  .gh-title { margin: 0; font-size: 11px; font-weight: 700; color: var(--accent); }
  .gh-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    margin-top: 6px;
    padding: 5px 8px;
    border: 1px solid var(--border);
    border-radius: 7px;
    background: color-mix(in srgb, var(--panel) 60%, transparent);
  }
  .gh-name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 11px; }
  .gh-out {
    flex: 0 0 auto;
    padding: 3px 8px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: none;
    color: var(--muted);
    font: inherit;
    font-size: 10px;
    cursor: pointer;
  }
  .gh-out:hover { color: var(--red); border-color: var(--red); }
  .gh-out.confirm { color: var(--red); border-color: var(--red); }
  .gh-login { display: flex; gap: 6px; margin-top: 10px; }
  .gh-login .set-input { flex: 1 1 auto; }
  .gh-login .pill { flex: 0 0 auto; }
</style>
