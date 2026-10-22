<script lang="ts">
  import { t } from '../i18n';
  import { toasts } from '../stores/toast.svelte';

  // Floating messages above the content (and above the phone tab bar).
</script>

<div class="toasts">
  {#each toasts.items as item (item.id)}
    <div
      class="toast"
      class:error={item.kind === 'error'}
      role={item.kind === 'error' ? 'alert' : 'status'}
    >
      <span>{item.text}</span>
      <button type="button" aria-label={t('app.dismiss')} onclick={() => toasts.dismiss(item.id)}
        >×</button
      >
    </div>
  {/each}
</div>

<style>
  .toasts {
    position: fixed;
    left: 50%;
    bottom: 16px;
    transform: translateX(-50%);
    z-index: 20;
    display: grid;
    gap: 8px;
    width: min(560px, calc(100% - 24px));
    pointer-events: none;
  }
  .toast {
    pointer-events: auto;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 12px;
    border-radius: 10px;
    background: var(--text);
    color: var(--bg);
    box-shadow: 0 4px 16px rgb(0 0 0 / 0.3);
    font-size: 0.9rem;
  }
  .toast.error {
    background: var(--danger);
    color: #fff;
  }
  span {
    flex: 1;
    word-break: break-word;
  }
  button {
    border: 0;
    background: none;
    color: inherit;
    font-size: 1.2rem;
    cursor: pointer;
    padding: 0 4px;
  }
  @media (max-width: 640px) {
    .toasts {
      bottom: calc(76px + env(safe-area-inset-bottom));
    }
  }
</style>
