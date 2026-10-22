<script lang="ts">
  import { api } from '../api';
  import { t } from '../i18n';
  import type { MessageKey } from '../i18n/pt-BR';
  import { href } from '../stores/router.svelte';
  import type { CheckError, Health, Series } from '../api/types';

  // The last check's error (RF-24), explained by its code in the user's
  // language; the technical message stays under "Details". When a recipe
  // keeps failing, it points to the Recipes screen (RF-55).
  let { series }: { series: Series } = $props();

  let mode = $state<Health['mode'] | null>(null);
  $effect(() => {
    if (series.check_error && mode === null) {
      api.health().then(
        (h) => (mode = h.mode),
        () => (mode = null), // unknown mode: the generic texts still work
      );
    }
  });

  let recipeBroken = $derived(
    series.source.method.startsWith('recipe:') && (series.check_error?.failures ?? 0) >= 3,
  );

  function explain(ce: CheckError, m: Health['mode'] | null): string {
    switch (ce.code) {
      case 'needs_browser':
        return t(
          m === 'android'
            ? 'check.needs_browser.android'
            : m === 'server'
              ? 'check.needs_browser.server'
              : 'check.needs_browser.desktop',
        );
      case 'no_browser':
        return t(m === 'android' ? 'check.no_browser.android' : 'check.no_browser.other');
      case '':
      case undefined:
        return ce.message;
      default:
        return t(`check.${ce.code}` as MessageKey);
    }
  }
</script>

{#if series.check_error}
  {@const ce = series.check_error}
  <div class="error" role="status">
    <p>{t('series.check_error', { n: ce.failures, message: explain(ce, mode) })}</p>
    {#if ce.code}
      <details>
        <summary>{t('check.details')}</summary>
        <code>{ce.message}</code>
      </details>
    {/if}
    {#if recipeBroken}
      <p>
        {t('series.recipe_broken')}
        <a href={href({ name: 'recipes', domain: series.source.domain, url: series.source.url })}>
          {t('series.recipe_fix')}
        </a>
      </p>
    {/if}
  </div>
{/if}

<style>
  .error {
    color: var(--warning);
  }
  .error p {
    margin: 0 0 4px;
  }
  details {
    color: var(--muted);
    font-size: 0.85rem;
  }
  code {
    overflow-wrap: anywhere;
  }
</style>
