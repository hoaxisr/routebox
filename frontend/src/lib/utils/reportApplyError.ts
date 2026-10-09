import { get } from 'svelte/store';
import { t } from 'svelte-i18n';
import { notifications, unsavedChanges } from '$lib/stores';
import { applyErrorText } from './applyTracker';

// One place for every apply button's catch. An outcome the tracker could not
// confirm may well have applied, so it stays as a warning and the pending
// state is re-read instead of claiming the changes are still there.
export function reportApplyError(err: unknown): boolean {
	const { text, uncertain } = applyErrorText(err, (k) => get(t)(k));
	if (uncertain) {
		notifications.warning(text, 0);
		unsavedChanges.refresh();
	} else {
		notifications.error(text);
	}
	return uncertain;
}
