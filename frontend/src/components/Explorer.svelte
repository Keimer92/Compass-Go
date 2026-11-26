<script lang="ts">
  import { onMount } from 'svelte'
  import { gitStatus, gitBranch, listDir, fileDelete, fileRename, fileCopy, fileMove } from '../lib/bridge'
  import type { Change, FileNode } from '../lib/bridge'
  import { onRepoChanged } from '../lib/repoChanged'
  import { peekExplorer, saveExplorer } from '../lib/panelCache'
  import { svg } from '../lib/icons'

  type Row = { node: FileNode; depth: number }

  let { projectPath = '' }: { projectPath?: string } = $props()

  // Si ya se visito la pestaña, se arranca con el snapshot anterior: aparece
  // pintada al instante y el refresco real ocurre en segundo plano.
  const snap = peekExplorer(projectPath)
  let changes = $state<Change[]>(snap?.changes ?? [])
  let branch = $state(snap?.branch ?? '')
  let rootNodes = $state<FileNode[]>(snap?.roots ?? [])
  let children = $state<Record<string, FileNode[]>>(snap?.children ?? {})
  let expanded = $state<Set<string>>(new Set(snap?.expanded ?? []))
  let selected = $state<FileNode | null>(null)
  let error = $state('')
  let refreshing = $state(false)
  let busy = $state(false)
  let queued = false

  // Menú contextual, portapapeles propio (no el de Windows: aquí se copian
  // rutas del proyecto) y edición en línea del nombre.
  let menu = $state<{ x: number; y: number; node: FileNode | null } | null>(null)
  let clip = $state<{ paths: string[]; mode: 'copy' | 'cut' } | null>(null)
  let renaming = $state<{ path: string; value: string } | null>(null)
  let confirming = $state<FileNode | null>(null)

  // El estado de git llega con rutas relativas ("src/main.go"); el nodo trae
  // la misma ruta relativa en `rel`, así se pintan los cambios en el árbol.
  let changeMap = $derived.by(() => {
    const map: Record<string, Change> = {}
    for (const c of changes) map[c.path] = c
    return map
  })

  let rows = $derived(flatten(rootNodes, 0, expanded, children))

  function flatten(
    nodes: FileNode[],
    depth: number,
    open: Set<string>,
    kids: Record<string, FileNode[]>
  ): Row[] {
    const out: Row[] = []
    for (const node of nodes) {
      out.push({ node, depth })
      if (node.isDir && open.has(node.path)) {
        out.push(...flatten(kids[node.path] ?? [], depth + 1, open, kids))
      }
    }
    return out
  }

  async function refresh() {
    // Igual que antes: si ya hay una lectura en curso, se repite al terminar
    // para no perder un cambio ocurrido mientras se leía.
    if (refreshing) {
      queued = true
      return
    }
    refreshing = true
    try {
      do {
        queued = false
        error = ''
        // Las tres consultas son independientes: en serie sumaban casi dos
        // segundos en Windows, en paralelo tardan lo que la mas lenta.
        const [ch, br, roots] = await Promise.all([gitStatus(), gitBranch(), listDir('')])
        changes = ch
        branch = br
        rootNodes = roots
        // Se recargan solo las carpetas abiertas, y se olvidan las que ya no
        // existen (borradas o renombradas desde fuera).
        const open = [...expanded]
        const lists = await Promise.all(
          open.map((path) => listDir(path).then((l) => [path, l] as const).catch(() => null))
        )
        const next: Record<string, FileNode[]> = {}
        for (const entry of lists) {
          if (entry) next[entry[0]] = entry[1]
        }
        children = next
        expanded = new Set(open.filter((p) => next[p] !== undefined))
        if (selected && !existsIn(next, rootNodes, selected.path)) selected = null
        saveExplorer(projectPath, {
          changes,
          branch,
          roots: rootNodes,
          children,
          expanded: [...expanded]
        })
      } while (queued)
    } catch (e) {
      error = (e as Error).message
    } finally {
      refreshing = false
    }
  }

  function existsIn(
    kids: Record<string, FileNode[]>,
    roots: FileNode[],
    path: string
  ): boolean {
    for (const list of [roots, ...Object.values(kids)]) {
      if (list.some((n) => n.path === path)) return true
    }
    return false
  }

  onMount(() => {
    refresh()
    return onRepoChanged(refresh)
  })

  async function toggle(node: FileNode) {
    const next = new Set(expanded)
    if (next.has(node.path)) {
      next.delete(node.path)
      expanded = next
      return
    }
    next.add(node.path)
    expanded = next
    if (!children[node.path]) {
      try {
        children[node.path] = await listDir(node.path)
      } catch (e) {
        error = (e as Error).message
      }
    }
  }

  function info(code: string) {
    if (code.includes('?')) return { letter: 'U', color: 'var(--green)' }
    if (code.includes('D')) return { letter: 'D', color: 'var(--red)' }
    if (code.includes('A')) return { letter: 'A', color: 'var(--green)' }
    if (code.includes('R')) return { letter: 'R', color: 'var(--brand)' }
    if (code.includes('M')) return { letter: 'M', color: 'var(--yellow)' }
    return { letter: '?', color: 'var(--muted)' }
  }

  function parentOf(path: string): string {
    return path.replace(/[\\/][^\\/]+$/, '')
  }

  // ── Menú contextual ───────────────────────────────────────────────────────
  function openMenu(e: MouseEvent, node: FileNode | null) {
    e.preventDefault()
    e.stopPropagation()
    if (node) selected = node
    const width = 196
    const height = 232
    menu = {
      x: Math.min(e.clientX, window.innerWidth - width - 8),
      y: Math.min(e.clientY, window.innerHeight - height - 8),
      node
    }
  }

  function closeMenu() {
    menu = null
  }

  function targetDir(node: FileNode | null): string {
    if (!node) return ''
    return node.isDir ? node.path : parentOf(node.path)
  }

  function copySelection(mode: 'copy' | 'cut') {
    const node = menu?.node ?? selected
    if (!node) return
    clip = { paths: [node.path], mode }
    closeMenu()
  }

  async function paste() {
    const node = menu?.node ?? selected
    if (!clip) return
    const dest = targetDir(node)
    closeMenu()
    busy = true
    try {
      if (clip.mode === 'copy') await fileCopy(clip.paths, dest)
      else await fileMove(clip.paths, dest)
      if (clip.mode === 'cut') clip = null
      error = ''
      await refresh()
    } catch (e) {
      error = (e as Error).message
    } finally {
      busy = false
    }
  }

  async function startRename(node: FileNode | null) {
    if (!node) return
    renaming = { path: node.path, value: node.name }
    closeMenu()
  }

  async function commitRename() {
    const target = renaming
    if (!target) return
    renaming = null
    if (target.value.trim() === '' || target.value === baseName(target.path)) return
    busy = true
    try {
      await fileRename(target.path, target.value)
      error = ''
      await refresh()
    } catch (e) {
      error = (e as Error).message
    } finally {
      busy = false
    }
  }

  function baseName(path: string): string {
    const parts = path.split(/[\\/]/)
    return parts[parts.length - 1]
  }

  async function confirmDelete() {
    const node = confirming
    confirming = null
    if (!node) return
    busy = true
    try {
      await fileDelete(node.path)
      error = ''
      if (selected?.path === node.path) selected = null
      await refresh()
    } catch (e) {
      error = (e as Error).message
    } finally {
      busy = false
    }
  }

  function onKey(e: KeyboardEvent) {
    if (renaming) return
    const ctrl = e.ctrlKey || e.metaKey
    if (ctrl && e.key.toLowerCase() === 'c' && selected) {
      e.preventDefault()
      clip = { paths: [selected.path], mode: 'copy' }
      return
    }
    if (ctrl && e.key.toLowerCase() === 'x' && selected) {
      e.preventDefault()
      clip = { paths: [selected.path], mode: 'cut' }
      return
    }
    if (ctrl && e.key.toLowerCase() === 'v' && clip) {
      e.preventDefault()
      paste()
      return
    }
    if (e.key === 'F2' && selected) {
      e.preventDefault()
      startRename(selected)
      return
    }
    if (e.key === 'Delete' && selected) {
      e.preventDefault()
      confirming = selected
    }
  }

  function iconFor(node: FileNode): string {
    if (node.isDir) return expanded.has(node.path) ? 'folderOpen' : 'folder'
    return 'file'
  }

  // Acción: al aparecer el input de renombrado, lo enfoca y selecciona el
  // nombre sin la extensión (como el Explorador de Windows).
  function focusSelect(node: HTMLInputElement) {
    node.focus()
    const dot = node.value.lastIndexOf('.')
    node.setSelectionRange(0, dot > 0 ? dot : node.value.length)
  }
</script>

<div class="head">
  <span>EXPLORADOR</span>
  <div class="head-actions">
    {#if clip}
      <button class="chip" onclick={() => (clip = null)} title="Cancelar la copia">
        {clip.mode === 'cut' ? 'Cortado' : 'Copiado'} ×
      </button>
    {/if}
    <button
      class="refresh"
      class:spin={refreshing}
      onclick={refresh}
      disabled={refreshing}
      title="Refrescar cambios git y archivos"
      aria-label="Refrescar explorador"
    >{@html svg('reload', 14)}</button>
  </div>
</div>
{#if error}<div class="error">{error}</div>{/if}

{#if branch}
  <div class="branch" title="Rama actual de git">
    <span class="branch-ico">{@html svg('branch', 13)}</span>
    <span class="branch-name">{branch}</span>
  </div>
{/if}

<div class="section">CAMBIOS GIT ({changes.length})</div>
<div class="changes">
  {#each changes as c}
    {@const s = info(c.code)}
    <div class="row change" title={c.path}>
      <span class="badge" style="color:{s.color};background:color-mix(in srgb,{s.color} 22%,transparent)">{s.letter}</span>
      <span class="file">{c.path}</span>
      <span class="status" style="color:{s.color}">{c.status}</span>
    </div>
  {:else}
    <div class="muted pad">Sin cambios pendientes.</div>
  {/each}
</div>

<div class="section">
  ARCHIVOS
  {#if busy}<span class="working">…</span>{/if}
</div>
<div
  class="tree"
  role="tree"
  tabindex="0"
  onkeydown={onKey}
  oncontextmenu={(e) => openMenu(e, null)}
>
  {#each rows as row (row.node.path)}
    {@const changed = changeMap[row.node.rel]}
    <div
      class="row node"
      class:selected={selected?.path === row.node.path}
      style="padding-left:{8 + Math.min(row.depth, 8) * 14}px"
      role="treeitem"
      aria-selected={selected?.path === row.node.path}
      title={row.node.rel}
      onclick={(e) => {
        selected = row.node
        // El teclado (Ctrl+C/X/V, F2, Supr) vive en el contenedor del árbol:
        // hay que darle el foco al seleccionar una fila.
        ;(e.currentTarget.closest('.tree') as HTMLElement | null)?.focus()
      }}
      ondblclick={() => row.node.isDir && toggle(row.node)}
      oncontextmenu={(e) => openMenu(e, row.node)}
    >
      {#if row.node.isDir}
        <button
          class="chev"
          class:open={expanded.has(row.node.path)}
          onclick={(e) => {
            e.stopPropagation()
            toggle(row.node)
          }}
          aria-label={expanded.has(row.node.path) ? 'Contraer' : 'Desplegar'}
        >{@html svg('chevron', 12)}</button>
      {:else}
        <span class="chev-space"></span>
      {/if}
      <span class="ico" class:dir={row.node.isDir}>{@html svg(iconFor(row.node), 14)}</span>
      {#if renaming?.path === row.node.path}
        <input
          class="rename"
          value={renaming.value}
          use:focusSelect
          oninput={(e) => renaming && (renaming.value = e.currentTarget.value)}
          onkeydown={(e) => {
            if (e.key === 'Enter') commitRename()
            if (e.key === 'Escape') renaming = null
          }}
          onblur={commitRename}
        />
      {:else}
        <span class="name" class:dir={row.node.isDir}>{row.node.name}</span>
      {/if}
      {#if changed}
        <span class="dot-change" style="background:{info(changed.code).color}"></span>
      {/if}
    </div>
  {:else}
    <div class="muted pad">Sin archivos.</div>
  {/each}
</div>

{#if menu}
  <div
    class="menu-backdrop"
    role="presentation"
    onclick={closeMenu}
    oncontextmenu={(e) => {
      e.preventDefault()
      closeMenu()
    }}
  ></div>
  <div class="ctx" style="left:{menu.x}px; top:{menu.y}px" role="menu">
    {#if menu.node}
      <div class="ctx-title" title={menu.node.rel}>{menu.node.name}</div>
      <button role="menuitem" onclick={() => copySelection('copy')}>
        <span class="ctx-ico">{@html svg('copy', 14)}</span>Copiar
      </button>
      <button role="menuitem" onclick={() => copySelection('cut')}>
        <span class="ctx-ico">{@html svg('cut', 14)}</span>Cortar
      </button>
      <button role="menuitem" disabled={!clip} onclick={paste}>
        <span class="ctx-ico">{@html svg('paste', 14)}</span>Pegar aquí
      </button>
      <div class="ctx-sep"></div>
      <button role="menuitem" onclick={() => startRename(menu?.node ?? null)}>
        <span class="ctx-ico">{@html svg('pencil', 14)}</span>Renombrar
      </button>
      <button role="menuitem" class="danger" onclick={() => { confirming = menu?.node ?? null; closeMenu() }}>
        <span class="ctx-ico">{@html svg('clean', 14)}</span>Eliminar
      </button>
    {:else}
      <button role="menuitem" disabled={!clip} onclick={paste}>
        <span class="ctx-ico">{@html svg('paste', 14)}</span>Pegar en la raíz
      </button>
      <button role="menuitem" onclick={refresh}>
        <span class="ctx-ico">{@html svg('reload', 14)}</span>Refrescar
      </button>
    {/if}
  </div>
{/if}

{#if confirming}
  <div class="modal-backdrop" role="presentation" onclick={() => (confirming = null)}>
    <div class="modal confirm" role="dialog" aria-label="Eliminar" onclick={(e) => e.stopPropagation()}>
      <div class="modal-head"><span>Eliminar</span></div>
      <p class="confirm-text">
        ¿Eliminar <strong>{confirming.name}</strong>{confirming.isDir ? ' y todo su contenido' : ''}?
        Esta acción no se puede deshacer.
      </p>
      <div class="confirm-actions">
        <button class="pill ghost" onclick={() => (confirming = null)}>Cancelar</button>
        <button class="pill danger" onclick={confirmDelete}>Eliminar</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .head { display:flex; align-items:center; justify-content:space-between; padding:14px 14px 8px; color:var(--accent); font-weight:700; font-size:11px; letter-spacing:.08em; }
  .head-actions { display:flex; align-items:center; gap:6px; }
  .chip { padding:2px 8px; border-radius:10px; border:1px solid var(--border); background:color-mix(in srgb, var(--accent) 16%, transparent); color:var(--text); font-size:10px; cursor:pointer; }
  .refresh { display:grid; place-items:center; width:26px; height:26px; padding:0; border:1px solid var(--border); border-radius:8px; background:color-mix(in srgb, var(--panel) 50%, transparent); color:var(--muted); cursor:pointer; transition:color .15s ease, background .15s ease, border-color .15s ease, transform .1s ease; }
  .refresh:hover:not(:disabled) { color:var(--accent); border-color:color-mix(in srgb, var(--accent) 55%, var(--border)); background:color-mix(in srgb, var(--accent) 14%, transparent); }
  .refresh:active:not(:disabled) { transform:scale(.9); }
  .refresh:disabled { opacity:.65; cursor:default; }
  .refresh.spin :global(svg) { animation:spin .8s linear infinite; }
  @keyframes spin { to { transform:rotate(360deg); } }
  .section { display:flex; align-items:center; gap:8px; padding:12px 14px 6px; color:var(--muted); font-size:10px; letter-spacing:.08em; }
  .branch { display:flex; align-items:center; gap:7px; margin:2px 12px 0; padding:5px 9px; border-radius:9px; background:color-mix(in srgb, var(--brand) 14%, transparent); border:1px solid color-mix(in srgb, var(--brand) 28%, transparent); color:var(--text); font-size:11px; }
  .branch-ico { display:grid; place-items:center; color:var(--brand); flex-shrink:0; }
  .branch-name { font-family:'Cascadia Mono', Consolas, monospace; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
  .working { color:var(--accent); }
  .row { display:flex; align-items:center; gap:7px; min-width:0; padding:4px 12px; font-size:12px; }
  .changes .row { padding:4px 14px; }
  .tree { outline:none; padding-bottom:10px; overflow-x:hidden; }
  .tree:focus-visible { box-shadow:inset 0 0 0 1px color-mix(in srgb, var(--accent) 45%, transparent); }
  .row.node { cursor:default; border-radius:6px; margin:0 6px; padding-top:3px; padding-bottom:3px; }
  .row.node:hover { background:color-mix(in srgb, var(--border) 45%, transparent); }
  .row.node.selected { background:color-mix(in srgb, var(--accent) 24%, transparent); }
  .badge { width:17px; height:17px; border-radius:50%; display:grid; place-items:center; font-weight:700; font-size:10px; flex-shrink:0; }
  .file { flex:1; min-width:0; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
  .name { flex:1; min-width:0; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
  .name.dir { color:var(--brand); }
  .status { font-size:10px; }
  .chev { display:grid; place-items:center; width:14px; height:14px; padding:0; border:none; background:none; color:var(--muted); cursor:pointer; transition:transform .14s ease, color .14s ease; }
  .chev.open { transform:rotate(90deg); color:var(--accent); }
  .chev:hover { color:var(--text); }
  .chev-space { width:14px; flex-shrink:0; }
  .ico { display:grid; place-items:center; color:var(--muted); flex-shrink:0; }
  .ico.dir { color:var(--brand); }
  .rename { flex:1; min-width:0; padding:2px 6px; border:1px solid var(--accent); border-radius:6px; background:var(--panel); color:var(--text); font:inherit; font-size:12px; outline:none; }
  .dot-change { width:6px; height:6px; border-radius:50%; flex-shrink:0; }
  .muted { color:var(--muted); font-size:12px; }
  .pad { padding:6px 14px; }
  .error { margin:6px 14px; color:var(--red); font-size:11px; }
  .menu-backdrop { position:fixed; inset:0; z-index:60; }
  .ctx { position:fixed; z-index:61; min-width:190px; padding:6px; border:1px solid var(--border); border-radius:12px; background:color-mix(in srgb, var(--panel) 92%, var(--bg)); box-shadow:0 14px 34px rgba(0,0,0,.35); backdrop-filter:blur(18px); }
  .ctx-title { padding:4px 8px 6px; color:var(--muted); font-size:10px; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
  .ctx button { display:flex; align-items:center; gap:9px; width:100%; padding:6px 8px; border:none; border-radius:8px; background:none; color:var(--text); font:inherit; font-size:12px; text-align:left; cursor:pointer; }
  .ctx button:hover:not(:disabled) { background:color-mix(in srgb, var(--accent) 22%, transparent); }
  .ctx button:disabled { opacity:.4; cursor:default; }
  .ctx button.danger:hover { background:color-mix(in srgb, var(--red) 26%, transparent); }
  .ctx-ico { display:grid; place-items:center; width:16px; color:var(--muted); }
  .ctx-sep { height:1px; margin:4px 6px; background:var(--border); }
  .confirm { width:330px; }
  .confirm-text { margin:0 0 16px; font-size:12px; color:var(--text); line-height:1.5; }
  .confirm-actions { display:flex; justify-content:flex-end; gap:8px; }
  .pill { padding:7px 14px; border:none; border-radius:14px; font:inherit; font-size:12px; cursor:pointer; }
  .pill.ghost { background:color-mix(in srgb, var(--border) 60%, transparent); color:var(--text); }
  .pill.danger { background:color-mix(in srgb, var(--red) 45%, var(--bg)); color:var(--text); }
</style>
