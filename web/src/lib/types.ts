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
}

export interface Health {
	ok: boolean;
	agent_ready: boolean;
	qloo_mode: 'mock' | 'live';
}

export type Mode = 'fill_date' | 'vet_offer';

export interface RunRequest {
	venue_id: string;
	mode: Mode;
	date?: string;
	offer_artist?: string;
	notes?: string;
}

export interface Pick {
	name: string;
	entity_id: string;
	rationale: string[];
}

export interface Memo {
	summary: string;
	confidence: 'low' | 'medium' | 'high';
	headliner: Pick;
	support: Pick;
	local_fit: string;
	momentum: string;
	audience: { summary: string; age_skew: string; ticket_tier: 'budget' | 'standard' | 'premium' };
	co_promo: { kind: 'place' | 'podcast' | 'brand'; name: string; idea: string }[];
	risks: string[];
	alternates: { name: string; entity_id: string; reason: string }[];
	evidence: { claim: string; tool: string }[];
}

export interface ToolCall {
	id: string;
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

export type AgentEvent =
	| { type: 'start'; data: { venue: Venue; model: string; qloo_mock: boolean } }
	| { type: 'progress' | 'text'; data: string }
	| { type: 'tool_call'; data: ToolCall }
	| { type: 'tool_result'; data: ToolResult }
	| { type: 'memo'; data: Memo }
	| { type: 'error'; data: { message: string } }
	| { type: 'done'; data?: { turns: number } };
