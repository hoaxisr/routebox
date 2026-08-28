import { describe, it, expect } from 'vitest';
import { tunnelGateway } from './subnet';

describe('tunnelGateway', () => {
	it('returns the first host of the subnet', () => {
		expect(tunnelGateway('10.10.64.0/24')).toBe('10.10.64.1');
		expect(tunnelGateway('10.8.0.0/16')).toBe('10.8.0.1');
		expect(tunnelGateway('192.168.100.0/30')).toBe('192.168.100.1');
	});

	it('masks an address that is not the network address, like the backend does', () => {
		expect(tunnelGateway('10.10.64.37/24')).toBe('10.10.64.1');
		expect(tunnelGateway('10.8.5.9/16')).toBe('10.8.0.1');
	});

	it('offers nothing for input that is not a usable IPv4 subnet', () => {
		for (const bad of ['', '   ', '10.10.64.0', '10.10.64.', 'fd00::/64', '999.1.1.0/24', '10.0.0.0/33']) {
			expect(tunnelGateway(bad)).toBe('');
		}
		expect(tunnelGateway(undefined)).toBe('');
	});

	it('offers nothing for /31 and /32, which have no host to hand out', () => {
		expect(tunnelGateway('10.0.0.0/31')).toBe('');
		expect(tunnelGateway('10.0.0.1/32')).toBe('');
	});
});
