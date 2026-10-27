import { api } from '$lib/api/client';
import type { RuleSet } from '$lib/types';

// Saving an edited rule set. Same tag → PUT in place: a set referenced by
// rules cannot be deleted, so delete+create would refuse every set in use.
// Renaming still goes delete+create, and that refusal is the right answer
// for a referenced set (the rules would point at a tag that no longer exists).
export async function saveEditedRuleSet(oldTag: string, ruleSet: RuleSet): Promise<void> {
	if (ruleSet.tag === oldTag) {
		await api.updateRuleSet(oldTag, ruleSet);
		return;
	}
	await api.deleteRuleSet(oldTag);
	await api.createRuleSet(ruleSet);
}

// Local rule-set format follows the file extension; anything else keeps the current choice.
export function formatForPath(path: string, current: 'binary' | 'source'): 'binary' | 'source' {
	const p = path.trim().toLowerCase();
	if (p.endsWith('.srs')) return 'binary';
	if (p.endsWith('.json')) return 'source';
	return current;
}
