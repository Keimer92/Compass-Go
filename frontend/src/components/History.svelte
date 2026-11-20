<script lang="ts">
  import { onMount } from 'svelte'
  import { gitLog, commitDetail } from '../lib/bridge'
  import type { CommitEntry, CommitDetail } from '../lib/bridge'
  import { onRepoChanged } from '../lib/repoChanged'
  import { peekLog, saveLog } from '../lib/panelCache'

  let { projectPath = '' }: { projectPath?: string } = $props()

  // Con cache la pestaña aparece pintada al instante; el `git log` real se
  // lanza igualmente para traer commits nuevos.
  const cached = peekLog(projectPath)
  let commits = $state<CommitEntry[]>(cached ?? [])
  let loading = $state(cached === null)
  let error = $state('')

  // Detalle del commit abierto. Mientras carga se muestra la cabecera que ya
  // tenemos del historial, asi el modal aparece al instante.
  let detail = $state<CommitDetail | null>(null)
  let detailLoading = $state(false)
  let detailError = $state('')

  async function load() {
    // Sin spinner si ya hay commits en pantalla: solo se actualiza la lista.
    loading = commits.length === 0
    error = ''
    try {
      commits = await gitLog()
      saveLog(projectPath, commits)
    } catch (e) {
      error = (e as Error).message
    } finally {
      loading = false
    }
  }

  async function open(c: CommitEntry) {
    detailLoading = true
    detailError = ''
    detail = { ...c, body: '', files: [] }
    try {
      detail = await commitDetail(c.hash)
    } catch (e) {
      detailError = (e as Error).message
    } finally {
      detailLoading = false
    }
  }

  function close() {
    detail = null
    detailError = ''
  }

  function statusLetter(status: string): string {
    switch (status) {
      case 'Added': return 'A'
      case 'Deleted': return 'D'
      case 'Renamed': return 'R'
      case 'Copied': return 'C'
      case 'Type': return 'T'
      default: return 'M'
    }
  }

  function statusColor(status: string): string {
    switch (status) {
      case 'Added': return 'var(--green)'
      case 'Deleted': return 'var(--red)'
      case 'Renamed':
      case 'Copied': return 'var(--brand)'
      default: return 'var(--yellow)'
    }
  }

  onMount(() => {
    load()
    return onRepoChanged(load)
  })
</script>

<svelte:window
  onkeydown={(e) => {
    if (e.key === 'Escape' && detail) close()
  }}
/>

<div class="head">
  <span>Historial de la rama</span>
  <button class="refresh" onclick={load} title="Actualizar" aria-label="Actualizar">⟳</button>
</div>

{#if loading}
  <div class="muted pad">Cargando…</div>
{:else if error}
  <div class="error pad">{error}</div>
{:else if !commits.length}
  <div class="muted pad">Sin commits en esta rama.</div>
{:else}
  <ul class="log">
    {#each commits as c (c.hash)}
      <li>
        <button class="entry" onclick={() => open(c)} title="Ver qué se commiteó">
          <div class="row">
            <span class="hash">{c.short}</span>
            <span class="date">{c.date}</span>
          </div>
          <div class="subject">{c.subject}</div>
          <div class="author">{c.author}</div>
        </button>
      </li>
    {/each}
  </ul>
{/if}

{#if detail}
  <div class="modal-backdrop" role="presentation" onclick={close}>
    <div class="modal commit" role="dialog" aria-label="Detalle del commit" onclick={(e) => e.stopPropagation()}>
      <div class="modal-head">
        <span class="modal-title">{detail.subject}</span>
        <button onclick={close} aria-label="Cerrar">×</button>
      </div>

      <div class="meta">
        <span class="hash">{detail.short}</span>
        <span>{detail.author}</span>
        <span>{detail.date}</span>
      </div>

      {#if detail.body}
        <pre class="body">{detail.body}</pre>
      {/if}

      {#if detailLoading}
        <div class="muted pad0">Leyendo archivos…</div>
      {:else if detailError}
        <div class="error pad0">{detailError}</div>
      {:else}
        <div class="files-head">
          ARCHIVOS ({detail.files.length})
        </div>
        <ul class="files">
          {#each detail.files as f (f.path)}
            {@const color = statusColor(f.status)}
            <li>
              <span class="badge" style="color:{color};background:color-mix(in srgb,{color} 22%,transparent)">
                {statusLetter(f.status)}
              </span>
              <span class="path" title={f.path}>{f.path}</span>
              {#if f.binary}
                <span class="bin">bin</span>
              {:else}
                <span class="stat"><b class="add">+{f.added}</b> <b class="del">−{f.removed}</b></span>
              {/if}
            </li>
          {:else}
            <li class="muted pad0">Sin archivos (commit vacío).</li>
          {/each}
        </ul>
      {/if}
    </div>
  </div>
{/if}

<style>
  .head { display:flex; justify-content:space-between; align-items:center; padding:14px; color:var(--accent); font-weight:700; font-size:13px; }
  .refresh { border:none; background:transparent; color:var(--muted); font-size:15px; cursor:pointer; line-height:1; }
  .refresh:hover { color:var(--text); }
  .pad { padding:0 14px 14px; font-size:12px; }
  .pad0 { padding:4px 0; font-size:12px; }
  .muted { color:var(--muted); }
  .error { color:var(--red); font-size:11px; white-space:pre-wrap; }
  .log { list-style:none; margin:0; padding:0 10px 14px; display:flex; flex-direction:column; gap:2px; }
  .log li { border-radius:8px; border:1px solid transparent; }
  .log li:hover { background:color-mix(in srgb, var(--border) 30%, transparent); border-color:var(--border); }
  .entry { display:block; width:100%; padding:8px 10px; border:none; border-radius:8px; background:none; color:inherit; font:inherit; text-align:left; cursor:pointer; }
  .row { display:flex; justify-content:space-between; align-items:center; gap:8px; }
  .hash { font-family:'Cascadia Mono', Consolas, monospace; font-size:11px; color:var(--accent); }
  .date { font-size:10px; color:var(--muted); flex-shrink:0; }
  .subject { margin-top:3px; font-size:12px; color:var(--text); overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
  .author { margin-top:2px; font-size:10px; color:var(--muted); }

  .modal-backdrop { position:fixed; inset:0; z-index:70; background:rgba(0,0,0,.6); display:grid; place-items:center; }
  .modal.commit { width:560px; max-width:92vw; max-height:82vh; display:flex; flex-direction:column; background:var(--bg); border:1px solid var(--border); border-radius:16px; padding:18px; box-shadow:0 18px 46px rgba(0,0,0,.42); }
  .modal-head { display:flex; align-items:flex-start; justify-content:space-between; gap:12px; }
  .modal-title { color:var(--text); font-size:14px; font-weight:600; line-height:1.35; }
  .modal-head button { flex-shrink:0; border:none; background:none; color:var(--muted); font-size:18px; line-height:1; cursor:pointer; }
  .modal-head button:hover { color:var(--text); }
  .meta { display:flex; flex-wrap:wrap; gap:10px; margin:8px 0 2px; color:var(--muted); font-size:11px; }
  .body { margin:10px 0 0; padding:10px 12px; border-radius:10px; background:color-mix(in srgb, var(--panel) 70%, transparent); color:var(--text); font-family:inherit; font-size:11.5px; line-height:1.5; white-space:pre-wrap; word-break:break-word; max-height:26vh; overflow-y:auto; }
  .files-head { margin:14px 0 6px; color:var(--muted); font-size:10px; letter-spacing:.08em; }
  .files { list-style:none; margin:0; padding:0; display:flex; flex-direction:column; gap:2px; overflow-y:auto; }
  .files li { display:flex; align-items:center; gap:8px; padding:4px 6px; border-radius:7px; font-size:11.5px; }
  .files li:hover { background:color-mix(in srgb, var(--border) 35%, transparent); }
  .badge { width:16px; height:16px; border-radius:50%; display:grid; place-items:center; font-weight:700; font-size:9px; flex-shrink:0; }
  .path { flex:1; min-width:0; font-family:'Cascadia Mono', Consolas, monospace; color:var(--text); white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
  .stat { flex-shrink:0; font-family:'Cascadia Mono', Consolas, monospace; font-size:10px; }
  .add { color:var(--green); font-weight:600; }
  .del { color:var(--red); font-weight:600; }
  .bin { flex-shrink:0; color:var(--muted); font-size:10px; }
</style>
