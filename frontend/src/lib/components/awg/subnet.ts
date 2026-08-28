/**
 * The address the server itself takes inside the tunnel: the first host of the
 * AWG subnet. Mirrors the backend's firstHost() (netip Masked().Next()), which
 * is what actually lands in the server's [Interface] Address — so the DNS the
 * button fills in is the address a client can really reach.
 *
 * Returns '' for anything that is not an IPv4 CIDR, so a half-typed subnet
 * simply offers nothing rather than a wrong address.
 */
export function tunnelGateway(subnet: string | undefined | null): string {
	const m = /^\s*(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})\/(\d{1,2})\s*$/.exec(subnet ?? '');
	if (!m) return '';
	const octets = m.slice(1, 5).map(Number);
	const bits = Number(m[5]);
	if (octets.some((o) => o > 255) || bits > 32) return '';
	// A /31 or /32 has no host below the broadcast address to hand out.
	if (bits > 30) return '';

	const addr = octets.reduce((acc, o) => acc * 256 + o, 0);
	const network = bits === 0 ? 0 : (addr >>> (32 - bits)) * 2 ** (32 - bits);
	const first = network + 1;
	return [24, 16, 8, 0].map((shift) => Math.floor(first / 2 ** shift) % 256).join('.');
}
