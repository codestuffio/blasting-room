import type { AgentEvent, Health, RunRequest, Venue } from './types';

async function getJSON<T>(path: string): Promise<T> {
	const res = await fetch(path);
	if (!res.ok) throw new Error(`${path}: HTTP ${res.status}`);
	return res.json();
}

export const getHealth = () => getJSON<Health>('/api/health');
export const getVenues = () => getJSON<Venue[]>('/api/venues');

/**
 * Starts an agent run and calls onEvent for each server-sent event.
 * The run is a POST, so this reads the SSE stream from fetch rather than EventSource.
 */
export async function runAgent(
	req: RunRequest,
	onEvent: (e: AgentEvent) => void,
	signal?: AbortSignal
): Promise<void> {
	const res = await fetch('/api/runs', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(req),
		signal
	});
	if (!res.ok || !res.body) {
		const body = await res.json().catch(() => ({}));
		throw new Error(body.error ?? `HTTP ${res.status}`);
	}

	const reader = res.body.pipeThrough(new TextDecoderStream()).getReader();
	let buf = '';
	for (;;) {
		const { value, done } = await reader.read();
		if (done) break;
		buf += value;
		let sep: number;
		while ((sep = buf.indexOf('\n\n')) >= 0) {
			const frame = buf.slice(0, sep);
			buf = buf.slice(sep + 2);
			const event = parseFrame(frame);
			if (event) onEvent(event);
		}
	}
}

function parseFrame(frame: string): AgentEvent | null {
	let type = '';
	let data = '';
	for (const line of frame.split('\n')) {
		if (line.startsWith('event: ')) type = line.slice(7);
		else if (line.startsWith('data: ')) data += line.slice(6);
	}
	if (!type) return null;
	return { type, data: data ? JSON.parse(data) : undefined } as AgentEvent;
}
