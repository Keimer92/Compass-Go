<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { versionInfo, applyVersion, type VersionPlanInfo, type VersionResult } from '../lib/bridge'
  import { notifyRepoChanged } from '../lib/repoChanged'

  // La card "Version" abre este modal en vez de una ConPTY: muestra qué
  // archivos llevan la versión, su valor actual y aplica el cambio de golpe.
  let { onClose }: { onClose: () => void } = $props()

  let box: HTMLDivElement | undefined = $state()
  let plan = $state<VersionPlanInfo | null>(null)
  let loading = $state(true)
  let loadError = $state('')
  let value = $state('')
  let busy = $state(false)
  let error = $state('')
  let result = $state<VersionResult | null>(null)

  // Mismo formato que valida el backend; aquí solo sirve para habilitar el
  // botón y avisar antes de llamar.
  const versionRe = /^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$/
  let valid = $derived(versionRe.test(value.trim()))

  async function load() {
    loading = true
    try {
      plan = await versionInfo()
      loadError = ''
    } catch (e) {
      loadError = (e as Error).message
    } finally {
      loading = false
    }
  }

  async function apply() {
    if (!valid || busy) return
    busy = true
    error = ''
    try {
      result = await applyVersion(value.trim())
      // Los archivos cambiaron en disco: el panel Git debe releer el estado.
      notifyRepoChanged()
      await load()
    } catch (e) {
      error = (e as Error).message
    } finally {
      busy = false
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
    class="modal version-panel"
    bind:this={box}
    tabindex="-1"
    role="dialog"
    aria-modal="true"
    aria-label="Versionar proyecto"
    onclick={(e) => e.stopPropagation()}
    onkeydown={onKey}
  >
    <div class="modal-head">
      <span>Versionar proyecto</span>
      <button onclick={onClose} aria-label="Cerrar">×</button>
    </div>

    {#if loading}
      <p class="hint">Leyendo archivos de versión…</p>
    {:else if loadError}
      <div class="msg error">{loadError}</div>
    {:else if plan}
      <div class="type-row">
        <span class="type-badge">{plan.type}</span>
        <span class="hint">
          {plan.files.length} archivo{plan.files.length === 1 ? '' : 's'} con versión
        </span>
      </div>

      <ul class="files">
        {#each plan.files as f (f.path)}
          <li class="file">
            <span class="file-path" title={f.path}>{f.path}</span>
            {#if f.missing}
              <span class="file-cur missing">sin versión · se insertará</span>
            {:else}
              <span class="file-cur">{f.current}</span>
            {/if}
            {#if f.hasCode}
              <span class="file-code" title="versionCode de Android: se incrementa solo">
                code {f.code} → {f.code + 1}
              </span>
            {/if}
          </li>
        {/each}
      </ul>

      {#if plan.omitted.length}
        <p class="hint omitted">
          Se omiten (no existen): {plan.omitted.join(', ')}
        </p>
      {/if}

      <label class="field">
        <span class="field-label">Nueva versión</span>
        <input
          class="field-input"
          type="text"
          bind:value={value}
          placeholder="1.2.3  ·  2.0.0-beta.1  ·  1.4.0+12"
          autocomplete="off"
          spellcheck="false"
          onkeydown={(e) => {
            if (e.key === 'Enter') apply()
          }}
        />
      </label>
      {#if value.trim() && !valid}
        <p class="hint warn">Formato inválido: usa X.Y.Z con sufijo opcional (-pre / +build).</p>
      {/if}
      {#if plan.type === 'FLUTTER'}
        <p class="hint">En Flutter el build number (+N) se incrementa solo si no lo indicas.</p>
      {/if}

      {#if error}<div class="msg error">{error}</div>{/if}
      {#if result}
        <div class="msg ok">
          Versión {result.version} aplicada en {result.files} archivo{result.files === 1 ? '' : 's'}.
          {#if result.added}
            Se insertó el campo version en {result.added}.
          {/if}
          {#if result.notes.length}
            {#each result.notes as n}<div class="note">{n}</div>{/each}
          {/if}
        </div>
      {/if}

      <div class="modal-actions">
        <button onclick={onClose}>{result ? 'Cerrar' : 'Cancelar'}</button>
        <button class="btn-primary" disabled={busy || !valid} onclick={apply}>
          {busy ? 'Aplicando…' : 'Aplicar versión'}
        </button>
      </div>
    {/if}
  </div>
</div>

<style>
  :global(.modal.version-panel) { width: 470px; max-width: 94vw; }
  .hint { margin: 6px 0 0; font-size: 10px; line-height: 1.45; color: var(--muted); }
  .hint.warn { color: var(--yellow); }
  .hint.omitted { word-break: break-word; }
  .type-row { display: flex; align-items: center; gap: 8px; }
  .type-badge {
    padding: 2px 8px; border-radius: 999px; font-size: 10px; font-weight: 700;
    color: var(--accent);
    background: color-mix(in srgb, var(--accent) 16%, transparent);
  }
  .files {
    list-style: none; margin: 10px 0 0; padding: 0;
    border: 1px solid var(--border); border-radius: 10px; overflow: hidden;
  }
  .file {
    display: flex; align-items: center; gap: 8px;
    padding: 6px 9px; font-size: 11px;
    border-bottom: 1px solid var(--border);
  }
  .file:last-child { border-bottom: none; }
  .file-path {
    flex: 1 1 auto; min-width: 0;
    font-family: 'Cascadia Mono', Consolas, monospace;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .file-cur { flex: 0 0 auto; color: var(--text); font-variant-numeric: tabular-nums; }
  .file-cur.missing { color: var(--muted); font-style: italic; }
  .file-code { flex: 0 0 auto; font-size: 10px; color: var(--muted); font-variant-numeric: tabular-nums; }
  .field { display: flex; flex-direction: column; gap: 4px; margin-top: 12px; }
  .field-label { font-size: 10px; color: var(--muted); }
  .field-input {
    width: 100%; box-sizing: border-box;
    padding: 7px 9px;
    border: 1px solid var(--border); border-radius: 8px;
    background: var(--panel); color: var(--text);
    font: inherit; font-size: 12px; outline: none;
  }
  .field-input:focus { border-color: var(--accent); }
  .msg {
    margin: 10px 0 0; padding: 7px 10px; border-radius: 8px;
    font-size: 11px; line-height: 1.45; white-space: pre-wrap; word-break: break-word;
  }
  .msg.error { color: var(--red); background: color-mix(in srgb, var(--red) 12%, transparent); }
  .msg.ok { color: var(--green); background: color-mix(in srgb, var(--green) 12%, transparent); }
  .note { margin-top: 4px; font-family: 'Cascadia Mono', Consolas, monospace; font-size: 10px; }
</style>
