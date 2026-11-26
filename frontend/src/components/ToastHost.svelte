<script lang="ts">
  import { dismissToast, toastList } from '../lib/toast.svelte'

  let items = $derived(toastList())
</script>

<div class="toast-host" aria-live="polite">
  {#each items as t (t.id)}
    <div class="toast {t.kind}" role={t.kind === 'error' ? 'alert' : 'status'}>
      <div class="toast-body">
        <div class="toast-title">{t.title}</div>
        {#if t.detail}<div class="toast-detail">{t.detail}</div>{/if}
      </div>
      <button class="toast-close" aria-label="Cerrar aviso" onclick={() => dismissToast(t.id)}>×</button>
    </div>
  {/each}
</div>

<style>
  .toast-host {
    position: fixed;
    top: 14px;
    right: 14px;
    z-index: 80;
    width: 320px;
    max-width: calc(100vw - 28px);
    display: flex;
    flex-direction: column;
    gap: 8px;
    pointer-events: none;
  }
  .toast {
    pointer-events: auto;
    display: flex;
    align-items: flex-start;
    gap: 8px;
    padding: 10px 12px;
    border: 1px solid var(--border);
    border-left: 3px solid var(--muted);
    border-radius: var(--r-md);
    background: color-mix(in srgb, var(--panel) 94%, var(--bg));
    box-shadow: var(--shadow-2);
    color: var(--text);
    font-size: 12px;
    animation: toast-in .16s ease-out;
  }
  .toast.success { border-left-color: var(--green); background: color-mix(in srgb, var(--green) 10%, var(--panel)); }
  .toast.error { border-left-color: var(--red); background: color-mix(in srgb, var(--red) 10%, var(--panel)); }
  .toast.warn { border-left-color: var(--yellow); background: color-mix(in srgb, var(--yellow) 10%, var(--panel)); }
  .toast.info { border-left-color: var(--brand); }
  .toast-body { flex: 1; min-width: 0; }
  .toast-title { font-weight: 700; }
  .toast-detail { margin-top: 2px; color: var(--muted); overflow-wrap: anywhere; }
  .toast-close {
    flex-shrink: 0;
    padding: 0 2px;
    border: none;
    background: none;
    color: var(--muted);
    font-size: 16px;
    line-height: 1;
    cursor: pointer;
  }
  .toast-close:hover { color: var(--text); }
  @keyframes toast-in {
    from { opacity: 0; transform: translateY(-6px); }
    to { opacity: 1; transform: translateY(0); }
  }
  @media (prefers-reduced-motion: reduce) {
    .toast { animation: none; }
  }
</style>
