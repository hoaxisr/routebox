import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { get } from 'svelte/store';
import { notifications } from './notifications';

// A toast with duration 0 must stay until the user closes it. The config-path
// banner leans on this for the one notice saying an adopted path could not be
// written down: five seconds is not enough to read two sentences with a
// filesystem error in them, and once it fades there is nowhere left to read it.
// Every toast carries a close button, so a sticky toast is never a dead end.
describe('notifications store — sticky toasts', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		notifications.clear();
	});

	afterEach(() => {
		vi.useRealTimers();
		notifications.clear();
	});

	it('keeps a duration-0 toast up indefinitely', () => {
		notifications.warning('adopted, but not saved', 0);
		vi.advanceTimersByTime(60_000);
		expect(get(notifications)).toHaveLength(1);
	});

	it('still auto-dismisses a default toast', () => {
		notifications.success('saved');
		expect(get(notifications)).toHaveLength(1);
		vi.advanceTimersByTime(5000);
		expect(get(notifications)).toHaveLength(0);
	});

	it('lets the user close a sticky toast', () => {
		const id = notifications.warning('adopted, but not saved', 0);
		notifications.remove(id);
		expect(get(notifications)).toHaveLength(0);
	});
});
