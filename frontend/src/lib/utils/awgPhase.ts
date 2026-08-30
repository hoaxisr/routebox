// The phases the AWG enable orchestrator reports on its way up (backend:
// awg.EnablePhase). `idle`, `ready` and `failed` are terminal — everything else
// means work is still going on, which on the kernel backend can be a multi-minute
// apt + DKMS build (#93).
export const ENABLE_IN_FLIGHT_PHASES = [
	'validating',
	'installing',
	'rendering',
	'starting',
	'health-check'
] as const;

export function isEnableInFlight(phase: string | undefined | null): boolean {
	return !!phase && (ENABLE_IN_FLIGHT_PHASES as readonly string[]).includes(phase);
}
