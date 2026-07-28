import { describe, it, expect } from 'vitest';
import { createSerialQueue } from './serialQueue';

/** A promise plus the handles to settle it from the test. */
function deferred<T = void>() {
	let resolve!: (v: T) => void;
	let reject!: (e: unknown) => void;
	const promise = new Promise<T>((res, rej) => {
		resolve = res;
		reject = rej;
	});
	return { promise, resolve, reject };
}

describe('createSerialQueue', () => {
	it('does not start a task until the previous one has settled', async () => {
		const run = createSerialQueue();
		const first = deferred();
		const log: string[] = [];

		const a = run(async () => {
			log.push('a:start');
			await first.promise;
			log.push('a:end');
		});
		const b = run(async () => {
			log.push('b:start');
		});

		// b must not have started while a is still pending — this is the whole point.
		await Promise.resolve();
		expect(log).toEqual(['a:start']);

		first.resolve();
		await Promise.all([a, b]);
		expect(log).toEqual(['a:start', 'a:end', 'b:start']);
	});

	it('preserves call order across many tasks', async () => {
		const run = createSerialQueue();
		const log: number[] = [];
		const tasks = [0, 1, 2, 3, 4].map((i) =>
			run(async () => {
				// Deliberately uneven: a naive implementation would let 4 finish first.
				await new Promise((r) => setTimeout(r, (5 - i) * 2));
				log.push(i);
			})
		);
		await Promise.all(tasks);
		expect(log).toEqual([0, 1, 2, 3, 4]);
	});

	// A failed write (409 in read-only mode, say) must not strand every later
	// action in the session.
	it('keeps running after a task rejects, and rejects only its own caller', async () => {
		const run = createSerialQueue();
		const log: string[] = [];

		const failing = run(async () => {
			log.push('boom');
			throw new Error('nope');
		});
		const after = run(async () => {
			log.push('after');
			return 'ok';
		});

		await expect(failing).rejects.toThrow('nope');
		await expect(after).resolves.toBe('ok');
		expect(log).toEqual(['boom', 'after']);
	});

	it('returns each task’s own value', async () => {
		const run = createSerialQueue();
		const [x, y] = await Promise.all([run(async () => 1), run(async () => 2)]);
		expect([x, y]).toEqual([1, 2]);
	});
});
