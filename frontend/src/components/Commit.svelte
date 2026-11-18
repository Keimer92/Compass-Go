<script lang="ts">
  import { onDestroy } from 'svelte'
  import { generateCommit, commit } from '../lib/bridge'

  let message = $state('')
  let loading = $state(false)
  let committing = $state(false)
  let error = $state('')
  let ok = $state('')

  // Cronometro de la generacion: muestra el tiempo transcurrido mientras el
  // modelo responde y conserva el ultimo tiempo al terminar.
  let elapsedMs = $state(0)
  let lastMs = $state(0)
  let timer: ReturnType<typeof setInterval> | null = null

  function startTimer() {
    const start = performance.now()
    elapsedMs = 0
    timer = setInterval(() => {
      elapsedMs = performance.now() - start
    }, 100)
  }

  function stopTimer() {
    if (timer) {
      clearInterval(timer)
      timer = null
    }
    lastMs = elapsedMs
  }

  onDestroy(() => {
    if (timer) clearInterval(timer)
  })

  async function generate() {
    loading = true
    error = ''
    ok = ''
    startTimer()
    try {
      message = await generateCommit()
    } catch (e) {
      error = (e as Error).message
    } finally {
      loading = false
      stopTimer()
    }
  }

  async function doCommit() {
    committing = true
    error = ''
    ok = ''
    try {
      ok = await commit(message)
      message = ''
    } catch (e) {
      error = (e as Error).message
    } finally {
      committing = false
    }
  }
</script>

<div class="head">
  <span>Commit con OpenRouter</span>
</div>
<textarea placeholder="Mensaje del commit..." bind:value={message}></textarea>
<button class="pill" onclick={generate} disabled={loading}>
  {loading ? 'Generando...' : 'Generar con OpenRouter'}
</button>
{#if loading}
  <div class="gen-row">
    <div class="progress" aria-label="Generando commit"><div class="bar"></div></div>
    <span class="elapsed">{(elapsedMs / 1000).toFixed(1)} s</span>
  </div>
{:else if lastMs > 0}
  <div class="elapsed done">Última generación: {(lastMs / 1000).toFixed(1)} s</div>
{/if}
<div class="hint">Se añadirán todos los cambios (git add -A)</div>
{#if error}<div class="error">{error}</div>{/if}
{#if ok}<div class="ok">{ok.split('\n')[0]}</div>{/if}
<button class="pill commit" onclick={doCommit} disabled={committing || !message.trim()}>
  {committing ? 'Commiteando...' : 'Commit'}
</button>

<style>
  .head { display:flex; justify-content:space-between; align-items:center; padding:14px; color:var(--accent); font-weight:700; font-size:13px; }
  textarea { width:calc(100% - 28px); margin:0 14px; height:120px; resize:vertical; background:var(--panel); color:var(--text); border:1px solid var(--border); border-radius:10px; padding:10px; font: inherit; font-size:12px; }
  .pill { display:block; margin:12px 14px 0; padding:8px 14px; border:none; border-radius:18px; background:color-mix(in srgb, var(--accent) 30%, var(--bg)); color:var(--text); font-size:12px; }
  .pill:hover { background:color-mix(in srgb, var(--accent) 50%, var(--bg)); }
  .pill:disabled { opacity:.55; cursor:default; }
  .gen-row { display:flex; align-items:center; gap:10px; margin:10px 14px 0; }
  .gen-row .progress { flex:1; }
  .progress { height:4px; border-radius:2px; background:color-mix(in srgb, var(--border) 55%, transparent); overflow:hidden; }
  .progress .bar { width:35%; height:100%; border-radius:2px; background:var(--accent); animation:indeterminate 1.1s ease-in-out infinite; }
  .elapsed { font-size:11px; color:var(--muted); font-variant-numeric:tabular-nums; flex-shrink:0; }
  .elapsed.done { margin:8px 14px 0; }
  @keyframes indeterminate { 0% { transform:translateX(-110%); } 100% { transform:translateX(320%); } }
  .commit { background:color-mix(in srgb, var(--green) 30%, var(--bg)); }
  .hint { margin:8px 14px; color:var(--muted); font-size:10px; }
  .error { margin:8px 14px; color:var(--red); font-size:11px; white-space:pre-wrap; }
  .ok { margin:8px 14px; color:var(--green); font-size:11px; }
</style>
