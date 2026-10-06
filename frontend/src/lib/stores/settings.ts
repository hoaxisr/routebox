import { writable, get } from 'svelte/store';

export type SpeedUnit = 'bits' | 'bytes';

export const speedUnit = writable<SpeedUnit>('bytes');

export function formatBytes(bytes: number): string {
	if (bytes === 0) return '0 B';
	const k = 1024;
	const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
	// Fractions of a byte (an hour's average of one short burst) have a
	// negative log — clamp, or the unit reads "undefined" (#99).
	let i = Math.max(0, Math.min(sizes.length - 1, Math.floor(Math.log(bytes) / Math.log(k))));
	// 1000..1023 of a unit reads as the next one: a four-digit number widened
	// the dashboard's speed and re-wrapped its row on a phone (#110).
	if (bytes / Math.pow(k, i) >= 999.95 && i < sizes.length - 1) i++;
	return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
}

export function formatSpeed(bytesPerSec: number): string {
	const unit = get(speedUnit);
	if (unit === 'bits') {
		const bits = bytesPerSec * 8;
		if (bits === 0) return '0 bps';
		const k = 1000;
		const sizes = ['bps', 'Kbps', 'Mbps', 'Gbps', 'Tbps'];
		let i = Math.max(0, Math.min(sizes.length - 1, Math.floor(Math.log(bits) / Math.log(k))));
		if (bits / Math.pow(k, i) >= 999.95 && i < sizes.length - 1) i++;
		return `${(bits / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
	}
	return `${formatBytes(bytesPerSec)}/s`;
}
