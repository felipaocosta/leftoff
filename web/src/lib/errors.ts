import { ApiError } from './api';
import { t } from './i18n';

/** Turns any thrown value into a message for the user. */
export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === 'unauthorized') return t('error.unauthorized');
    if (err.code === 'blocked') return t('error.blocked');
    if (err.code === 'no_browser') return t('error.no_browser');
    if (err.code === 'chapter_page') return t('error.chapter_page');
    if (err.code === 'llm_disabled') return t('error.llm_disabled');
    if (err.code === 'no_recipe') return t('error.no_recipe');
    if (err.code === 'llm_error') return t('error.llm_error', { message: err.message });
    if (err.code === 'upstream_error') return t('error.upstream');
    if (err.code === 'needs_browser') return t('error.needs_browser');
    if (err.code === 'timeout') return t('error.timeout');
    return t('error.generic', { message: err.message });
  }
  if (err instanceof TypeError) return t('error.network'); // fetch network failure
  return t('error.generic', { message: String(err) });
}
