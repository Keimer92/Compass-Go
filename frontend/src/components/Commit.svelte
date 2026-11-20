<script lang="ts">
  import { draftFor, runFor, setMessage, generateFor, cancelFor, commitFor } from '../lib/commitStore.svelte'

  let { projectPath }: { projectPath: string } = $props()

  // El mensaje y el estado de generación viven en el store, no aquí: así
  // sobreviven al cambio de proyecto y la generación sigue en curso aunque se
  // cierre esta pestaña lateral.
  let draft = $derived(draftFor(projectPath))
  let state = $derived(runFor(projectPath))
</script>

<div class="head">
  <span>Commit con OpenRouter</span>
</div>
<textarea
  placeholder="Mensaje del commit..."
  value={draft.message}
  oninput={(e) => setMessage(projectPath, e.currentTarget.value)}
></textarea>
<div class="actions">
  <button
    class="pill generate"
    onclick={() => generateFor(projectPath)}
    disabled={state.loading || !!draft.message.trim()}
  >
    {state.loading ? 'Generando...' : 'Generar con OpenRouter'}
  </button>
  <button
    class="pill commit"
    onclick={() => commitFor(projectPath)}
    disabled={state.committing || !draft.message.trim()}
  >
    {state.committing ? 'Commiteando...' : 'Commit'}
  </button>
</div>
{#if state.loading}
  <div class="gen-row">
    <div class="progress" aria-label="Generando commit"><div class="bar"></div></div>
    <span class="elapsed">{(state.elapsedMs / 1000).toFixed(1)} s</span>
  </div>
  <button class="pill cancel" onclick={() => cancelFor(projectPath)}>Cancelar</button>
{:else if draft.lastMs > 0}
  <div class="elapsed done">Última generación: {(draft.lastMs / 1000).toFixed(1)} s</div>
{/if}
<div class="hint">Se añadirán todos los cambios (git add -A)</div>
{#if state.error}<div class="error">{state.error}</div>{/if}
{#if state.ok}<div class="ok">{state.ok}</div>{/if}

<style>
  .head { display:flex; justify-content:space-between; align-items:center; padding:14px; color:var(--accent); font-weight:700; font-size:13px; }
  textarea { width:calc(100% - 28px); margin:0 14px; height:120px; resize:vertical; background:var(--panel); color:var(--text); border:1px solid var(--border); border-radius:10px; padding:10px; font: inherit; font-size:12px; }
  .pill { display:block; margin:12px 14px 0; padding:8px 14px; border:none; border-radius:18px; background:color-mix(in srgb, var(--accent) 30%, var(--bg)); color:var(--text); font-size:12px; }
  .pill:hover { background:color-mix(in srgb, var(--accent) 50%, var(--bg)); }
  .pill:disabled { opacity:.55; cursor:default; }
  .actions { display:flex; gap:8px; margin:12px 14px 0; }
  .actions .pill { margin:0; }
  .actions .generate { flex:1; }
  .gen-row { display:flex; align-items:center; gap:10px; margin:10px 14px 0; }
  .gen-row .progress { flex:1; }
  .progress { height:4px; border-radius:2px; background:color-mix(in srgb, var(--border) 55%, transparent); overflow:hidden; }
  .progress .bar { width:35%; height:100%; border-radius:2px; background:var(--accent); animation:indeterminate 1.1s ease-in-out infinite; }
  .elapsed { font-size:11px; color:var(--muted); font-variant-numeric:tabular-nums; flex-shrink:0; }
  .elapsed.done { margin:8px 14px 0; }
  @keyframes indeterminate { 0% { transform:translateX(-110%); } 100% { transform:translateX(320%); } }
  .commit { background:color-mix(in srgb, var(--green) 30%, var(--bg)); }
  .cancel { background:color-mix(in srgb, var(--red) 26%, var(--bg)); }
  .hint { margin:8px 14px; color:var(--muted); font-size:10px; }
  .error { margin:8px 14px; color:var(--red); font-size:11px; white-space:pre-wrap; }
  .ok { margin:8px 14px; color:var(--green); font-size:11px; }
</style>
