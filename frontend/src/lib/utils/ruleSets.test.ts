import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('$lib/api/client', () => ({
	api: { updateRuleSet: vi.fn(), deleteRuleSet: vi.fn(), createRuleSet: vi.fn() }
}));

import { api } from '$lib/api/client';
import { formatForPath, saveEditedRuleSet } from './ruleSets';
import type { RuleSet } from '$lib/types';

const local: RuleSet = { tag: 'yt', type: 'local', format: 'binary', path: '/opt/srs/yt.srs' };

describe('saveEditedRuleSet', () => {
	beforeEach(() => vi.clearAllMocks());

	it('same tag → PUT in place, never delete (a referenced set cannot be deleted)', async () => {
		await saveEditedRuleSet('yt', local);
		expect(api.updateRuleSet).toHaveBeenCalledWith('yt', local);
		expect(api.deleteRuleSet).not.toHaveBeenCalled();
	});

	it('renamed → delete old, create new', async () => {
		await saveEditedRuleSet('old', local);
		expect(api.deleteRuleSet).toHaveBeenCalledWith('old');
		expect(api.createRuleSet).toHaveBeenCalledWith(local);
		expect(api.updateRuleSet).not.toHaveBeenCalled();
	});
});

describe('formatForPath', () => {
	it('follows the extension, case-insensitively', () => {
		expect(formatForPath('/a/x.json', 'binary')).toBe('source');
		expect(formatForPath('/a/X.SRS ', 'source')).toBe('binary');
	});

	it('unknown extension keeps the current format', () => {
		expect(formatForPath('/a/x', 'source')).toBe('source');
	});
});
