import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import OfflineBanner from './OfflineBanner.svelte';
import { connection } from '../stores/connection.svelte';

describe('OfflineBanner', () => {
  it('shows while offline and retries on demand', async () => {
    const probe = vi.fn(async () => connection.report(true));
    connection.watch(probe);
    render(OfflineBanner);
    expect(screen.queryByRole('alert')).toBeNull();
    connection.report(false);
    expect(await screen.findByRole('alert')).toHaveTextContent('Sem conexão com o servidor');
    await fireEvent.click(screen.getByRole('button', { name: 'Tentar de novo' }));
    expect(probe).toHaveBeenCalled();
    await vi.waitFor(() => expect(screen.queryByRole('alert')).toBeNull());
  });
});
