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
	parseKeepalive,
	isValidKeepalive,
	parseNaive,
	parseShadowsocks,
	splitHostPort,
	parseVless,
	parseHysteria2,
	parseTrojan,
	parseConfig,
	toSingboxConfig,
	parseMieruLink,
	normalizeMieruPort
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

describe('parseTrojan', () => {
	it('parses a reality trojan link with ws transport + fp', () => {
		const uri = 'trojan://pw-secret@vpn.ex.com:8443?security=reality&pbk=PBK&sid=0123abcd&sni=www.microsoft.com&fp=chrome&type=ws&path=%2Fp&host=cdn.ex.com#Phone';
		const r = parseTrojan(uri);
		expect(r.success).toBe(true);
		const c = r.config as Extract<NonNullable<typeof r.config>, { type: 'trojan' }>;
		expect(c.type).toBe('trojan');
		expect(c.password).toBe('pw-secret');
		expect(c.server).toBe('vpn.ex.com');
		expect(c.port).toBe(8443);
		expect(c.security).toBe('reality');
		expect(c.pbk).toBe('PBK');
		expect(c.sid).toBe('0123abcd');
		expect(c.sni).toBe('www.microsoft.com');
		expect(c.fingerprint).toBe('chrome');
		expect(c.transport).toBe('ws');
		expect(c.path).toBe('/p');
		expect(c.host).toBe('cdn.ex.com');
		expect(c.name).toBe('Phone');
	});
	it('parses plain tls + grpc', () => {
		const c = parseTrojan('trojan://pw@s.com:443?security=tls&sni=a.com&fp=firefox&type=grpc&serviceName=gsvc#n').config as Extract<NonNullable<ReturnType<typeof parseTrojan>['config']>, { type: 'trojan' }>;
		expect(c.security).toBe('tls');
		expect(c.fingerprint).toBe('firefox');
		expect(c.transport).toBe('grpc');
		expect(c.serviceName).toBe('gsvc');
	});
	it('parses httpupgrade host', () => {
		const c = parseTrojan('trojan://pw@s.com:443?type=httpupgrade&path=%2Fu&host=h.com#n').config as Extract<NonNullable<ReturnType<typeof parseTrojan>['config']>, { type: 'trojan' }>;
		expect(c.transport).toBe('httpupgrade');
		expect(c.path).toBe('/u');
		expect(c.host).toBe('h.com');
	});
	it('decodes percent-encoded password', () => {
		const c = parseTrojan('trojan://p%40ss@s.com:443#n').config as Extract<NonNullable<ReturnType<typeof parseTrojan>['config']>, { type: 'trojan' }>;
		expect(c.password).toBe('p@ss');
	});
	it('rejects a non-trojan URI', () => {
		expect(parseTrojan('vless://x@h:1').success).toBe(false);
	});
	it('parseConfig dispatches trojan://', () => {
		const r = parseConfig('trojan://pw@h:443?security=tls&sni=h#n');
		expect(r.success).toBe(true);
		expect(r.config?.type).toBe('trojan');
	});
});

describe('toSingboxConfig host-matrix', () => {
	it('vless httpupgrade emits top-level host string', () => {
		const { outbound } = toSingboxConfig({ type: 'vless', name: 'n', server: 's', port: 443, uuid: 'u',
			security: 'tls', sni: 's', transport: 'httpupgrade', path: '/hu', host: 'cdn.example.com' } as never);
		expect((outbound as unknown as Record<string, unknown>).transport).toEqual({ type: 'httpupgrade', path: '/hu', host: 'cdn.example.com' });
	});
	it('vless ws still emits headers.Host (regression guard)', () => {
		const { outbound } = toSingboxConfig({ type: 'vless', name: 'n', server: 's', port: 443, uuid: 'u',
			security: 'tls', sni: 's', transport: 'ws', path: '/p', host: 'c.com' } as never);
		expect((outbound as unknown as Record<string, unknown>).transport).toEqual({ type: 'ws', path: '/p', headers: { Host: 'c.com' } });
	});
	it('trojan reality + ws → headers.Host + utls.fingerprint + reality block', () => {
		const { outbound } = toSingboxConfig({ type: 'trojan', name: 'n', server: 's', port: 8443, password: 'pw',
			security: 'reality', pbk: 'K', sid: 's1', sni: 'sni.example', fingerprint: 'chrome',
			transport: 'ws', path: '/ws', host: 'cdn.example.com' } as never);
		const ob = outbound as Record<string, any>;
		expect(ob.type).toBe('trojan');
		expect(ob.password).toBe('pw');
		expect(ob.tls.server_name).toBe('sni.example');
		expect(ob.tls.utls).toEqual({ enabled: true, fingerprint: 'chrome' });
		expect(ob.tls.reality).toEqual({ enabled: true, public_key: 'K', short_id: 's1' });
		expect(ob.transport).toEqual({ type: 'ws', path: '/ws', headers: { Host: 'cdn.example.com' } });
	});
	it('trojan httpupgrade → top-level host string', () => {
		const { outbound } = toSingboxConfig({ type: 'trojan', name: 'n', server: 's', port: 8443, password: 'pw',
			security: 'tls', sni: 's', transport: 'httpupgrade', path: '/hu', host: 'cdn.example.com' } as never);
		expect((outbound as unknown as Record<string, unknown>).transport).toEqual({ type: 'httpupgrade', path: '/hu', host: 'cdn.example.com' });
	});
});

describe('normalizeMieruPort', () => {
	it('keeps single ports and ranges', () => {
		expect(normalizeMieruPort('443')).toBe('443');
		expect(normalizeMieruPort('9000-9010')).toBe('9000-9010');
	});
	it('rejects bad forms', () => {
		for (const s of ['0', '65536', '-1', 'abc', '9000:9010', '9010-9000', '1-2-3', ''])
			expect(normalizeMieruPort(s)).toBeNull();
	});
});

describe('parseMieruLink', () => {
	it('single TCP port → server_port', () => {
		const r = parseMieruLink('mierus://alice:s3cret@example.com?profile=home&port=443&protocol=TCP');
		expect(r.success).toBe(true);
		const c = r.config as any;
		expect(c).toMatchObject({ type: 'mieru', name: 'home', server: 'example.com', username: 'alice', password: 's3cret', transport: 'TCP', server_port: 443 });
		expect(c.server_ports).toBeUndefined();
	});
	it('two single TCP ports → server_port + degenerate range (never bare)', () => {
		const c = parseMieruLink('mierus://a:b@h?profile=p&port=443&port=8443&protocol=TCP').config as any;
		expect(c.server_port).toBe(443);
		expect(c.server_ports).toEqual(['8443-8443']);
	});
	it('range → server_ports', () => {
		const c = parseMieruLink('mierus://a:b@h?profile=p&port=9000-9010&protocol=TCP').config as any;
		expect(c.server_ports).toEqual(['9000-9010']);
	});
	it('duplicate identical port specs are de-duplicated', () => {
		const c = parseMieruLink('mierus://a:b@h?profile=p&port=443&port=443&protocol=TCP').config as any;
		expect(c.server_port).toBe(443);
		expect(c.server_ports).toBeUndefined();
	});
	it('single protocol broadcasts to all ports', () => {
		const c = parseMieruLink('mierus://a:b@h?profile=p&port=443&port=8443&protocol=UDP').config as any;
		expect(c.transport).toBe('UDP');
		expect(c.server_port).toBe(443);
		expect(c.server_ports).toEqual(['8443-8443']);
	});
	it('missing protocol → reject (no TCP default)', () => {
		const r = parseMieruLink('mierus://a:b@h?profile=p&port=443');
		expect(r.success).toBe(false);
		expect(r.error).toContain('protocol');
	});
	it('mixed TCP+UDP without a choice → signals transports', () => {
		const r = parseMieruLink('mierus://a:b@h?profile=p&port=443&protocol=TCP&port=444&protocol=UDP');
		expect(r.success).toBe(false);
		expect(r.mieruTransports).toEqual(['TCP', 'UDP']);
	});
	it('mixed link with a chosen transport → that transport only', () => {
		const c = parseMieruLink('mierus://a:b@h?profile=p&port=443&protocol=TCP&port=444&protocol=UDP', 'UDP').config as any;
		expect(c.transport).toBe('UDP');
		expect(c.server_port).toBe(444);
		expect(c.server_ports).toBeUndefined();
	});
	it('IPv6 host loses the URL brackets', () => {
		const c = parseMieruLink('mierus://a:b@[2001:db8::1]?profile=p&port=443&protocol=TCP').config as any;
		expect(c.server).toBe('2001:db8::1');
	});
	it('multiplexing (incl DEFAULT) + traffic-pattern with + fixed to space', () => {
		const c = parseMieruLink('mierus://a:b@h?profile=p&port=443&protocol=TCP&multiplexing=MULTIPLEXING_DEFAULT&traffic-pattern=YQ b').config as any;
		expect(c.multiplexing).toBe('MULTIPLEXING_DEFAULT');
		expect(c.traffic_pattern).toBe('YQ+b'); // space restored to +
	});
	it('percent-encoded userinfo round-trips', () => {
		const c = parseMieruLink('mierus://a:p%40ss%3Aw%23rd%25@h?profile=p&port=443&protocol=TCP').config as any;
		expect(c.password).toBe('p@ss:w#rd%');
	});
	it('rejects non-Std-base64 traffic-pattern (bad alphabet)', () => {
		expect(parseMieruLink('mierus://a:b@h?profile=p&port=443&protocol=TCP&traffic-pattern=ab$d').success).toBe(false);
	});
	it('rejects unpadded traffic-pattern (length % 4 !== 0 — Go StdEncoding would fail at apply)', () => {
		expect(parseMieruLink('mierus://a:b@h?profile=p&port=443&protocol=TCP&traffic-pattern=YQ').success).toBe(false);
		expect(parseMieruLink('mierus://a:b@h?profile=p&port=443&protocol=TCP&traffic-pattern=YWJjZA%3D').success).toBe(false);
	});
	it('rejects missing userinfo / profile / port / bad multiplexing / oversize pattern', () => {
		expect(parseMieruLink('mierus://h?profile=p&port=443&protocol=TCP').success).toBe(false);
		expect(parseMieruLink('mierus://a:b@h?port=443&protocol=TCP').success).toBe(false);
		expect(parseMieruLink('mierus://a:b@h?profile=p&protocol=TCP').success).toBe(false);
		expect(parseMieruLink('mierus://a:b@h?profile=p&port=443&protocol=TCP&multiplexing=BOGUS').success).toBe(false);
		const big = 'A'.repeat(90000);
		expect(parseMieruLink(`mierus://a:b@h?profile=p&port=443&protocol=TCP&traffic-pattern=${big}`).success).toBe(false);
	});
	it('rejects port in authority', () => {
		const r = parseMieruLink('mierus://a:b@h:443?profile=p&port=443&protocol=TCP');
		expect(r.success).toBe(false);
		expect(r.error).toContain('port must be in the query');
	});
	it('rejects mismatched port/protocol counts with >=2 protocols', () => {
		const r = parseMieruLink('mierus://a:b@h?profile=p&port=1&port=2&port=3&protocol=TCP&protocol=TCP');
		expect(r.success).toBe(false);
		expect(r.error).toContain('port/protocol count mismatch');
	});
	it('catch-path error is fixed and never leaks the credential', () => {
		// '%' alone in the username makes decodeURIComponent throw (URIError)
		const r = parseMieruLink('mierus://a%:secretpass@h?profile=p&port=443&protocol=TCP');
		expect(r.success).toBe(false);
		expect(r.error).toBe('Failed to parse mieru link');
		expect(r.error).not.toContain('secretpass');
	});
});

// AWG 3.0 made PersistentKeepalive a range the device redraws on every timer
// arm; a .conf carrying "22-30" must survive import as typed, not as NaN.
describe('keepalive', () => {
	it('keeps a range verbatim and a plain value as a number', () => {
		expect(parseKeepalive('22-30')).toBe('22-30');
		expect(parseKeepalive(' 25 ')).toBe(25);
	});

	it('treats absent, zero and junk as no keepalive', () => {
		expect(parseKeepalive(undefined)).toBeUndefined();
		expect(parseKeepalive('')).toBeUndefined();
		expect(parseKeepalive('0')).toBeUndefined();
		expect(parseKeepalive('abc')).toBeUndefined();
	});

	it('accepts only what the device can parse', () => {
		for (const ok of [undefined, '', 25, '25', '0-80', '22-30']) {
			expect(isValidKeepalive(ok)).toBe(true);
		}
		for (const bad of ['abc', '30-22', '22-', '-5', '70000', '1-2-3']) {
			expect(isValidKeepalive(bad)).toBe(false);
		}
	});
});
