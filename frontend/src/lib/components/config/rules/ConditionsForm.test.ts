import { render, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { beforeAll, describe, expect, it } from 'vitest';
import { addMessages, init } from 'svelte-i18n';

import ConditionsForm from './ConditionsForm.svelte';
import en from '$lib/i18n/locales/en.json';
import type { RuleSet } from '$lib/types';

beforeAll(() => {
	addMessages('en', en as never);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

const ruleSets = [{ tag: 'geosite-discord', type: 'remote', format: 'binary' }] as RuleSet[];

// The form copies `conditions` into its own state at init and writes it back on
// every change. RuleForm adds a tag to conditions.rule_set from outside after
// creating a rule set from inside the form — that tag was never shown as chosen
// and the next write-back dropped it, so the rule saved without it.
describe('ConditionsForm', () => {
	it('adopts a rule set added to conditions from outside', async () => {
		const { rerender } = render(ConditionsForm, { props: { conditions: {}, ruleSets } });

		await screen.getByRole('button', { name: 'Advanced' }).click();
		await tick();
		const ruleSetButton = () => screen.getByRole('button', { name: /geosite-discord/ });
		expect(ruleSetButton().querySelector('svg')).toBeNull();

		// What RuleForm does after createRuleSet() succeeds.
		await rerender({ conditions: { rule_set: ['geosite-discord'] }, ruleSets });
		await tick();
		expect(ruleSetButton().querySelector('svg')).not.toBeNull();

		// And survives the write-back that used to wipe it.
		await tick();
		expect(ruleSetButton().querySelector('svg')).not.toBeNull();
	});
});
