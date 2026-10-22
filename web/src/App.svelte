<script lang="ts">
  import About from './routes/About.svelte';
  import Add from './routes/Add.svelte';
  import Library from './routes/Library.svelte';
  import News from './routes/News.svelte';
  import Recipes from './routes/Recipes.svelte';
  import SeriesPage from './routes/Series.svelte';
  import Settings from './routes/Settings.svelte';
  import { t } from './lib/i18n';
  import NavBar from './lib/components/NavBar.svelte';
  import OfflineBanner from './lib/components/OfflineBanner.svelte';
  import Toasts from './lib/components/Toasts.svelte';
  import { news } from './lib/stores/news.svelte';
  import { applyTheme, prefs } from './lib/stores/prefs.svelte';
  import { href, router } from './lib/stores/router.svelte';

  applyTheme(prefs.theme);
  let route = $derived(router.current);

  // The "news" badge follows periodic checks while the app is open.
  const NEWS_POLL_MS = 10 * 60 * 1000;
  $effect(() => {
    news.refresh();
    const timer = setInterval(() => news.refresh(), NEWS_POLL_MS);
    return () => clearInterval(timer);
  });
</script>

<OfflineBanner />
<NavBar current={route.name} />
<Toasts />

<main>
  {#if route.name === 'library'}
    <Library />
  {:else if route.name === 'series'}
    {#key route.id}<SeriesPage id={route.id} />{/key}
  {:else if route.name === 'news'}
    <News />
  {:else if route.name === 'recipes'}
    {#key route}<Recipes domain={route.domain} url={route.url} />{/key}
  {:else if route.name === 'add'}
    {#key route}<Add sharedUrl={route.url} />{/key}
  {:else if route.name === 'settings'}
    <Settings />
  {:else if route.name === 'about'}
    <About />
  {:else}
    <p>{t('app.not_found')}</p>
    <a href={href({ name: 'library' })}>{t('app.back_home')}</a>
  {/if}
</main>

<style>
  main {
    max-width: 1200px;
    margin: 0 auto;
    padding: 16px;
  }
  /* Phones: room for the bottom tab bar and the system gesture area. */
  @media (max-width: 640px) {
    main {
      padding-bottom: calc(80px + env(safe-area-inset-bottom));
    }
  }
</style>
