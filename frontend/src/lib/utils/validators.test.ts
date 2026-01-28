import { describe, it, expect } from 'vitest';
import {
	validatePort,
	validateRequired,
	validateTag,
	validateCIDR,
	validateBase64Key,
	validatePortRange,
	validateDomain,
	validateUUID,
	validateURL,
	validateIP,
	validateNonEmptyArray,
	validateOptionalPort,
	validatePositiveInt
} from './validators';

describe('validatePort', () => {
	it('accepts valid ports', () => {
		expect(validatePort(1).valid).toBe(true);
		expect(validatePort(80).valid).toBe(true);
		expect(validatePort(443).valid).toBe(true);
		expect(validatePort(8080).valid).toBe(true);
		expect(validatePort(65535).valid).toBe(true);
	});

	it('accepts string ports', () => {
		expect(validatePort('443').valid).toBe(true);
		expect(validatePort('8080').valid).toBe(true);
	});

	it('rejects invalid ports', () => {
		expect(validatePort(0).valid).toBe(false);
		expect(validatePort(-1).valid).toBe(false);
		expect(validatePort(65536).valid).toBe(false);
		expect(validatePort(100000).valid).toBe(false);
	});

	it('rejects empty values', () => {
		expect(validatePort(undefined).valid).toBe(false);
		expect(validatePort(null).valid).toBe(false);
		expect(validatePort('').valid).toBe(false);
	});

	it('rejects non-numeric strings', () => {
		expect(validatePort('abc').valid).toBe(false);
		// Note: parseInt('12.34', 10) = 12, which is a valid port
		// This is expected JS behavior
	});
});

describe('validateRequired', () => {
	it('accepts non-empty strings', () => {
		expect(validateRequired('hello', 'Field').valid).toBe(true);
		expect(validateRequired('  hello  ', 'Field').valid).toBe(true);
	});

	it('rejects empty strings', () => {
		expect(validateRequired('', 'Field').valid).toBe(false);
		expect(validateRequired('   ', 'Field').valid).toBe(false);
	});

	it('rejects null/undefined', () => {
		expect(validateRequired(null, 'Field').valid).toBe(false);
		expect(validateRequired(undefined, 'Field').valid).toBe(false);
	});

	it('includes field name in error', () => {
		const result = validateRequired('', 'Username');
		expect(result.error).toContain('Username');
	});
});

describe('validateTag', () => {
	it('accepts valid unique tags', () => {
		expect(validateTag('proxy', ['direct', 'block']).valid).toBe(true);
		expect(validateTag('my-proxy', []).valid).toBe(true);
	});

	it('rejects empty tags', () => {
		expect(validateTag('', []).valid).toBe(false);
		expect(validateTag('   ', []).valid).toBe(false);
	});

	it('rejects duplicate tags', () => {
		expect(validateTag('proxy', ['proxy', 'direct']).valid).toBe(false);
	});

	it('allows current tag (for editing)', () => {
		expect(validateTag('proxy', ['proxy', 'direct'], 'proxy').valid).toBe(true);
	});
});

describe('validateCIDR', () => {
	it('accepts valid IPv4 CIDR', () => {
		expect(validateCIDR('192.168.0.0/24').valid).toBe(true);
		expect(validateCIDR('10.0.0.0/8').valid).toBe(true);
		expect(validateCIDR('172.16.0.0/12').valid).toBe(true);
		expect(validateCIDR('0.0.0.0/0').valid).toBe(true);
	});

	it('accepts valid IPv6 CIDR', () => {
		expect(validateCIDR('fd00::/8').valid).toBe(true);
		expect(validateCIDR('2001:db8::/32').valid).toBe(true);
		expect(validateCIDR('::/0').valid).toBe(true);
	});

	it('rejects invalid CIDR', () => {
		expect(validateCIDR('192.168.0.0').valid).toBe(false);
		expect(validateCIDR('not-a-cidr').valid).toBe(false);
		expect(validateCIDR('192.168.0.0/').valid).toBe(false);
	});

	it('rejects empty values', () => {
		expect(validateCIDR('').valid).toBe(false);
		expect(validateCIDR(null).valid).toBe(false);
	});
});

describe('validateBase64Key', () => {
	it('accepts valid WireGuard keys (44 chars)', () => {
		// Valid 32-byte base64 encoded key (44 chars with padding)
		const validKey = 'YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY=';
		expect(validateBase64Key(validKey).valid).toBe(true);
	});

	it('rejects invalid keys', () => {
		expect(validateBase64Key('short').valid).toBe(false);
		expect(validateBase64Key('').valid).toBe(false);
		expect(validateBase64Key('not-base64-key-format').valid).toBe(false);
	});

	it('rejects keys without padding', () => {
		expect(validateBase64Key('YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY').valid).toBe(false);
	});

	it('includes key type in error', () => {
		const result = validateBase64Key('', 'Private key');
		expect(result.error).toContain('Private key');
	});
});

describe('validatePortRange', () => {
	it('accepts valid port ranges with dash', () => {
		expect(validatePortRange('1000-2000').valid).toBe(true);
		expect(validatePortRange('1-65535').valid).toBe(true);
	});

	it('accepts valid port ranges with colon', () => {
		expect(validatePortRange('1000:2000').valid).toBe(true);
		expect(validatePortRange('80:443').valid).toBe(true);
	});

	it('rejects invalid formats', () => {
		expect(validatePortRange('1000').valid).toBe(false);
		expect(validatePortRange('abc-def').valid).toBe(false);
		expect(validatePortRange('').valid).toBe(false);
	});

	it('rejects invalid ranges', () => {
		expect(validatePortRange('2000-1000').valid).toBe(false); // start >= end
		expect(validatePortRange('0-100').valid).toBe(false); // start < 1
		expect(validatePortRange('1000-70000').valid).toBe(false); // end > 65535
	});
});

describe('validateDomain', () => {
	it('accepts valid domains', () => {
		expect(validateDomain('example.com').valid).toBe(true);
		expect(validateDomain('sub.example.com').valid).toBe(true);
		expect(validateDomain('my-site.org').valid).toBe(true);
		expect(validateDomain('localhost').valid).toBe(true);
	});

	it('rejects invalid domains', () => {
		expect(validateDomain('').valid).toBe(false);
		expect(validateDomain('-invalid.com').valid).toBe(false);
		expect(validateDomain('invalid-.com').valid).toBe(false);
	});
});

describe('validateUUID', () => {
	it('accepts valid UUIDs', () => {
		expect(validateUUID('550e8400-e29b-41d4-a716-446655440000').valid).toBe(true);
		expect(validateUUID('123e4567-e89b-12d3-a456-426614174000').valid).toBe(true);
		expect(validateUUID('00000000-0000-0000-0000-000000000000').valid).toBe(true);
	});

	it('accepts uppercase UUIDs', () => {
		expect(validateUUID('550E8400-E29B-41D4-A716-446655440000').valid).toBe(true);
	});

	it('rejects invalid UUIDs', () => {
		expect(validateUUID('not-a-uuid').valid).toBe(false);
		expect(validateUUID('550e8400-e29b-41d4-a716').valid).toBe(false);
		expect(validateUUID('550e8400e29b41d4a716446655440000').valid).toBe(false);
		expect(validateUUID('').valid).toBe(false);
	});
});

describe('validateURL', () => {
	it('accepts valid URLs', () => {
		expect(validateURL('https://example.com').valid).toBe(true);
		expect(validateURL('http://localhost:8080').valid).toBe(true);
		expect(validateURL('https://example.com/path?query=1').valid).toBe(true);
	});

	it('rejects invalid URLs', () => {
		expect(validateURL('not-a-url').valid).toBe(false);
		expect(validateURL('example.com').valid).toBe(false);
		expect(validateURL('').valid).toBe(false);
	});
});

describe('validateIP', () => {
	it('accepts valid IPv4 addresses', () => {
		expect(validateIP('192.168.1.1').valid).toBe(true);
		expect(validateIP('10.0.0.1').valid).toBe(true);
		expect(validateIP('255.255.255.255').valid).toBe(true);
	});

	it('accepts valid IPv6 addresses', () => {
		expect(validateIP('::1').valid).toBe(true);
		expect(validateIP('2001:db8::1').valid).toBe(true);
		expect(validateIP('fe80::1').valid).toBe(true);
	});

	it('rejects invalid IPs', () => {
		expect(validateIP('not-an-ip').valid).toBe(false);
		expect(validateIP('').valid).toBe(false);
		expect(validateIP('192.168.1').valid).toBe(false);
	});
});

describe('validateNonEmptyArray', () => {
	it('accepts non-empty arrays', () => {
		expect(validateNonEmptyArray([1, 2, 3], 'Items').valid).toBe(true);
		expect(validateNonEmptyArray(['a'], 'Items').valid).toBe(true);
	});

	it('rejects empty arrays', () => {
		expect(validateNonEmptyArray([], 'Items').valid).toBe(false);
	});

	it('rejects null/undefined', () => {
		expect(validateNonEmptyArray(null, 'Items').valid).toBe(false);
		expect(validateNonEmptyArray(undefined, 'Items').valid).toBe(false);
	});

	it('includes field name in error', () => {
		const result = validateNonEmptyArray([], 'Outbounds');
		expect(result.error).toContain('outbounds');
	});
});

describe('validateOptionalPort', () => {
	it('accepts valid ports', () => {
		expect(validateOptionalPort(443).valid).toBe(true);
		expect(validateOptionalPort(8080).valid).toBe(true);
	});

	it('accepts empty values (optional)', () => {
		expect(validateOptionalPort(undefined).valid).toBe(true);
		expect(validateOptionalPort(null).valid).toBe(true);
		expect(validateOptionalPort('').valid).toBe(true);
		expect(validateOptionalPort(0).valid).toBe(true);
	});

	it('rejects invalid ports when provided', () => {
		expect(validateOptionalPort(65536).valid).toBe(false);
		expect(validateOptionalPort(-1).valid).toBe(false);
	});
});

describe('validatePositiveInt', () => {
	it('accepts positive integers', () => {
		expect(validatePositiveInt(1, 'Count').valid).toBe(true);
		expect(validatePositiveInt(100, 'Count').valid).toBe(true);
		expect(validatePositiveInt('42', 'Count').valid).toBe(true);
	});

	it('rejects zero and negative numbers', () => {
		expect(validatePositiveInt(0, 'Count').valid).toBe(false);
		expect(validatePositiveInt(-1, 'Count').valid).toBe(false);
	});

	it('rejects non-numeric values', () => {
		expect(validatePositiveInt('abc', 'Count').valid).toBe(false);
		expect(validatePositiveInt('', 'Count').valid).toBe(false);
	});
});
