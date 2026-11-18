<script lang="ts">
  import { svg } from '../lib/icons'
  import type { MenuItemInfo } from '../lib/bridge'
  let { item, accent, onOpen }: { item: MenuItemInfo; accent?: string; onOpen: () => void } = $props()
  const color = $derived(accent || 'var(--accent)')

  let showTip = $state(false)
  let tipX = $state(0)
  let tipY = $state(0)

  function moveTip(e: MouseEvent) {
    tipX = e.clientX
    tipY = e.clientY
  }
</script>

<button
  class="card"
  onclick={onOpen}
  style="--accent:{color}"
  onmouseenter={(e) => { moveTip(e); showTip = true }}
  onmousemove={moveTip}
  onmouseleave={() => (showTip = false)}
>
  <span class="icon">{@html svg(item.color, 22)}</span>
  <span class="text">
    <span class="label">{item.label}</span>
    <span class="desc">{item.description}</span>
  </span>
</button>

{#if showTip}
  <span class="tip" role="tooltip" style="left:{tipX}px; top:{tipY}px">
    <span class="tip-head">{item.label} <span class="tip-section">· {item.section}</span></span>
    <span class="tip-body">{item.description}</span>
  </span>
{/if}

<style>
  .card {
    display: flex; align-items: center; gap: 14px;
    background: color-mix(in srgb, var(--border) 40%, var(--bg));
    border: 1px solid var(--border);
    border-left: 3px solid var(--accent);
    border-radius: 12px; padding: 14px;
    text-align: left; color: var(--text);
    transition: background .12s ease, box-shadow .12s ease;
  }
  .card:hover {
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
  .tip {
    position: fixed; z-index: 1000;
    display: flex; flex-direction: column; gap: 4px;
    width: max-content; max-width: 280px;
    padding: 9px 11px;
    border: 1px solid var(--border); border-radius: 8px;
    background: var(--panel); color: var(--text);
    box-shadow: 0 6px 18px rgba(0,0,0,.45);
    pointer-events: none;
    transform: translate(-50%, calc(-100% - 16px));
  }
  .tip-head { font-size: 12px; font-weight: 700; color: var(--accent); }
  .tip-section { font-weight: 400; font-size: 10px; color: var(--muted); }
  .tip-body { font-size: 11px; color: var(--text); white-space: normal; line-height: 1.45; }
</style>
