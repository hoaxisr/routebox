import { describe, it, expect } from 'vitest';
import { recoverAfterDisconnect, type RecoveryOutcome } from './updateRecovery';
import type { UpdateProgress } from '$lib/types';

const prog = (o: Partial<UpdateProgress>): UpdateProgress => ({
	seq: 1,
	target: 'amnezia-box',
	phase: 'idle',
	downloaded_bytes: 0,
	total_bytes: 0,
	...o
});

// Virtual clock: sleep advances time, poll answers by elapsed seconds.
function run(script: (elapsedMs: number) => UpdateProgress | Error, alive = () => true): Promise<RecoveryOutcome> {
	let clock = 0;
	const seen: UpdateProgress[] = [];
	return recoverAfterDisconnect('amnezia-box', 1, {
		poll: async () => {
			const r = script(clock);
			if (r instanceof Error) throw r;
			return r;
		},
		sleep: async (ms) => {
			clock += ms;
		},
		now: () => clock,
		alive,
		onProgress: (p) => seen.push(p)
	});
}

describe('recoverAfterDisconnect', () => {
	it('tunnel back after 20s, server done → done', async () => {
		const out = await run((t) => (t < 20000 ? new Error('net') : prog({ seq: 2, phase: 'done' })));
		expect(out.kind).toBe('done');
	});

	it('server error with text → error carrying the message', async () => {
		const out = await run(() => prog({ seq: 2, phase: 'error', error: 'did not start' }));
		expect(out).toMatchObject({ kind: 'error', progress: { error: 'did not start' } });
	});

	it('still downloading for minutes, then done → done (no premature give-up)', async () => {
		const out = await run((t) => (t < 180000 ? prog({ seq: 2, phase: 'download' }) : prog({ seq: 2, phase: 'done' })));
		expect(out.kind).toBe('done');
	});

	it('tunnel never comes back → lost, in bounded time', async () => {
		const out = await run(() => new Error('net'));
		expect(out.kind).toBe('lost');
	});

	it('request never reached the server (seq unchanged, stale done) → not-started, not a false success', async () => {
		const out = await run(() => prog({ seq: 1, phase: 'done' }));
		expect(out.kind).toBe('not-started');
	});

	it('progress belongs to another target → not-started', async () => {
		const out = await run(() => prog({ seq: 2, target: 'routebox', phase: 'done' }));
		expect(out.kind).toBe('not-started');
	});

	it('page left → stops promptly', async () => {
		let polls = 0;
		const out = await run(
			() => {
				polls++;
				return prog({ seq: 2, phase: 'download' });
			},
			() => polls < 3
		);
		expect(out.kind).toBe('lost');
		expect(polls).toBe(3);
	});
});
