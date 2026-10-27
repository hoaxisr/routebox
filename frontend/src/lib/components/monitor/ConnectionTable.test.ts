import { render, fireEvent } from '@testing-library/svelte';
import { beforeAll, describe, expect, it } from 'vitest';
import { addMessages, init } from 'svelte-i18n';

import ConnectionTable from './ConnectionTable.svelte';
import en from '$lib/i18n/locales/en.json';
import type { ClashConnection } from '$lib/types';

beforeAll(() => {
	addMessages('en', en as never);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

const conn = (id: string, chains: string[]): ClashConnection => ({
	id, chains, upload: 0, download: 0, start: new Date().toISOString(), rule: '', rulePayload: '',
	metadata: { network: 'tcp', type: 'mixed', sourceIP: '10.0.0.2', sourcePort: '1', destinationIP: '1.1.1.1', destinationPort: '443', host: id }
});

// #116: a direct exit is blue, as in the dashboard's exits block.
describe('ConnectionTable: direct chips', () => {
	it('marks only chips whose tag is a direct outbound', async () => {
		const { container, getByLabelText } = render(ConnectionTable, {
			props: { connections: [conn('a.com', ['my-direct']), conn('b.com', ['awg'])], onClose: () => {}, direct: new Set(['my-direct']) }
		});
		await fireEvent.click(getByLabelText('Group by client')); // flat list: rows show their chips
		const marked = [...container.querySelectorAll('.selection-chip.route-direct')].map((e) => e.textContent?.trim());
		expect(marked.length).toBeGreaterThan(0);
		expect(new Set(marked)).toEqual(new Set(['my-direct']));
	});
});
