/**
 * Runs tasks one at a time, in call order.
 *
 * Route rules are addressed by their position in an array, and the position a
 * click captured stops being true the moment another mutation lands. Two
 * overlapping actions — say switching a destination while a drag is still in
 * flight — could therefore edit a DIFFERENT rule than the one clicked, silently,
 * with no error anywhere. Serializing the writes is only half the fix: the
 * caller must also recompute the index when its turn comes (see withRule), and
 * that recomputation is only meaningful once the previous mutation has already
 * been applied to local state — which is exactly what this guarantees.
 *
 * A failed task does not poison the queue: the next one still runs. Callers get
 * their own rejection, once.
 */
export function createSerialQueue(): <T>(task: () => Promise<T>) => Promise<T> {
	let tail: Promise<unknown> = Promise.resolve();
	return function run<T>(task: () => Promise<T>): Promise<T> {
		// Chain off the tail regardless of how it settled, so one failure cannot
		// strand everything queued behind it.
		const result = tail.then(task, task);
		tail = result.catch(() => undefined);
		return result;
	};
}
