/**
 * mieru `traffic_pattern` is a base64-encoded protobuf (appctlpb.TrafficPattern).
 * The UI edits the fields; this module is the codec.
 *
 * ponytail: hand-rolled protobuf writer/reader instead of a codegen runtime —
 * the message is 6 fields deep and pinned by golden vectors from the mieru Go
 * library (see mieruPattern.test.ts). Swap in protobufjs if the schema grows.
 *
 * Presence matters: an explicitly-set 0 disables a feature, while an absent
 * field lets mieru generate an implicit value from the seed. `undefined` is
 * "not set"; anything else is encoded, including 0/false.
 */

export interface TcpFragment {
	enable?: boolean;
	maxSleepMs?: number;
}

export interface NoncePattern {
	type?: number;
	applyToAllUDPPacket?: boolean;
	minLen?: number;
	maxLen?: number;
	customHexStrings?: string[];
}

export interface PaddingPattern {
	maxMiddlePaddingLen?: number;
	maxEndPaddingLen?: number;
}

export interface LowEntropyPattern {
	mode?: number;
	maskRotation?: number;
}

export interface TrafficPattern {
	seed?: number;
	unlockAll?: boolean;
	tcpFragment?: TcpFragment;
	nonce?: NoncePattern;
	padding?: PaddingPattern;
	lowEntropy?: LowEntropyPattern;
}

// NonceType enum values.
export const NONCE_RANDOM = 0;
export const NONCE_PRINTABLE = 1;
export const NONCE_PRINTABLE_SUBSET = 2;
export const NONCE_FIXED = 3;

// LowEntropyMode enum values (bits of payload per 64-bit chunk).
export const LOW_ENTROPY_MODES = [0, 1, 2, 3, 4] as const; // OFF, 32, 40, 48, 56

export const MAX_SEED = 2147483647;
export const MAX_SLEEP_MS = 100;
export const MAX_NONCE_LEN = 12;
export const MAX_PADDING_LEN = 255;
export const MAX_ROTATION_STEPS = 15;

/** Mask rotation is a single enum: 0 = none, 1..15 = rotate right, N*16 = rotate left. */
export function rotationValue(direction: 'none' | 'right' | 'left', steps: number): number {
	if (direction === 'none') return 0;
	const n = Math.min(Math.max(Math.round(steps), 1), MAX_ROTATION_STEPS);
	return direction === 'right' ? n : n * 16;
}

export function rotationParts(value: number): { direction: 'none' | 'right' | 'left'; steps: number } {
	if (!value) return { direction: 'none', steps: 1 };
	if (value <= MAX_ROTATION_STEPS) return { direction: 'right', steps: value };
	return { direction: 'left', steps: value / 16 };
}

// --- protobuf wire helpers -------------------------------------------------

function putVarint(out: number[], value: number) {
	let v = Math.round(value);
	while (v > 127) {
		out.push((v & 0x7f) | 0x80);
		v = Math.floor(v / 128);
	}
	out.push(v);
}

function putTag(out: number[], field: number, wire: number) {
	putVarint(out, field * 8 + wire);
}

function putVarintField(out: number[], field: number, value: number | undefined) {
	if (value === undefined) return;
	putTag(out, field, 0);
	putVarint(out, value);
}

function putBoolField(out: number[], field: number, value: boolean | undefined) {
	if (value === undefined) return;
	putTag(out, field, 0);
	putVarint(out, value ? 1 : 0);
}

function putBytesField(out: number[], field: number, bytes: number[]) {
	putTag(out, field, 2);
	putVarint(out, bytes.length);
	out.push(...bytes);
}

function putMessageField(out: number[], field: number, body: number[]) {
	if (body.length === 0) return; // an all-unset submessage carries no information
	putBytesField(out, field, body);
}

function putStringField(out: number[], field: number, value: string) {
	putBytesField(out, field, Array.from(new TextEncoder().encode(value)));
}

class Reader {
	private i = 0;
	constructor(private buf: Uint8Array) {}

	get done(): boolean {
		return this.i >= this.buf.length;
	}

	varint(): number {
		let result = 0;
		let shift = 1;
		for (;;) {
			if (this.done) throw new Error('truncated varint');
			const b = this.buf[this.i++];
			result += (b & 0x7f) * shift;
			if ((b & 0x80) === 0) return result;
			shift *= 128;
		}
	}

	bytes(): Uint8Array {
		const len = this.varint();
		if (this.i + len > this.buf.length) throw new Error('truncated field');
		const out = this.buf.subarray(this.i, this.i + len);
		this.i += len;
		return out;
	}

	/** Skips a field of the given wire type, keeping unknown fields harmless. */
	skip(wire: number) {
		if (wire === 0) this.varint();
		else if (wire === 1) this.i += 8;
		else if (wire === 2) this.bytes();
		else if (wire === 5) this.i += 4;
		else throw new Error(`unsupported wire type ${wire}`);
		if (this.i > this.buf.length) throw new Error('truncated field');
	}
}

function eachField(buf: Uint8Array, fn: (field: number, wire: number, r: Reader) => boolean) {
	const r = new Reader(buf);
	while (!r.done) {
		const key = r.varint();
		const field = Math.floor(key / 8);
		const wire = key % 8;
		if (!fn(field, wire, r)) r.skip(wire);
	}
}

// --- encode / decode -------------------------------------------------------

function encodeMessage(p: TrafficPattern): number[] {
	const out: number[] = [];
	putVarintField(out, 1, p.seed);
	putBoolField(out, 2, p.unlockAll);

	if (p.tcpFragment) {
		const f: number[] = [];
		putBoolField(f, 1, p.tcpFragment.enable);
		putVarintField(f, 2, p.tcpFragment.maxSleepMs);
		putMessageField(out, 3, f);
	}
	if (p.nonce) {
		const n: number[] = [];
		putVarintField(n, 1, p.nonce.type);
		putBoolField(n, 2, p.nonce.applyToAllUDPPacket);
		putVarintField(n, 3, p.nonce.minLen);
		putVarintField(n, 4, p.nonce.maxLen);
		for (const hex of p.nonce.customHexStrings ?? []) putStringField(n, 5, hex);
		putMessageField(out, 4, n);
	}
	if (p.padding) {
		const pad: number[] = [];
		putVarintField(pad, 1, p.padding.maxMiddlePaddingLen);
		putVarintField(pad, 2, p.padding.maxEndPaddingLen);
		putMessageField(out, 5, pad);
	}
	if (p.lowEntropy) {
		const le: number[] = [];
		putVarintField(le, 1, p.lowEntropy.mode);
		putVarintField(le, 2, p.lowEntropy.maskRotation);
		putMessageField(out, 6, le);
	}
	return out;
}

/** Encodes a pattern to the base64 string sing-box expects. Empty pattern → "". */
export function encodeTrafficPattern(p: TrafficPattern): string {
	const bytes = encodeMessage(p);
	if (bytes.length === 0) return '';
	let bin = '';
	for (const b of bytes) bin += String.fromCharCode(b);
	return btoa(bin);
}

/** Decodes a base64 pattern. Returns null when the string is not a valid pattern. */
export function decodeTrafficPattern(encoded: string): TrafficPattern | null {
	const s = encoded.trim();
	if (!s) return {};
	let buf: Uint8Array;
	try {
		const bin = atob(s);
		buf = Uint8Array.from(bin, (c) => c.charCodeAt(0));
	} catch {
		return null;
	}
	const p: TrafficPattern = {};
	try {
		eachField(buf, (field, wire, r) => {
			if (field === 1 && wire === 0) p.seed = r.varint();
			else if (field === 2 && wire === 0) p.unlockAll = r.varint() !== 0;
			else if (field === 3 && wire === 2) {
				const f: TcpFragment = {};
				eachField(r.bytes(), (sf, sw, sr) => {
					if (sf === 1 && sw === 0) f.enable = sr.varint() !== 0;
					else if (sf === 2 && sw === 0) f.maxSleepMs = sr.varint();
					else return false;
					return true;
				});
				p.tcpFragment = f;
			} else if (field === 4 && wire === 2) {
				const n: NoncePattern = {};
				eachField(r.bytes(), (sf, sw, sr) => {
					if (sf === 1 && sw === 0) n.type = sr.varint();
					else if (sf === 2 && sw === 0) n.applyToAllUDPPacket = sr.varint() !== 0;
					else if (sf === 3 && sw === 0) n.minLen = sr.varint();
					else if (sf === 4 && sw === 0) n.maxLen = sr.varint();
					else if (sf === 5 && sw === 2) (n.customHexStrings ??= []).push(new TextDecoder().decode(sr.bytes()));
					else return false;
					return true;
				});
				p.nonce = n;
			} else if (field === 5 && wire === 2) {
				const pad: PaddingPattern = {};
				eachField(r.bytes(), (sf, sw, sr) => {
					if (sf === 1 && sw === 0) pad.maxMiddlePaddingLen = sr.varint();
					else if (sf === 2 && sw === 0) pad.maxEndPaddingLen = sr.varint();
					else return false;
					return true;
				});
				p.padding = pad;
			} else if (field === 6 && wire === 2) {
				const le: LowEntropyPattern = {};
				eachField(r.bytes(), (sf, sw, sr) => {
					if (sf === 1 && sw === 0) le.mode = sr.varint();
					else if (sf === 2 && sw === 0) le.maskRotation = sr.varint();
					else return false;
					return true;
				});
				p.lowEntropy = le;
			} else return false;
			return true;
		});
	} catch {
		return null;
	}
	return p;
}

/** True when nothing in the pattern is set (encodes to an empty string). */
export function isEmptyPattern(p: TrafficPattern): boolean {
	return encodeMessage(p).length === 0;
}
