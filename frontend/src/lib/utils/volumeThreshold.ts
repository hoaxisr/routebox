export type VolumeUnit = 'KB' | 'MB' | 'GB';

const UNIT_MULTIPLIERS: Record<VolumeUnit, number> = {
	KB: 1024,
	MB: 1024 * 1024,
	GB: 1024 * 1024 * 1024
};

/**
 * Converts a value+unit pair to bytes. Clamps negative/non-finite inputs to 0.
 */
export function bytesFromUnit(value: number, unit: VolumeUnit): number {
	if (!Number.isFinite(value) || value < 0) return 0;
	return Math.round(value * UNIT_MULTIPLIERS[unit]);
}

/**
 * Splits a byte count into the largest unit that gives value ≥ 1.
 * Used to populate the custom-value input when the popover opens.
 * 0 bytes returns { value: 0, unit: 'MB' } (sensible default unit).
 */
export function splitBytes(bytes: number): { value: number; unit: VolumeUnit } {
	if (!Number.isFinite(bytes) || bytes <= 0) return { value: 0, unit: 'MB' };
	if (bytes >= UNIT_MULTIPLIERS.GB) {
		return { value: bytes / UNIT_MULTIPLIERS.GB, unit: 'GB' };
	}
	if (bytes >= UNIT_MULTIPLIERS.MB) {
		return { value: bytes / UNIT_MULTIPLIERS.MB, unit: 'MB' };
	}
	return { value: bytes / UNIT_MULTIPLIERS.KB, unit: 'KB' };
}

export const PRESETS: ReadonlyArray<{ label: string; value: number }> = [
	{ label: 'Off', value: 0 },
	{ label: '10 MB', value: 10 * UNIT_MULTIPLIERS.MB },
	{ label: '100 MB', value: 100 * UNIT_MULTIPLIERS.MB },
	{ label: '1 GB', value: 1 * UNIT_MULTIPLIERS.GB }
];
