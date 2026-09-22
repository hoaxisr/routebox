import type { UpdateProgress } from '$lib/types';

export type RecoveryOutcome =
	| { kind: 'done'; progress: UpdateProgress }
	| { kind: 'error'; progress: UpdateProgress }
	| { kind: 'not-started' } // reachable, but seq never moved: the request never got there
	| { kind: 'lost' }; // unreachable too long, or the overall ceiling hit

export interface RecoveryDeps {
	poll: () => Promise<UpdateProgress>;
	sleep: (ms: number) => Promise<void>;
	now: () => number;
	alive: () => boolean;
	onProgress?: (p: UpdateProgress) => void;
}

export const RECOVERY_LIMITS = {
	unreachableMs: 180000, // tunnel down after the proxy restart; clients re-handshake slowly
	notStartedMs: 15000, // handler checks GitHub before Apply bumps seq
	ceilingMs: 15 * 60000 // server download client times out at 10 min
};

// The apply request died without a response; the server keeps going.
// Poll progress until it reports a terminal phase for THIS apply (seq past
// seqBefore, same target). Always terminates: every branch is bounded.
export async function recoverAfterDisconnect(
	target: string,
	seqBefore: number,
	deps: RecoveryDeps,
	limits = RECOVERY_LIMITS
): Promise<RecoveryOutcome> {
	const startedAt = deps.now();
	let unreachableSince: number | null = null;
	let delay = 1000;
	while (deps.alive() && deps.now() - startedAt < limits.ceilingMs) {
		await deps.sleep(delay);
		let p: UpdateProgress;
		try {
			p = await deps.poll();
		} catch {
			unreachableSince ??= deps.now();
			if (deps.now() - unreachableSince > limits.unreachableMs) return { kind: 'lost' };
			delay = Math.min(Math.round(delay * 1.5), 5000);
			continue;
		}
		unreachableSince = null;
		delay = 1000;
		if (p.seq === seqBefore || p.target !== target) {
			if (deps.now() - startedAt > limits.notStartedMs) return { kind: 'not-started' };
			continue;
		}
		deps.onProgress?.(p);
		if (p.phase === 'error') return { kind: 'error', progress: p };
		if (p.phase === 'done') return { kind: 'done', progress: p };
	}
	return { kind: 'lost' };
}
