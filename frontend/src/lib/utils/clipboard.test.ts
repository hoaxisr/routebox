import { describe, it, expect, vi, afterEach } from 'vitest';
import { copyText } from './clipboard';

afterEach(() => {
	vi.unstubAllGlobals();
	vi.restoreAllMocks();
});

describe('copyText', () => {
	it('uses the async clipboard API in a secure context', async () => {
		const writeText = vi.fn().mockResolvedValue(undefined);
		vi.stubGlobal('navigator', { clipboard: { writeText } });
		vi.stubGlobal('window', { isSecureContext: true });

		expect(await copyText('vpn://abc')).toBe(true);
		expect(writeText).toHaveBeenCalledWith('vpn://abc');
	});

	// The case the helper exists for: router mode over plain HTTP. Chrome still
	// exposes navigator.clipboard there, but writeText rejects — so the secure-context
	// check, not the presence check, is what has to route us to the fallback.
	it('falls back when clipboard exists but the context is insecure', async () => {
		const writeText = vi.fn().mockRejectedValue(new Error('not allowed'));
		vi.stubGlobal('navigator', { clipboard: { writeText } });
		vi.stubGlobal('window', { isSecureContext: false });

		// A bare mockReturnValue(true) would pass even if the helper never wrote
		// the text into the textarea (or never selected it) before calling
		// execCommand — read the live element from inside the mock, before the
		// helper removes it, so the assertion actually pins the copied value.
		let captured: string | undefined;
		let selection: { start: number | null; end: number | null } | undefined;
		const execCommand = vi.spyOn(document, 'execCommand').mockImplementation(() => {
			const ta = document.querySelector('textarea');
			captured = ta?.value;
			selection = ta ? { start: ta.selectionStart, end: ta.selectionEnd } : undefined;
			return true;
		});

		expect(await copyText('vpn://abc')).toBe(true);
		expect(writeText).not.toHaveBeenCalled();
		expect(execCommand).toHaveBeenCalledWith('copy');
		expect(captured).toBe('vpn://abc');
		expect(selection).toEqual({ start: 0, end: 'vpn://abc'.length });
	});

	it('cleans up the scratch textarea', async () => {
		vi.stubGlobal('navigator', {});
		vi.stubGlobal('window', { isSecureContext: false });
		vi.spyOn(document, 'execCommand').mockReturnValue(true);

		await copyText('vpn://abc');
		expect(document.querySelector('textarea')).toBeNull();
	});

	it('reports failure rather than throwing', async () => {
		vi.stubGlobal('navigator', {});
		vi.stubGlobal('window', { isSecureContext: false });
		vi.spyOn(document, 'execCommand').mockReturnValue(false);

		expect(await copyText('vpn://abc')).toBe(false);
	});
});
