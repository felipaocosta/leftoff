<script lang="ts">
  import { t } from '../i18n';
  import { href } from '../stores/router.svelte';
  import type { FailingSeries } from '../api/types';

  // Series a recipe stopped reading (RF-55), each with a link that reopens
  // the editor on its page, ready to test or regenerate the recipe.
  let { domain, failing }: { domain: string; failing: FailingSeries[] } = $props();
</script>

{#if failing.length > 0}
  <div class="health" role="status">
    <p>{t('recipes.failing')}</p>
    <ul>
      {#each failing as f (f.series_id)}
        <li>
          <strong>{f.title}</strong> — {t(`recipes.reason.${f.reason}`)}
          {#if f.message}<small>({f.message})</small>{/if}
          · <a href={href({ name: 'recipes', domain, url: f.url })}>{t('recipes.use_url')}</a>
        </li>
      {/each}
    </ul>
  </div>
{/if}

<style>
  .health {
    border: 1px solid var(--warning);
    border-radius: 8px;
    padding: 8px 12px;
    font-size: 0.85rem;
    margin-bottom: 10px;
  }
  p,
  ul {
    margin: 0 0 4px;
  }
  small {
    color: var(--text-muted);
    word-break: break-word;
  }
</style>
