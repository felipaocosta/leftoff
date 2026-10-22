import { describe, expect, it } from 'vitest';
import { ApiError } from './api/client';
import { errorMessage } from './errors';

describe('errorMessage', () => {
  it.each([
    [new ApiError(401, 'unauthorized', 'x'), /Token inválido/],
    [new ApiError(502, 'blocked', 'x'), /verificação anti-robô/],
    [new ApiError(502, 'no_browser', 'x'), /nenhum navegador Chrome/],
    [new ApiError(404, 'not_found', 'page gone'), /Algo deu errado: page gone/],
    [new ApiError(502, 'upstream_error', 'dial tcp: refused'), /O site não respondeu/],
    [new ApiError(504, 'timeout', 'x'), /demorou demais/],
    [new TypeError('Failed to fetch'), /Não foi possível falar com o servidor/],
    ['weird', /Algo deu errado: weird/],
  ])('%s', (err, want) => expect(errorMessage(err)).toMatch(want));
});
