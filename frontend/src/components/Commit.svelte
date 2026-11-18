<script lang="ts">
  import { generateCommit, commit } from '../lib/bridge'

  let message = $state('')
  let loading = $state(false)
  let committing = $state(false)
  let error = $state('')
  let ok = $state('')

  async function generate() {
    loading = true
    error = ''
    ok = ''
    try {
      message = await generateCommit()
    } catch (e) {
      error = (e as Error).message
    } finally {
      loading = false
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
  <span>Commit con Ollama</span>
</div>
<textarea placeholder="Mensaje del commit..." bind:value={message}></textarea>
<button class="pill" onclick={generate} disabled={loading}>
  {loading ? 'Generando...' : 'Generar con Ollama'}
</button>
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
  .commit { background:color-mix(in srgb, var(--green) 30%, var(--bg)); }
  .hint { margin:8px 14px; color:var(--muted); font-size:10px; }
  .error { margin:8px 14px; color:var(--red); font-size:11px; white-space:pre-wrap; }
  .ok { margin:8px 14px; color:var(--green); font-size:11px; }
</style>
