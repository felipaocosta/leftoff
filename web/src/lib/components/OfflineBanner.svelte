<script lang="ts">
  import { t } from '../i18n';
  import { connection } from '../stores/connection.svelte';

  // Shown while the server does not answer; it clears by itself.
</script>

{#if connection.offline}
  <div class="offline" role="alert">
    <span>{t('app.offline')}</span>
    <button type="button" onclick={() => connection.retry()}>{t('app.retry')}</button>
  </div>
{/if}

<style>
  .offline {
    position: sticky;
    top: 0;
    z-index: 15;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 12px;
    padding: 8px 16px;
    background: var(--warning);
    color: #111;
    font-size: 0.9rem;
  }
  button {
    padding: 4px 10px;
    border: 1px solid rgb(0 0 0 / 0.3);
    border-radius: 6px;
    background: rgb(255 255 255 / 0.6);
    cursor: pointer;
  }
</style>
