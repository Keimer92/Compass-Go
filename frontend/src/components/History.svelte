<script lang="ts">
  import { onMount } from 'svelte'
  import { gitLog } from '../lib/bridge'
  import type { CommitEntry } from '../lib/bridge'

  let commits = $state<CommitEntry[]>([])
  let loading = $state(true)
  let error = $state('')

  async function load() {
    loading = true
    error = ''
    try {
      commits = await gitLog()
    } catch (e) {
      error = (e as Error).message
    } finally {
      loading = false
    }
  }

  onMount(load)
</script>

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
      <li title={c.hash}>
        <div class="row">
          <span class="hash">{c.short}</span>
          <span class="date">{c.date}</span>
        </div>
        <div class="subject">{c.subject}</div>
        <div class="author">{c.author}</div>
      </li>
    {/each}
  </ul>
{/if}

<style>
  .head { display:flex; justify-content:space-between; align-items:center; padding:14px; color:var(--accent); font-weight:700; font-size:13px; }
  .refresh { border:none; background:transparent; color:var(--muted); font-size:15px; cursor:pointer; line-height:1; }
  .refresh:hover { color:var(--text); }
  .pad { padding:0 14px 14px; font-size:12px; }
  .muted { color:var(--muted); }
  .error { color:var(--red); font-size:11px; white-space:pre-wrap; }
  .log { list-style:none; margin:0; padding:0 10px 14px; display:flex; flex-direction:column; gap:2px; }
  .log li { padding:8px 10px; border-radius:8px; border:1px solid transparent; }
  .log li:hover { background:color-mix(in srgb, var(--border) 30%, transparent); border-color:var(--border); }
  .row { display:flex; justify-content:space-between; align-items:center; gap:8px; }
  .hash { font-family:'Cascadia Mono', Consolas, monospace; font-size:11px; color:var(--accent); }
  .date { font-size:10px; color:var(--muted); flex-shrink:0; }
  .subject { margin-top:3px; font-size:12px; color:var(--text); overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
  .author { margin-top:2px; font-size:10px; color:var(--muted); }
</style>
