<script lang="ts">
  import { api } from '../lib/api';
  import { t } from '../lib/i18n';
  import type { MessageKey } from '../lib/i18n/pt-BR';
  import type { Health } from '../lib/api/types';

  // About screen: how the app works, in the UI language. Each section is
  // a title and a body whose paragraphs are separated by blank lines.
  const sections = [
    'what',
    'extract',
    'recipes',
    'updates',
    'cloudflare',
    'data',
    'languages',
  ] as const;

  const AUTHOR_EMAIL = 'fcostarodrigues@gmail.com';
  const SOURCE_URL = 'https://github.com/felipaocosta/leftoff';

  let health = $state<Health | null>(null);

  // The local apps (app mode, desktop, Android) say so; a server says "server".
  function modeKey(mode: Health['mode']): MessageKey {
    if (mode === 'android') return 'about.mode.android';
    return mode === 'server' ? 'about.mode.server' : 'about.mode.app';
  }

  $effect(() => {
    api.health().then(
      (h) => (health = h),
      () => (health = null), // the texts do not depend on the server
    );
  });

  const paragraphs = (text: string) => text.split('\n\n');

  // "FlareSolverr, navegador deste computador" (unknown names as they come).
  function solverList(names: string[]): string {
    return names
      .map((n) =>
        n === 'flaresolverr' || n === 'browser' || n === 'webview' ? t(`about.solver.${n}`) : n,
      )
      .join(', ');
  }
</script>

<article>
  <h1>{t('about.title')}</h1>
  <p class="intro">{t('about.intro')}</p>
  {#if health}
    <p class="status">
      {t('about.version', {
        version: health.version,
        mode: t(modeKey(health.mode)),
      })}
      <br />
      {t(health.llm ? 'about.llm_on' : 'about.llm_off')}
      <br />
      {health.solvers?.length
        ? t('about.solvers', { list: solverList(health.solvers) })
        : t('about.solvers_none')}
    </p>
  {/if}

  {#each sections as s (s)}
    <section>
      <h2>{t(`about.${s}.title`)}</h2>
      {#each paragraphs(t(`about.${s}.body`)) as p, i (i)}
        <p>{p}</p>
      {/each}
    </section>
  {/each}

  <footer class="credits">
    <p>{t('about.credits')}</p>
    <p>{t('about.contact')}: <a href="mailto:{AUTHOR_EMAIL}">{AUTHOR_EMAIL}</a></p>
    <p>
      {t('about.license')} <a href={SOURCE_URL} target="_blank" rel="noreferrer">{SOURCE_URL}</a>
    </p>
  </footer>
</article>

<style>
  .credits {
    margin-top: 32px;
    padding-top: 12px;
    border-top: 1px solid var(--border);
    color: var(--muted);
    font-size: 0.9rem;
  }
  .credits p {
    margin: 0 0 4px;
  }
  article {
    max-width: 720px;
    line-height: 1.55;
  }
  .intro {
    font-size: 1.05rem;
  }
  .status {
    padding: 10px 12px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
    color: var(--text-muted);
    font-size: 0.9rem;
  }
  h2 {
    margin: 1.4em 0 0.4em;
    font-size: 1.1rem;
  }
  p {
    margin: 0 0 0.7em;
    white-space: pre-line;
  }
</style>
