/**
 * Line-based diff using LCS (Longest Common Subsequence) algorithm.
 * Produces side-by-side diff data with word-level change highlighting.
 */

export interface DiffLine {
	type: 'same' | 'added' | 'removed' | 'changed';
	leftNum?: number;
	rightNum?: number;
	leftText?: string;
	rightText?: string;
	/** Word-level diff segments for changed lines */
	leftSegments?: DiffSegment[];
	rightSegments?: DiffSegment[];
}

export interface DiffSegment {
	text: string;
	highlight: boolean;
}

/**
 * Compute side-by-side diff between two strings.
 */
export function computeSideBySideDiff(oldText: string, newText: string): DiffLine[] {
	const oldLines = oldText.split('\n');
	const newLines = newText.split('\n');

	const ops = lcsBacktrack(oldLines, newLines);
	const result: DiffLine[] = [];

	let leftNum = 1;
	let rightNum = 1;

	for (const op of ops) {
		switch (op.type) {
			case 'equal':
				result.push({
					type: 'same',
					leftNum: leftNum++,
					rightNum: rightNum++,
					leftText: op.oldLine!,
					rightText: op.newLine!
				});
				break;

			case 'delete':
				result.push({
					type: 'removed',
					leftNum: leftNum++,
					leftText: op.oldLine!
				});
				break;

			case 'insert':
				result.push({
					type: 'added',
					rightNum: rightNum++,
					rightText: op.newLine!
				});
				break;

			case 'replace': {
				const segments = computeWordDiff(op.oldLine!, op.newLine!);
				result.push({
					type: 'changed',
					leftNum: leftNum++,
					rightNum: rightNum++,
					leftText: op.oldLine!,
					rightText: op.newLine!,
					leftSegments: segments.left,
					rightSegments: segments.right
				});
				break;
			}
		}
	}

	return result;
}

interface EditOp {
	type: 'equal' | 'delete' | 'insert' | 'replace';
	oldLine?: string;
	newLine?: string;
}

/**
 * LCS-based diff producing edit operations.
 * Groups adjacent insert+delete pairs into 'replace' ops for side-by-side display.
 */
function lcsBacktrack(oldLines: string[], newLines: string[]): EditOp[] {
	const m = oldLines.length;
	const n = newLines.length;

	// Build LCS table
	const dp: number[][] = Array.from({ length: m + 1 }, () => new Array(n + 1).fill(0));
	for (let i = 1; i <= m; i++) {
		for (let j = 1; j <= n; j++) {
			if (oldLines[i - 1] === newLines[j - 1]) {
				dp[i][j] = dp[i - 1][j - 1] + 1;
			} else {
				dp[i][j] = Math.max(dp[i - 1][j], dp[i][j - 1]);
			}
		}
	}

	// Backtrack to get raw ops
	const rawOps: EditOp[] = [];
	let i = m, j = n;
	while (i > 0 || j > 0) {
		if (i > 0 && j > 0 && oldLines[i - 1] === newLines[j - 1]) {
			rawOps.push({ type: 'equal', oldLine: oldLines[i - 1], newLine: newLines[j - 1] });
			i--; j--;
		} else if (j > 0 && (i === 0 || dp[i][j - 1] >= dp[i - 1][j])) {
			rawOps.push({ type: 'insert', newLine: newLines[j - 1] });
			j--;
		} else {
			rawOps.push({ type: 'delete', oldLine: oldLines[i - 1] });
			i--;
		}
	}
	rawOps.reverse();

	// Merge adjacent delete+insert into replace ops
	const merged: EditOp[] = [];
	let idx = 0;
	while (idx < rawOps.length) {
		if (rawOps[idx].type === 'delete') {
			// Collect consecutive deletes
			const deletes: EditOp[] = [];
			while (idx < rawOps.length && rawOps[idx].type === 'delete') {
				deletes.push(rawOps[idx++]);
			}
			// Collect consecutive inserts
			const inserts: EditOp[] = [];
			while (idx < rawOps.length && rawOps[idx].type === 'insert') {
				inserts.push(rawOps[idx++]);
			}
			// Pair them as replacements
			const pairCount = Math.min(deletes.length, inserts.length);
			for (let k = 0; k < pairCount; k++) {
				merged.push({
					type: 'replace',
					oldLine: deletes[k].oldLine,
					newLine: inserts[k].newLine
				});
			}
			// Remaining deletes
			for (let k = pairCount; k < deletes.length; k++) {
				merged.push(deletes[k]);
			}
			// Remaining inserts
			for (let k = pairCount; k < inserts.length; k++) {
				merged.push(inserts[k]);
			}
		} else {
			merged.push(rawOps[idx++]);
		}
	}

	return merged;
}

/**
 * Word-level diff for a pair of changed lines.
 * Splits by word boundaries and highlights differences.
 */
function computeWordDiff(
	oldLine: string,
	newLine: string
): { left: DiffSegment[]; right: DiffSegment[] } {
	const oldTokens = tokenize(oldLine);
	const newTokens = tokenize(newLine);

	const m = oldTokens.length;
	const n = newTokens.length;

	// LCS on tokens
	const dp: number[][] = Array.from({ length: m + 1 }, () => new Array(n + 1).fill(0));
	for (let i = 1; i <= m; i++) {
		for (let j = 1; j <= n; j++) {
			if (oldTokens[i - 1] === newTokens[j - 1]) {
				dp[i][j] = dp[i - 1][j - 1] + 1;
			} else {
				dp[i][j] = Math.max(dp[i - 1][j], dp[i][j - 1]);
			}
		}
	}

	// Backtrack
	const leftSegments: DiffSegment[] = [];
	const rightSegments: DiffSegment[] = [];
	let i = m, j = n;

	const leftStack: DiffSegment[] = [];
	const rightStack: DiffSegment[] = [];

	while (i > 0 || j > 0) {
		if (i > 0 && j > 0 && oldTokens[i - 1] === newTokens[j - 1]) {
			leftStack.push({ text: oldTokens[i - 1], highlight: false });
			rightStack.push({ text: newTokens[j - 1], highlight: false });
			i--; j--;
		} else if (j > 0 && (i === 0 || dp[i][j - 1] >= dp[i - 1][j])) {
			rightStack.push({ text: newTokens[j - 1], highlight: true });
			j--;
		} else {
			leftStack.push({ text: oldTokens[i - 1], highlight: true });
			i--;
		}
	}

	leftStack.reverse();
	rightStack.reverse();

	// Merge adjacent segments with same highlight state
	return {
		left: mergeSegments(leftStack),
		right: mergeSegments(rightStack)
	};
}

function tokenize(line: string): string[] {
	// Split into words and whitespace, preserving everything
	return line.match(/\S+|\s+/g) || [];
}

function mergeSegments(segments: DiffSegment[]): DiffSegment[] {
	const result: DiffSegment[] = [];
	for (const seg of segments) {
		if (result.length > 0 && result[result.length - 1].highlight === seg.highlight) {
			result[result.length - 1].text += seg.text;
		} else {
			result.push({ ...seg });
		}
	}
	return result;
}
