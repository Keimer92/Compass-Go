<script lang="ts">
  import { onMount } from 'svelte'
  import { gitStatus, fileTree } from '../lib/bridge'
  import type { Change, FileNode } from '../lib/bridge'

  let changes = $state<Change[]>([])
  let tree = $state<FileNode[]>([])
  let expanded = $state<Set<string>>(new Set())
  let error = $state('')

  async function refresh() {
    try {
      error = ''
      changes = await gitStatus()
      tree = await fileTree()
    } catch (e) {
      error = (e as Error).message
    }
  }

  onMount(refresh)

  function toggle(path: string) {
    const next = new Set(expanded)
    next.has(path) ? next.delete(path) : next.add(path)
    expanded = next
  }

  function info(code: string) {
    if (code.includes('?')) return { letter: 'U', label: 'Untracked', color: 'var(--green)' }
    if (code.includes('D')) return { letter: 'D', label: 'Deleted', color: 'var(--red)' }
    if (code.includes('A')) return { letter: 'A', label: 'Added', color: 'var(--green)' }
    if (code.includes('R')) return { letter: 'R', label: 'Renamed', color: 'var(--brand)' }
    if (code.includes('M')) return { letter: 'M', label: 'Modified', color: 'var(--yellow)' }
    return { letter: '?', label: 'Changed', color: 'var(--muted)' }
  }

  function shortName(p: string) {
    const parts = p.split(/[\\/]/)
    return parts[parts.length - 1]
  }
</script>

<div class="head">
  <span>EXPLORADOR</span>
  <button title="Refrescar" onclick={refresh}>↻</button>
</div>
{#if error}<div class="error">{error}</div>{/if}
<div class="section">CAMBIOS GIT ({changes.length})</div>
<div class="changes">
  {#each changes as c}
    {@const s = info(c.code)}
    <div class="row" title={c.path}>
      <span class="badge" style="color:{s.color};background:color-mix(in srgb,{s.color} 22%,transparent)">{s.letter}</span>
      <span class="file">{c.path}</span>
      <span class="status" style="color:{s.color}">{s.label}</span>
    </div>
  {:else}
    <div class="muted pad">Sin cambios pendientes.</div>
  {/each}
</div>
<div class="section">ARCHIVOS</div>
<div class="tree">
  {#each tree as node}
    <div class="row">
      {#if node.isDir && node.children?.length}
        <button class="chev" onclick={() => toggle(node.path)}>{expanded.has(node.path) ? '▾' : '▸'}</button>
      {:else}
        <span class="chev-space"></span>
      {/if}
      <span class="file" class:dir={node.isDir}>{node.name}</span>
    </div>
    {#if node.isDir && expanded.has(node.path)}
      {#each node.children ?? [] as child}
        <div class="row child">
          <span class="chev-space"></span>
          <span class="file" class:dir={child.isDir}>{child.name}</span>
        </div>
      {/each}
    {/if}
  {/each}
</div>

<style>
  .head { display:flex; justify-content:space-between; padding:14px 14px 8px; color:var(--accent); font-weight:700; font-size:12px; letter-spacing:.08em; }
  .head button { background:none; border:none; color:var(--muted); font-size:15px; }
  .section { padding:12px 14px 6px; color:var(--muted); font-size:11px; letter-spacing:.08em; }
  .row { display:flex; align-items:center; gap:9px; padding:4px 12px; font-size:12px; }
  .row.child { padding-left:26px; }
  .badge { width:17px; height:17px; border-radius:50%; display:grid; place-items:center; font-weight:700; font-size:10px; flex-shrink:0; }
  .file { flex:1; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
  .file.dir { color:var(--brand); }
  .status { font-size:10px; }
  .chev { width:14px; background:none; border:none; color:var(--muted); font-size:10px; padding:0; }
  .chev-space { width:14px; flex-shrink:0; }
  .muted { color:var(--muted); font-size:12px; }
  .pad { padding:6px 14px; }
  .error { margin:6px 14px; color:var(--red); font-size:11px; }
</style>
