/** A starting recipe for a domain (SPEC §5.3), to be filled in. */
export function recipeTemplate(domain: string): string {
  return JSON.stringify(
    {
      schema: 1,
      domain,
      aliases: [],
      series: {
        title: { css: 'h1' },
        cover: { css: '.cover img', attr: 'src' },
        status: { css: '.status' },
        synopsis: { css: '.description' },
        alt_titles: { css: '.alternative', split: ',' },
      },
      chapters: {
        list_url: null,
        item: { css: '.chapter-list li' },
        link: { css: 'a', attr: 'href' },
        label: { css: 'a' },
        date: { css: '.date' },
      },
      requires_browser: false,
      generated_by: 'manual',
      updated_at: new Date().toISOString(),
    },
    null,
    2,
  );
}
