// Mirrors the Go types in server/internal/agent.

export interface Venue {
	id: string;
	name: string;
	neighborhood: string;
	city: string;
	lat: number;
	lon: number;
	capacity: number;
	vibe: string;
	seeds: string[];
	house_costs: number;
	bar_net_per_head: number;
}

export interface Health {
	ok: boolean;
	agent_ready: boolean;
	qloo_mode: 'mock' | 'live';
	// Whether competing-show lookups (Ticketmaster) are configured.
	shows_check: boolean;
	mock_scenario?: MockScenario;
	// Present when the server caps runs per UTC day (BR_DAILY_RUNS).
	runs_left_today?: number;
}

// Which synthetic data the mock serves (QLOO_MOCK_SCENARIO on the server).
export type MockScenario = 'full' | 'sparse' | 'degraded' | 'empty';

export type Mode = 'fill_date' | 'vet_offer';

export interface RunRequest {
	venue_id: string;
	mode: Mode;
	date?: string;
	offer_artist?: string;
	ask?: string;
	notes?: string;
}

export interface Pick {
	name: string;
	entity_id: string;
	rationale: string[];
}

export type Verdict = 'book' | 'book_with_changes' | 'pass';

export interface Memo {
	verdict: Verdict;
	summary: string;
	confidence: 'low' | 'medium' | 'high';
	headliner: Pick;
	support: Pick;
	local_fit: string;
	momentum: string;
	audience: { summary: string; age_skew: string; ticket_tier: 'budget' | 'standard' | 'premium' };
	co_promo: { kind: 'place' | 'podcast' | 'brand'; name: string; idea: string }[];
	deal: Deal;
	routing: { fit: RouteFit; summary: string };
	risks: string[];
	// Nearby shows the agent judged against the booking; empty when none compete or there was no check.
	competing_shows: { name: string; venue: string; date: string; threat: 'high' | 'medium' | 'low'; note: string }[];
	alternates: { name: string; entity_id: string; reason: string }[];
	// ref is the trace call ("Q3") behind the claim; empty when the server could not match it to one.
	evidence: { claim: string; tool: string; ref: string }[];
}

// The offer. The model sets the terms; math is computed on the server from
// the terms and the venue's costs, and is absent on a pass with no price.
export interface Deal {
	guarantee: number;
	backend_pct: number;
	support_fee: number;
	ticket_price_advance: number;
	ticket_price_door: number;
	expected_tickets: number;
	terms: string;
	walk_away: string;
	rationale: string;
	math?: DealMath;
}

export interface DealMath {
	capacity: number;
	house_costs: number;
	bar_net_per_head: number;
	split_point: number;
	break_even_tickets: number;
	break_even_with_bar: number;
	scenarios: {
		label: string;
		tickets: number;
		gross: number;
		artist_total: number;
		venue_door: number;
		venue_bar: number;
		venue_total: number;
	}[];
}

export type RouteFit =
	| 'on_the_way'
	| 'detour'
	| 'off_route'
	| 'already_in_market'
	| 'already_booked'
	| 'no_tour_data'
	| 'not_checked';

// An act's tour from tour_dates (the "route" event). For the UI only; the model sees a summary.
export interface RouteStop {
	date: string;
	venue: string;
	city: string;
	region?: string;
	miles: number;
}

export interface Route {
	tool_call_id: string;
	artist: string;
	entity_id?: string;
	venue: GeoPoint;
	date?: string;
	fit: RouteFit;
	stops: RouteStop[];
	windows: { from: string; to: string; after?: string; before?: string; detour_miles: number }[];
}

export interface ToolCall {
	id: string;
	ref: string;
	name: string;
	input: unknown;
}

export interface ToolResult {
	id: string;
	name: string;
	ok: boolean;
	error?: string;
	duration_ms: number;
	bytes: number;
}

// Map data from local_heatmap, co_promo_partners and competing_shows (the "geo" event). It is
// for the UI only; the model sees compact summaries.
export interface GeoPoint {
	name: string;
	lat: number;
	lon: number;
}

export interface HeatCell {
	lat: number;
	lon: number;
	affinity: number;
}

export interface GeoPlace {
	entity_id: string;
	name: string;
	lat: number;
	lon: number;
	address?: string;
}

export interface Geo {
	tool_call_id: string;
	// shows: one place per venue with nearby shows; its address lists them.
	kind: 'heatmap' | 'partners' | 'shows';
	entity_ids: string[];
	venue: GeoPoint;
	cells?: HeatCell[];
	places?: GeoPlace[];
}

// Popularity series from the momentum tool (the "trend" event). Like Geo it is
// for the UI only; the model sees a start/end/direction summary.
export interface TrendPoint {
	date: string;
	percentile: number;
	velocity?: number;
}

export interface TrendSeries {
	entity_id: string;
	points: TrendPoint[];
}

export interface Trend {
	tool_call_id: string;
	series: TrendSeries[];
}

export type AgentEvent =
	| { type: 'start'; data: { venue: Venue; model: string; qloo_mock: boolean } }
	| { type: 'progress' | 'text'; data: string }
	| { type: 'tool_call'; data: ToolCall }
	| { type: 'tool_result'; data: ToolResult }
	| { type: 'geo'; data: Geo }
	| { type: 'trend'; data: Trend }
	| { type: 'route'; data: Route }
	| { type: 'memo'; data: Memo }
	| { type: 'error'; data: { message: string } }
	| { type: 'done'; data?: { turns: number } };

// UI only: an event plus when it arrived, in ms since the run started. The trace shows it as a timecode.
export type TraceEvent = AgentEvent & { at: number };
