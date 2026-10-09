import { describe, it, expect } from 'vitest';
import { trackApply, applyErrorText, ApplyTrackError, type ApplyStatus, type ApplyResult } from './applyTracker';

// Virtual clock: sleep advances time instantly. `script` maps the status poll
// index to an answer (an ApplyStatus, or 'down' for an unreachable panel).
// postSettlesAt resolves a pending POST once the clock reaches `at`.
function run(
	post: () => Promise<ApplyResult>,
	script: (i: number) => ApplyStatus | 'down',
	postSettlesAt?: { at: number; with: ApplyResult }
) {
	let t = 0;
	let i = 0;
	const phases: string[] = [];
	let resolvePost: ((r: ApplyResult) => void) | null = null;
	const result = trackApply({
		post: postSettlesAt ? () => new Promise<ApplyResult>((r) => (resolvePost = r)) : post,
		status: async () => {
			const s = script(i++);
			if (s === 'down') throw new TypeError('Failed to fetch');
			return s;
		},
		sleep: async (ms) => {
			t += ms;
			if (postSettlesAt && t >= postSettlesAt.at && resolvePost) {
				resolvePost(postSettlesAt.with);
				resolvePost = null;
			}
		},
		now: () => t,
		onPhase: (p) => phases.push(p)
	});
	return { result, phases, clock: () => t, polls: () => i };
}
const never = () => new Promise<ApplyResult>(() => {});
const lost = () => Promise.reject(new TypeError('Failed to fetch'));
const steps = (...s: (ApplyStatus | 'down')[]) => (i: number) => s[Math.min(i, s.length - 1)];
const kindOf = (p: Promise<unknown>) => p.then(() => 'resolved', (e) => (e instanceof ApplyTrackError ? e.kind : String(e)));

describe('trackApply', () => {
	it('returns the real answer when it arrives', async () => {
		const ok = { message: 'Config applied (hot reload)', warning: 'w' };
		const { result } = run(() => Promise.resolve(ok), () => ({ seq: 3, phase: 'done' }));
		await expect(result).resolves.toEqual(ok);
	});

	it('hung request: finishes from progress and shows each phase once', async () => {
		const { result, phases } = run(
			never,
			steps({ seq: 3, phase: 'done' }, { seq: 4, phase: 'checking' }, { seq: 4, phase: 'reloading' }, 'down', { seq: 4, phase: 'done' })
		);
		await expect(result).resolves.toEqual({ message: 'Config applied' });
		expect(phases).toEqual(['checking', 'reloading', 'reconnecting']);
	});

	it('recovered from progress, the dest warning survives', async () => {
		const { result } = run(lost, steps({ seq: 3, phase: 'done' }, { seq: 4, phase: 'done', warning: 'naive old' }));
		await expect(result).resolves.toEqual({ message: 'Config applied', warning: 'naive old' });
	});

	it('lost response, server error: shows the server error', async () => {
		const { result } = run(lost, steps({ seq: 3, phase: 'done' }, { seq: 4, phase: 'error', error: 'Saved but failed to restart: x' }));
		await expect(result).rejects.toThrow('Saved but failed to restart: x');
	});

	it('error without a message still rejects with something readable', async () => {
		const { result } = run(never, steps({ seq: 3, phase: 'done' }, { seq: 4, phase: 'error' }));
		await expect(result).rejects.toThrow('apply failed');
	});

	it('a previous apply that ended in error is not mistaken for this one', async () => {
		const { result } = run(
			never,
			steps({ seq: 3, phase: 'error', error: 'old' }, { seq: 3, phase: 'error', error: 'old' }, { seq: 4, phase: 'done' })
		);
		await expect(result).resolves.toEqual({ message: 'Config applied' });
	});

	it('"not reached" only after the window, not on the first poll', async () => {
		const { result, clock, polls } = run(lost, () => ({ seq: 3, phase: 'done' }));
		expect(await kindOf(result)).toBe('notReached');
		expect(clock()).toBeGreaterThan(3000);
		expect(polls()).toBeGreaterThanOrEqual(4);
	});

	it('"connection lost" only after the window, phase was reconnecting', async () => {
		const { result, clock, phases } = run(never, (i) => (i === 0 ? { seq: 3, phase: 'done' } : 'down'));
		expect(await kindOf(result)).toBe('connectionLost');
		expect(clock()).toBeGreaterThan(60000);
		expect(phases.at(-1)).toBe('reconnecting');
	});

	it('a panel that comes back resets the outage clock', async () => {
		const script = (i: number): ApplyStatus | 'down' => {
			if (i === 0) return { seq: 3, phase: 'done' };
			if (i <= 40) return 'down';
			if (i === 41) return { seq: 4, phase: 'reloading' };
			if (i <= 81) return 'down';
			return { seq: 4, phase: 'done' };
		};
		await expect(run(never, script).result).resolves.toEqual({ message: 'Config applied' });
	});

	it('RouteBox restarted mid-apply (seq reset): says so instead of waiting forever', async () => {
		const { result } = run(lost, steps({ seq: 3, phase: 'done' }, { seq: 0, phase: '' }));
		expect(await kindOf(result)).toBe('restarted');
	});

	it('a run that never finishes hits the ceiling', async () => {
		const { result, clock } = run(never, steps({ seq: 3, phase: 'done' }, { seq: 4, phase: 'reloading' }));
		expect(await kindOf(result)).toBe('timeout');
		expect(clock()).toBeGreaterThan(5 * 60000);
	});

	it('HTTP 502 from a front proxy counts as a lost response', async () => {
		const { result } = run(() => Promise.reject(new Error('HTTP 502')), steps({ seq: 3, phase: 'done' }, { seq: 4, phase: 'done' }));
		await expect(result).resolves.toEqual({ message: 'Config applied' });
	});

	it('HTTP 500 and validation errors answered by the panel pass through', async () => {
		const p500 = run(() => Promise.reject(new Error('HTTP 500')), steps({ seq: 3, phase: 'done' }, { seq: 4, phase: 'done' }));
		await expect(p500.result).rejects.toThrow('HTTP 500');
		const pVal = run(() => Promise.reject(new Error('Config validation failed: x')), () => ({ seq: 3, phase: 'done' }));
		await expect(pVal.result).rejects.toThrow('Config validation failed: x');
	});

	it('progress down twice before POST: plain request, no phases', async () => {
		const ok = { message: 'Config applied (hot reload)' };
		const { result, phases, polls } = run(() => Promise.resolve(ok), () => 'down');
		await expect(result).resolves.toEqual(ok);
		expect(polls()).toBe(2);
		expect(phases).toEqual([]);
		await expect(run(lost, () => 'down').result).rejects.toThrow('Failed to fetch');
	});

	it('one failed first poll does not switch tracking off', async () => {
		const { result } = run(never, steps('down', { seq: 3, phase: 'done' }, { seq: 4, phase: 'done' }));
		await expect(result).resolves.toEqual({ message: 'Config applied' });
	});

	it('grace: the real answer landing just after "done" wins', async () => {
		const ok = { message: 'Config applied (hot reload)', warning: 'naive' };
		const { result } = run(never, steps({ seq: 3, phase: 'done' }, { seq: 4, phase: 'reloading' }, { seq: 4, phase: 'done' }), { at: 2500, with: ok });
		await expect(result).resolves.toEqual(ok);
	});

	it('grace: an answer later than the window is not waited for', async () => {
		const { result, clock } = run(never, steps({ seq: 3, phase: 'done' }, { seq: 4, phase: 'done' }), { at: 10000, with: { message: 'late' } });
		await expect(result).resolves.toEqual({ message: 'Config applied' });
		expect(clock()).toBeLessThan(10000);
	});

	// Real timers on purpose: the virtual clock advances inside sleep() even
	// when nobody awaits it, so it cannot see a loop that skips the wait.
	it('honours the poll interval after the POST was lost (no busy loop)', async () => {
		let calls = 0;
		const limits = { pollMs: 10, notReachedMs: 60, unreachableMs: 60000, ceilingMs: 60000, graceMs: 10 };
		await trackApply(
			{
				post: lost,
				status: async () => (calls++, { seq: 3, phase: 'done' }),
				sleep: (ms) => new Promise((r) => setTimeout(r, ms)),
				now: Date.now
			},
			limits
		).catch(() => {});
		expect(calls).toBeLessThan(20);
	});
});

describe('applyErrorText', () => {
	const t = (k: string) => `<${k}>`;
	it('maps tracker outcomes to keys; only notReached is certain', () => {
		expect(applyErrorText(new ApplyTrackError('notReached'), t)).toEqual({ text: '<changes.applyNotReached>', uncertain: false });
		expect(applyErrorText(new ApplyTrackError('connectionLost'), t)).toEqual({ text: '<changes.applyConnectionLost>', uncertain: true });
		expect(applyErrorText(new ApplyTrackError('restarted'), t).uncertain).toBe(true);
		expect(applyErrorText(new ApplyTrackError('timeout'), t).uncertain).toBe(true);
	});
	it('keeps the server message for real errors', () => {
		expect(applyErrorText(new Error('Config validation failed: x'), t)).toEqual({
			text: '<changes.failedApply>: Config validation failed: x',
			uncertain: false
		});
	});
});
