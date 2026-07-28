import { describe, it, expect } from 'vitest';
import { createSerialQueue } from './serialQueue';
import { reorderArray } from './routeRules';

/**
 * The routes page addresses every rule by its position, so two overlapping
 * actions could apply the second one's index to a rule the first one had
 * already moved. This models that exact scenario against a fake server: a
 * destination change on the last row, issued while a drag of the first row is
 * still in flight.
 *
 * It is not a test of the page (this repo has no Svelte component harness) but
 * of the primitives the page's handlers are built from — a queue plus a lookup
 * of the rule's CURRENT position by identity. Both halves are needed: the queue
 * alone still sends the index captured at click time.
 */
type Rule = { id: string; outbound: string };

/** A server that applies operations in the order it receives them. */
function fakeServer(initial: Rule[]) {
	let rules = initial.map((r) => ({ ...r }));
	const pending: Array<() => void> = [];
	return {
		/** Queue an op; it lands only when flush() is called. */
		reorder(from: number, to: number) {
			return new Promise<void>((resolve) => {
				pending.push(() => {
					rules = reorderArray(rules, from, to);
					resolve();
				});
			});
		},
		update(index: number, outbound: string) {
			return new Promise<void>((resolve) => {
				pending.push(() => {
					rules[index] = { ...rules[index], outbound };
					resolve();
				});
			});
		},
		flush() {
			while (pending.length) pending.shift()!();
		},
		order: () => rules.map((r) => `${r.id}:${r.outbound}`).join(' ')
	};
}

const seed: Rule[] = [
	{ id: 'A', outbound: 'direct' },
	{ id: 'B', outbound: 'direct' },
	{ id: 'C', outbound: 'direct' },
	{ id: 'D', outbound: 'direct' }
];

describe('overlapping rule mutations', () => {
	// What the panel used to do: fire immediately, with the index the click saw.
	it('corrupts the config when indexes are captured at click time', async () => {
		const server = fakeServer(seed);
		let local = seed.map((r) => ({ ...r }));

		// Action 1: drag A (index 0) to the end.
		const drag = server.reorder(0, 3).then(() => {
			local = reorderArray(local, 0, 3);
		});
		// Action 2, issued before the drag lands: set D's destination. D is at
		// index 3 on screen — but the drag puts A there.
		const pick = server.update(3, 'vpn').then(() => {
			local[3] = { ...local[3], outbound: 'vpn' };
		});

		server.flush();
		await Promise.all([drag, pick]);

		// The wrong rule was changed: A took D's place and got D's destination.
		expect(server.order()).toBe('B:direct C:direct D:direct A:vpn');
		expect(server.order()).not.toContain('D:vpn');
	});

	// What it does now: one at a time, each re-locating its rule when its turn
	// comes — which is only possible because the previous action has already
	// been applied to local state.
	it('changes the rule the user clicked when actions are serialized by identity', async () => {
		const server = fakeServer(seed);
		let local = seed.map((r) => ({ ...r }));
		const run = createSerialQueue();

		const movedRule = local[0]; // A
		const targetRule = local[3]; // D — the position A should take
		const pickedRule = local[3]; // D — whose destination the user is changing

		const drag = run(async () => {
			const from = local.indexOf(movedRule);
			const to = local.indexOf(targetRule);
			const p = server.reorder(from, to);
			server.flush();
			await p;
			local = reorderArray(local, from, to);
		});
		const pick = run(async () => {
			const index = local.indexOf(pickedRule);
			const p = server.update(index, 'vpn');
			server.flush();
			await p;
			local = local.map((r, i) => (i === index ? { ...r, outbound: 'vpn' } : r));
		});

		await Promise.all([drag, pick]);

		// D kept its identity through the reorder and got the destination.
		expect(server.order()).toBe('B:direct C:direct D:vpn A:direct');
		// Local state agrees with the server — the divergence that used to make
		// every LATER edit hit the wrong rule.
		expect(local.map((r) => `${r.id}:${r.outbound}`).join(' ')).toBe(server.order());
	});

	it('reports rather than guesses when the rule is gone by the time it runs', async () => {
		let local = seed.map((r) => ({ ...r }));
		const run = createSerialQueue();
		const doomed = local[1]; // B

		const remove = run(async () => {
			local = local.filter((r) => r !== doomed);
		});
		let sawGone = false;
		const late = run(async () => {
			const index = local.indexOf(doomed);
			if (index < 0) {
				sawGone = true;
				return;
			}
			local[index] = { ...local[index], outbound: 'vpn' };
		});

		await Promise.all([remove, late]);
		expect(sawGone).toBe(true);
		// Nothing was written to a neighbouring rule by mistake.
		expect(local.map((r) => r.outbound)).toEqual(['direct', 'direct', 'direct']);
	});
});
