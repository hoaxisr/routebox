import { api } from '$lib/api/client';
import type { RuleSet } from '$lib/types';

// Saving an edited rule set. Same tag → PUT in place: a set referenced by
// rules cannot be deleted, so delete+create would refuse every set in use.
// A rename creates the new tag first and deletes the old one second, undoing
// the create when the delete is refused (the old tag is still referenced —
// the right answer, the rules would point at a tag that no longer exists).
// Either way a failure leaves the draft as it was, never without the set.
export async function saveEditedRuleSet(oldTag: string, ruleSet: RuleSet): Promise<void> {
	if (ruleSet.tag === oldTag) {
		await api.updateRuleSet(oldTag, ruleSet);
		return;
	}
	await api.createRuleSet(ruleSet);
	try {
		await api.deleteRuleSet(oldTag);
	} catch (e) {
		try {
			await api.deleteRuleSet(ruleSet.tag);
		} catch {
			/* the original error is the one to report */
		}
		throw e;
	}
}

// Local rule-set format follows the file extension; anything else keeps the current choice.
export function formatForPath(path: string, current: 'binary' | 'source'): 'binary' | 'source' {
	const p = path.trim().toLowerCase();
	if (p.endsWith('.srs')) return 'binary';
	if (p.endsWith('.json')) return 'source';
	return current;
}
