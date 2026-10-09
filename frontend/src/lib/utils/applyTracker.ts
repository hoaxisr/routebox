// Apply reloads sing-box, which drops every connection through it — including
// the panel's own request when the phone reaches home through a VPN served by
// this same sing-box. The answer then either fails (TypeError) or never comes
// at all. So the apply is followed through GET /config/apply/progress, which
// the backend keeps for exactly this (seq + phase): the phase is shown live
// and the outcome is read from there when the response is lost.

export type ApplyResult = { message: string; reloaded?: boolean; restarted?: boolean; warning?: string };
export type ApplyStatus = { seq: number; phase: string; error?: string; warning?: string };
export type ApplyPhase = 'checking' | 'reloading' | 'reconnecting';

// The outcomes only the tracker knows about; callers turn `kind` into a
// localized message (changes.applyNotReached / applyConnectionLost / ...).
export type ApplyTrackKind = 'notReached' | 'connectionLost' | 'restarted' | 'timeout';
export class ApplyTrackError extends Error {
	constructor(public kind: ApplyTrackKind) {
		super(`apply: ${kind}`);
	}
}

export interface TrackDeps {
	post: () => Promise<ApplyResult>;
	status: () => Promise<ApplyStatus>; // rejects while the panel is unreachable
	sleep: (ms: number) => Promise<void>;
	now: () => number;
	onPhase?: (p: ApplyPhase) => void;
}

export const TRACK_LIMITS = {
	pollMs: 1000,
	notReachedMs: 3000, // POST failed and seq never moved: it never got there (seq bumps on entry)
	unreachableMs: 60000, // the VPN usually re-handshakes within seconds
	ceilingMs: 5 * 60000, // sing-box check / systemctl restart are bounded well below this
	graceMs: 1500 // progress says done; give the real answer a moment
};

const lostResponse = (err: unknown) =>
	err instanceof TypeError || /^HTTP (50[234]|524)$/.test(String((err as Error)?.message));

type Settled = { ok: ApplyResult } | { err: unknown } | null;

export async function trackApply(deps: TrackDeps, limits = TRACK_LIMITS): Promise<ApplyResult> {
	// One retry: a slow tunnel failing the first poll shouldn't switch tracking off.
	const before = await deps
		.status()
		.catch(() => deps.status())
		.then((s) => s.seq, () => null);
	if (before === null) return deps.post(); // nothing to follow (older backend) — plain request

	let settled: Settled = null;
	const post = deps.post().then(
		(ok) => void (settled = { ok }),
		(err) => void (settled = { err })
	);
	let shown: ApplyPhase | null = null;
	const show = (p: ApplyPhase) => {
		if (p !== shown) deps.onPhase?.((shown = p));
	};
	show('checking');

	const startedAt = deps.now();
	let unreachableSince: number | null = null;
	for (;;) {
		// Once the POST has settled the race would resolve at once: wait the
		// interval for real, or the loop polls back-to-back.
		const s0 = settled as Settled;
		await (s0 ? deps.sleep(limits.pollMs) : Promise.race([post, deps.sleep(limits.pollMs)]));
		const s = settled as Settled;
		if (s) {
			if ('ok' in s) return s.ok;
			if (!lostResponse(s.err)) throw s.err;
		}
		if (deps.now() - startedAt > limits.ceilingMs) throw new ApplyTrackError('timeout');

		let st: ApplyStatus;
		try {
			st = await deps.status();
			unreachableSince = null;
		} catch {
			unreachableSince ??= deps.now();
			show('reconnecting');
			if (deps.now() - unreachableSince > limits.unreachableMs) throw new ApplyTrackError('connectionLost');
			continue;
		}
		if (st.seq === before) {
			if (s && deps.now() - startedAt > limits.notReachedMs) throw new ApplyTrackError('notReached');
			continue;
		}
		// seq went backwards or the run has no phase: RouteBox itself restarted
		// mid-apply and the outcome is gone with it.
		if (st.seq < before || st.phase === '') throw new ApplyTrackError('restarted');
		if (st.phase === 'error') throw new Error(st.error || 'apply failed');
		if (st.phase === 'done') {
			await Promise.race([post, deps.sleep(limits.graceMs)]);
			const late = settled as Settled;
			if (late && 'ok' in late) return late.ok;
			return { message: 'Config applied', ...(st.warning ? { warning: st.warning } : {}) };
		}
		if (st.phase === 'checking' || st.phase === 'reloading') show(st.phase);
	}
}

// Message for an apply failure, localized for the tracker's own outcomes.
// connectionLost/restarted/timeout mean "may well have applied": callers show
// them as warnings and re-read the draft state.
export function applyErrorText(err: unknown, t: (key: string) => string): { text: string; uncertain: boolean } {
	if (err instanceof ApplyTrackError) {
		const key = { notReached: 'applyNotReached', connectionLost: 'applyConnectionLost', restarted: 'applyRestarted', timeout: 'applyTimeout' }[err.kind];
		return { text: t(`changes.${key}`), uncertain: err.kind !== 'notReached' };
	}
	return { text: `${t('changes.failedApply')}: ${err instanceof Error ? err.message : String(err)}`, uncertain: false };
}
