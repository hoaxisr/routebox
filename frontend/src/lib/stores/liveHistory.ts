// The dashboard's last-minute samples, kept at module level so leaving the page
// and coming back does not start every sparkline from zero (#99).
// ponytail: plain memory, filled only while the dashboard is open — a route
// change leaves a gap that is not drawn. A server-side time series (needed for
// the 24 h view anyway) replaces this.
import type { RouteFlow } from '$lib/utils/routeSplit';

export type DashboardPeriod = '60s' | '1h' | '24h';

export const liveHistory = {
	down: [] as number[],
	up: [] as number[],
	cpu: [] as number[],
	// The route graph's live minute: download through direct vs a proxy (#110).
	direct: [] as number[],
	proxy: [] as number[],
	// The same live minute per (client, final outbound), one array per tick:
	// the rankings beside the graph sum these, so they count exactly the bytes
	// of the share above them.
	flows: [] as RouteFlow[][],
	// The graph's chosen period survives navigation like the samples do (#101).
	period: '60s' as DashboardPeriod
};
