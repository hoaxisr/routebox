// The dashboard's last-minute samples, kept at module level so leaving the page
// and coming back does not start every sparkline from zero (#99).
// ponytail: plain memory, filled only while the dashboard is open — a route
// change leaves a gap that is not drawn. A server-side time series (needed for
// the 24 h view anyway) replaces this.
export type DashboardPeriod = '60s' | '1h' | '24h';

// The breakdown beside the graph: whose traffic, or through what.
export type DashboardDim = 'source' | 'chain';

export const liveHistory = {
	down: [] as number[],
	up: [] as number[],
	cpu: [] as number[],
	// The graph's chosen period survives navigation like the samples do, and so
	// does the breakdown shown beside it (#101).
	period: '60s' as DashboardPeriod,
	dim: 'source' as DashboardDim
};
