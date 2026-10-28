<script lang="ts">
	import type { Deal, Venue } from '#lib/types.ts';

	// The offer to send, priced against the room. The math comes from the
	// server, computed from the terms and the venue's costs.
	let {
		deal,
		venue,
		headliner,
		support = '',
		date = ''
	}: { deal: Deal; venue: Venue; headliner: string; support?: string; date?: string } = $props();

	let math = $derived(deal.math);
	const usd = (n: number) => `${n < 0 ? '−' : ''}$${Math.abs(Math.round(n)).toLocaleString('en-US')}`;
	const pct = (n: number) => `${Math.round(n * 100)}%`;
	// Marker positions on the capacity gauge, clamped so labels stay inside it.
	const share = (tickets: number) => Math.min(100, Math.max(0, (tickets / (math?.capacity || 1)) * 100));
	const at = (tickets: number) => `${share(tickets)}%`;
	// A label near either end hangs inward instead of centering past the edge.
	const edge = (tickets: number) => (share(tickets) < 12 ? 'lo' : share(tickets) > 88 ? 'hi' : '');

	let expected = $derived(math ? Math.min(deal.expected_tickets, math.capacity) : 0);
	let breakEven = $derived(math?.break_even_tickets ?? 0);
	let safe = $derived(expected >= breakEven);

	const longDate = (d: string) =>
		new Intl.DateTimeFormat('en-US', { weekday: 'short', month: 'short', day: 'numeric', timeZone: 'UTC' }).format(new Date(Date.parse(d)));

	let offerText = $derived(
		[
			`Hi, this is ${venue.name} in ${venue.neighborhood}, ${venue.city} (${venue.capacity} cap).`,
			`We'd like to make an offer for ${headliner}${date ? ` on ${longDate(date)}` : ''}${support ? `, with ${support} supporting` : ''}:`,
			deal.terms,
			`Tickets $${deal.ticket_price_advance} advance / $${deal.ticket_price_door} day of show.`,
			`Happy to hold the date while you check. Thanks!`
		].join('\n')
	);

	let copied = $state<'idle' | 'done' | 'failed'>('idle');
	async function copy() {
		try {
			await navigator.clipboard.writeText(offerText);
			copied = 'done';
		} catch {
			copied = 'failed';
		}
		setTimeout(() => (copied = 'idle'), 2500);
	}
</script>

<section class="deal" aria-labelledby="deal-title">
	<div class="head">
		<span class="tape">The offer</span>
		<button type="button" class="copy" onclick={copy} title={offerText}>
			{copied === 'done' ? 'Copied' : copied === 'failed' ? 'Copy failed' : 'Copy offer'}
		</button>
	</div>
	<h3 id="deal-title" class="terms">{deal.terms}</h3>
	<p class="prices">
		<span><strong>{usd(deal.guarantee)}</strong> guarantee</span>
		{#if deal.backend_pct > 0}<span><strong>{deal.backend_pct}%</strong> after the split</span>{/if}
		<span><strong>${deal.ticket_price_advance}</strong> adv / <strong>${deal.ticket_price_door}</strong> door</span>
		{#if deal.support_fee > 0}<span><strong>{usd(deal.support_fee)}</strong> support</span>{/if}
	</p>

	{#if math}
		<div class="gauge" role="img" aria-label={`Break-even at ${breakEven} of ${math.capacity} tickets; expected about ${expected}.`}>
			<div class="track">
				<span class="fill" style:width={at(expected)}></span>
			</div>
			<span class="mark be {edge(breakEven)}" style:left={at(breakEven)}>
				<i aria-hidden="true"></i>
				<span>Break-even {breakEven}</span>
			</span>
			{#if expected > 0}
				<span class="mark exp {edge(expected)}" style:left={at(expected)}>
					<i aria-hidden="true"></i>
					<span>Expected ~{expected}{edge(expected) === 'hi' ? ` of ${math.capacity} cap` : ''}</span>
				</span>
			{/if}
			<!-- An end label gives way to a marker that hangs over that end. -->
			{#if edge(expected) !== 'lo'}<span class="end start">0</span>{/if}
			{#if edge(expected) !== 'hi'}<span class="end stop">{math.capacity} cap</span>{/if}
		</div>
		<p class="note">
			The door covers the guarantee and the room's {usd(math.house_costs)} nightly costs at <strong>{breakEven}</strong> tickets
			({pct(breakEven / math.capacity)} of capacity), or {math.break_even_with_bar} counting the bar.
			{#if expected > 0}
				{safe ? `The expected ~${expected} clears it.` : `The expected ~${expected} falls short on the door; the bar has to carry the night.`}
			{/if}
		</p>

		<table>
			<thead>
				<tr><th>If we sell</th><th>Artist takes</th><th>Room: door</th><th>Room: bar</th><th>Room total</th></tr>
			</thead>
			<tbody>
				{#each math.scenarios as s (s.label)}
					<tr class:expected={s.label === 'Expected'}>
						<td><strong>{s.tickets}</strong> <span class="muted">{s.label.toLowerCase()}</span></td>
						<td class="num">{usd(s.artist_total)}</td>
						<td class="num" class:loss={s.venue_door < 0}>{usd(s.venue_door)}</td>
						<td class="num">{usd(s.venue_bar)}</td>
						<td class="num" class:loss={s.venue_total < 0}><strong>{usd(s.venue_total)}</strong></td>
					</tr>
				{/each}
			</tbody>
		</table>
		<p class="fine muted">Tickets counted at the advance price, before fees and taxes. Bar is net of cost of goods.</p>
	{/if}

	<div class="why">
		<p><span class="label">Walk away</span> {deal.walk_away}</p>
		<p><span class="label">Why these numbers</span> {deal.rationale}</p>
	</div>
</section>

<style>
	.deal {
		display: grid;
		gap: 0.7rem;
		padding: 1rem 1.1rem;
		border: 1px solid var(--accent);
		border-radius: 12px;
		background: var(--panel);
	}
	.head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 1rem;
	}
	.copy {
		min-height: 36px;
		padding: 0 0.9rem;
		font-family: var(--font-mono);
		font-size: 0.75rem;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		border-radius: 8px;
		border: 1px solid var(--line);
		background: transparent;
		color: var(--text);
		cursor: pointer;
	}
	.copy:hover {
		border-color: var(--accent);
	}
	.terms {
		margin: 0;
		font-family: var(--font-display);
		font-weight: 400;
		font-size: 1.05rem;
		letter-spacing: 0.03em;
		line-height: 1.4;
		color: var(--text);
		text-transform: none;
	}
	.prices {
		display: flex;
		flex-wrap: wrap;
		gap: 0.3rem 1.2rem;
		margin: 0;
		font-size: 0.9rem;
		color: var(--muted);
	}
	.prices strong {
		color: var(--text);
		font-weight: 600;
	}
	/* Capacity gauge: markers above and below a thin track. */
	.gauge {
		position: relative;
		height: 4.4rem;
		margin: 0.2rem 0.4rem 0;
	}
	.track {
		position: absolute;
		left: 0;
		right: 0;
		top: 2rem;
		height: 8px;
		border-radius: 4px;
		background: var(--recess);
		border: 1px solid var(--line);
		overflow: hidden;
	}
	.fill {
		display: block;
		height: 100%;
		background: var(--accent);
		opacity: 0.55;
		print-color-adjust: exact;
	}
	.mark {
		position: absolute;
		display: grid;
		justify-items: center;
		transform: translateX(-50%);
		font-family: var(--font-mono);
		font-size: 0.72rem;
		white-space: nowrap;
	}
	.mark.lo {
		transform: translateX(-1px);
		justify-items: start;
	}
	.mark.hi {
		transform: translateX(calc(-100% + 1px));
		justify-items: end;
	}
	.mark.be {
		top: 0;
	}
	.mark.be i {
		order: 2;
		width: 2px;
		height: 1.6rem;
		margin-top: 0.15rem;
		background: var(--text);
	}
	.mark.exp {
		top: 1.85rem;
	}
	.mark.exp i {
		width: 12px;
		height: 12px;
		border-radius: 50%;
		background: var(--accent);
		border: 2px solid var(--panel);
		print-color-adjust: exact;
	}
	.mark.exp span {
		margin-top: 0.15rem;
		color: var(--accent-text);
	}
	.end {
		position: absolute;
		top: 3.3rem;
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--muted);
	}
	.end.start {
		left: 0;
	}
	.end.stop {
		right: 0;
	}
	.note {
		margin: 0;
		font-size: 0.9rem;
	}
	table {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.85rem;
	}
	th,
	td {
		text-align: left;
		padding: 0.35rem 0.4rem;
		border-bottom: 1px solid var(--line);
	}
	th {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		font-weight: 400;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--muted);
	}
	.num {
		text-align: right;
		font-variant-numeric: tabular-nums;
	}
	th:not(:first-child) {
		text-align: right;
	}
	tr.expected td {
		background: color-mix(in srgb, var(--accent) 9%, transparent);
	}
	.loss {
		color: var(--bad);
	}
	.fine {
		margin: 0;
		font-size: 0.75rem;
	}
	.muted {
		color: var(--muted);
	}
	.why p {
		margin: 0 0 0.35rem;
		font-size: 0.9rem;
	}
	.why .label {
		margin-right: 0.4rem;
	}
	@media print {
		.copy {
			display: none;
		}
	}
	@media (max-width: 520px) {
		th:nth-child(3),
		td:nth-child(3),
		th:nth-child(4),
		td:nth-child(4) {
			display: none;
		}
	}
</style>
