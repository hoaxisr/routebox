import { describe, it, expect } from 'vitest';
import {
	parseLines,
	parseCSV,
	parsePorts,
	parseIntArray,
	parseAddresses,
	formatLines,
	formatCSV,
	formatCSVCompact,
	parseDuration,
	parseKeyValuePairs,
	formatKeyValuePairs,
	parsePortRanges,
	extractDomain,
	parseReservedBytes,
	formatReservedBytes,
	parseNaive,
	parseShadowsocks,
	splitHostPort,
	parseVless,
	parseHysteria2
} from './parsers';

describe('parseLines', () => {
	it('parses multiline text into array', () => {
		expect(parseLines('a\nb\nc')).toEqual(['a', 'b', 'c']);
	});

	it('trims whitespace from lines', () => {
		expect(parseLines('  a  \n  b  ')).toEqual(['a', 'b']);
	});

	it('filters empty lines', () => {
		expect(parseLines('a\n\nb\n  \nc')).toEqual(['a', 'b', 'c']);
	});

	it('returns empty array for empty input', () => {
		expect(parseLines('')).toEqual([]);
		expect(parseLines(null)).toEqual([]);
		expect(parseLines(undefined)).toEqual([]);
	});
});

describe('parseCSV', () => {
	it('parses comma-separated values', () => {
		expect(parseCSV('a,b,c')).toEqual(['a', 'b', 'c']);
	});

	it('trims whitespace', () => {
		expect(parseCSV('a , b , c')).toEqual(['a', 'b', 'c']);
	});

	it('filters empty values', () => {
		expect(parseCSV('a,,b,  ,c')).toEqual(['a', 'b', 'c']);
	});

	it('returns empty array for empty input', () => {
		expect(parseCSV('')).toEqual([]);
		expect(parseCSV(null)).toEqual([]);
	});
});

describe('parsePorts', () => {
	it('parses valid ports', () => {
		expect(parsePorts('80,443,8080')).toEqual([80, 443, 8080]);
	});

	it('filters invalid ports', () => {
		expect(parsePorts('80,0,443,70000,8080')).toEqual([80, 443, 8080]);
	});

	it('handles whitespace', () => {
		expect(parsePorts(' 80 , 443 ')).toEqual([80, 443]);
	});

	it('filters non-numeric values', () => {
		expect(parsePorts('80,abc,443')).toEqual([80, 443]);
	});

	it('returns empty array for empty input', () => {
		expect(parsePorts('')).toEqual([]);
		expect(parsePorts(null)).toEqual([]);
	});
});

describe('parseIntArray', () => {
	it('parses integers from CSV', () => {
		expect(parseIntArray('1,2,3')).toEqual([1, 2, 3]);
	});

	it('handles negative numbers', () => {
		expect(parseIntArray('-1,0,1')).toEqual([-1, 0, 1]);
	});

	it('filters non-numeric values', () => {
		expect(parseIntArray('1,abc,2')).toEqual([1, 2]);
	});

	it('returns empty array for empty input', () => {
		expect(parseIntArray('')).toEqual([]);
	});
});

describe('parseAddresses', () => {
	it('parses comma-separated addresses', () => {
		expect(parseAddresses('192.168.1.1,10.0.0.1')).toEqual(['192.168.1.1', '10.0.0.1']);
	});

	it('parses newline-separated addresses', () => {
		expect(parseAddresses('192.168.1.1\n10.0.0.1')).toEqual(['192.168.1.1', '10.0.0.1']);
	});

	it('handles mixed separators', () => {
		expect(parseAddresses('192.168.1.1,10.0.0.1\n172.16.0.1')).toEqual([
			'192.168.1.1',
			'10.0.0.1',
			'172.16.0.1'
		]);
	});

	it('trims and filters empty', () => {
		expect(parseAddresses(' 192.168.1.1 , , 10.0.0.1 ')).toEqual(['192.168.1.1', '10.0.0.1']);
	});
});

describe('formatLines', () => {
	it('joins array with newlines', () => {
		expect(formatLines(['a', 'b', 'c'])).toBe('a\nb\nc');
	});

	it('returns empty string for empty array', () => {
		expect(formatLines([])).toBe('');
		expect(formatLines(null)).toBe('');
		expect(formatLines(undefined)).toBe('');
	});
});

describe('formatCSV', () => {
	it('joins array with comma and space', () => {
		expect(formatCSV(['a', 'b', 'c'])).toBe('a, b, c');
	});

	it('handles numbers', () => {
		expect(formatCSV([1, 2, 3])).toBe('1, 2, 3');
	});

	it('handles mixed types', () => {
		expect(formatCSV(['a', 1, 'b'])).toBe('a, 1, b');
	});

	it('returns empty string for empty array', () => {
		expect(formatCSV([])).toBe('');
		expect(formatCSV(null)).toBe('');
	});
});

describe('formatCSVCompact', () => {
	it('joins array with comma only (no space)', () => {
		expect(formatCSVCompact(['a', 'b', 'c'])).toBe('a,b,c');
	});

	it('returns empty string for empty array', () => {
		expect(formatCSVCompact([])).toBe('');
	});
});

describe('parseDuration', () => {
	it('returns valid duration strings', () => {
		expect(parseDuration('30s')).toBe('30s');
		expect(parseDuration('5m')).toBe('5m');
		expect(parseDuration('1h')).toBe('1h');
		expect(parseDuration('7d')).toBe('7d');
	});

	it('trims whitespace', () => {
		expect(parseDuration('  30s  ')).toBe('30s');
	});

	it('returns input as-is for unrecognized format', () => {
		expect(parseDuration('custom')).toBe('custom');
	});

	it('returns empty string for empty input', () => {
		expect(parseDuration('')).toBe('');
		expect(parseDuration(null)).toBe('');
	});
});

describe('parseKeyValuePairs', () => {
	it('parses key=value pairs', () => {
		expect(parseKeyValuePairs('a=1\nb=2')).toEqual({ a: '1', b: '2' });
	});

	it('handles values with equals signs', () => {
		expect(parseKeyValuePairs('key=value=with=equals')).toEqual({ key: 'value=with=equals' });
	});

	it('trims keys and values', () => {
		expect(parseKeyValuePairs('  key  =  value  ')).toEqual({ key: 'value' });
	});

	it('skips lines without equals', () => {
		expect(parseKeyValuePairs('a=1\ninvalid\nb=2')).toEqual({ a: '1', b: '2' });
	});

	it('returns empty object for empty input', () => {
		expect(parseKeyValuePairs('')).toEqual({});
		expect(parseKeyValuePairs(null)).toEqual({});
	});
});

describe('formatKeyValuePairs', () => {
	it('formats object as key=value lines', () => {
		expect(formatKeyValuePairs({ a: '1', b: '2' })).toBe('a=1\nb=2');
	});

	it('returns empty string for empty object', () => {
		expect(formatKeyValuePairs({})).toBe('');
		expect(formatKeyValuePairs(null)).toBe('');
	});
});

describe('parsePortRanges', () => {
	it('parses port ranges with dash', () => {
		expect(parsePortRanges('1000-2000')).toEqual(['1000:2000']);
	});

	it('parses multiple ranges', () => {
		expect(parsePortRanges('1000-2000,3000-4000')).toEqual(['1000:2000', '3000:4000']);
	});

	it('normalizes dash to colon', () => {
		expect(parsePortRanges('1000-2000')).toEqual(['1000:2000']);
	});

	it('handles colon format', () => {
		expect(parsePortRanges('1000:2000')).toEqual(['1000:2000']);
	});

	it('filters invalid ranges', () => {
		expect(parsePortRanges('1000-2000,invalid,3000-4000')).toEqual(['1000:2000', '3000:4000']);
	});

	it('returns empty array for empty input', () => {
		expect(parsePortRanges('')).toEqual([]);
	});
});

describe('extractDomain', () => {
	it('extracts domain from URL', () => {
		expect(extractDomain('https://example.com/path')).toBe('example.com');
		expect(extractDomain('http://sub.example.com')).toBe('sub.example.com');
	});

	it('handles URLs without protocol', () => {
		expect(extractDomain('example.com/path')).toBe('example.com');
	});

	it('handles www prefix (keeps it as valid subdomain)', () => {
		// www.example.com is treated as a valid domain with www subdomain
		// The function extracts hostname, not the bare domain
		expect(extractDomain('https://www.example.com')).toBe('www.example.com');
	});

	it('returns domain as-is if just domain', () => {
		expect(extractDomain('example.com')).toBe('example.com');
	});

	it('returns empty string for empty input', () => {
		expect(extractDomain('')).toBe('');
		expect(extractDomain(null)).toBe('');
	});
});

describe('parseReservedBytes', () => {
	it('parses comma-separated bytes', () => {
		expect(parseReservedBytes('0, 0, 0')).toEqual([0, 0, 0]);
		expect(parseReservedBytes('1, 2, 3')).toEqual([1, 2, 3]);
	});

	it('handles bracket format', () => {
		expect(parseReservedBytes('[0, 0, 0]')).toEqual([0, 0, 0]);
		expect(parseReservedBytes('[1, 2, 3]')).toEqual([1, 2, 3]);
	});

	it('limits to 3 bytes', () => {
		expect(parseReservedBytes('1, 2, 3, 4, 5')).toEqual([1, 2, 3]);
	});

	it('returns empty array for empty input', () => {
		expect(parseReservedBytes('')).toEqual([]);
		expect(parseReservedBytes(null)).toEqual([]);
	});
});

describe('formatReservedBytes', () => {
	it('formats bytes array', () => {
		expect(formatReservedBytes([0, 0, 0])).toBe('0, 0, 0');
		expect(formatReservedBytes([1, 2, 3])).toBe('1, 2, 3');
	});

	it('returns empty string for empty array', () => {
		expect(formatReservedBytes([])).toBe('');
		expect(formatReservedBytes(null)).toBe('');
	});
});

describe('parseNaive', () => {
	it('parses full naive+https link', () => {
		const r = parseNaive('naive+https://user:pass@example.com:443#My%20Proxy');
		expect(r.success).toBe(true);
		expect(r.config).toEqual({
			type: 'naive',
			name: 'My Proxy',
			server: 'example.com',
			port: 443,
			username: 'user',
			password: 'pass'
		});
	});

	it('parses naive+quic link with quic flag', () => {
		const r = parseNaive('naive+quic://user:pass@example.com:443#q');
		expect(r.success).toBe(true);
		expect(r.config).toMatchObject({ type: 'naive', quic: true });
	});

	it('parses link without credentials', () => {
		const r = parseNaive('naive+https://example.com:8443');
		expect(r.success).toBe(true);
		expect(r.config).toEqual({
			type: 'naive',
			name: 'NaiveProxy',
			server: 'example.com',
			port: 8443
		});
	});

	it('url-decodes credentials', () => {
		const r = parseNaive('naive+https://user%40mail:p%40ss@example.com:443');
		expect(r.success).toBe(true);
		expect(r.config).toMatchObject({ username: 'user@mail', password: 'p@ss' });
	});

	it('ignores unknown query params', () => {
		const r = parseNaive('naive+https://u:p@example.com:443?padding=1#x');
		expect(r.success).toBe(true);
		expect(r.config).toMatchObject({ server: 'example.com', port: 443 });
	});

	it('fails on missing port', () => {
		const r = parseNaive('naive+https://user:pass@example.com');
		expect(r.success).toBe(false);
	});

	it('fails on wrong prefix', () => {
		const r = parseNaive('https://example.com:443');
		expect(r.success).toBe(false);
	});

	it('parses username without password colon', () => {
		const r = parseNaive('naive+https://onlyuser@example.com:443');
		expect(r.success).toBe(true);
		expect(r.config).toMatchObject({ username: 'onlyuser', password: '' });
	});
});

describe('parseShadowsocks', () => {
	it('parses standard base64 userinfo', () => {
		// btoa('aes-256-gcm:test1234') === 'YWVzLTI1Ni1nY206dGVzdDEyMzQ='
		const r = parseShadowsocks('ss://YWVzLTI1Ni1nY206dGVzdDEyMzQ=@example.com:8388#MyServer');
		expect(r.success).toBe(true);
		expect(r.config).toMatchObject({
			type: 'ss',
			name: 'MyServer',
			server: 'example.com',
			port: 8388,
			method: 'aes-256-gcm',
			password: 'test1234'
		});
	});

	it('parses unpadded base64url userinfo with - and _ characters', () => {
		// base64url('aes-256-gcm:k+?/>~~') — std form is 'YWVzLTI1Ni1nY206ays/Lz5+fg=='
		// → '+' becomes '-', '/' becomes '_', padding stripped
		const r = parseShadowsocks('ss://YWVzLTI1Ni1nY206ays_Lz5-fg@example.com:8388#URLSafe');
		expect(r.success).toBe(true);
		expect(r.config).toMatchObject({
			method: 'aes-256-gcm',
			password: 'k+?/>~~',
			server: 'example.com',
			port: 8388
		});
	});

	it('still falls back to percent-encoded plaintext userinfo', () => {
		const r = parseShadowsocks('ss://aes-256-gcm%3Apassword@example.com:8388');
		expect(r.success).toBe(true);
		expect(r.config).toMatchObject({
			method: 'aes-256-gcm',
			password: 'password'
		});
	});
});

describe('splitHostPort', () => {
	it('splits plain host:port', () => {
		expect(splitHostPort('example.com:443')).toEqual({ host: 'example.com', port: 443 });
	});

	it('splits bracketed IPv6', () => {
		expect(splitHostPort('[::1]:443')).toEqual({ host: '::1', port: 443 });
		expect(splitHostPort('[2001:db8::1]:8443')).toEqual({ host: '2001:db8::1', port: 8443 });
	});

	it('returns null when port is missing', () => {
		expect(splitHostPort('example.com')).toBeNull();
		expect(splitHostPort('[::1]')).toBeNull();
	});

	it('returns null for invalid port or empty host', () => {
		expect(splitHostPort('example.com:0')).toBeNull();
		expect(splitHostPort('example.com:70000')).toBeNull();
		expect(splitHostPort('example.com:abc')).toBeNull();
		expect(splitHostPort(':443')).toBeNull();
	});
});

describe('IPv6 bracket notation in link parsers', () => {
	it('parseVless handles [IPv6]:port', () => {
		const r = parseVless('vless://b831381d-6324-4d53-ad4f-8cca48b30811@[2001:db8::1]:443?security=tls#v6');
		expect(r.success).toBe(true);
		expect(r.config).toMatchObject({ server: '2001:db8::1', port: 443 });
	});

	it('parseHysteria2 handles [IPv6]:port', () => {
		const r = parseHysteria2('hy2://pass@[2001:db8::1]:8443#v6');
		expect(r.success).toBe(true);
		expect(r.config).toMatchObject({ server: '2001:db8::1', port: 8443 });
	});

	it('parseShadowsocks handles [IPv6]:port', () => {
		const r = parseShadowsocks('ss://YWVzLTI1Ni1nY206dGVzdDEyMzQ=@[2001:db8::1]:8388#v6');
		expect(r.success).toBe(true);
		expect(r.config).toMatchObject({ server: '2001:db8::1', port: 8388 });
	});

	it('parseNaive handles [IPv6]:port', () => {
		const r = parseNaive('naive+https://u:p@[2001:db8::1]:443#v6');
		expect(r.success).toBe(true);
		expect(r.config).toMatchObject({ server: '2001:db8::1', port: 443 });
	});
});
