<script lang="ts">
  import { svg, platformSvg } from '../lib/icons'
  import type { MenuItemInfo } from '../lib/bridge'
  let { item, accent, onOpen }: { item: MenuItemInfo; accent?: string; onOpen: () => void } = $props()
  const color = $derived(accent || 'var(--accent)')

  // Etiqueta legible del distintivo de plataforma.
  const platformLabel: Record<string, string> = {
    windows: 'Tarea de Windows',
    android: 'Tarea de Android',
    go: 'Tarea de Go',
    general: 'Tarea general',
  }

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
  <span class="icon">
    {@html svg(item.color, 22)}
    <span
      class="platform {item.platform}"
      title={platformLabel[item.platform] ?? 'Tarea general'}
      aria-label={platformLabel[item.platform] ?? 'Tarea general'}
    >{@html platformSvg(item.platform, 16)}</span>
  </span>
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
    display: flex; align-items: center; gap: 13px;
    background: color-mix(in srgb, var(--border) 32%, var(--bg));
    border: 1px solid color-mix(in srgb, var(--border) 80%, transparent);
    border-radius: 14px; padding: 13px 14px;
    text-align: left; color: var(--text);
    box-shadow: var(--shadow-1);
    transition: background .16s ease, box-shadow .16s ease, transform .16s ease, border-color .16s ease;
  }
  .card:hover {
    background: color-mix(in srgb, var(--accent) 14%, var(--bg));
    border-color: color-mix(in srgb, var(--accent) 42%, var(--border));
    box-shadow: var(--shadow-2);
    transform: translateY(-1px);
  }
  .card:active { transform: translateY(0) scale(.995); }
  .icon {
    position: relative;
    display: grid; place-items: center;
    width: 36px; height: 36px; border-radius: 11px;
    background: color-mix(in srgb, var(--accent) 26%, transparent);
    color: var(--accent); flex-shrink: 0;
    box-shadow: inset 0 1px 0 rgba(255,255,255,.12);
  }
  /* Distintivo de plataforma: se superpone al icono de la card. */
  .platform {
    position: absolute; right: -7px; bottom: -7px;
    width: 25px; height: 25px; border-radius: 50%;
    display: grid; place-items: center;
    background: var(--panel);
    border: 1px solid color-mix(in srgb, currentColor 55%, var(--border));
    box-shadow: 0 1px 4px rgba(0,0,0,.45);
  }
  .platform.windows { color: #4cc2ff; }
  .platform.android { color: #3ddc84; }
  .platform.go { color: #00add8; }
  .platform.general { color: var(--muted); }
  .text { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
  .label { font-size: 13.5px; font-weight: 600; letter-spacing: -0.01em; }
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
