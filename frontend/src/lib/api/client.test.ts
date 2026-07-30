import { describe, it, expect, vi, afterEach } from 'vitest';
import { api } from './client';

afterEach(() => vi.unstubAllGlobals());

describe('getAwgPeerVpnLink', () => {
	// AWG public keys are std-base64 and contain "/" and "+". Without escaping,
	// routing breaks for a fraction of peers — the kind of bug that only shows up
	// in production.
	it('escapes the key into the path', async () => {
		const fetchMock = vi.fn().mockResolvedValue(new Response('vpn://abc', { status: 200 }));
		vi.stubGlobal('fetch', fetchMock);

		await api.getAwgPeerVpnLink('a/b+c=');
		expect(fetchMock).toHaveBeenCalledWith('/api/awg/peers/a%2Fb%2Bc%3D/vpn-link');
	});

	// A bare "HTTP 503" strands the operator; the backend's text is actionable.
	it('rejects with the backend message, not the status code', async () => {
		vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
			new Response('server address not set — set it on the AWG page\n', { status: 503 })
		));
		await expect(api.getAwgPeerVpnLink('k')).rejects.toThrow(/set it on the AWG page/);
	});

	// 400s come back as a JSON envelope from writeError; the raw JSON must not be
	// shown to the operator.
	it('unwraps a JSON error envelope', async () => {
		vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
			new Response(JSON.stringify({ success: false, error: 'invalid public key' }), { status: 400 })
		));
		await expect(api.getAwgPeerVpnLink('k')).rejects.toThrow(/^invalid public key$/);
	});

	// Unlike 400/422/503, a 404 on this route only ever comes from the router's
	// own stock http.NotFound handler, never from our code — its body ("404 page
	// not found") is not operator-actionable and must not be echoed.
	it('does not echo the stock 404 body', async () => {
		vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
			new Response('404 page not found\n', { status: 404 })
		));
		await expect(api.getAwgPeerVpnLink('k')).rejects.toThrow(/^HTTP 404$/);
	});
});
