<script lang="ts">
  import { onMount } from 'svelte'
  import { getSystemInfo, type SystemInfo } from '../lib/bridge'

  // Ultimo snapshot del equipo. Se refresca cada 2 s; si una lectura falla se
  // conserva la anterior para no parpadear.
  let info = $state<SystemInfo | null>(null)

  const GB = 1024 ** 3
  const gb = (n: number) => (n / GB).toFixed(1)
  const pct = (used: number, total: number) =>
    total > 0 ? Math.min(100, Math.max(0, (used / total) * 100)) : 0

  onMount(() => {
    let alive = true
    const tick = async () => {
      try {
        const next = await getSystemInfo()
        if (alive) info = next
      } catch {
        /* se mantiene el ultimo valor */
      }
    }
    void tick()
    const timer = setInterval(tick, 2000)
    return () => {
      alive = false
      clearInterval(timer)
    }
  })
</script>

<div class="sys">
  <div class="sys-title">Sistema</div>
  {#if !info}
    <p class="sys-load">Leyendo…</p>
  {:else}
    <div class="row" title={info.cpu.name || 'CPU'}>
      <span>CPU</span>
      <b>{info.cpu.usage.toFixed(0)}%</b>
    </div>
    <div class="bar"><i style={`width:${info.cpu.usage}%`}></i></div>
    <div class="sub" title={info.cpu.name}>
      {info.cpu.name || `${info.cpu.cores} hilos`}
    </div>

    <div class="row">
      <span>RAM</span>
      <b>{gb(info.ram.used)} / {gb(info.ram.total)} GB</b>
    </div>
    <div class="bar"><i style={`width:${pct(info.ram.used, info.ram.total)}%`}></i></div>

    <div class="row"><span>GPU</span></div>
    <div class="sub" title={info.gpu.join(' · ')}>
      {info.gpu.length ? info.gpu.join(' · ') : 'Sin datos'}
    </div>

    <div class="row disks"><span>Discos</span></div>
    {#each info.disks as d (d.letter)}
      {@const used = pct(d.total - d.free, d.total)}
      <div class="disk" title={`${d.label || d.letter}: ${gb(d.free)} GB libres de ${gb(d.total)} GB`}>
        <span class="disk-name">{d.letter} {d.label}</span>
        <b>{gb(d.free)} GB libres</b>
      </div>
      <div class="bar thin">
        <i class:warn={used >= 75 && used < 90} class:hot={used >= 90} style={`width:${used}%`}></i>
      </div>
    {/each}
  {/if}
</div>

<style>
  .sys {
    flex: 0 0 auto;
    max-height: 42%;
    overflow-y: auto;
    padding: 8px 12px 10px;
    border-top: 1px solid var(--border);
    background: color-mix(in srgb, var(--panel) 62%, var(--bg));
    font-size: 11px;
  }
  .sys-title {
    font-size: 10px;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--muted);
    margin-bottom: 2px;
  }
  .sys-load { margin: 4px 0 0; color: var(--muted); }
  .row { display: flex; justify-content: space-between; gap: 8px; margin-top: 7px; color: var(--muted); }
  .row b { color: var(--text); font-weight: 600; font-variant-numeric: tabular-nums; }
  .row.disks { margin-bottom: 0; }
  .bar {
    height: 4px;
    margin-top: 3px;
    border-radius: 2px;
    overflow: hidden;
    background: color-mix(in srgb, var(--border) 60%, transparent);
  }
  .bar i {
    display: block;
    height: 100%;
    border-radius: 2px;
    background: var(--accent);
    transition: width 0.4s ease;
  }
  .bar i.warn { background: var(--yellow); }
  .bar i.hot { background: var(--red); }
  .bar.thin { height: 3px; margin-top: 2px; }
  .sub {
    margin-top: 2px;
    font-size: 10px;
    color: var(--muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .disk { display: flex; justify-content: space-between; gap: 8px; margin-top: 6px; }
  .disk-name { min-width: 0; color: var(--text); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .disk b { color: var(--muted); font-weight: 500; white-space: nowrap; font-variant-numeric: tabular-nums; }
</style>
