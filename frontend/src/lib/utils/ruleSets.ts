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

// An IP set in a DNS rule without match_response matches nothing: the backend
// flags such rules rule_set_ip_cidr_match_source, and sing-box refuses a purely
// IP set at start while `check` passes. So the DNS form does not offer them.
// Same test as awg-manager; a remote set's content is unknown, hence the names.
export function isDnsAddressFilterRuleSet(rs: RuleSet): boolean {
	if (/^geoip([-_:]|$)/i.test(rs.tag)) return true;
	const url = rs.url ?? '';
	if (/[?&]kind=geoip\b/i.test(url) || /\/geoip(?:[-_/]|\.srs|\b)/i.test(url)) return true;
	if (rs.path && /geoip/i.test(rs.path)) return true;
	return (
		rs.type === 'inline' &&
		(rs.rules ?? []).some((rule) => (rule.ip_cidr?.length ?? 0) > 0)
	);
}
