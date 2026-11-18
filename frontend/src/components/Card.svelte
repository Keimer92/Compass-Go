<script lang="ts">
  import { svg } from '../lib/icons'
  import type { MenuItemInfo } from '../lib/bridge'
  let { item, accent, onOpen }: { item: MenuItemInfo; accent?: string; onOpen: () => void } = $props()
  const color = $derived(accent || 'var(--accent)')
</script>

<button class="card" onclick={onOpen} style="--accent:{color}">
  <span class="icon">{@html svg(item.color, 22)}</span>
  <span class="text">
    <span class="label">{item.label}</span>
    <span class="desc">{item.description}</span>
  </span>
</button>

<style>
  .card {
    display: flex; align-items: center; gap: 14px;
    background: color-mix(in srgb, var(--border) 40%, var(--bg));
    border: 1px solid var(--border);
    border-left: 3px solid var(--accent);
    border-radius: 12px; padding: 14px;
    text-align: left; color: var(--text);
    transition: transform .12s ease, background .12s ease, box-shadow .12s ease;
  }
  .card:hover {
    transform: translateY(-2px);
    background: color-mix(in srgb, var(--accent) 18%, var(--bg));
    box-shadow: 0 8px 20px rgba(0,0,0,.35);
  }
  .icon {
    display: grid; place-items: center;
    width: 38px; height: 38px; border-radius: 10px;
    background: color-mix(in srgb, var(--accent) 30%, transparent);
    color: var(--accent); flex-shrink: 0;
  }
  .text { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
  .label { font-size: 14px; font-weight: 600; }
  .desc {
    font-size: 11px; color: var(--muted);
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
</style>
